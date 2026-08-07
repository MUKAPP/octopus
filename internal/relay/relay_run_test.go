package relay

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/gin-gonic/gin"
	"github.com/looplj/axonhub/llm"
)

// TestNewRelayRunActualModelStartsEmpty 防止未知实际模型再次被请求模型预填：
// 初始化时 ActualModel 必须为空，选择真实候选上游模型后才赋值。
func TestNewRelayRunActualModelStartsEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	if err := db.InitDB("sqlite", filepath.Join(t.TempDir(), "relay.db"), false); err != nil {
		t.Fatalf("初始化测试数据库失败：%v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	ctx := context.Background()

	channel := model.Channel{
		Name:    "test-channel",
		Type:    llm.APIFormatOpenAIChatCompletion,
		Enabled: true,
		BaseUrls: []model.BaseUrl{
			{URL: "http://127.0.0.1:9"},
		},
		Keys: []model.ChannelKey{
			{Enabled: true, ChannelKey: "sk-test"},
		},
	}
	if err := op.ChannelCreate(&channel, ctx); err != nil {
		t.Fatalf("创建渠道失败：%v", err)
	}
	group := model.Group{
		Name: "gpt-4o",
		Mode: model.GroupModeRoundRobin,
		Items: []model.GroupItem{
			{ChannelID: channel.ID, ModelName: "gpt-4o-2024"},
		},
	}
	if err := op.GroupCreate(&group, ctx); err != nil {
		t.Fatalf("创建分组失败：%v", err)
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req, err := http.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatalf("构造请求失败：%v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	c.Set("api_key_id", 1)

	run, err := newRelayRun(c, llm.APIFormatOpenAIChatCompletion, newInbound(llm.APIFormatOpenAIChatCompletion))
	if err != nil {
		t.Fatalf("newRelayRun: %v", err)
	}
	if run.metrics.ActualModel != "" {
		t.Fatalf("newRelayRun 时 ActualModel = %q, want 空字符串", run.metrics.ActualModel)
	}
	if run.metrics.RequestModel != "gpt-4o" {
		t.Fatalf("RequestModel = %q, want gpt-4o", run.metrics.RequestModel)
	}

	if !run.iter.Next() {
		t.Fatal("iter.Next() 应选中首个候选")
	}
	attempt, err := run.prepareAttempt()
	if err != nil {
		t.Fatalf("prepareAttempt: %v", err)
	}
	if attempt == nil {
		t.Fatal("prepareAttempt 返回 nil attempt")
	}
	if run.metrics.ActualModel != "gpt-4o-2024" {
		t.Fatalf("选中 attempt 后 ActualModel = %q, want gpt-4o-2024", run.metrics.ActualModel)
	}
}
