package op

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bestruirui/octopus/internal/db"
)

func initLogTestDB(t *testing.T) context.Context {
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
	return ctx
}

func resetRelayLogState() {
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
