package op

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
)

func initLogTestDB(t *testing.T) context.Context {
	t.Helper()
	ctx, _ := initLogTestDBWithPath(t)
	return ctx
}

func initLogTestDBWithPath(t *testing.T) (context.Context, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "log.db")
	if err := db.InitDB("sqlite", path, false); err != nil {
		t.Fatalf("初始化测试数据库失败：%v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	ctx := context.Background()
	if err := settingRefreshCache(ctx); err != nil {
		t.Fatalf("刷新设置缓存失败：%v", err)
	}
	// 测试共享包级全局状态，逐个清理避免相互污染
	resetRelayLogState()
	return ctx, path
}

func resetRelayLogState() {
	relayLogCacheLock.Lock()
	relayLogLiveCache = make([]model.RelayLog, 0, relayLogLiveMaxSize)
	relayLogPending = make([]model.RelayLog, 0, relayLogPendingFlushSize)
	relayLogCacheLock.Unlock()
	activeRelayRequestsLock.Lock()
	activeRelayRequests = make(map[int64]*ActiveRelayRequest)
	activeRelayRequestsLock.Unlock()
}

func TestRelayActiveLifecycle(t *testing.T) {
	initLogTestDB(t)

	sub := RelayActiveSubscribe()
	defer RelayActiveUnsubscribe(sub)

	request := ActiveRelayRequest{
		ID:                1001,
		Time:              1700000000,
		RequestModelName:  "gpt-4o",
		RequestAPIKeyName: "key-a",
	}
	RelayActiveAdd(request)

	event := <-sub
	if event.Type != RelayActiveEventStart {
		t.Fatalf("start 事件类型错误：got %q, want %q", event.Type, RelayActiveEventStart)
	}
	if event.Request.ID != request.ID || event.Request.RequestModelName != request.RequestModelName {
		t.Fatalf("start 事件请求内容错误：%+v", event.Request)
	}

	active := RelayActiveList()
	if len(active) != 1 || active[0].ID != request.ID {
		t.Fatalf("进行中列表错误：%+v", active)
	}

	RelayActiveUpdate(request.ID, "channel-x", "gpt-4o-2024-11-20")

	event = <-sub
	if event.Type != RelayActiveEventUpdate {
		t.Fatalf("update 事件类型错误：got %q, want %q", event.Type, RelayActiveEventUpdate)
	}
	if event.Request.ChannelName != "channel-x" || event.Request.ActualModelName != "gpt-4o-2024-11-20" {
		t.Fatalf("update 事件内容错误：%+v", event.Request)
	}

	active = RelayActiveList()
	if len(active) != 1 || active[0].ChannelName != "channel-x" {
		t.Fatalf("update 未生效：%+v", active)
	}

	// 更新不存在的 ID 不应报错也不应产生事件
	RelayActiveUpdate(9999, "channel-y", "model-y")
	select {
	case event := <-sub:
		t.Fatalf("更新不存在的请求不应产生事件：%+v", event)
	default:
	}

	RelayActiveRemove(request.ID)

	event = <-sub
	if event.Type != RelayActiveEventEnd || event.ID != request.ID {
		t.Fatalf("end 事件错误：%+v", event)
	}

	if active := RelayActiveList(); len(active) != 0 {
		t.Fatalf("移除后仍存在进行中请求：%+v", active)
	}

	// 重复移除不应产生事件
	RelayActiveRemove(request.ID)
	select {
	case event := <-sub:
		t.Fatalf("重复移除不应产生事件：%+v", event)
	default:
	}
}

func TestRelayLogAddPreservesID(t *testing.T) {
	ctx := initLogTestDB(t)

	relayLog := model.RelayLog{
		ID:               1002,
		Time:             1700000000,
		RequestModelName: "gpt-4o",
		ChannelName:      "channel-x",
	}
	if err := RelayLogAdd(ctx, relayLog); err != nil {
		t.Fatalf("添加日志失败：%v", err)
	}

	logs, err := RelayLogList(ctx, nil, nil, 1, 20)
	if err != nil {
		t.Fatalf("查询日志失败：%v", err)
	}
	if len(logs) != 1 || logs[0].ID != relayLog.ID {
		t.Fatalf("日志 ID 未被保留：got %+v, want ID %d", logs, relayLog.ID)
	}
}

func TestRelayLogAddGeneratesIDWhenEmpty(t *testing.T) {
	ctx := initLogTestDB(t)

	relayLog := model.RelayLog{
		Time:             1700000000,
		RequestModelName: "gpt-4o",
	}
	if err := RelayLogAdd(ctx, relayLog); err != nil {
		t.Fatalf("添加日志失败：%v", err)
	}

	logs, err := RelayLogList(ctx, nil, nil, 1, 20)
	if err != nil {
		t.Fatalf("查询日志失败：%v", err)
	}
	if len(logs) != 1 || logs[0].ID == 0 {
		t.Fatalf("空 ID 未生成：%+v", logs)
	}
}

func TestRelayLogFlushRetainsPendingOnFailure(t *testing.T) {
	ctx, dbPath := initLogTestDBWithPath(t)
	if err := SettingSetString(model.SettingKeyRelayLogKeepEnabled, "true"); err != nil {
		t.Fatalf("启用保存失败：%v", err)
	}

	const n = 5
	for i := 0; i < n; i++ {
		if err := RelayLogAdd(ctx, model.RelayLog{ID: int64(2000 + i), Time: 1700000000, RequestModelName: "gpt-4o"}); err != nil {
			t.Fatalf("添加日志 %d 失败：%v", i, err)
		}
	}

	// 关闭数据库模拟 flush 失败
	if err := db.Close(); err != nil {
		t.Fatalf("关闭数据库失败：%v", err)
	}
	if err := relayLogFlushToDB(ctx); err == nil {
		t.Fatalf("数据库关闭后 flush 应失败")
	}

	relayLogCacheLock.Lock()
	if len(relayLogPending) != n {
		relayLogCacheLock.Unlock()
		t.Fatalf("flush 失败后 pending 丢失：got %d, want %d", len(relayLogPending), n)
	}
	relayLogCacheLock.Unlock()

	// 恢复数据库：原批次原序保留，重试后只写一次
	if err := db.InitDB("sqlite", dbPath, false); err != nil {
		t.Fatalf("重新初始化数据库失败：%v", err)
	}
	if err := settingRefreshCache(ctx); err != nil {
		t.Fatalf("刷新设置缓存失败：%v", err)
	}
	if err := relayLogFlushToDB(ctx); err != nil {
		t.Fatalf("恢复后 flush 失败：%v", err)
	}

	var count int64
	if err := db.GetDB().Model(&model.RelayLog{}).Where("id >= ? AND id < ?", 2000, 2000+n).Count(&count).Error; err != nil {
		t.Fatalf("统计行数失败：%v", err)
	}
	if count != n {
		t.Fatalf("恢复后只写一次：got %d 行, want %d", count, n)
	}
	relayLogCacheLock.Lock()
	defer relayLogCacheLock.Unlock()
	if len(relayLogPending) != 0 {
		t.Fatalf("恢复后 pending 未排空：%d", len(relayLogPending))
	}
}

func TestRelayLogSwitchBetweenAddAndFlush(t *testing.T) {
	ctx := initLogTestDB(t)
	now := time.Now().Unix()
	if err := SettingSetString(model.SettingKeyRelayLogKeepEnabled, "true"); err != nil {
		t.Fatalf("启用保存失败：%v", err)
	}

	// 开启时 add：进入 pending；add 与 flush 之间关闭开关，flush 仍应排空已接受的 pending
	if err := RelayLogAdd(ctx, model.RelayLog{ID: 3001, Time: now}); err != nil {
		t.Fatalf("添加日志失败：%v", err)
	}
	if err := SettingSetString(model.SettingKeyRelayLogKeepEnabled, "false"); err != nil {
		t.Fatalf("关闭保存失败：%v", err)
	}
	if err := relayLogFlushToDB(ctx); err != nil {
		t.Fatalf("关闭后 flush 失败：%v", err)
	}
	var count int64
	if err := db.GetDB().Model(&model.RelayLog{}).Where("id = ?", 3001).Count(&count).Error; err != nil {
		t.Fatalf("统计失败：%v", err)
	}
	if count != 1 {
		t.Fatalf("开关切换不应丢日志：got %d 行, want 1", count)
	}

	// 关闭时 add：只进 live ring，不写数据库
	if err := RelayLogAdd(ctx, model.RelayLog{ID: 3002, Time: now}); err != nil {
		t.Fatalf("添加日志失败：%v", err)
	}
	if err := relayLogFlushToDB(ctx); err != nil {
		t.Fatalf("flush 失败：%v", err)
	}
	count = 0
	if err := db.GetDB().Model(&model.RelayLog{}).Where("id = ?", 3002).Count(&count).Error; err != nil {
		t.Fatalf("统计失败：%v", err)
	}
	if count != 0 {
		t.Fatalf("关闭时 add 不应写库：got %d 行, want 0", count)
	}

	// 重新开启：新 add 正常入 pending，旧行不滞留
	if err := SettingSetString(model.SettingKeyRelayLogKeepEnabled, "true"); err != nil {
		t.Fatalf("重新启用保存失败：%v", err)
	}
	if err := RelayLogAdd(ctx, model.RelayLog{ID: 3003, Time: now}); err != nil {
		t.Fatalf("添加日志失败：%v", err)
	}
	relayLogCacheLock.Lock()
	if len(relayLogPending) != 1 || relayLogPending[0].ID != 3003 {
		relayLogCacheLock.Unlock()
		t.Fatalf("重新启用后 pending 异常：%+v", relayLogPending)
	}
	relayLogCacheLock.Unlock()
	if err := RelayLogSaveDBTask(ctx); err != nil {
		t.Fatalf("保存任务失败：%v", err)
	}
	count = 0
	if err := db.GetDB().Model(&model.RelayLog{}).Where("id = ?", 3003).Count(&count).Error; err != nil {
		t.Fatalf("统计失败：%v", err)
	}
	if count != 1 {
		t.Fatalf("重新启用后日志未落库：got %d 行, want 1", count)
	}
}

func TestRelayLogClearWipesDBLiveAndPending(t *testing.T) {
	ctx := initLogTestDB(t)
	now := time.Now().Unix()
	if err := SettingSetString(model.SettingKeyRelayLogKeepEnabled, "true"); err != nil {
		t.Fatalf("启用保存失败：%v", err)
	}

	for i := 0; i < 5; i++ {
		if err := RelayLogAdd(ctx, model.RelayLog{ID: int64(4000 + i), Time: now}); err != nil {
			t.Fatalf("添加日志失败：%v", err)
		}
	}
	if err := RelayLogSaveDBTask(ctx); err != nil {
		t.Fatalf("保存任务失败：%v", err)
	}
	for i := 0; i < 3; i++ {
		if err := RelayLogAdd(ctx, model.RelayLog{ID: int64(4100 + i), Time: now}); err != nil {
			t.Fatalf("添加日志失败：%v", err)
		}
	}

	if err := RelayLogClear(ctx); err != nil {
		t.Fatalf("清空失败：%v", err)
	}
	var count int64
	if err := db.GetDB().Model(&model.RelayLog{}).Count(&count).Error; err != nil {
		t.Fatalf("统计失败：%v", err)
	}
	if count != 0 {
		t.Fatalf("清空后数据库仍有 %d 行", count)
	}
	relayLogCacheLock.Lock()
	if len(relayLogLiveCache) != 0 || len(relayLogPending) != 0 {
		relayLogCacheLock.Unlock()
		t.Fatalf("清空后内存残留：live=%d pending=%d", len(relayLogLiveCache), len(relayLogPending))
	}
	relayLogCacheLock.Unlock()

	// clear 之后到达的日志按当前开关重新入队
	if err := RelayLogAdd(ctx, model.RelayLog{ID: 4200, Time: now}); err != nil {
		t.Fatalf("清空后添加日志失败：%v", err)
	}
	relayLogCacheLock.Lock()
	defer relayLogCacheLock.Unlock()
	if len(relayLogLiveCache) != 1 || len(relayLogPending) != 1 || relayLogLiveCache[0].ID != 4200 || relayLogPending[0].ID != 4200 {
		t.Fatalf("清空后入队异常：live=%v pending=%v", relayLogLiveCache, relayLogPending)
	}
}

func TestRelayLogCleanupRetention(t *testing.T) {
	ctx := initLogTestDB(t)
	now := time.Now().Unix()
	old := now - 3*24*3600 // 3 天前

	// 保存开启 + 保留 1 天：只删超过保留期的行
	if err := SettingSetString(model.SettingKeyRelayLogKeepEnabled, "true"); err != nil {
		t.Fatalf("启用保存失败：%v", err)
	}
	if err := SettingSetString(model.SettingKeyRelayLogKeepPeriod, "1"); err != nil {
		t.Fatalf("设置保留期失败：%v", err)
	}
	if err := db.GetDB().Create(&model.RelayLog{ID: 5001, Time: old}).Error; err != nil {
		t.Fatalf("造旧行失败：%v", err)
	}
	if err := db.GetDB().Create(&model.RelayLog{ID: 5002, Time: now}).Error; err != nil {
		t.Fatalf("造新行失败：%v", err)
	}
	if err := RelayLogSaveDBTask(ctx); err != nil {
		t.Fatalf("保存任务失败：%v", err)
	}
	var count int64
	if err := db.GetDB().Model(&model.RelayLog{}).Count(&count).Error; err != nil {
		t.Fatalf("统计失败：%v", err)
	}
	if count != 1 {
		t.Fatalf("保留期清理错误：got %d 行, want 1", count)
	}
	var remains model.RelayLog
	if err := db.GetDB().First(&remains, 5002).Error; err != nil {
		t.Fatalf("保留期内新行被删除：%v", err)
	}

	// 保存关闭：删除全部已持久化日志，而不是保留关闭前的旧行
	if err := SettingSetString(model.SettingKeyRelayLogKeepEnabled, "false"); err != nil {
		t.Fatalf("关闭保存失败：%v", err)
	}
	if err := RelayLogSaveDBTask(ctx); err != nil {
		t.Fatalf("保存任务失败：%v", err)
	}
	count = 0
	if err := db.GetDB().Model(&model.RelayLog{}).Count(&count).Error; err != nil {
		t.Fatalf("统计失败：%v", err)
	}
	if count != 0 {
		t.Fatalf("保存关闭后清理错误：got %d 行, want 0", count)
	}
}

func TestRelayLogTimeIndex(t *testing.T) {
	initLogTestDB(t)
	if !db.GetDB().Migrator().HasIndex(&model.RelayLog{}, "idx_relay_logs_time") {
		t.Fatalf("idx_relay_logs_time 索引缺失")
	}
	// 造足够多的行，让查询计划选择索引而不是全表扫描
	now := time.Now().Unix()
	for i := 0; i < 2000; i++ {
		if err := db.GetDB().Create(&model.RelayLog{ID: int64(6000 + i), Time: now - int64(i)}).Error; err != nil {
			t.Fatalf("造行失败：%v", err)
		}
	}
	var plan []struct {
		Detail string
	}
	cutoff := now - 3600
	if err := db.GetDB().Raw("EXPLAIN QUERY PLAN DELETE FROM relay_logs WHERE time < ?", cutoff).Scan(&plan).Error; err != nil {
		t.Fatalf("EXPLAIN 失败：%v", err)
	}
	found := false
	for _, p := range plan {
		if strings.Contains(p.Detail, "idx_relay_logs_time") {
			found = true
		}
	}
	if !found {
		t.Fatalf("删除计划未使用 idx_relay_logs_time：%+v", plan)
	}
}

func TestRelayLogConcurrentAddFlushClear(t *testing.T) {
	ctx := initLogTestDB(t)
	if err := SettingSetString(model.SettingKeyRelayLogKeepEnabled, "true"); err != nil {
		t.Fatalf("启用保存失败：%v", err)
	}

	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 40; i++ {
				id := int64(g*10000 + i)
				_ = RelayLogAdd(ctx, model.RelayLog{ID: id, Time: time.Now().Unix(), RequestModelName: "gpt-4o"})
			}
		}(g)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 8; i++ {
			_ = relayLogFlushToDB(ctx)
			time.Sleep(2 * time.Millisecond)
		}
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(5 * time.Millisecond)
		for i := 0; i < 3; i++ {
			_ = RelayLogClear(ctx)
			time.Sleep(3 * time.Millisecond)
		}
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 6; i++ {
			_ = SettingSetString(model.SettingKeyRelayLogKeepEnabled, "true")
			time.Sleep(time.Millisecond)
			_ = SettingSetString(model.SettingKeyRelayLogKeepEnabled, "false")
			time.Sleep(time.Millisecond)
		}
	}()
	wg.Wait()

	// 最终一致性：数据库中同一 ID 不重复
	var rows []model.RelayLog
	if err := db.GetDB().Find(&rows).Error; err != nil {
		t.Fatalf("查询失败：%v", err)
	}
	seen := make(map[int64]bool)
	for _, r := range rows {
		if seen[r.ID] {
			t.Fatalf("数据库出现重复行 ID %d", r.ID)
		}
		seen[r.ID] = true
	}
}

