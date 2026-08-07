package op

import (
	"context"
	"math"
	"net/url"
	"path/filepath"
	"sync"
	"testing"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// resetStatsUsageCache 清空共享的 delta cache，避免测试间相互污染。
func resetStatsUsageCache(t *testing.T) {
	t.Helper()
	statsUsageDeltaCacheLock.Lock()
	defer statsUsageDeltaCacheLock.Unlock()
	clear(statsUsageDeltaCache)
}

func testUsageRow(date, requestModel, actualModel string, apiKeyID int, apiKeyName string, channelID int, channelName string, success bool, metrics model.StatsMetrics, cachedToken int64) model.StatsUsage {
	return model.StatsUsage{
		Date:             date,
		RequestModelName: requestModel,
		ActualModelName:  actualModel,
		APIKeyID:         apiKeyID,
		APIKeyName:       apiKeyName,
		ChannelID:        channelID,
		ChannelName:      channelName,
		Success:          success,
		CachedToken:      cachedToken,
		StatsMetrics:     metrics,
	}
}

func assertFloatClose(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("cost = %f, want %f", got, want)
	}
}

func assertUsageMetrics(t *testing.T, got StatsAnalyticsMetrics, want model.StatsMetrics, wantCached int64) {
	t.Helper()
	if got.InputToken != want.InputToken {
		t.Fatalf("input_token = %d, want %d", got.InputToken, want.InputToken)
	}
	if got.OutputToken != want.OutputToken {
		t.Fatalf("output_token = %d, want %d", got.OutputToken, want.OutputToken)
	}
	assertFloatClose(t, got.InputCost, want.InputCost)
	assertFloatClose(t, got.OutputCost, want.OutputCost)
	if got.WaitTime != want.WaitTime {
		t.Fatalf("wait_time = %d, want %d", got.WaitTime, want.WaitTime)
	}
	if got.RequestSuccess != want.RequestSuccess {
		t.Fatalf("request_success = %d, want %d", got.RequestSuccess, want.RequestSuccess)
	}
	if got.RequestFailed != want.RequestFailed {
		t.Fatalf("request_failed = %d, want %d", got.RequestFailed, want.RequestFailed)
	}
	if got.CachedToken != wantCached {
		t.Fatalf("cached_token = %d, want %d", got.CachedToken, wantCached)
	}
}

