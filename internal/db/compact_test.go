package db

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// openLegacySQLite 打开不带 auto_vacuum pragma 的临时库，模拟存量 auto_vacuum=0 数据库。
func openLegacySQLite(t *testing.T, path string) *gorm.DB {
	t.Helper()
	conn, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("open legacy sqlite: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, closeErr := conn.DB(); closeErr == nil {
			_ = sqlDB.Close()
		}
	})
	return conn
}

func readAutoVacuum(t *testing.T, conn *gorm.DB) int {
	t.Helper()
	var av int
	if err := conn.Raw("PRAGMA auto_vacuum").Scan(&av).Error; err != nil {
		t.Fatalf("query auto_vacuum: %v", err)
	}
	return av
}

func TestFreshSQLiteDBHasIncrementalAutoVacuum(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fresh.db")
	if err := InitDB("sqlite", path, false); err != nil {
		t.Fatalf("init db: %v", err)
	}
	t.Cleanup(func() { _ = Close() })

	if av := readAutoVacuum(t, GetDB()); av != 2 {
		t.Fatalf("fresh db auto_vacuum = %d, want 2", av)
	}
}

func TestReclaimSQLiteFreePagesFreshDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fresh.db")
	if err := InitDB("sqlite", path, false); err != nil {
		t.Fatalf("init db: %v", err)
	}
	t.Cleanup(func() { _ = Close() })

	// 造大日志：50 行 × 每行 400 KiB 正文，约 20 MB
	big := strings.Repeat("x", 200*1024)
	for i := 0; i < 50; i++ {
		if err := GetDB().Create(&model.RelayLog{
			ID:              int64(i + 1),
			Time:            int64(i),
			RequestContent:  big,
			ResponseContent: big,
		}).Error; err != nil {
			t.Fatalf("create rows: %v", err)
		}
	}

	// 删除全部行：auto_vacuum=2 下释放页进入 freelist，文件不收缩
	if err := GetDB().Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&model.RelayLog{}).Error; err != nil {
		t.Fatalf("delete rows: %v", err)
	}
	afterDelete, err := ReadSQLitePageStats(GetDB())
	if err != nil {
		t.Fatalf("stats after delete: %v", err)
	}
	if afterDelete.FreelistCount == 0 {
		t.Fatalf("delete should leave freelist pages, got 0")
	}

	// 同一物理连接上执行有界增量回收
	if err := GetDB().Connection(func(tx *gorm.DB) error {
		return ReclaimSQLiteFreePages(tx)
	}); err != nil {
		t.Fatalf("reclaim: %v", err)
	}
	after, err := ReadSQLitePageStats(GetDB())
	if err != nil {
		t.Fatalf("stats after reclaim: %v", err)
	}
	if after.FreelistCount >= afterDelete.FreelistCount {
		t.Fatalf("freelist not reclaimed: %d -> %d", afterDelete.FreelistCount, after.FreelistCount)
	}
	if after.PageCount >= afterDelete.PageCount {
		t.Fatalf("page_count not reduced: %d -> %d", afterDelete.PageCount, after.PageCount)
	}
}

func TestReclaimSQLiteFreePagesLegacyDBOnlyWarns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	conn := openLegacySQLite(t, path)

	if err := conn.Exec(`CREATE TABLE relay_logs (id INTEGER PRIMARY KEY, time INTEGER, request_content TEXT)`).Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
	big := strings.Repeat("y", 100*1024)
	for i := 0; i < 30; i++ {
		if err := conn.Exec("INSERT INTO relay_logs (id, time, request_content) VALUES (?, ?, ?)", i+1, i, big).Error; err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	if av := readAutoVacuum(t, conn); av != 0 {
		t.Fatalf("legacy db auto_vacuum = %d, want 0", av)
	}
	if err := conn.Exec("DELETE FROM relay_logs").Error; err != nil {
		t.Fatalf("delete: %v", err)
	}

	// 存量模式 0 在线维护只告警：不报错且不改变模式
	if err := conn.Connection(func(tx *gorm.DB) error {
		return ReclaimSQLiteFreePages(tx)
	}); err != nil {
		t.Fatalf("legacy reclaim should not error: %v", err)
	}
	if av := readAutoVacuum(t, conn); av != 0 {
		t.Fatalf("legacy db auto_vacuum changed to %d, want 0", av)
	}
}

func TestCompactSQLite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	conn := openLegacySQLite(t, path)

	if err := conn.Exec(`CREATE TABLE relay_logs (id INTEGER PRIMARY KEY, time INTEGER, request_content TEXT)`).Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
	big := strings.Repeat("z", 100*1024)
	for i := 0; i < 200; i++ {
		if err := conn.Exec("INSERT INTO relay_logs (id, time, request_content) VALUES (?, ?, ?)", i+1, i, big).Error; err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	// 删除一半行制造可观测 freelist
	if err := conn.Exec("DELETE FROM relay_logs WHERE id <= 100").Error; err != nil {
		t.Fatalf("delete: %v", err)
	}
	if sqlDB, err := conn.DB(); err == nil {
		_ = sqlDB.Close()
	}
	sizeBefore, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat before: %v", err)
	}

	before, after, err := CompactSQLite(path)
	if err != nil {
		t.Fatalf("compact: %v", err)
	}
	if before.AutoVacuum != 0 {
		t.Fatalf("legacy before auto_vacuum = %d, want 0", before.AutoVacuum)
	}
	if after.AutoVacuum != 2 {
		t.Fatalf("after compact auto_vacuum = %d, want 2", after.AutoVacuum)
	}
	if after.FreelistCount != 0 {
		t.Fatalf("after compact freelist_count = %d, want 0", after.FreelistCount)
	}
	if after.PageCount >= before.PageCount {
		t.Fatalf("page_count not reduced: %d -> %d", before.PageCount, after.PageCount)
	}
	sizeAfter, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat after: %v", err)
	}
	if sizeAfter.Size() >= sizeBefore.Size() {
		t.Fatalf("file not shrunk: %d -> %d", sizeBefore.Size(), sizeAfter.Size())
	}
}
