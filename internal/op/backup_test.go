package op

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
)

func TestDBBackupUsageExportImport(t *testing.T) {
	initLogTestDB(t)
	resetStatsUsageCache(t)
	ctx := context.Background()

	row := testUsageRow("20260805", "gpt-4o", "gpt-4o-2024", 1, "prod", 10, "openai", true,
		model.StatsMetrics{InputToken: 100, OutputToken: 40, InputCost: 0.1, OutputCost: 0.2, WaitTime: 1000, RequestSuccess: 1}, 20)
	// 两次累加后导出：未 flush 的 delta 也必须进入导出
	if err := StatsUsageUpdate(row); err != nil {
		t.Fatalf("StatsUsageUpdate: %v", err)
	}
	if err := StatsUsageUpdate(row); err != nil {
		t.Fatalf("StatsUsageUpdate: %v", err)
	}

	dump, err := DBExportAll(ctx, false, true)
	if err != nil {
		t.Fatalf("DBExportAll: %v", err)
	}
	if len(dump.StatsUsage) != 1 {
		t.Fatalf("导出 stats_usage 行数 = %d, want 1（含未 flush delta）", len(dump.StatsUsage))
	}
	if dump.StatsUsage[0].RequestSuccess != 2 || dump.StatsUsage[0].InputToken != 200 ||
		dump.StatsUsage[0].CachedToken != 40 || dump.StatsUsage[0].KeyHash == "" {
		t.Fatalf("导出内容错误：%+v", dump.StatsUsage[0])
	}
	statsUsageDeltaCacheLock.Lock()
	deltaLen := len(statsUsageDeltaCache)
	statsUsageDeltaCacheLock.Unlock()
	if deltaLen != 0 {
		t.Fatalf("导出后 delta cache 应已 flush，got %d", deltaLen)
	}

	// 导入空库后响应一致
	if err := db.Close(); err != nil {
		t.Fatalf("关闭数据库：%v", err)
	}
	importPath := filepath.Join(t.TempDir(), "import.db")
	if err := db.InitDB("sqlite", importPath, false); err != nil {
		t.Fatalf("初始化导入库失败：%v", err)
	}
	resetStatsUsageCache(t)

	result, err := DBImportIncremental(ctx, dump)
	if err != nil {
		t.Fatalf("DBImportIncremental: %v", err)
	}
	if result.RowsAffected["stats_usage"] != 1 {
		t.Fatalf("首次导入 stats_usage 行数 = %d, want 1", result.RowsAffected["stats_usage"])
	}

	res, err := StatsAnalyticsGet(ctx, StatsAnalyticsFilter{})
	if err != nil {
		t.Fatalf("StatsAnalyticsGet: %v", err)
	}
	assertUsageMetrics(t, res.Summary,
		model.StatsMetrics{InputToken: 200, OutputToken: 80, InputCost: 0.2, OutputCost: 0.4, WaitTime: 2000, RequestSuccess: 2},
		40)

	// 目标库已有同 key 新值：旧备份不得覆盖，重复导入幂等
	if err := StatsUsageUpdate(row); err != nil {
		t.Fatalf("StatsUsageUpdate: %v", err)
	}
	if err := StatsSaveDB(ctx); err != nil {
		t.Fatalf("StatsSaveDB: %v", err)
	}
	result2, err := DBImportIncremental(ctx, dump)
	if err != nil {
		t.Fatalf("DBImportIncremental: %v", err)
	}
	if result2.RowsAffected["stats_usage"] != 0 {
		t.Fatalf("冲突行应保留目标值，got %d", result2.RowsAffected["stats_usage"])
	}
	res2, err := StatsAnalyticsGet(ctx, StatsAnalyticsFilter{})
	if err != nil {
		t.Fatalf("StatsAnalyticsGet: %v", err)
	}
	if res2.Summary.RequestSuccess != 3 {
		t.Fatalf("目标值被旧备份覆盖：%d, want 3", res2.Summary.RequestSuccess)
	}
	result3, err := DBImportIncremental(ctx, dump)
	if err != nil {
		t.Fatalf("DBImportIncremental: %v", err)
	}
	if result3.RowsAffected["stats_usage"] != 0 {
		t.Fatalf("重复导入应幂等，got %d", result3.RowsAffected["stats_usage"])
	}

	// key_hash 校验：损坏行拒绝导入
	corrupt := &model.DBDump{
		Version:      dbDumpVersion,
		IncludeStats: true,
		StatsUsage:   append([]model.StatsUsage(nil), dump.StatsUsage...),
	}
	corrupt.StatsUsage[0].KeyHash = "deadbeef"
	if _, err := DBImportIncremental(ctx, corrupt); err == nil {
		t.Fatal("key_hash 不匹配应拒绝导入")
	}

	// includeStats=false 不导入
	if err := db.Close(); err != nil {
		t.Fatalf("关闭数据库：%v", err)
	}
	noStatsPath := filepath.Join(t.TempDir(), "nostats.db")
	if err := db.InitDB("sqlite", noStatsPath, false); err != nil {
		t.Fatalf("初始化测试数据库失败：%v", err)
	}
	resetStatsUsageCache(t)
	noStats := &model.DBDump{Version: dbDumpVersion, IncludeStats: false, StatsUsage: dump.StatsUsage}
	if _, err := DBImportIncremental(ctx, noStats); err != nil {
		t.Fatalf("DBImportIncremental: %v", err)
	}
	var count int64
	if err := db.GetDB().WithContext(ctx).Table("stats_usages").Count(&count).Error; err != nil {
		t.Fatalf("统计行数：%v", err)
	}
	if count != 0 {
		t.Fatalf("includeStats=false 不应导入 stats_usage，got %d", count)
	}
}

func TestDBImportRejectsInvalidSettings(t *testing.T) {
	ctx := initLogTestDB(t)

	dump := &model.DBDump{
		Version: dbDumpVersion,
		Settings: []model.Setting{
			{Key: model.SettingKeyModelInfoUpdateInterval, Value: "not-an-integer"},
		},
	}
	if _, err := DBImportIncremental(ctx, dump); err == nil {
		t.Fatalf("非法整数设置应整体拒绝导入")
	}

	var count int64
	if err := db.GetDB().Model(&model.Setting{}).Count(&count).Error; err != nil {
		t.Fatalf("统计失败：%v", err)
	}
	if count != int64(len(model.DefaultSettings())) {
		t.Fatalf("非法导入不应写入 setting：got %d 行", count)
	}

	dump.Settings = []model.Setting{
		{Key: model.SettingKeyModelInfoUpdateInterval, Value: "20"},
	}
	if _, err := DBImportIncremental(ctx, dump); err != nil {
		t.Fatalf("合法 setting 导入失败：%v", err)
	}
}