func TestStatsUsageAnalyticsCombinedFilter(t *testing.T) {
	initLogTestDB(t)
	resetStatsUsageCache(t)
	ctx := context.Background()

	rowA := testUsageRow("20260805", "gpt-4o", "gpt-4o-2024", 1, "prod", 10, "openai", true,
		model.StatsMetrics{InputToken: 100, OutputToken: 40, InputCost: 0.1, OutputCost: 0.2, WaitTime: 1000, RequestSuccess: 1}, 20)
	rowB := testUsageRow("20260806", "gpt-4o", "gpt-4o-2024", 1, "prod", 10, "openai", false,
		model.StatsMetrics{WaitTime: 500, RequestFailed: 1}, 0)
	rowC := testUsageRow("20260806", "claude", "claude-3", 2, "key2", 20, "anthropic", true,
		model.StatsMetrics{InputToken: 50, WaitTime: 200, RequestSuccess: 1}, 0)

	for _, row := range []model.StatsUsage{rowA, rowB, rowC} {
		if err := StatsUsageUpdate(row); err != nil {
			t.Fatalf("StatsUsageUpdate: %v", err)
		}
	}
	if err := StatsSaveDB(ctx); err != nil {
		t.Fatalf("StatsSaveDB: %v", err)
	}

	start, end := "20260805", "20260806"
	requestModel, apiKeyName, channelName := "gpt-4o", "prod", "openai"
	apiKeyID, channelID := 1, 10
	filter := StatsAnalyticsFilter{
		StartDate:        &start,
		EndDate:          &end,
		RequestModelName: &requestModel,
		APIKeyID:         &apiKeyID,
		APIKeyName:       &apiKeyName,
		ChannelID:        &channelID,
		ChannelName:      &channelName,
	}

	res, err := StatsAnalyticsGet(ctx, filter)
	if err != nil {
		t.Fatalf("StatsAnalyticsGet: %v", err)
	}
	assertUsageMetrics(t, res.Summary,
		model.StatsMetrics{InputToken: 100, OutputToken: 40, InputCost: 0.1, OutputCost: 0.2, WaitTime: 1500, RequestSuccess: 1, RequestFailed: 1},
		20)
	if res.AvailableFrom != "20260805" {
		t.Fatalf("available_from = %q, want 20260805", res.AvailableFrom)
	}
	if len(res.Trend) != 2 || res.Trend[0].Date != "20260805" || res.Trend[1].Date != "20260806" {
		t.Fatalf("trend = %+v, want two sorted days 20260805/20260806", res.Trend)
	}
	if res.Trend[0].RequestSuccess != 1 || res.Trend[1].RequestFailed != 1 {
		t.Fatalf("trend 拆分错误：%+v", res.Trend)
	}
	if len(res.ByModel) != 1 || res.ByModel[0].Name != "gpt-4o" {
		t.Fatalf("by_model = %+v, want only gpt-4o", res.ByModel)
	}
	assertUsageMetrics(t, res.ByModel[0].StatsAnalyticsMetrics,
		model.StatsMetrics{InputToken: 100, OutputToken: 40, InputCost: 0.1, OutputCost: 0.2, WaitTime: 1500, RequestSuccess: 1, RequestFailed: 1},
		20)
	if len(res.ByActualModel) != 1 || res.ByActualModel[0].Name != "gpt-4o-2024" {
		t.Fatalf("by_actual_model = %+v, want only gpt-4o-2024", res.ByActualModel)
	}
	if len(res.ByAPIKey) != 1 || res.ByAPIKey[0].ID != 1 || res.ByAPIKey[0].Name != "prod" {
		t.Fatalf("by_api_key = %+v, want only (1,prod)", res.ByAPIKey)
	}
	if len(res.ByChannel) != 1 || res.ByChannel[0].ID != 10 || res.ByChannel[0].Name != "openai" {
		t.Fatalf("by_channel = %+v, want only (10,openai)", res.ByChannel)
	}

	// status=success 后只剩第一行
	status := "success"
	filter.Status = status
	res2, err := StatsAnalyticsGet(ctx, filter)
	if err != nil {
		t.Fatalf("StatsAnalyticsGet: %v", err)
	}
	assertUsageMetrics(t, res2.Summary,
		model.StatsMetrics{InputToken: 100, OutputToken: 40, InputCost: 0.1, OutputCost: 0.2, WaitTime: 1000, RequestSuccess: 1},
		20)
	if len(res2.Trend) != 1 || res2.Trend[0].Date != "20260805" {
		t.Fatalf("trend = %+v, want only 20260805", res2.Trend)
	}
	if len(res2.ByAPIKey) != 1 || len(res2.ByChannel) != 1 {
		t.Fatalf("success 筛选后拆分应只剩匹配维度：%+v", res2)
	}

	// 未 flush 的 delta 也要并入查询结果
	rowD := testUsageRow("20260807", "gpt-4o", "gpt-4o-2024", 1, "prod", 10, "openai", true,
		model.StatsMetrics{InputToken: 10, WaitTime: 60, RequestSuccess: 1}, 5)
	if err := StatsUsageUpdate(rowD); err != nil {
		t.Fatalf("StatsUsageUpdate: %v", err)
	}
	filter.Status = ""
	end = "20260807"
	res3, err := StatsAnalyticsGet(ctx, filter)
	if err != nil {
		t.Fatalf("StatsAnalyticsGet: %v", err)
	}
	if res3.Summary.RequestSuccess != 2 || res3.Summary.CachedToken != 25 {
		t.Fatalf("delta 未并入结果：%+v", res3.Summary)
	}
	if res3.Trend[2].Date != "20260807" || res3.Trend[2].RequestSuccess != 1 {
		t.Fatalf("delta trend 未并入：%+v", res3.Trend)
	}
}

