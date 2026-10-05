package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/gin-gonic/gin"
	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/pipeline"
)

func TestWriteFinalErrorForwardsUpstreamHTTPStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	run := &relayRun{
		c:         ctx,
		inAdapter: newInbound(llm.APIFormatOpenAIChatCompletion),
		metrics:   &RelayMetrics{},
	}

	run.writeFinalError(context.Background(), pipeline.WrapUpstreamError(&llm.ResponseError{
		StatusCode: http.StatusTooManyRequests,
		Detail: llm.ErrorDetail{
			Message: "rate limited",
			Type:    "rate_limit_error",
			Code:    "rate_limit_exceeded",
		},
	}))

	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusTooManyRequests)
	}

	var response struct {
		Error llm.ErrorDetail `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Error != (llm.ErrorDetail{
		Message: "rate limited",
		Type:    "rate_limit_error",
		Code:    "rate_limit_exceeded",
	}) {
		t.Fatalf("error = %#v, want upstream error details", response.Error)
	}
}

func TestWriteFinalErrorUsesFailedDependencyForTransportFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	run := &relayRun{
		c:         ctx,
		inAdapter: newInbound(llm.APIFormatOpenAIChatCompletion),
		metrics:   &RelayMetrics{},
	}

	run.writeFinalError(context.Background(), errors.New("dial upstream: connection refused"))

	if recorder.Code != http.StatusFailedDependency {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusFailedDependency)
	}
}

func TestWriteFinalErrorWritesCommittedSSEError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	run := &relayRun{
		c:         ctx,
		inAdapter: newInbound(llm.APIFormatOpenAIChatCompletion),
		metrics:   &RelayMetrics{},
	}
	ctx.Header("Content-Type", "text/event-stream")
	ctx.Status(http.StatusOK)
	if _, err := ctx.Writer.Write(relaySSEHeartbeatComment); err != nil {
		t.Fatal(err)
	}
	ctx.Writer.Flush()
	body := recorder.Body.String()

	run.writeFinalError(context.Background(), fmt.Errorf("channel primary failed: %w", pipeline.WrapUpstreamError(&llm.ResponseError{
		StatusCode: http.StatusTooManyRequests,
		Detail: llm.ErrorDetail{
			Message: "rate limited",
			Type:    "rate_limit_error",
		},
	})))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	got := recorder.Body.String()
	if got == body || !strings.Contains(got, "event:error\n") {
		t.Fatalf("body = %q, want an SSE error event after %q", got, body)
	}
	if !strings.Contains(got, "data: {\"error\":") || !strings.Contains(got, "rate limited") || !strings.Contains(got, "rate_limit_error") {
		t.Fatalf("body = %q, want OpenAI-formatted upstream error data", got)
	}
}

func TestRelayStatusTransportObservesActualStatusAndIsolatesAttempts(t *testing.T) {
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{}, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/late" {
			close(firstStarted)
			<-releaseFirst
			w.WriteHeader(520)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(func() {
		close(releaseFirst)
		upstream.Close()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sharedClient := upstream.Client()
	base := sharedClient.Transport
	first := &relayAttempt{}
	firstClient := *sharedClient
	firstClient.Transport = &relayStatusTransport{base: base, statusCode: &first.upstreamStatusCode}
	firstRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, upstream.URL+"/late", nil)
	if err != nil {
		t.Fatal(err)
	}
	firstDone := make(chan error, 1)
	go func() {
		response, err := firstClient.Do(firstRequest)
		if response != nil {
			response.Body.Close()
		}
		firstDone <- err
	}()
	select {
	case <-firstStarted:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	// 模拟首 token 超时后主流程保存当前观察值，再开始下一次尝试。
	metrics := &RelayMetrics{UpstreamStatusCode: int(first.upstreamStatusCode.Load())}
	if metrics.UpstreamStatusCode != 0 {
		t.Fatalf("unobserved first status = %d, want 0", metrics.UpstreamStatusCode)
	}
	second := &relayAttempt{}
	secondClient := *sharedClient
	secondClient.Transport = &relayStatusTransport{base: base, statusCode: &second.upstreamStatusCode}
	secondRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, upstream.URL+"/next", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := secondClient.Do(secondRequest)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	metrics.UpstreamStatusCode = int(second.upstreamStatusCode.Load())
	if metrics.UpstreamStatusCode != http.StatusCreated {
		t.Fatalf("second observed status = %d, want 201", metrics.UpstreamStatusCode)
	}
	if sharedClient.Transport != base {
		t.Fatal("shared client transport was changed")
	}

	releaseFirst <- struct{}{}
	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if got := first.upstreamStatusCode.Load(); got != 520 {
		t.Fatalf("late first status = %d, want 520", got)
	}
	if metrics.UpstreamStatusCode != http.StatusCreated || second.upstreamStatusCode.Load() != http.StatusCreated {
		t.Fatal("late first response changed second attempt or request status")
	}

	// 无 HTTP 响应的失败必须覆盖旧观察值，而不是沿用此前的响应码。
	canceledCtx, cancelRequest := context.WithCancel(context.Background())
	cancelRequest()
	canceledRequest, err := http.NewRequestWithContext(canceledCtx, http.MethodGet, upstream.URL+"/next", nil)
	if err != nil {
		t.Fatal(err)
	}
	if response, err := secondClient.Do(canceledRequest); err == nil || response != nil {
		t.Fatalf("canceled request returned response=%v, error=%v", response, err)
	}
	if got := second.upstreamStatusCode.Load(); got != 0 {
		t.Fatalf("status after transport failure = %d, want 0", got)
	}
}

func TestRelayUpstreamErrorDiagnosticsAndCause(t *testing.T) {
	body := strings.Repeat("fixture detail ", 80) + "\n请减少输入长度"
	cases := []struct {
		name string
		err  relayUpstreamError
		want string
	}{
		{
			name: "complete raw body",
			err:  relayUpstreamError{statusCode: 520, body: []byte(" \n" + body + "\n ")},
			want: "upstream HTTP 520: " + body,
		},
		{
			name: "unknown status without body",
			err:  relayUpstreamError{statusCode: 520},
			want: "upstream HTTP 520",
		},
		{
			name: "known status without body",
			err:  relayUpstreamError{statusCode: http.StatusTooManyRequests},
			want: "upstream HTTP 429: Too Many Requests",
		},
		{
			name: "detail metadata",
			err: relayUpstreamError{statusCode: 429, detail: llm.ErrorDetail{
				Message: "fixture rejected", Type: "rate_limit_error", Code: "fixture_code", Param: "model", RequestID: "fixture-id",
			}},
			want: "upstream HTTP 429: fixture rejected, type: rate_limit_error, code: fixture_code, param: model, request_id: fixture-id",
		},
		{
			name: "body without HTTP response",
			err:  relayUpstreamError{body: []byte("fixture detail")},
			want: "upstream error: fixture detail",
		},
		{
			name: "ordinary local cause",
			err:  relayUpstreamError{cause: errors.New("dial upstream: connection refused")},
			want: "dial upstream: connection refused",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := test.err.Error(); got != test.want {
				t.Fatalf("diagnostic = %q, want %q", got, test.want)
			}
		})
	}
	wrapped := &relayUpstreamError{cause: context.Canceled, statusCode: 520, body: []byte("fixture detail")}
	if !errors.Is(wrapped, context.Canceled) {
		t.Fatal("upstream facts lost cancellation cause")
	}
}

// relayGatewayHarness 通过真实上游 HTTP、Handler 和最终内存日志验证完整转发链路。
// 数据库和缓存为进程全局状态，使用此 fixture 的测试不得调用 t.Parallel。
type relayGatewayHarness struct {
	requestModel string
	channel      model.Channel
	group        model.Group
	upstream     *httptest.Server
}

func newRelayGatewayHarness(t *testing.T, outboundType llm.APIFormat, upstreamHandler http.Handler) *relayGatewayHarness {
	t.Helper()
	previousMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previousMode) })
	if err := db.InitDB("sqlite", filepath.Join(t.TempDir(), "gateway.db"), false); err != nil {
		t.Fatalf("初始化测试数据库失败：%v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("关闭测试数据库失败：%v", err)
		}
	})
	op.RelayLogStoreClear()
	if err := op.InitCache(); err != nil {
		t.Fatalf("初始化测试缓存失败：%v", err)
	}

	fixture := &relayGatewayHarness{requestModel: "fixture-" + t.Name()}
	t.Cleanup(func() {
		ctx := context.Background()
		op.RelayLogStoreClear()
		// 先把 usage 等增量刷新到临时库，避免带入下一个 fixture；不访问 data/。
		if err := op.StatsSaveDB(ctx); err != nil {
			t.Errorf("清理测试统计增量失败：%v", err)
		}
		if fixture.group.ID != 0 {
			if err := op.GroupDel(fixture.group.ID, ctx); err != nil {
				t.Errorf("清理测试分组失败：%v", err)
			}
		}
		if fixture.channel.ID != 0 {
			if err := op.ChannelDel(fixture.channel.ID, ctx); err != nil {
				t.Errorf("清理测试渠道失败：%v", err)
			}
		}
		for _, stats := range []any{
			&model.StatsTotal{}, &model.StatsDaily{}, &model.StatsHourly{},
			&model.StatsChannel{}, &model.StatsModel{}, &model.StatsAPIKey{}, &model.StatsUsage{},
		} {
			if err := db.GetDB().WithContext(ctx).Where("1 = 1").Delete(stats).Error; err != nil {
				t.Errorf("清理临时统计 %T 失败：%v", stats, err)
			}
		}
		if err := op.InitCache(); err != nil {
			t.Errorf("清理测试缓存失败：%v", err)
		}
	})

	upstream := httptest.NewServer(upstreamHandler)
	t.Cleanup(upstream.Close)
	fixture.upstream = upstream
	fixture.channel = model.Channel{
		Name:    fixture.requestModel,
		Type:    outboundType,
		Enabled: true,
		BaseUrls: []model.BaseUrl{
			{URL: upstream.URL},
		},
		Keys: []model.ChannelKey{
			{Enabled: true, ChannelKey: "fixture-only"},
		},
	}
	if err := op.ChannelCreate(&fixture.channel, context.Background()); err != nil {
		t.Fatalf("创建测试渠道失败：%v", err)
	}
	fixture.group = model.Group{
		Name: fixture.requestModel,
		Mode: model.GroupModeFailover,
		Items: []model.GroupItem{
			{ChannelID: fixture.channel.ID, ModelName: fixture.requestModel},
		},
	}
	if err := op.GroupCreate(&fixture.group, context.Background()); err != nil {
		t.Fatalf("创建测试分组失败：%v", err)
	}
	return fixture
}

func (fixture *relayGatewayHarness) request(t *testing.T, inboundType llm.APIFormat, path, body string) (*httptest.ResponseRecorder, op.RelayLogOverview) {
	t.Helper()
	router := gin.New()
	// 鉴权中间件不属于此回归范围；Handler 使用与既有 relay fixture 相同的上下文身份。
	router.POST(path, func(c *gin.Context) {
		c.Set("api_key_id", 0)
		c.Next()
	}, Handler(inboundType))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	logs := op.RelayLogStoreList(nil, nil, 1, 100)
	for _, relayLog := range logs {
		if relayLog.RequestModelName == fixture.requestModel {
			if relayLog.CompletedAt == nil || relayLog.State == op.RelayLogStateRunning || relayLog.State == op.RelayLogStateCommitted {
				t.Fatalf("Handler 返回后日志尚未完成：%#v", relayLog)
			}
			return recorder, relayLog
		}
	}
	t.Fatalf("Handler 未保存最终日志：status=%d, body=%q", recorder.Code, recorder.Body.String())
	return nil, op.RelayLogOverview{}
}

func TestRelayGatewayHTTPErrorDetails(t *testing.T) {
	const nativeBody = `{"error":{"message":"fixture rejected","type":"invalid_request_error","code":"fixture_code","param":"model","request_id":"fixture-id"},"extra":"retained"}`
	const anthropicBody = `{"type":"error","error":{"type":"rate_limit_error","message":"fixture anthropic rejected"},"request_id":"fixture-id","extra":"retained"}`
	const geminiBody = `{"error":{"code":503,"message":"fixture gemini rejected","status":"UNAVAILABLE"},"request_id":"fixture-id"}`
	const numericBody = `{"error":{"message":"fixture numeric rejected","type":"invalid_request_error","code":1234,"param":"model","request_id":"fixture-id"},"extra":"retained"}`
	const plaintext = "fixture upstream detail\n请减少输入长度"
	largeMessage := "fixture UTF-8 start\n" + strings.Repeat("请减少输入长度", 14000) + "\nfixture UTF-8 end"
	largeBody, err := json.Marshal(map[string]any{
		"error": map[string]string{"message": largeMessage, "type": "api_error"},
		"extra": "retained",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(largeBody) <= 262144 || len(largeBody) >= 1<<20 {
		t.Fatalf("large error fixture size = %d, want between log and upstream read limits", len(largeBody))
	}

	type httpErrorCase struct {
		name            string
		outbound        llm.APIFormat
		inbound         llm.APIFormat
		upstreamStatus  int
		clientStatus    int
		body            string
		contentType     string
		headers         map[string]string
		detail          llm.ErrorDetail
		native          bool
		local           bool
		closeUpstream   bool
		diagnosticParts []string
	}
	cases := []httpErrorCase{
		{name: "520_plaintext", upstreamStatus: 520, clientStatus: 520, body: plaintext, contentType: "text/plain; charset=utf-8", detail: llm.ErrorDetail{Message: plaintext, Type: "api_error"}, diagnosticParts: []string{plaintext, "520"}},
		{name: "520_empty", upstreamStatus: 520, clientStatus: 520, contentType: "text/plain", detail: llm.ErrorDetail{Message: "upstream HTTP 520", Type: "api_error"}, diagnosticParts: []string{"520"}},
		{name: "520_plaintext_responses", inbound: llm.APIFormatOpenAIResponse, upstreamStatus: 520, clientStatus: 520, body: plaintext, contentType: "text/plain", detail: llm.ErrorDetail{Message: plaintext, Type: "api_error"}},
		{name: "520_plaintext_messages", inbound: llm.APIFormatAnthropicMessage, upstreamStatus: 520, clientStatus: 520, body: plaintext, contentType: "text/plain", detail: llm.ErrorDetail{Message: plaintext, Type: "api_error"}},
		{name: "openai_numeric_code_native", upstreamStatus: 400, clientStatus: 400, body: numericBody, native: true, detail: llm.ErrorDetail{Message: "fixture numeric rejected", Type: "invalid_request_error", Code: "1234", Param: "model", RequestID: "fixture-id"}},
		{name: "openai_numeric_code_messages", inbound: llm.APIFormatAnthropicMessage, upstreamStatus: 400, clientStatus: 400, body: numericBody, detail: llm.ErrorDetail{Message: "fixture numeric rejected", Type: "invalid_request_error", Code: "1234", Param: "model", RequestID: "fixture-id"}},
		{name: "embedding_native", outbound: llm.APIFormatOpenAIEmbedding, inbound: llm.APIFormatOpenAIEmbedding, upstreamStatus: 400, clientStatus: 400, body: nativeBody, native: true, detail: llm.ErrorDetail{Message: "fixture rejected", Type: "invalid_request_error", Code: "fixture_code", Param: "model", RequestID: "fixture-id"}},
		{name: "image_native", outbound: llm.APIFormatOpenAIImageGeneration, inbound: llm.APIFormatOpenAIImageGeneration, upstreamStatus: 500, clientStatus: 500, body: nativeBody, native: true, detail: llm.ErrorDetail{Message: "fixture rejected", Type: "invalid_request_error", Code: "fixture_code", Param: "model", RequestID: "fixture-id"}},
		{name: "doubao_native", outbound: model.ChannelTypeDoubao, upstreamStatus: 429, clientStatus: 429, body: nativeBody, native: true, detail: llm.ErrorDetail{Message: "fixture rejected", Type: "invalid_request_error", Code: "fixture_code", Param: "model", RequestID: "fixture-id"}},
		{name: "200_chat_error", upstreamStatus: 200, clientStatus: 500, body: nativeBody, native: true, detail: llm.ErrorDetail{Message: "fixture rejected", Type: "invalid_request_error", Code: "fixture_code", Param: "model", RequestID: "fixture-id"}},
		{name: "200_anthropic_error", outbound: llm.APIFormatAnthropicMessage, inbound: llm.APIFormatAnthropicMessage, upstreamStatus: 200, clientStatus: 500, body: anthropicBody, native: true, detail: llm.ErrorDetail{Message: "fixture anthropic rejected", Type: "rate_limit_error", RequestID: "fixture-id"}},
		{name: "200_gemini_error", outbound: llm.APIFormatGeminiContents, upstreamStatus: 200, clientStatus: 500, body: geminiBody, detail: llm.ErrorDetail{Message: "fixture gemini rejected", Type: "UNAVAILABLE", Code: "503", RequestID: "fixture-id"}},
		{name: "200_responses_failed", outbound: llm.APIFormatOpenAIResponse, inbound: llm.APIFormatOpenAIResponse, upstreamStatus: 200, clientStatus: 500, native: true, body: `{"id":"resp-fixture","object":"response","status":"failed","error":{"message":"fixture responses rejected","type":"server_error","code":"fixture_response","param":"model"},"output":[],"request_id":"fixture-id"}`, detail: llm.ErrorDetail{Message: "fixture responses rejected", Type: "server_error", Code: "fixture_response", Param: "model", RequestID: "fixture-id"}},
		{name: "invalid_json_424", upstreamStatus: 200, clientStatus: 424, body: "not-json", local: true, diagnosticParts: []string{"invalid character"}},
		{name: "empty_body_424", upstreamStatus: 200, clientStatus: 424, local: true, diagnosticParts: []string{"empty"}},
		{name: "empty_content_424", upstreamStatus: 200, clientStatus: 424, body: `{"id":"chatcmpl-fixture","object":"chat.completion","created":1,"model":"fixture","choices":[{"index":0,"message":{"role":"assistant","content":""},"finish_reason":"stop"}]}`, local: true, diagnosticParts: []string{"empty response"}},
		{name: "connection_failure_424", clientStatus: 424, local: true, closeUpstream: true, diagnosticParts: []string{"127.0.0.1"}},
		{name: "large_utf8_native", upstreamStatus: 500, clientStatus: 500, body: string(largeBody), native: true, detail: llm.ErrorDetail{Message: largeMessage, Type: "api_error"}},
		{name: "large_utf8_messages", inbound: llm.APIFormatAnthropicMessage, upstreamStatus: 500, clientStatus: 500, body: string(largeBody), detail: llm.ErrorDetail{Message: largeMessage, Type: "api_error"}},
		{name: "nonstandard_json_reason", upstreamStatus: 500, clientStatus: 500, body: `{"error":{"reason":"fixture reason only"}}`, detail: llm.ErrorDetail{Message: `{"error":{"reason":"fixture reason only"}}`, Type: "api_error"}},
		{name: "empty_json_message", upstreamStatus: 500, clientStatus: 500, body: `{"error":{"message":"","type":"server_error","code":"fixture_empty"}}`, detail: llm.ErrorDetail{Message: `{"error":{"message":"","type":"server_error","code":"fixture_empty"}}`, Type: "server_error", Code: "fixture_empty"}},
		{name: "200_errors_envelope", upstreamStatus: 200, clientStatus: 500, body: `{"errors":{"message":"fixture errors rejected","type":"invalid_request_error","code":1234,"param":"model","request_id":"fixture-id"}}`, detail: llm.ErrorDetail{Message: "fixture errors rejected", Type: "invalid_request_error", Code: "1234", Param: "model", RequestID: "fixture-id"}},
		{name: "200_data_error", upstreamStatus: 200, clientStatus: 500, body: `{"data":{"error":{"message":"fixture data rejected","type":"rate_limit_error","code":"fixture_data","param":"model"},"request_id":"fixture-id"}}`, detail: llm.ErrorDetail{Message: "fixture data rejected", Type: "rate_limit_error", Code: "fixture_data", Param: "model", RequestID: "fixture-id"}},
		{name: "nested_request_id_priority", upstreamStatus: 400, clientStatus: 400, body: `{"error":{"message":"fixture nested rejected","type":"api_error","request_id":"fixture-nested-id"},"request_id":"fixture-top-id","data":{"request_id":"fixture-data-id"}}`, native: true, detail: llm.ErrorDetail{Message: "fixture nested rejected", Type: "api_error", RequestID: "fixture-nested-id"}},
		{name: "error_string_top_request_id_priority", upstreamStatus: 400, clientStatus: 400, body: `{"error":"fixture string rejected","request_id":"fixture-top-id","data":{"request_id":"fixture-data-id"}}`, detail: llm.ErrorDetail{Message: "fixture string rejected", Type: "api_error", RequestID: "fixture-top-id"}},
		{name: "error_string_data_request_id_priority", upstreamStatus: 400, clientStatus: 400, body: `{"error":"fixture string rejected","data":{"request_id":"fixture-data-id"}}`, detail: llm.ErrorDetail{Message: "fixture string rejected", Type: "api_error", RequestID: "fixture-data-id"}},
		{name: "error_string_x_request_id_fallback", upstreamStatus: 400, clientStatus: 400, body: `{"error":"fixture string rejected"}`, detail: llm.ErrorDetail{Message: "fixture string rejected", Type: "api_error", RequestID: "fixture-id"}},
		{name: "error_string_request_id_header_fallback", upstreamStatus: 400, clientStatus: 400, body: `{"error":"fixture string rejected"}`, headers: map[string]string{"Request-Id": "fixture-header-id"}, detail: llm.ErrorDetail{Message: "fixture string rejected", Type: "api_error", RequestID: "fixture-header-id"}},
	}
	for _, status := range []int{400, 401, 403, 429, 500} {
		cases = append(cases, httpErrorCase{
			name: fmt.Sprintf("%d_native_json", status), upstreamStatus: status, clientStatus: status,
			body: nativeBody, native: true,
			detail: llm.ErrorDetail{Message: "fixture rejected", Type: "invalid_request_error", Code: "fixture_code", Param: "model", RequestID: "fixture-id"},
		})
	}
	for _, target := range []struct {
		name   string
		format llm.APIFormat
	}{
		{name: "chat", format: llm.APIFormatOpenAIChatCompletion},
		{name: "responses", format: llm.APIFormatOpenAIResponse},
		{name: "messages", format: llm.APIFormatAnthropicMessage},
	} {
		cases = append(cases,
			httpErrorCase{name: "anthropic_429_" + target.name, outbound: llm.APIFormatAnthropicMessage, inbound: target.format, upstreamStatus: 429, clientStatus: 429, body: anthropicBody, native: target.format == llm.APIFormatAnthropicMessage, detail: llm.ErrorDetail{Message: "fixture anthropic rejected", Type: "rate_limit_error", RequestID: "fixture-id"}},
			httpErrorCase{name: "gemini_503_" + target.name, outbound: llm.APIFormatGeminiContents, inbound: target.format, upstreamStatus: 503, clientStatus: 503, body: geminiBody, detail: llm.ErrorDetail{Message: "fixture gemini rejected", Type: "UNAVAILABLE", Code: "503", RequestID: "fixture-id"}},
		)
	}
	cases = append(cases,
		httpErrorCase{name: "openai_429_responses_native", inbound: llm.APIFormatOpenAIResponse, upstreamStatus: 429, clientStatus: 429, body: nativeBody, native: true, detail: llm.ErrorDetail{Message: "fixture rejected", Type: "invalid_request_error", Code: "fixture_code", Param: "model", RequestID: "fixture-id"}},
		httpErrorCase{name: "openai_429_messages", inbound: llm.APIFormatAnthropicMessage, upstreamStatus: 429, clientStatus: 429, body: nativeBody, detail: llm.ErrorDetail{Message: "fixture rejected", Type: "invalid_request_error", Code: "fixture_code", Param: "model", RequestID: "fixture-id"}},
	)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.outbound == "" {
				tc.outbound = llm.APIFormatOpenAIChatCompletion
			}
			if tc.inbound == "" {
				tc.inbound = llm.APIFormatOpenAIChatCompletion
			}
			if tc.contentType == "" {
				tc.contentType = "application/json; charset=utf-8"
			}
			if tc.headers == nil {
				tc.headers = relayGatewayAllowedErrorHeaders()
			}
			upstreamRequests := make(chan string, 8)
			fixture := newRelayGatewayHarness(t, tc.outbound, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				upstreamRequests <- r.Method + " " + r.URL.Path
				w.Header().Set("Content-Type", tc.contentType)
				w.Header().Set("Content-Length", strconv.Itoa(len(tc.body)))
				for key, value := range tc.headers {
					w.Header().Set(key, value)
				}
				w.Header().Set("Set-Cookie", "fixture-secret=must-not-leak")
				w.Header().Set("Connection", "close")
				w.Header().Set("Content-Encoding", "identity")
				w.Header().Set("WWW-Authenticate", "Bearer fixture-secret")
				w.Header().Set("Authorization", "Bearer fixture-secret")
				w.WriteHeader(tc.upstreamStatus)
				_, _ = w.Write([]byte(tc.body))
			}))
			if tc.closeUpstream {
				// 关闭 OS 分配的监听器后保留渠道 URL，走真实连接失败而不是伪造 transport。
				fixture.upstream.Close()
			}
			path, body := fixture.nonStreamRequest(t, tc.inbound)
			recorder, relayLog := fixture.request(t, tc.inbound, path, body)
			if !tc.closeUpstream {
				select {
				case got := <-upstreamRequests:
					if !strings.HasPrefix(got, "POST ") {
						t.Errorf("upstream request = %q, want POST", got)
					}
				default:
					t.Fatal("Handler did not request the real HTTP upstream")
				}
			}
			if recorder.Code != tc.clientStatus {
				t.Errorf("downstream status = %d, want %d", recorder.Code, tc.clientStatus)
			}
			if !utf8.Valid(recorder.Body.Bytes()) {
				t.Error("downstream error body contains invalid UTF-8")
			}
			if got := recorder.Header().Get("Content-Type"); tc.native {
				if got != tc.contentType {
					t.Errorf("native Content-Type = %q, want %q", got, tc.contentType)
				}
			} else if !strings.HasPrefix(got, "application/json") {
				t.Errorf("transformed Content-Type = %q, want application/json", got)
			}
			response := decodeRelayGatewayJSON(t, recorder.Body.String())
			if tc.native {
				if recorder.Body.String() != tc.body {
					t.Error("native error body bytes were not preserved")
				}
				if !reflect.DeepEqual(response, decodeRelayGatewayJSON(t, tc.body)) {
					t.Error("native error envelope lost or changed upstream fields")
				}
			}
			if tc.local {
				message, _ := response["message"].(string)
				if message == "" {
					t.Error("local 424 error message is empty")
				}
				for _, part := range tc.diagnosticParts {
					if !strings.Contains(message, part) {
						t.Errorf("local client message does not contain cause detail %q", part)
					}
				}
			} else {
				assertRelayGatewayErrorDetail(t, response, recorder.Header(), tc.inbound, tc.detail, tc.native)
				for key, value := range tc.headers {
					if got := recorder.Header().Get(key); got != value {
						t.Errorf("forwarded %s = %q, want %q", key, got, value)
					}
				}
				if !tc.native {
					if got := recorder.Header().Get("Content-Length"); got != "" && got != strconv.Itoa(recorder.Body.Len()) {
						t.Errorf("rebuilt body Content-Length = %q, actual length = %d", got, recorder.Body.Len())
					}
				}
			}
			for _, key := range []string{"Set-Cookie", "Connection", "Content-Encoding", "Transfer-Encoding", "WWW-Authenticate", "Authorization"} {
				if got := recorder.Header().Get(key); got != "" {
					t.Errorf("unsafe upstream header %s leaked: %q", key, got)
				}
			}
			parts := append([]string(nil), tc.diagnosticParts...)
			if !tc.local && tc.body != "" {
				// 日志保留源 bytes；JSON 中的换行保持转义，而非再次格式化 message。
				parts = append(parts, tc.body)
			} else if tc.detail.Message != "" {
				parts = append(parts, tc.detail.Message)
			}
			assertRelayGatewayFailureLog(t, relayLog, tc.upstreamStatus, parts)
			assertRelayGatewayResponseBody(t, relayLog, recorder.Body.String())
		})
	}

	const successPrefix = `{"id":"chatcmpl-fixture","object":"chat.completion","created":1,"model":"fixture","choices":[{"index":0,"message":{"role":"assistant","content":"fixture success with error word"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}`
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "200_error_null_success", body: successPrefix + `,"error":null}`},
		{name: "200_error_empty_string_success", body: successPrefix + `,"error":""}`},
		{name: "200_error_empty_object_success", body: successPrefix + `,"error":{}}`},
		{name: "200_success_text_contains_error", body: successPrefix + `}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newRelayGatewayHarness(t, llm.APIFormatOpenAIChatCompletion, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			path, body := fixture.nonStreamRequest(t, llm.APIFormatOpenAIChatCompletion)
			recorder, relayLog := fixture.request(t, llm.APIFormatOpenAIChatCompletion, path, body)
			if recorder.Code != http.StatusOK || relayLog.State != op.RelayLogStateSuccess {
				t.Fatalf("success response status=%d, log state=%q, error=%q", recorder.Code, relayLog.State, relayLog.Error)
			}
			var response llm.Response
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode successful client response: %v", err)
			}
			if len(response.Choices) != 1 || response.Choices[0].Message == nil || response.Choices[0].Message.Content.Content == nil || *response.Choices[0].Message.Content.Content != "fixture success with error word" {
				t.Error("ordinary successful content containing error was changed or lost")
			}
			if relayLog.UpstreamStatusCode != 200 || len(relayLog.Attempts) != 1 {
				t.Fatalf("success request upstream status=%d, attempt count=%d", relayLog.UpstreamStatusCode, len(relayLog.Attempts))
			}
			if attempt := relayLog.Attempts[0]; attempt.UpstreamStatusCode != 200 || attempt.Status != model.AttemptSuccess {
				t.Errorf("success attempt = %#v, want successful HTTP 200", attempt)
			}
			if relayLog.Error != "" {
				t.Errorf("successful request has diagnostic error %q", relayLog.Error)
			}
		})
	}
}

