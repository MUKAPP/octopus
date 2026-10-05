package relay

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
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

func (fixture *relayGatewayHarness) addFallback(t *testing.T, url string) model.Channel {
	t.Helper()
	channel := model.Channel{
		Name: fixture.requestModel + "-fallback", Type: llm.APIFormatOpenAIChatCompletion, Enabled: true,
		BaseUrls: []model.BaseUrl{{URL: url}}, Keys: []model.ChannelKey{{Enabled: true, ChannelKey: "fixture-fallback"}},
	}
	if err := op.ChannelCreate(&channel, context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := op.ChannelDel(channel.ID, context.Background()); err != nil {
			t.Errorf("清理备用测试渠道失败：%v", err)
		}
	})
	group, err := op.GroupUpdate(&model.GroupUpdateRequest{
		ID:         fixture.group.ID,
		ItemsToAdd: []model.GroupItemAddRequest{{ChannelID: channel.ID, ModelName: fixture.requestModel + "-fallback", Priority: 1, Weight: 1}},
	}, context.Background())
	if err != nil {
		t.Fatal(err)
	}
	fixture.group = *group
	return channel
}

func TestRelayGatewayRetryErrorHistory(t *testing.T) {
	for _, mode := range []string{"success", "all_http_fail", "final_connection_fail"} {
		t.Run(mode, func(t *testing.T) {
			release := make(chan struct{})
			started := make(chan struct{})
			var releaseOnce, startedOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			t.Cleanup(unblock)
			var firstHits, fallbackHits atomic.Int32
			fixture := newRelayGatewayHarness(t, llm.APIFormatOpenAIChatCompletion, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				firstHits.Add(1)
				startedOnce.Do(func() { close(started) })
				select {
				case <-release:
				case <-r.Context().Done():
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = io.WriteString(w, `{"error":{"message":"first fixture rejected","type":"rate_limit_error","code":"first_fixture","param":"model"}}`)
			}))
			fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fallbackHits.Add(1)
				if mode != "success" {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusServiceUnavailable)
					_, _ = io.WriteString(w, `{"error":{"message":"last fixture rejected","type":"api_error","code":"last_fixture"}}`)
					return
				}
				// 不能把成功流假定成 200：上游实际给出 201，下游 SSE 仍为 200。
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusCreated)
				writeRelayGatewaySSE(w, "", relayGatewayChatChunk("retry fixture success", false, ""))
				writeRelayGatewaySSE(w, "", relayGatewayChatChunk("", true, ""))
				writeRelayGatewaySSE(w, "", "[DONE]")
			}))
			t.Cleanup(fallback.Close)
			if mode == "final_connection_fail" {
				fallback.Close()
			}
			fixture.addFallback(t, fallback.URL)
			before := op.StatsTotalGet()
			writer := newRecordingRelayWriter(0)
			_, done := fixture.startStreamRequest(t, llm.APIFormatOpenAIChatCompletion, writer)
			waitClosed(t, started, "first HTTP request")
			waitRelayGatewayFlush(t, writer, ": ping\n\n")
			if got := writer.body(); !strings.HasPrefix(got, ": ping\n\n") || strings.Contains(got, "data:") {
				t.Fatalf("pre-content heartbeat emitted an error/model frame: %q", got)
			}
			unblock()
			waitRelayGatewayDone(t, done)
			if writer.recorder.Code != http.StatusOK {
				t.Errorf("heartbeat-committed retry status=%d, want 200", writer.recorder.Code)
			}
			relayLog := fixture.finalLog(t)
			if len(relayLog.Attempts) != 2 {
				t.Fatalf("retry attempt count=%d, want 2: %#v", len(relayLog.Attempts), relayLog)
			}
			first, last := relayLog.Attempts[0], relayLog.Attempts[1]
			if first.Status != model.AttemptFailed || first.UpstreamStatusCode != 429 || !strings.Contains(first.Msg, "first fixture rejected") || !strings.Contains(first.Msg, "first_fixture") {
				t.Errorf("first failed attempt lost source status/detail: %#v", first)
			}
			failures := relayGatewayFailureFrames(relayGatewaySSEFrames(t, writer.body()))
			after := op.StatsTotalGet()
			if mode == "success" {
				if relayLog.State != op.RelayLogStateSuccess || relayLog.Error != "" || last.Status != model.AttemptSuccess || last.UpstreamStatusCode != 201 || relayLog.UpstreamStatusCode != 201 {
					t.Errorf("successful retry lost actual final status/state: %#v", relayLog)
				}
				if len(failures) != 0 || !strings.Contains(writer.body(), "retry fixture success") || strings.Contains(writer.body(), "first fixture rejected") {
					t.Errorf("successful retry leaked earlier error: %q", writer.body())
				}
				response := decodeRelayGatewayJSON(t, relayLog.ResponseContent)
				choices, _ := response["choices"].([]any)
				if len(choices) != 1 {
					t.Fatalf("successful retry aggregated choices=%#v, want one", response["choices"])
				}
				choice, _ := choices[0].(map[string]any)
				message, _ := choice["message"].(map[string]any)
				if message["content"] != "retry fixture success" || choice["finish_reason"] != "stop" || response["error"] != nil {
					t.Errorf("successful retry stored wrong aggregate: %#v", response)
				}
				if strings.Contains(relayLog.ResponseContent, "first fixture rejected") || strings.Contains(relayLog.ResponseContent, ": ping") {
					t.Error("successful retry response body contains preceding error or heartbeat")
				}
				assertRelayGatewayResponseBody(t, relayLog, relayLog.ResponseContent)
				if after.RequestSuccess-before.RequestSuccess != 1 || after.RequestFailed-before.RequestFailed != 0 {
					t.Errorf("successful retry statistics delta: before=%#v after=%#v", before, after)
				}
			} else {
				wantStatus, wantMessage := 503, "last fixture rejected"
				if mode == "final_connection_fail" {
					wantStatus, wantMessage = 0, ""
				}
				if relayLog.State != op.RelayLogStateFailed || last.Status != model.AttemptFailed || last.UpstreamStatusCode != wantStatus || relayLog.UpstreamStatusCode != wantStatus || last.Msg == "" {
					t.Errorf("failed retry inherited preceding 429/lost terminal state: %#v", relayLog)
				}
				for _, frame := range relayGatewaySSEFrames(t, writer.body()) {
					if frame.done {
						t.Error("exhausted retry emitted a normal completion marker")
					}
				}
				if len(failures) != 1 {
					t.Fatalf("exhausted retry failure count=%d, want only last failure: %q", len(failures), writer.body())
				}
				assertRelayGatewayResponseBody(t, relayLog, failures[0].data)
				detail, _ := failures[0].payload["error"].(map[string]any)
				message, _ := detail["message"].(string)
				if message == "" || !strings.Contains(message, wantMessage) || strings.Contains(writer.body(), "first fixture rejected") {
					t.Errorf("exhausted retry emitted wrong source failure: %q", writer.body())
				}
				if wantMessage != "" && (!strings.Contains(last.Msg, wantMessage) || !strings.Contains(relayLog.Error, wantMessage)) {
					t.Errorf("last source diagnostic lost: %#v", relayLog)
				}
				if after.RequestFailed-before.RequestFailed != 1 || after.RequestSuccess-before.RequestSuccess != 0 {
					t.Errorf("failed retry statistics delta: before=%#v after=%#v", before, after)
				}
			}
			wantFallbackHits := int32(1)
			if mode == "final_connection_fail" {
				wantFallbackHits = 0
			}
			if firstHits.Load() != 1 || fallbackHits.Load() != wantFallbackHits {
				t.Errorf("unexpected upstream calls: first=%d fallback=%d", firstHits.Load(), fallbackHits.Load())
			}
		})
	}
}
