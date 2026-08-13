package db

import (
	"fmt"
	"net/url"
	"os"

	"github.com/bestruirui/octopus/internal/utils/log"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// sqliteIncrementalVacuumPages 每个维护周期最多回收的页数（每页 4 KiB，约 8 MiB）。
const sqliteIncrementalVacuumPages = 2048

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

// ReclaimSQLiteFreePages 在 conn 提供的同一物理连接上执行有界增量回收，供日志清理复用。
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
		if err := conn.Exec(fmt.Sprintf("PRAGMA incremental_vacuum(%d)", sqliteIncrementalVacuumPages)).Error; err != nil {
			return fmt.Errorf("incremental_vacuum: %w", err)
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
// 先 truncate checkpoint（busy 即失败），再开启 INCREMENTAL 增量回收并 VACUUM，
// 最后再次 checkpoint 并验证 auto_vacuum=2 与 quick_check=ok。
// 返回压缩前后的页级统计；任何失败都以非零错误返回，绝不继续。
func CompactSQLite(path string) (before, after SQLitePageStats, err error) {
	if _, statErr := os.Stat(path); statErr != nil {
		return before, after, fmt.Errorf("stat database file: %w", statErr)
	}

	conn, openErr := OpenSQLiteForMaintenance(path)
	if openErr != nil {
		return before, after, fmt.Errorf("open database: %w", openErr)
	}
	defer func() {
		if sqlDB, closeErr := conn.DB(); closeErr == nil {
			_ = sqlDB.Close()
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
	if err = conn.Exec("VACUUM").Error; err != nil {
		return before, after, fmt.Errorf("vacuum: %w", err)
	}
	if err = checkpointTruncate(conn); err != nil {
		return before, after, err
	}

	after, err = ReadSQLitePageStats(conn)
	if err != nil {
		return before, after, err
	}
	if after.AutoVacuum != 2 {
		return before, after, fmt.Errorf("verify auto_vacuum: got %d, want 2", after.AutoVacuum)
	}

	var checks []string
	if err = conn.Raw("SELECT quick_check FROM pragma_quick_check").Pluck("quick_check", &checks).Error; err != nil {
		return before, after, fmt.Errorf("quick_check: %w", err)
	}
	for _, check := range checks {
		if check != "ok" {
			return before, after, fmt.Errorf("quick_check failed: %s", check)
		}
	}
	return before, after, nil
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
