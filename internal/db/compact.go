package db

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/bestruirui/octopus/internal/utils/log"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// sqliteIncrementalVacuumPages 每个维护周期最多回收的页数（每页 4 KiB，约 8 MiB）。
const sqliteIncrementalVacuumPages = 2048

var sqliteReclaimMu sync.Mutex

// SQLiteReclaimTask 回收历史空闲页，不依赖日志删除，也不执行完整 VACUUM。
func SQLiteReclaimTask() {
	if !sqliteReclaimMu.TryLock() {
		return
	}
	defer sqliteReclaimMu.Unlock()
	if GetDB().Dialector.Name() != "sqlite" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := GetDB().WithContext(ctx).Connection(ReclaimSQLiteFreePages); err != nil {
		log.Warnf("sqlite space reclamation failed: %v", err)
	}
}

// SQLitePageStats 记录 SQLite 文件的页级统计。
type SQLitePageStats struct {
	PageSize      int
	PageCount     int
	FreelistCount int
	AutoVacuum    int
}

// SQLitePageStats 读取 conn 连接上数据库的页级统计。
func ReadSQLitePageStats(conn *gorm.DB) (SQLitePageStats, error) {
	var s SQLitePageStats
	if err := conn.Raw("PRAGMA page_size").Scan(&s.PageSize).Error; err != nil {
		return s, fmt.Errorf("query page_size: %w", err)
	}
	if err := conn.Raw("PRAGMA page_count").Scan(&s.PageCount).Error; err != nil {
		return s, fmt.Errorf("query page_count: %w", err)
	}
	if err := conn.Raw("PRAGMA freelist_count").Scan(&s.FreelistCount).Error; err != nil {
		return s, fmt.Errorf("query freelist_count: %w", err)
	}
	if err := conn.Raw("PRAGMA auto_vacuum").Scan(&s.AutoVacuum).Error; err != nil {
		return s, fmt.Errorf("query auto_vacuum: %w", err)
	}
	return s, nil
}

// ReclaimSQLiteFreePages 在 conn 提供的同一物理连接上执行有界增量回收。
// 非 SQLite 方言直接跳过；auto_vacuum=2 时每次最多回收 sqliteIncrementalVacuumPages 页；
// auto_vacuum=0 的存量库无法在线回收，只记录提示，交由离线 database compact 转换。
func ReclaimSQLiteFreePages(conn *gorm.DB) error {
	if conn == nil || conn.Dialector == nil || conn.Dialector.Name() != "sqlite" {
		return nil
	}
	var autoVacuum int
	if err := conn.Raw("PRAGMA auto_vacuum").Scan(&autoVacuum).Error; err != nil {
		return fmt.Errorf("query auto_vacuum: %w", err)
	}
	switch autoVacuum {
	case 2:
		// incremental_vacuum 每一步返回一行，必须读取到结束；Exec 可能只执行一步。
		rows, err := conn.Raw(fmt.Sprintf("PRAGMA incremental_vacuum(%d)", sqliteIncrementalVacuumPages)).Rows()
		if err != nil {
			return fmt.Errorf("incremental_vacuum: %w", err)
		}
		for rows.Next() {
		}
		rowsErr := rows.Err()
		closeErr := rows.Close()
		if rowsErr != nil {
			return fmt.Errorf("incremental_vacuum: %w", rowsErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close incremental_vacuum: %w", closeErr)
		}
		// PASSIVE 不等待读者退出；未完成的 checkpoint 留给后续周期。
		var checkpoint struct {
			Busy         int
			Log          int
			Checkpointed int
		}
		if err := conn.Raw("PRAGMA wal_checkpoint(PASSIVE)").Scan(&checkpoint).Error; err != nil {
			return fmt.Errorf("wal_checkpoint(PASSIVE): %w", err)
		}
	case 0:
		log.Warnf("sqlite auto_vacuum is 0 (legacy database); run `octopus database compact` to enable incremental reclamation")
	}
	return nil
}

// OpenSQLiteForMaintenance 以读写模式打开现有 SQLite 文件，供离线维护使用；
// 设置 busy_timeout=5000 与 locking_mode=EXCLUSIVE，不执行 AutoMigrate。
func OpenSQLiteForMaintenance(path string) (*gorm.DB, error) {
	params := url.Values{}
	params.Add("_pragma", "busy_timeout(5000)")
	params.Add("_pragma", "locking_mode(EXCLUSIVE)")
	return gorm.Open(sqlite.Open(path+"?"+params.Encode()), &gorm.Config{Logger: logger.Discard})
}

// CompactSQLite 对现有 SQLite 文件执行离线压缩：
// 先 checkpoint，再通过 VACUUM INTO 生成紧凑副本；副本通过完整性检查后原子替换原文件。
// 该流程保留原文件直到副本验证成功，适用于 auto_vacuum=0 的存量库。
func CompactSQLite(path string) (before, after SQLitePageStats, err error) {
	sourceInfo, statErr := os.Stat(path)
	if statErr != nil {
		return before, after, fmt.Errorf("stat database file: %w", statErr)
	}

	conn, openErr := OpenSQLiteForMaintenance(path)
	if openErr != nil {
		return before, after, fmt.Errorf("open database: %w", openErr)
	}
	var tempPath string
	replaced := false
	defer func() {
		if conn != nil {
			_ = closeSQLiteDB(conn)
		}
		if tempPath != "" && !replaced {
			_ = removeSQLiteArtifacts(tempPath)
		}
	}()

	before, err = ReadSQLitePageStats(conn)
	if err != nil {
		return before, after, err
	}
	if err = checkpointTruncate(conn); err != nil {
		return before, after, err
	}
	if err = conn.Exec("PRAGMA auto_vacuum=INCREMENTAL").Error; err != nil {
		return before, after, fmt.Errorf("set auto_vacuum=INCREMENTAL: %w", err)
	}

	tempFile, tempErr := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".compact-*")
	if tempErr != nil {
		return before, after, fmt.Errorf("create compact copy: %w", tempErr)
	}
	tempPath = tempFile.Name()
	if err = tempFile.Close(); err != nil {
		return before, after, fmt.Errorf("close compact copy: %w", err)
	}
	// VACUUM INTO 要求目标文件不存在；CreateTemp 只用于安全生成唯一文件名。
	if err = os.Remove(tempPath); err != nil {
		return before, after, fmt.Errorf("prepare compact copy: %w", err)
	}
	if err = conn.Exec("VACUUM INTO ?", tempPath).Error; err != nil {
		return before, after, fmt.Errorf("vacuum into compact copy: %w", err)
	}
	if err = closeSQLiteDB(conn); err != nil {
		return before, after, fmt.Errorf("close source database: %w", err)
	}
	conn = nil

	after, err = inspectCompactedSQLite(tempPath)
	if err != nil {
		return before, after, err
	}
	if err = removeSQLiteArtifacts(tempPath); err != nil {
		return before, after, err
	}
	if err = os.Chmod(tempPath, sourceInfo.Mode().Perm()); err != nil {
		return before, after, fmt.Errorf("preserve database permissions: %w", err)
	}
	if err = syncSQLiteFile(tempPath); err != nil {
		return before, after, err
	}
	// checkpointTruncate 已确认源 WAL 可安全移除；先清理旁车文件，避免新库继承旧快照。
	if err = removeSQLiteArtifacts(path); err != nil {
		return before, after, err
	}
	if err = os.Rename(tempPath, path); err != nil {
		return before, after, fmt.Errorf("replace database file: %w", err)
	}
	replaced = true
	if err = syncSQLiteDirectory(filepath.Dir(path)); err != nil {
		return before, after, fmt.Errorf("database replaced but directory sync failed: %w", err)
	}
	return before, after, nil
}

