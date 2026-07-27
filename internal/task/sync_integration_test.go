package task

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/looplj/axonhub/llm"
)

func TestSyncModelsDoesNotReAddRemovedModelToAutoGroup(t *testing.T) {
	if err := db.InitDB("sqlite", filepath.Join(t.TempDir(), "sync.db"), false); err != nil {
		t.Fatalf("初始化测试数据库失败：%v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("关闭测试数据库失败：%v", err)
		}
	})
	if err := op.InitCache(); err != nil {
		t.Fatalf("初始化测试缓存失败：%v", err)
	}
	ctx := context.Background()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/sub2api/billing":
			http.NotFound(w, r)
		case "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"gpt-keep"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	channel := model.Channel{
		Name:      "auto-sync-channel",
		Type:      llm.APIFormatOpenAIChatCompletion,
		BaseUrls:  []model.BaseUrl{{URL: upstream.URL}},
		Model:     "gpt-keep,gpt-removed",
		AutoSync:  true,
		AutoGroup: model.AutoGroupTypeExact,
	}
	if err := op.ChannelCreate(&channel, ctx); err != nil {
		t.Fatalf("创建测试渠道失败：%v", err)
	}
	group := model.Group{Name: "gpt-removed"}
	if err := op.GroupCreate(&group, ctx); err != nil {
		t.Fatalf("创建测试分组失败：%v", err)
	}
	if err := op.GroupItemAdd(&model.GroupItem{
		GroupID:   group.ID,
		ChannelID: channel.ID,
		ModelName: "gpt-removed",
		Priority:  1,
		Weight:    1,
	}, ctx); err != nil {
		t.Fatalf("创建测试分组项失败：%v", err)
	}
	if err := op.LLMCreate(model.LLMInfo{Name: "gpt-keep"}, ctx); err != nil {
		t.Fatalf("创建测试模型价格失败：%v", err)
	}

	SyncModelsTask()

	updated, err := op.ChannelGet(channel.ID, ctx)
	if err != nil {
		t.Fatalf("获取更新后的渠道失败：%v", err)
	}
	if updated.Model != "gpt-keep" {
		t.Fatalf("渠道模型 = %q, want %q", updated.Model, "gpt-keep")
	}
	items, err := op.GroupItemList(group.ID, ctx)
	if err != nil {
		t.Fatalf("查询分组项失败：%v", err)
	}
	if len(items) != 0 {
		t.Fatalf("已移除的模型不应被自动分组重新加入：%+v", items)
	}
}