func (fixture *relayGatewayHarness) nonStreamRequest(t *testing.T, format llm.APIFormat) (string, string) {
	t.Helper()
	request := map[string]any{"model": fixture.requestModel}
	var path string
	switch format {
	case llm.APIFormatOpenAIResponse:
		path = "/v1/responses"
		request["input"] = "hi"
	case llm.APIFormatOpenAIEmbedding:
		path = "/v1/embeddings"
		request["input"] = "hi"
	case llm.APIFormatOpenAIImageGeneration:
		path = "/v1/images/generations"
		request["prompt"] = "fixture image"
	case llm.APIFormatOpenAIChatCompletion, llm.APIFormatAnthropicMessage:
		path = "/v1/chat/completions"
		request["messages"] = []map[string]string{{"role": "user", "content": "hi"}}
		if format == llm.APIFormatAnthropicMessage {
			path = "/v1/messages"
			request["max_tokens"] = 16
		}
	default:
		t.Fatalf("unsupported inbound fixture format %q", format)
	}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	return path, string(body)
}

func relayGatewayAllowedErrorHeaders() map[string]string {
	return map[string]string{
		"Retry-After": "7", "X-Request-Id": "fixture-id", "Request-Id": "fixture-header-id",
		"X-RateLimit-Limit-Requests": "10", "X-RateLimit-Remaining-Requests": "0", "RateLimit-Reset": "7",
	}
}