func TestStatsUsageKeyHashDistinguishesNames(t *testing.T) {
	initLogTestDB(t)
	resetStatsUsageCache(t)
	ctx := context.Background()

	base := testUsageRow("20260805", "gpt-4o", "gpt-4o-2024", 1, "prod", 10, "openai", true,
		model.StatsMetrics{InputToken: 10, WaitTime: 100, RequestSuccess: 1}, 0)
	renamed := base
	renamed.APIKeyName = "prod-v2" // 相同数值 ID，不同名称
	renamed.ChannelName = "openai-eu"

	h1, err := base.Key().Hash()
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	h2, err := renamed.Key().Hash()
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if h1 == h2 {
		t.Fatal("不同名称应生成不同 key_hash")
	}

	for _, row := range []model.StatsUsage{base, renamed} {
		if err := StatsUsageUpdate(row); err != nil {
			t.Fatalf("StatsUsageUpdate: %v", err)
		}
	}
	if err := StatsSaveDB(ctx); err != nil {
		t.Fatalf("StatsSaveDB: %v", err)
	}

	dims, err := StatsAnalyticsDimensionsGet(ctx)
	if err != nil {
		t.Fatalf("StatsAnalyticsDimensionsGet: %v", err)
	}
	if len(dims.APIKeys) != 2 {
		t.Fatalf("api_keys = %+v, want two distinct items", dims.APIKeys)
	}
	if len(dims.Channels) != 2 {
		t.Fatalf("channels = %+v, want two distinct items", dims.Channels)
	}
	foundProdV2 := false
	for _, k := range dims.APIKeys {
		if k.ID == 1 && k.Name == "prod-v2" {
			foundProdV2 = true
		}
	}
	if !foundProdV2 {
		t.Fatalf("api_keys 缺少 (1,prod-v2)：%+v", dims.APIKeys)
	}

	// 按 ID+名称过滤不串数据
	apiKeyID, apiKeyName := 1, "prod-v2"
	res, err := StatsAnalyticsGet(ctx, StatsAnalyticsFilter{APIKeyID: &apiKeyID, APIKeyName: &apiKeyName})
	if err != nil {
		t.Fatalf("StatsAnalyticsGet: %v", err)
	}
	if res.Summary.RequestSuccess != 1 || len(res.ByChannel) != 1 || res.ByChannel[0].Name != "openai-eu" {
		t.Fatalf("按 (1,prod-v2) 过滤应只命中改名行：%+v", res)
	}

	// 指向空字符串的 filter 精确筛选未知实际模型/未分配渠道，而不是等同 all
	empty := ""
	unknown := testUsageRow("20260806", "gpt-4o", "", 1, "prod", 0, "", false,
		model.StatsMetrics{WaitTime: 300, RequestFailed: 1}, 0)
	if err := StatsUsageUpdate(unknown); err != nil {
		t.Fatalf("StatsUsageUpdate: %v", err)
	}
	zero := 0
	resUnknown, err := StatsAnalyticsGet(ctx, StatsAnalyticsFilter{
		ActualModelName: &empty, ChannelID: &zero, ChannelName: &empty,
	})
	if err != nil {
		t.Fatalf("StatsAnalyticsGet: %v", err)
	}
	if resUnknown.Summary.RequestFailed != 1 || resUnknown.Summary.RequestSuccess != 0 {
		t.Fatalf("空名称筛选应精确命中未知行：%+v", resUnknown.Summary)
	}
	resAll, err := StatsAnalyticsGet(ctx, StatsAnalyticsFilter{})
	if err != nil {
		t.Fatalf("StatsAnalyticsGet: %v", err)
	}
	if resAll.Summary.RequestSuccess != 2 || resAll.Summary.RequestFailed != 1 {
		t.Fatalf("无筛选应包含全部三行：%+v", resAll.Summary)
	}
	resNamed, err := StatsAnalyticsGet(ctx, StatsAnalyticsFilter{ActualModelName: &renamed.ActualModelName})
	if err != nil {
		t.Fatalf("StatsAnalyticsGet: %v", err)
	}
	if resNamed.Summary.RequestFailed != 0 {
		t.Fatalf("精确实际模型筛选不应命中空名称行：%+v", resNamed.Summary)
	}
}