func inspectCompactedSQLite(path string) (stats SQLitePageStats, err error) {
	conn, openErr := OpenSQLiteForMaintenance(path)
	if openErr != nil {
		return stats, fmt.Errorf("open compact copy: %w", openErr)
	}
	defer func() {
		if closeErr := closeSQLiteDB(conn); err == nil && closeErr != nil {
			err = fmt.Errorf("close compact copy: %w", closeErr)
		}
	}()

	if err = checkpointTruncate(conn); err != nil {
		return stats, err
	}
	stats, err = ReadSQLitePageStats(conn)
	if err != nil {
		return stats, err
	}
	if stats.AutoVacuum != 2 {
		return stats, fmt.Errorf("verify auto_vacuum: got %d, want 2", stats.AutoVacuum)
	}
	if stats.FreelistCount != 0 {
		return stats, fmt.Errorf("verify freelist_count: got %d, want 0", stats.FreelistCount)
	}
	var checks []string
	if err = conn.Raw("SELECT quick_check FROM pragma_quick_check").Pluck("quick_check", &checks).Error; err != nil {
		return stats, fmt.Errorf("quick_check: %w", err)
	}
	for _, check := range checks {
		if check != "ok" {
			return stats, fmt.Errorf("quick_check failed: %s", check)
		}
	}
	return stats, nil
}

func closeSQLiteDB(conn *gorm.DB) error {
	if conn == nil {
		return nil
	}
	sqlDB, err := conn.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func removeSQLiteArtifacts(path string) error {
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.Remove(path + suffix); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", path+suffix, err)
		}
	}
	return nil
}

func syncSQLiteFile(path string) error {
	file, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("open compact copy for sync: %w", err)
	}
	if err = file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync compact copy: %w", err)
	}
	if err = file.Close(); err != nil {
		return fmt.Errorf("close synced compact copy: %w", err)
	}
	return nil
}

func syncSQLiteDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open database directory for sync: %w", err)
	}
	if err = dir.Sync(); err != nil {
		_ = dir.Close()
		return fmt.Errorf("sync database directory: %w", err)
	}
	if err = dir.Close(); err != nil {
		return fmt.Errorf("close database directory: %w", err)
	}
	return nil
}

// checkpointTruncate 执行 wal_checkpoint(TRUNCATE)；busy 列非 0 说明仍有其他连接持有锁，立即失败。
func checkpointTruncate(conn *gorm.DB) error {
	var cp struct {
		Busy         int
		Log          int
		Checkpointed int
	}
	if err := conn.Raw("PRAGMA wal_checkpoint(TRUNCATE)").Scan(&cp).Error; err != nil {
		return fmt.Errorf("wal_checkpoint(TRUNCATE): %w", err)
	}
	if cp.Busy != 0 {
		return fmt.Errorf("wal_checkpoint(TRUNCATE) busy: %d, another connection holds the database", cp.Busy)
	}
	return nil
}