func decodeRelayGatewayJSON(t *testing.T, body string) map[string]any {
	t.Helper()
	var result map[string]any
	decoder := json.NewDecoder(strings.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&result); err != nil {
		t.Fatalf("decode client/upstream JSON envelope: %v", err)
	}
	return result
}

func assertRelayGatewayErrorDetail(t *testing.T, response map[string]any, headers http.Header, format llm.APIFormat, want llm.ErrorDetail, native bool) {
	t.Helper()
	detail, ok := response["error"].(map[string]any)
	if !ok {
		t.Fatalf("client protocol error is not an object: %#v", response["error"])
	}
	message, _ := detail["message"].(string)
	if message == "" || !strings.Contains(message, want.Message) {
		t.Errorf("client message length=%d does not contain complete source message length=%d", len(message), len(want.Message))
	}
	if got, _ := detail["type"].(string); got != want.Type {
		t.Errorf("client error type = %q, want %q", got, want.Type)
	}
	if format == llm.APIFormatAnthropicMessage {
		if response["type"] != "error" {
			t.Errorf("Messages error envelope type = %#v, want error", response["type"])
		}
		if !native {
			for key, value := range map[string]string{"code": want.Code, "param": want.Param} {
				if value != "" && !strings.Contains(message, " ["+key+"="+value+"]") {
					t.Errorf("Messages message lost source %s=%q", key, value)
				}
			}
		}
		if want.RequestID != "" && response["request_id"] != want.RequestID {
			t.Errorf("Messages request_id = %#v, want %q", response["request_id"], want.RequestID)
		}
		return
	}
	for key, value := range map[string]string{"code": want.Code, "param": want.Param, "request_id": want.RequestID} {
		if value == "" {
			continue
		}
		if format == llm.APIFormatOpenAIResponse && !native && key == "param" {
			if !strings.Contains(message, " [param="+value+"]") {
				t.Errorf("Responses message lost source param=%q", value)
			}
		} else if format == llm.APIFormatOpenAIResponse && !native && key == "request_id" {
			if got := headers.Get("X-Request-Id"); got != value {
				t.Errorf("Responses X-Request-Id = %q, want %q", got, value)
			}
		} else if native && key == "request_id" && detail[key] == nil && response[key] == value {
			// 原生 envelope 的 request_id 可在顶层；保持原始字段位置。
		} else if got := fmt.Sprint(detail[key]); got != value {
			t.Errorf("client error %s = %q, want %q", key, got, value)
		}
	}
}