func TestRelayLogListDedupAfterFlush(t *testing.T) {
	ctx := initLogTestDB(t)
	now := time.Now().Unix()
	if err := SettingSetString(model.SettingKeyRelayLogKeepEnabled, "true"); err != nil {
		t.Fatalf("启用保存失败：%v", err)
	}

	for i := 0; i < 5; i++ {
		if err := RelayLogAdd(ctx, model.RelayLog{ID: int64(7000 + i), Time: now}); err != nil {
			t.Fatalf("添加日志失败：%v", err)
		}
	}
	if err := relayLogFlushToDB(ctx); err != nil {
		t.Fatalf("flush 失败：%v", err)
	}

	// flush 后日志仍在 live ring 中，数据库也有同名行；列表必须去重
	logs, err := RelayLogList(ctx, nil, nil, 1, 20)
	if err != nil {
		t.Fatalf("查询日志失败：%v", err)
	}
	if len(logs) != 5 {
		t.Fatalf("flush 后列表重复：got %d 条, want 5", len(logs))
	}
	seen := make(map[int64]bool)
	for _, l := range logs {
		if seen[l.ID] {
			t.Fatalf("列表出现重复 ID %d", l.ID)
		}
		seen[l.ID] = true
	}

	// 深分页：越过 live ring 后从数据库补足且不重复
	logs2, err := RelayLogList(ctx, nil, nil, 2, 5)
	if err != nil {
		t.Fatalf("查询第二页失败：%v", err)
	}
	if len(logs2) != 0 {
		t.Fatalf("第二页应为空，got %d", len(logs2))
	}
}