func TestStatsUsageConcurrentFlush(t *testing.T) {
	initLogTestDB(t)
	resetStatsUsageCache(t)
	ctx := context.Background()

	row := testUsageRow("20260805", "gpt-4o", "gpt-4o-2024", 1, "prod", 10, "openai", true,
		model.StatsMetrics{InputToken: 1, WaitTime: 10, RequestSuccess: 1}, 0)

	const goroutines = 8
	const perGoroutine = 25
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				if err := StatsUsageUpdate(row); err != nil {
					t.Errorf("StatsUsageUpdate: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()

	if err := StatsSaveDB(ctx); err != nil {
		t.Fatalf("StatsSaveDB: %v", err)
	}

	statsUsageDeltaCacheLock.Lock()
	deltaLen := len(statsUsageDeltaCache)
	statsUsageDeltaCacheLock.Unlock()
	if deltaLen != 0 {
		t.Fatalf("事务成功后 delta cache 应清空，got %d", deltaLen)
	}

	var rows []model.StatsUsage
	if err := db.GetDB().WithContext(ctx).Find(&rows).Error; err != nil {
		t.Fatalf("查询 usage 行：%v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("行数 = %d, want 1", len(rows))
	}
	want := int64(goroutines * perGoroutine)
	if rows[0].RequestSuccess != want || rows[0].InputToken != want || rows[0].WaitTime != want*10 {
		t.Fatalf("合并结果 = %+v, want 精确总数 %d", rows[0], want)
	}

	// 两个独立连接并发 flush 相同 key：原子加法得到精确总数
	if err := db.Close(); err != nil {
		t.Fatalf("关闭数据库：%v", err)
	}
	dbPath := filepath.Join(t.TempDir(), "flush.db")
	if err := db.InitDB("sqlite", dbPath, false); err != nil {
		t.Fatalf("初始化测试数据库失败：%v", err)
	}
	resetStatsUsageCache(t)

	openConn := func() *gorm.DB {
		params := url.Values{}
		params.Add("_pragma", "busy_timeout(10000)")
		conn, err := gorm.Open(sqlite.Open(dbPath+"?"+params.Encode()), &gorm.Config{})
		if err != nil {
			t.Fatalf("打开独立连接失败：%v", err)
		}
		return conn
	}
	connA := openConn()
	connB := openConn()
	t.Cleanup(func() {
		for _, conn := range []*gorm.DB{connA, connB} {
			if sqlDB, err := conn.DB(); err == nil {
				_ = sqlDB.Close()
			}
		}
	})

	key := row.Key()
	keyHash, err := key.Hash()
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	snapshotA := map[model.StatsUsageKey]model.StatsUsage{}
	rowA := testUsageRow("20260805", "gpt-4o", "gpt-4o-2024", 1, "prod", 10, "openai", true,
		model.StatsMetrics{RequestSuccess: 10}, 0)
	rowA.KeyHash = keyHash
	snapshotA[key] = rowA
	snapshotB := map[model.StatsUsageKey]model.StatsUsage{}
	rowB := testUsageRow("20260805", "gpt-4o", "gpt-4o-2024", 1, "prod", 10, "openai", true,
		model.StatsMetrics{RequestSuccess: 7}, 0)
	rowB.KeyHash = keyHash
	snapshotB[key] = rowB

	var wg2 sync.WaitGroup
	wg2.Add(2)
	var errA, errB error
	go func() {
		defer wg2.Done()
		errA = persistStatsUsageRows(ctx, connA, snapshotA)
	}()
	go func() {
		defer wg2.Done()
		errB = persistStatsUsageRows(ctx, connB, snapshotB)
	}()
	wg2.Wait()
	if errA != nil {
		t.Fatalf("连接 A flush：%v", errA)
	}
	if errB != nil {
		t.Fatalf("连接 B flush：%v", errB)
	}

	var merged []model.StatsUsage
	if err := db.GetDB().WithContext(ctx).Find(&merged).Error; err != nil {
		t.Fatalf("查询 usage 行：%v", err)
	}
	if len(merged) != 1 || merged[0].RequestSuccess != 17 {
		t.Fatalf("并发 flush 结果 = %+v, want 单行 17", merged)
	}

	// 关闭并重新初始化同一临时数据库后查询仍一致
	if err := db.Close(); err != nil {
		t.Fatalf("关闭数据库：%v", err)
	}
	if err := db.InitDB("sqlite", dbPath, false); err != nil {
		t.Fatalf("重新初始化测试数据库失败：%v", err)
	}
	start, end := "20260805", "20260805"
	res, err := StatsAnalyticsGet(ctx, StatsAnalyticsFilter{StartDate: &start, EndDate: &end})
	if err != nil {
		t.Fatalf("StatsAnalyticsGet: %v", err)
	}
	if res.Summary.RequestSuccess != 17 {
		t.Fatalf("重开数据库后请求数 = %d, want 17", res.Summary.RequestSuccess)
	}
}

func TestStatsUsageAnalyticsEmptyDB(t *testing.T) {
	initLogTestDB(t)
	resetStatsUsageCache(t)
	ctx := context.Background()

	start, end := "20260801", "20260831"
	res, err := StatsAnalyticsGet(ctx, StatsAnalyticsFilter{StartDate: &start, EndDate: &end})
	if err != nil {
		t.Fatalf("空库查询不应报错：%v", err)
	}
	assertUsageMetrics(t, res.Summary, model.StatsMetrics{}, 0)
	if res.AvailableFrom != "" {
		t.Fatalf("available_from = %q, want empty", res.AvailableFrom)
	}
	if len(res.Trend) != 0 || len(res.ByModel) != 0 || len(res.ByActualModel) != 0 ||
		len(res.ByAPIKey) != 0 || len(res.ByChannel) != 0 {
		t.Fatalf("空库拆分应为空：%+v", res)
	}

	dims, err := StatsAnalyticsDimensionsGet(ctx)
	if err != nil {
		t.Fatalf("空库 dimensions 不应报错：%v", err)
	}
	if len(dims.Models) != 0 || len(dims.ActualModels) != 0 || len(dims.APIKeys) != 0 || len(dims.Channels) != 0 {
		t.Fatalf("空库 dimensions 应为空：%+v", dims)
	}

	// 排序：模型按 name，实体按 (name,id) 升序
	rows := []model.StatsUsage{
		testUsageRow("20260801", "zeta", "mid", 3, "b", 3, "z", true, model.StatsMetrics{RequestSuccess: 1}, 0),
		testUsageRow("20260801", "alpha", "early", 1, "a", 1, "a", true, model.StatsMetrics{RequestSuccess: 1}, 0),
		testUsageRow("20260801", "alpha", "early", 2, "a", 2, "a", true, model.StatsMetrics{RequestSuccess: 1}, 0),
		testUsageRow("20260801", "alpha", "early", 1, "a", 1, "a", false, model.StatsMetrics{RequestFailed: 1}, 0),
	}
	for _, row := range rows {
		if err := StatsUsageUpdate(row); err != nil {
			t.Fatalf("StatsUsageUpdate: %v", err)
		}
	}
	if err := StatsSaveDB(ctx); err != nil {
		t.Fatalf("StatsSaveDB: %v", err)
	}

	res2, err := StatsAnalyticsGet(ctx, StatsAnalyticsFilter{})
	if err != nil {
		t.Fatalf("StatsAnalyticsGet: %v", err)
	}
	if len(res2.ByModel) != 2 || res2.ByModel[0].Name != "alpha" || res2.ByModel[1].Name != "zeta" {
		t.Fatalf("by_model 排序错误：%+v", res2.ByModel)
	}
	if len(res2.ByAPIKey) != 3 || res2.ByAPIKey[0].Name != "a" || res2.ByAPIKey[0].ID != 1 || res2.ByAPIKey[1].ID != 2 || res2.ByAPIKey[2].ID != 3 {
		t.Fatalf("by_api_key 应按 (name,id) 排序：%+v", res2.ByAPIKey)
	}
	if len(res2.ByChannel) != 3 || res2.ByChannel[0].Name != "a" || res2.ByChannel[0].ID != 1 || res2.ByChannel[1].ID != 2 || res2.ByChannel[2].Name != "z" {
		t.Fatalf("by_channel 排序错误：%+v", res2.ByChannel)
	}

	dims2, err := StatsAnalyticsDimensionsGet(ctx)
	if err != nil {
		t.Fatalf("StatsAnalyticsDimensionsGet: %v", err)
	}
	if len(dims2.Models) != 2 || dims2.Models[0] != "alpha" || dims2.Models[1] != "zeta" {
		t.Fatalf("models 排序错误：%+v", dims2.Models)
	}
	if len(dims2.APIKeys) != 3 || dims2.APIKeys[0].Name != "a" || dims2.APIKeys[0].ID != 1 || dims2.APIKeys[1].ID != 2 || dims2.APIKeys[2].ID != 3 {
		t.Fatalf("api_keys 排序错误：%+v", dims2.APIKeys)
	}
}