func assertRelayGatewayFailureLog(t *testing.T, relayLog op.RelayLogOverview, upstreamStatus int, parts []string) {
	t.Helper()
	if relayLog.State != op.RelayLogStateFailed {
		t.Errorf("final log state = %q, want failed", relayLog.State)
	}
	if relayLog.UpstreamStatusCode != upstreamStatus {
		t.Errorf("request upstream status = %d, want %d", relayLog.UpstreamStatusCode, upstreamStatus)
	}
	if relayLog.Error == "" {
		t.Error("final log diagnostic is empty")
	}
	if len(relayLog.Attempts) != 1 {
		t.Fatalf("attempt count = %d, want 1", len(relayLog.Attempts))
	}
	attempt := relayLog.Attempts[0]
	if attempt.Status != model.AttemptFailed {
		t.Errorf("attempt status = %q, want failed", attempt.Status)
	}
	if attempt.UpstreamStatusCode != upstreamStatus {
		t.Errorf("attempt upstream status = %d, want %d", attempt.UpstreamStatusCode, upstreamStatus)
	}
	if attempt.Msg == "" {
		t.Error("attempt diagnostic is empty")
	}
	for _, part := range parts {
		if !strings.Contains(relayLog.Error, part) {
			t.Errorf("final log diagnostic length=%d lost source detail length=%d", len(relayLog.Error), len(part))
		}
		if !strings.Contains(attempt.Msg, part) {
			t.Errorf("attempt diagnostic length=%d lost source detail length=%d", len(attempt.Msg), len(part))
		}
	}
}

// 正文读取合同在 op getter 上验证；鉴权 REST 路由由隔离实例验证，不在此引入 server 包循环依赖。
func assertRelayGatewayResponseBody(t *testing.T, relayLog op.RelayLogOverview, clientBody string) {
	t.Helper()
	const responseMaxBytes = 262144
	wantBody := clientBody
	wantTruncated := len(clientBody) > responseMaxBytes
	if wantTruncated {
		wantBody = clientBody[:responseMaxBytes]
		for !utf8.ValidString(wantBody) {
			wantBody = wantBody[:len(wantBody)-1]
		}
	}
	if relayLog.ResponseContent != wantBody || relayLog.ResponseContentTruncated != wantTruncated {
		t.Errorf("final log response body length=%d truncated=%t, want client body prefix length=%d truncated=%t", len(relayLog.ResponseContent), relayLog.ResponseContentTruncated, len(wantBody), wantTruncated)
	}
	body, truncated, found := op.RelayLogStoreBody(relayLog.ID, true)
	if !found || body != wantBody || truncated != wantTruncated {
		t.Errorf("response body getter found=%t length=%d truncated=%t, want length=%d truncated=%t", found, len(body), truncated, len(wantBody), wantTruncated)
	}
	if len(body) > responseMaxBytes || !utf8.ValidString(body) {
		t.Errorf("stored response body exceeds limit or splits UTF-8: length=%d valid=%t", len(body), utf8.ValidString(body))
	}
}
