package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/gin-gonic/gin"
	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/httpclient"
	"github.com/looplj/axonhub/llm/pipeline"
	"github.com/looplj/axonhub/llm/transformer"
)

type manualRelaySSEHeartbeatTicker struct {
	ticks   chan time.Time
	stopped chan struct{}
	once    sync.Once
}

func newManualRelaySSEHeartbeatTicker() *manualRelaySSEHeartbeatTicker {
	return &manualRelaySSEHeartbeatTicker{
		ticks:   make(chan time.Time, 8),
		stopped: make(chan struct{}),
	}
}

func (t *manualRelaySSEHeartbeatTicker) Chan() <-chan time.Time { return t.ticks }

func (t *manualRelaySSEHeartbeatTicker) Stop() {
	t.once.Do(func() { close(t.stopped) })
}

func (t *manualRelaySSEHeartbeatTicker) isStopped() bool {
	select {
	case <-t.stopped:
		return true
	default:
		return false
	}
}

type relayStreamStep struct {
	event *httpclient.StreamEvent
	err   error
	ended bool
}

type controllableRelayStream struct {
	steps   chan relayStreamStep
	entered chan struct{}
	closed  chan struct{}
	once    sync.Once

	mu      sync.Mutex
	current *httpclient.StreamEvent
	err     error
}

func newControllableRelayStream() *controllableRelayStream {
	return &controllableRelayStream{
		steps:   make(chan relayStreamStep, 8),
		entered: make(chan struct{}),
		closed:  make(chan struct{}),
	}
}

func (s *controllableRelayStream) Next() bool {
	select {
	case <-s.entered:
	default:
		close(s.entered)
	}
	select {
	case step := <-s.steps:
		s.mu.Lock()
		s.current = step.event
		s.err = step.err
		s.mu.Unlock()
		return !step.ended && step.err == nil
	case <-s.closed:
		return false
	}
}

func (s *controllableRelayStream) Current() *httpclient.StreamEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current
}

func (s *controllableRelayStream) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

func (s *controllableRelayStream) Close() error {
	s.once.Do(func() { close(s.closed) })
	return nil
}

type recordingRelayWriter struct {
	recorder *httptest.ResponseRecorder
	failAt   int

	mu        sync.Mutex
	writes    int
	snapshots []string
	flushed   chan string
}

func newRecordingRelayWriter(failAt int) *recordingRelayWriter {
	return &recordingRelayWriter{
		recorder: httptest.NewRecorder(),
		failAt:   failAt,
		flushed:  make(chan string, 128),
	}
}

func (w *recordingRelayWriter) Header() http.Header { return w.recorder.Header() }

func (w *recordingRelayWriter) WriteHeader(statusCode int) { w.recorder.WriteHeader(statusCode) }

func (w *recordingRelayWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.writes++
	if w.failAt > 0 && w.writes == w.failAt {
		return 0, errRelayWriterSentinel
	}
	return w.recorder.Write(data)
}

func (w *recordingRelayWriter) Flush() {
	w.mu.Lock()
	w.recorder.Flush()
	snapshot := w.recorder.Body.String()
	w.snapshots = append(w.snapshots, snapshot)
	w.mu.Unlock()
	w.flushed <- snapshot
}
func (w *recordingRelayWriter) waitFlush(t *testing.T) string {
	t.Helper()
	select {
	case snapshot := <-w.flushed:
		return snapshot
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for response flush")
		return ""
	}
}

func (w *recordingRelayWriter) body() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.recorder.Body.String()
}

var errRelayWriterSentinel = errors.New("relay writer sentinel")

// deterministicInbound 只提供流聚合结果，直接流测试不会调用其它转换方法。
type deterministicInbound struct {
	transformer.Inbound
}

func (deterministicInbound) AggregateStreamChunks(context.Context, []*httpclient.StreamEvent) ([]byte, llm.ResponseMeta, error) {
	return []byte(`{"ok":true}`), llm.ResponseMeta{}, nil
}

func newTestRelayAttempt(t *testing.T, writer *recordingRelayWriter) *relayAttempt {
	t.Helper()
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(writer)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return &relayAttempt{relayRun: &relayRun{
		c:         ctx,
		inAdapter: deterministicInbound{},
		metrics:   &RelayMetrics{StartTime: time.Now()},
		group:     model.Group{},
	}}
}

func waitClosed(t *testing.T, ch <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}

func TestRelayAttemptProcessStreamWithHeartbeatWritesBeforePipelineWait(t *testing.T) {
	writer := newRecordingRelayWriter(0)
	attempt := newTestRelayAttempt(t, writer)
	ticker := newManualRelaySSEHeartbeatTicker()
	started := make(chan struct{})
	release := make(chan struct{})
	stream := newControllableRelayStream()

	resultCh := make(chan struct {
		result *pipeline.Result
		ticker relaySSEHeartbeatTicker
		cancel context.CancelFunc
		err    error
	}, 1)
	go func() {
		result, transferredTicker, cancel, err := attempt.processStreamWithHeartbeat(context.Background(), ticker, func(context.Context) (*pipeline.Result, error) {
			close(started)
			<-release
			return &pipeline.Result{Stream: true, EventStream: stream}, nil
		})
		resultCh <- struct {
			result *pipeline.Result
			ticker relaySSEHeartbeatTicker
			cancel context.CancelFunc
			err    error
		}{result, transferredTicker, cancel, err}
	}()

	waitClosed(t, started, "pipeline start")
	if got := writer.waitFlush(t); got != ": ping\n\n" {
		t.Fatalf("initial flush = %q, want %q", got, ": ping\n\n")
	}
	if writer.recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", writer.recorder.Code)
	}
	for key, want := range map[string]string{
		"Content-Type":      "text/event-stream",
		"Cache-Control":     "no-cache",
		"Connection":        "keep-alive",
		"X-Accel-Buffering": "no",
	} {
		if got := writer.Header().Get(key); got != want {
			t.Errorf("header %s = %q, want %q", key, got, want)
		}
	}

	ticker.ticks <- time.Now()
	if got := writer.waitFlush(t); got != ": ping\n\n: ping\n\n" {
		t.Fatalf("first tick body = %q", got)
	}
	ticker.ticks <- time.Now()
	if got := writer.waitFlush(t); got != ": ping\n\n: ping\n\n: ping\n\n" {
		t.Fatalf("second tick body = %q", got)
	}

	close(release)
	processed := <-resultCh
	if processed.err != nil || processed.result == nil || !processed.result.Stream {
		t.Fatalf("process result = %#v, err = %v", processed.result, processed.err)
	}
	if processed.ticker != ticker || processed.cancel == nil {
		t.Fatal("expected ticker and process cancel ownership transfer")
	}

	writeDone := make(chan error, 1)
	go func() {
		writeDone <- attempt.writeStreamWithHeartbeatTicker(context.Background(), stream, processed.ticker)
	}()
	stream.steps <- relayStreamStep{ended: true}
	if err := <-writeDone; err != nil {
		t.Fatalf("write stream: %v", err)
	}
	processed.cancel()
	waitClosed(t, ticker.stopped, "ticker stop")
	waitClosed(t, stream.closed, "stream close")
}

func TestRelayAttemptWriteStreamPassesEventAfterHeartbeat(t *testing.T) {
	writer := newRecordingRelayWriter(0)
	attempt := newTestRelayAttempt(t, writer)
	if err := attempt.prepareSSEStreamResponse(); err != nil {
		t.Fatal(err)
	}
	ticker := newManualRelaySSEHeartbeatTicker()
	stream := newControllableRelayStream()
	writeDone := make(chan error, 1)
	go func() { writeDone <- attempt.writeStreamWithHeartbeatTicker(context.Background(), stream, ticker) }()

	ticker.ticks <- time.Now()
	if got := writer.waitFlush(t); !strings.HasSuffix(got, ": ping\n\n") {
		t.Fatalf("heartbeat body = %q", got)
	}
	stream.steps <- relayStreamStep{event: &httpclient.StreamEvent{Type: "message", Data: []byte("payload")}}
	stream.steps <- relayStreamStep{ended: true}
	if err := <-writeDone; err != nil {
		t.Fatalf("write stream: %v", err)
	}
	body := writer.body()
	if strings.Count(body, "payload") != 1 || !strings.Contains(body, "message") {
		t.Fatalf("body = %q, want one message event with payload", body)
	}
	if !attempt.streamEventWritten || !attempt.responseFinalized() {
		t.Fatal("real stream event should finalize response")
	}
	waitClosed(t, ticker.stopped, "ticker stop")
	waitClosed(t, stream.closed, "stream close")
}

func TestRelayAttemptWriteStreamDoesNotCountDoneAsFirstToken(t *testing.T) {
	writer := newRecordingRelayWriter(0)
	attempt := newTestRelayAttempt(t, writer)
	if err := attempt.prepareSSEStreamResponse(); err != nil {
		t.Fatal(err)
	}
	ticker := newManualRelaySSEHeartbeatTicker()
	stream := newControllableRelayStream()
	writeDone := make(chan error, 1)
	go func() { writeDone <- attempt.writeStreamWithHeartbeatTicker(context.Background(), stream, ticker) }()

	stream.steps <- relayStreamStep{event: &httpclient.StreamEvent{Data: []byte("[DONE]")}}
	stream.steps <- relayStreamStep{err: errRelayReadSentinel}
	if err := <-writeDone; !errors.Is(err, errRelayReadSentinel) {
		t.Fatalf("write stream error = %v, want sentinel", err)
	}
	if !attempt.metrics.FirstTokenTime.IsZero() {
		t.Fatal("[DONE] must not set first token time")
	}
	if !attempt.streamTerminated || !attempt.responseFinalized() {
		t.Fatal("[DONE] must finalize response without counting as content")
	}
	if attempt.canWriteCommittedSSEError(context.Background(), errRelayReadSentinel) {
		t.Fatal("read failure after [DONE] must not permit a supplemental error")
	}
	waitClosed(t, ticker.stopped, "ticker stop")
	waitClosed(t, stream.closed, "stream close")
}

// Messages 的 inbound 只在源流无错误结束后生成 message_stop；这里控制最终下游流，
// 验证客户端已收到原生终止帧后发生读错误时不会补发 error，避免等待 EOF 的循环。
func TestRelayAttemptWriteStreamDoesNotWriteErrorAfterTerminal(t *testing.T) {
	for _, format := range []llm.APIFormat{llm.APIFormatAnthropicMessage, llm.APIFormatOpenAIResponse} {
		terminal := &httpclient.StreamEvent{Type: "message_stop", Data: []byte(`{"type":"message_stop"}`)}
		partial := &httpclient.StreamEvent{Type: "content_block_delta", Data: []byte(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"partial-fixture"}}`)}
		if format == llm.APIFormatOpenAIResponse {
			terminal = &httpclient.StreamEvent{Type: "response.completed", Data: []byte(`{"type":"response.completed","sequence_number":1,"response":{"id":"resp-fixture","status":"completed","output":[]}}`)}
			partial = &httpclient.StreamEvent{Type: "response.output_text.delta", Data: []byte(`{"type":"response.output_text.delta","sequence_number":0,"delta":"partial-fixture"}`)}
		}
		for _, terminalType := range []string{terminal.Type, ""} {
			t.Run(string(format)+"/event_type="+terminalType, func(t *testing.T) {
				writer := newRecordingRelayWriter(0)
				attempt := newTestRelayAttempt(t, writer)
				attempt.inboundType = format
				if err := attempt.prepareSSEStreamResponse(); err != nil {
					t.Fatal(err)
				}
				ticker := newManualRelaySSEHeartbeatTicker()
				stream := newControllableRelayStream()
				writeDone := make(chan error, 1)
				go func() { writeDone <- attempt.writeStreamWithHeartbeatTicker(context.Background(), stream, ticker) }()

				stream.steps <- relayStreamStep{event: partial}
				waitRelayGatewayFlush(t, writer, relayGatewayPartial)
				stream.steps <- relayStreamStep{event: &httpclient.StreamEvent{Type: terminalType, Data: terminal.Data}}
				snapshot := waitRelayGatewayFlush(t, writer, string(terminal.Data))
				// 只有客户端真正 Flush 终止帧后才允许后续读失败。
				stream.steps <- relayStreamStep{err: errRelayReadSentinel}
				var readErr error
				select {
				case readErr = <-writeDone:
				case <-time.After(time.Second):
					t.Fatal("terminal stream did not return the later read error")
				}
				if !errors.Is(readErr, errRelayReadSentinel) {
					t.Fatalf("write stream error = %v, want sentinel", readErr)
				}
				if !attempt.streamTerminated || !attempt.streamEventWritten || !attempt.responseFinalized() {
					t.Fatal("flushed terminal stream must remain finalized after read failure")
				}
				if attempt.canWriteCommittedSSEError(context.Background(), readErr) {
					t.Fatal("read failure after a flushed native terminal must not permit a supplemental error")
				}
				if writer.recorder.Code != http.StatusOK || writer.body() != snapshot {
					t.Fatalf("terminal response changed after read failure: status=%d body=%q", writer.recorder.Code, writer.body())
				}
				frames := relayGatewaySSEFrames(t, writer.body())
				if len(frames) != 2 || frames[1].payload["type"] != terminal.Type || len(relayGatewayFailureFrames(frames)) != 0 {
					t.Errorf("terminal stream must contain partial then exactly one completion and no error: %#v", frames)
				}
				waitClosed(t, ticker.stopped, "ticker stop")
				waitClosed(t, stream.closed, "stream close")
			})
		}
	}
}

func TestRelayAttemptWriteStreamKeepsRetryOpenAfterHeartbeatError(t *testing.T) {
	writer := newRecordingRelayWriter(0)
	attempt := newTestRelayAttempt(t, writer)
	if err := attempt.prepareSSEStreamResponse(); err != nil {
		t.Fatal(err)
	}
	ticker := newManualRelaySSEHeartbeatTicker()
	stream := newControllableRelayStream()
	writeDone := make(chan error, 1)
	go func() { writeDone <- attempt.writeStreamWithHeartbeatTicker(context.Background(), stream, ticker) }()

	stream.steps <- relayStreamStep{err: errRelayReadSentinel}
	err := <-writeDone
	if !errors.Is(err, errRelayReadSentinel) {
		t.Fatalf("error = %v, want sentinel", err)
	}
	if attempt.streamEventWritten || attempt.clientStreamWriteFailed || attempt.responseFinalized() {
		t.Fatal("heartbeat-only read failure must remain retryable")
	}
	waitClosed(t, ticker.stopped, "ticker stop")
	waitClosed(t, stream.closed, "stream close")
}

var errRelayReadSentinel = errors.New("relay read sentinel")

func TestRelayAttemptProcessStreamFinalizesOnHeartbeatWriteFailure(t *testing.T) {
	writer := newRecordingRelayWriter(2)
	attempt := newTestRelayAttempt(t, writer)
	ticker := newManualRelaySSEHeartbeatTicker()
	started := make(chan struct{})
	canceled := make(chan struct{})
	resultCh := make(chan error, 1)
	go func() {
		_, _, _, err := attempt.processStreamWithHeartbeat(context.Background(), ticker, func(ctx context.Context) (*pipeline.Result, error) {
			close(started)
			<-ctx.Done()
			close(canceled)
			return nil, ctx.Err()
		})
		resultCh <- err
	}()
	waitClosed(t, started, "pipeline start")
	if got := writer.waitFlush(t); got != ": ping\n\n" {
		t.Fatalf("initial body = %q", got)
	}
	ticker.ticks <- time.Now()
	err := <-resultCh
	if !errors.Is(err, errRelayWriterSentinel) {
		t.Fatalf("error = %v, want writer sentinel", err)
	}
	waitClosed(t, canceled, "pipeline cancel")
	if !attempt.clientStreamWriteFailed || !attempt.responseFinalized() {
		t.Fatal("heartbeat write failure must finalize response")
	}
	if !ticker.isStopped() {
		t.Fatal("ticker should be stopped")
	}
}

func TestRelayAttemptWriteStreamClosesBlockedStreamOnClientDisconnect(t *testing.T) {
	writer := newRecordingRelayWriter(0)
	attempt := newTestRelayAttempt(t, writer)
	if err := attempt.prepareSSEStreamResponse(); err != nil {
		t.Fatal(err)
	}
	initialBody := writer.body()
	ticker := newManualRelaySSEHeartbeatTicker()
	stream := newControllableRelayStream()
	ctx, cancel := context.WithCancel(context.Background())
	writeDone := make(chan error, 1)
	go func() { writeDone <- attempt.writeStreamWithHeartbeatTicker(ctx, stream, ticker) }()
	waitClosed(t, stream.entered, "blocked stream read")
	cancel()
	if err := <-writeDone; err != nil {
		t.Fatalf("write stream = %v, want nil", err)
	}
	waitClosed(t, stream.closed, "stream close")
	waitClosed(t, ticker.stopped, "ticker stop")
	if got := writer.body(); got != initialBody {
		t.Fatalf("body after disconnect = %q, want %q", got, initialBody)
	}
}

var _ io.Writer = (*recordingRelayWriter)(nil)

const relayGatewayPartial = "partial-fixture"

func relayGatewayChatChunk(content string, finish bool, extra string) string {
	finishReason := "null"
	if finish {
		finishReason = `"stop"`
	}
	return fmt.Sprintf(`{"id":"chatcmpl-fixture","object":"chat.completion.chunk","created":1,"model":"fixture-stream","choices":[{"index":0,"delta":{"role":"assistant","content":%q},"finish_reason":%s}]%s}`, content, finishReason, extra)
}

func writeRelayGatewaySSE(w http.ResponseWriter, event, data string) {
	if event != "" {
		_, _ = fmt.Fprintf(w, "event: %s\n", event)
	}
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
	w.(http.Flusher).Flush()
}

// 源错误必须等下游实际 Flush partial 后释放，避免误测 pipeline 首事件预读失败。
func (fixture *relayGatewayHarness) startStreamRequest(t *testing.T, format llm.APIFormat, writer *recordingRelayWriter) (context.CancelFunc, <-chan struct{}) {
	t.Helper()
	path, body := fixture.nonStreamRequest(t, format)
	request := decodeRelayGatewayJSON(t, body)
	request["stream"] = true
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.POST(path, func(c *gin.Context) {
		c.Set("api_key_id", fixture.apiKey.ID)
		c.Next()
	}, Handler(format))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(encoded))).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	done := make(chan struct{})
	go func() {
		defer close(done)
		router.ServeHTTP(writer, req)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("Handler did not stop after cancellation")
		}
	})
	return cancel, done
}

func (fixture *relayGatewayHarness) finalLog(t *testing.T) op.RelayLogOverview {
	t.Helper()
	for _, relayLog := range op.RelayLogStoreList(nil, nil, 1, 100) {
		if relayLog.RequestModelName == fixture.requestModel {
			if relayLog.CompletedAt == nil || relayLog.State == op.RelayLogStateRunning || relayLog.State == op.RelayLogStateCommitted {
				t.Fatalf("Handler returned without a terminal log: %#v", relayLog)
			}
			return relayLog
		}
	}
	t.Fatal("Handler did not save a request log")
	return op.RelayLogOverview{}
}

func waitRelayGatewayFlush(t *testing.T, writer *recordingRelayWriter, part string) string {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case snapshot := <-writer.flushed:
			if strings.Contains(snapshot, part) {
				return snapshot
			}
		case <-deadline.C:
			t.Fatalf("downstream never flushed %q; body=%q", part, writer.body())
		}
	}
}

func waitRelayGatewayDone(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Handler did not finish")
	}
}

type relayGatewaySSEFrame struct {
	event   string
	payload map[string]any
	data    string
	done    bool
}

func relayGatewaySSEFrames(t *testing.T, body string) []relayGatewaySSEFrame {
	t.Helper()
	var frames []relayGatewaySSEFrame
	for _, block := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n\n") {
		frame := relayGatewaySSEFrame{}
		var data []string
		for _, line := range strings.Split(block, "\n") {
			if strings.HasPrefix(line, "event:") {
				frame.event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			}
			if strings.HasPrefix(line, "data:") {
				data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
			}
		}
		if len(data) == 0 {
			continue
		}
		payload := strings.Join(data, "\n")
		frame.data = payload
		if payload == "[DONE]" {
			frame.done = true
		} else {
			frame.payload = decodeRelayGatewayJSON(t, payload)
		}
		frames = append(frames, frame)
	}
	return frames
}

func relayGatewayFailureFrames(frames []relayGatewaySSEFrame) []relayGatewaySSEFrame {
	var failures []relayGatewaySSEFrame
	for _, frame := range frames {
		kind, _ := frame.payload["type"].(string)
		_, hasError := frame.payload["error"].(map[string]any)
		if frame.event == "error" || frame.event == "response.failed" || kind == "error" || kind == "response.failed" || hasError {
			failures = append(failures, frame)
		}
	}
	return failures
}

func assertRelayGatewayStreamFailure(t *testing.T, writer *recordingRelayWriter, format llm.APIFormat, message, code, param, errorType string) relayGatewaySSEFrame {
	t.Helper()
	if writer.recorder.Code != http.StatusOK {
		t.Errorf("committed stream status = %d, want 200", writer.recorder.Code)
	}
	frames := relayGatewaySSEFrames(t, writer.body())
	failures := relayGatewayFailureFrames(frames)
	if len(failures) != 1 {
		t.Fatalf("native failure count = %d, want 1; body=%q", len(failures), writer.body())
	}
	for _, frame := range frames {
		kind, _ := frame.payload["type"].(string)
		if frame.done || frame.event == "message_stop" || kind == "message_stop" || kind == "response.completed" {
			t.Errorf("error stream contains normal completion: %#v", frame)
		}
		if choices, ok := frame.payload["choices"].([]any); ok {
			for _, raw := range choices {
				choice, _ := raw.(map[string]any)
				if choice["finish_reason"] != nil {
					t.Errorf("error stream contains finish_reason: %#v", choice)
				}
			}
		}
	}
	failure := failures[0]
	detail, _ := failure.payload["error"].(map[string]any)
	if format == llm.APIFormatOpenAIResponse {
		if response, ok := failure.payload["response"].(map[string]any); ok {
			detail, _ = response["error"].(map[string]any)
			if response["status"] != "failed" || response["id"] == "" || response["object"] != "response" || response["output"] == nil {
				t.Errorf("response.failed lost response metadata: %#v", response)
			}
			if _, ok := failure.payload["sequence_number"]; !ok {
				t.Errorf("response.failed lost sequence number: %#v", failure.payload)
			}
		} else {
			detail = failure.payload
		}
	} else if format == llm.APIFormatAnthropicMessage && failure.payload["type"] != "error" {
		t.Errorf("Messages failure is not native: %#v", failure.payload)
	}
	gotMessage, _ := detail["message"].(string)
	if gotMessage == "" {
		t.Errorf("native failure has an empty message: %#v", failure.payload)
	}
	if !strings.Contains(gotMessage, message) {
		t.Errorf("failure message=%q, want source %q", gotMessage, message)
	}
	if code != "" {
		if format == llm.APIFormatAnthropicMessage {
			if !strings.Contains(gotMessage, "[code="+code+"]") {
				t.Errorf("Messages message lost source code: %q", gotMessage)
			}
		} else if detail["code"] != code {
			t.Errorf("failure code=%v, want %q", detail["code"], code)
		}
	}
	if param != "" {
		if format == llm.APIFormatAnthropicMessage || (format == llm.APIFormatOpenAIResponse && failure.payload["response"] != nil) {
			if !strings.Contains(gotMessage, "[param="+param+"]") {
				t.Errorf("failure message lost param: %q", gotMessage)
			}
		} else if detail["param"] != param {
			t.Errorf("failure param=%v, want %q", detail["param"], param)
		}
	}
	if errorType != "" && (format != llm.APIFormatOpenAIResponse || failure.payload["response"] != nil) && detail["type"] != errorType {
		t.Errorf("failure type=%v, want %q", detail["type"], errorType)
	}
	return failure
}

func assertRelayGatewayFailedStats(t *testing.T, channelID int, before model.StatsTotal) {
	t.Helper()
	after := op.StatsTotalGet()
	if after.RequestFailed-before.RequestFailed != 1 || after.RequestSuccess-before.RequestSuccess != 0 {
		t.Errorf("request statistics delta: failed=%d success=%d", after.RequestFailed-before.RequestFailed, after.RequestSuccess-before.RequestSuccess)
	}
	channel := op.StatsChannelGet(channelID)
	if channel.RequestFailed != 1 || channel.RequestSuccess != 0 {
		t.Errorf("channel statistics = %#v, want one failed request", channel)
	}
}

func TestRelayGatewayStreamErrorDetails(t *testing.T) {
	formats := []llm.APIFormat{llm.APIFormatOpenAIChatCompletion, llm.APIFormatAnthropicMessage, llm.APIFormatOpenAIResponse}
	for _, format := range formats {
		for _, source := range []string{"event_error", "event_top_level_error", "data_error", "error_string", "payload_type_error", "empty_event_error", "responses_failed", "parse_failure", "read_failure"} {
			t.Run(string(format)+"/"+source, func(t *testing.T) {
				release := make(chan struct{})
				var releaseOnce sync.Once
				unblock := func() { releaseOnce.Do(func() { close(release) }) }
				t.Cleanup(unblock)
				var hits atomic.Int32
				outbound := llm.APIFormatOpenAIChatCompletion
				if source == "responses_failed" {
					outbound = llm.APIFormatOpenAIResponse
				}
				fixture := newRelayGatewayHarness(t, outbound, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					hits.Add(1)
					w.Header().Set("Content-Type", "text/event-stream")
					if source == "responses_failed" {
						writeRelayGatewaySSE(w, "response.created", `{"type":"response.created","sequence_number":0,"response":{"id":"resp-fixture","object":"response","created_at":1,"status":"in_progress","model":"fixture-stream","output":[]}}`)
						writeRelayGatewaySSE(w, "response.output_text.delta", `{"type":"response.output_text.delta","sequence_number":1,"item_id":"msg-fixture","output_index":0,"content_index":0,"delta":"partial-fixture"}`)
					} else {
						writeRelayGatewaySSE(w, "", relayGatewayChatChunk(relayGatewayPartial, false, ""))
					}
					select {
					case <-release:
					case <-r.Context().Done():
						return
					}
					switch source {
					case "event_error":
						writeRelayGatewaySSE(w, "error", `{"error":{"message":"fixture stream failed","type":"rate_limit_error","code":"fixture_stream","param":"model"},"request_id":"fixture-stream-id"}`)
					case "event_top_level_error":
						writeRelayGatewaySSE(w, "error", `{"message":"fixture stream failed","type":"rate_limit_error","code":"fixture_stream","param":"model","request_id":"fixture-stream-id"}`)
					case "data_error":
						writeRelayGatewaySSE(w, "", `{"data":{"error":{"message":"fixture stream failed","type":"rate_limit_error","code":"fixture_stream","param":"model"},"request_id":"fixture-stream-id"}}`)
					case "error_string":
						writeRelayGatewaySSE(w, "", `{"error":"fixture stream failed","request_id":"fixture-stream-id"}`)
					case "payload_type_error":
						writeRelayGatewaySSE(w, "", `{"type":"error","message":"fixture stream failed","code":"fixture_stream","param":"model"}`)
					case "empty_event_error":
						writeRelayGatewaySSE(w, "error", "")
					case "responses_failed":
						writeRelayGatewaySSE(w, "response.failed", `{"type":"response.failed","sequence_number":2,"response":{"id":"resp-fixture","object":"response","created_at":1,"status":"failed","model":"fixture-stream","output":[],"error":{"message":"fixture stream failed","type":"rate_limit_error","code":"fixture_stream","param":"model"}}}`)
					case "parse_failure":
						writeRelayGatewaySSE(w, "", `{"choices":`)
					case "read_failure":
						// 关闭未结束 chunked 编码的真实 socket，确定地产生 unexpected EOF。
						conn, _, err := w.(http.Hijacker).Hijack()
						if err == nil {
							_ = conn.Close()
						}
					}
				}))
				fixture.addFallback(t, fixture.upstream.URL)
				before := op.StatsTotalGet()
				writer := newRecordingRelayWriter(0)
				_, done := fixture.startStreamRequest(t, format, writer)
				waitRelayGatewayFlush(t, writer, relayGatewayPartial)
				unblock()
				waitRelayGatewayDone(t, done)
				message, code, param, errorType := "fixture stream failed", "fixture_stream", "model", "rate_limit_error"
				parts := []string{message, code}
				switch source {
				case "payload_type_error":
					errorType = "api_error"
				case "error_string":
					code, param, errorType = "", "", "api_error"
					parts = []string{message}
				case "empty_event_error":
					message, code, param, errorType = "upstream stream error", "", "", "stream_error"
					parts = []string{message}
				case "parse_failure":
					message, code, param, errorType = "", "", "", ""
					parts = []string{"unmarshal"}
				case "read_failure":
					message, code, param, errorType = "", "", "", ""
					parts = []string{"EOF"}
				}
				failure := assertRelayGatewayStreamFailure(t, writer, format, message, code, param, errorType)
				if source == "event_error" || source == "event_top_level_error" || source == "data_error" || source == "error_string" {
					if format == llm.APIFormatAnthropicMessage && failure.payload["request_id"] != "fixture-stream-id" {
						t.Errorf("Messages native failure lost source request_id: %#v", failure.payload)
					}
					if format == llm.APIFormatOpenAIChatCompletion {
						detail, _ := failure.payload["error"].(map[string]any)
						if detail["request_id"] != "fixture-stream-id" {
							t.Errorf("Chat native failure lost source request_id: %#v", detail)
						}
					}
				}
				if source == "responses_failed" && format == llm.APIFormatOpenAIResponse {
					response, _ := failure.payload["response"].(map[string]any)
					if response["id"] != "resp-fixture" || response["created_at"] != json.Number("1") {
						t.Errorf("client-generated response.failed lost source metadata: %#v", response)
					}
				}
				relayLog := fixture.finalLog(t)
				assertRelayGatewayFailureLog(t, relayLog, http.StatusOK, parts)
				assertRelayGatewayResponseBody(t, relayLog, failure.data)
				assertRelayGatewayFailedStats(t, fixture.channel.ID, before)
				if hits.Load() != 1 {
					t.Errorf("committed failure retried upstream %d times", hits.Load())
				}
			})
		}
	}
	for _, format := range formats {
		for _, sourceError := range []string{"null", `""`, `{}`} {
			t.Run(string(format)+"/success/error="+sourceError, func(t *testing.T) {
				fixture := newRelayGatewayHarness(t, llm.APIFormatOpenAIChatCompletion, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "text/event-stream")
					writeRelayGatewaySSE(w, "", relayGatewayChatChunk("normal text mentioning error", false, `,"error":`+sourceError))
					writeRelayGatewaySSE(w, "", relayGatewayChatChunk("", true, ""))
					writeRelayGatewaySSE(w, "", "[DONE]")
				}))
				writer := newRecordingRelayWriter(0)
				_, done := fixture.startStreamRequest(t, format, writer)
				waitRelayGatewayDone(t, done)
				if !strings.Contains(writer.body(), "normal text mentioning error") {
					t.Errorf("non-error value discarded the actual content chunk: %q", writer.body())
				}
				if failures := relayGatewayFailureFrames(relayGatewaySSEFrames(t, writer.body())); len(failures) != 0 {
					t.Errorf("non-error value became native failure: %#v", failures)
				}
				relayLog := fixture.finalLog(t)
				if relayLog.State != op.RelayLogStateSuccess || relayLog.UpstreamStatusCode != 200 || len(relayLog.Attempts) != 1 || relayLog.Attempts[0].Status != model.AttemptSuccess {
					t.Errorf("normal stream final log = %#v", relayLog)
				}
			})
		}
	}
}

func TestRelayGatewayStreamStopsWithoutRetry(t *testing.T) {
	for _, format := range []llm.APIFormat{llm.APIFormatOpenAIChatCompletion, llm.APIFormatAnthropicMessage, llm.APIFormatOpenAIResponse} {
		modes := []string{"client_disconnect", "client_write_failure"}
		if format != llm.APIFormatAnthropicMessage {
			modes = append(modes, "after_done_read_failure")
		}
		for _, mode := range modes {
			t.Run(string(format)+"/"+mode, func(t *testing.T) {
				release := make(chan struct{})
				var once sync.Once
				unblock := func() { once.Do(func() { close(release) }) }
				t.Cleanup(unblock)
				var hits atomic.Int32
				outbound := llm.APIFormatOpenAIChatCompletion
				if mode == "after_done_read_failure" && format == llm.APIFormatOpenAIResponse {
					outbound = llm.APIFormatOpenAIResponse
				}
				fixture := newRelayGatewayHarness(t, outbound, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					hits.Add(1)
					w.Header().Set("Content-Type", "text/event-stream")
					if outbound == llm.APIFormatOpenAIResponse {
						writeRelayGatewaySSE(w, "response.created", `{"type":"response.created","sequence_number":0,"response":{"id":"resp-fixture","object":"response","created_at":1,"status":"in_progress","model":"fixture-stream","output":[]}}`)
						writeRelayGatewaySSE(w, "response.output_text.delta", `{"type":"response.output_text.delta","sequence_number":1,"item_id":"msg-fixture","output_index":0,"content_index":0,"delta":"partial-fixture"}`)
						// 原生 completed 的 usage 使 Responses inbound 在 EOF 前完成；
						// 没有 usage 的 Chat finish_reason/[DONE] 会继续等待无错误 EOF。
						writeRelayGatewaySSE(w, "response.completed", `{"type":"response.completed","sequence_number":2,"response":{"id":"resp-fixture","object":"response","created_at":1,"status":"completed","model":"fixture-stream","output":[{"id":"msg-fixture","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"partial-fixture","annotations":[]}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`)
					} else {
						writeRelayGatewaySSE(w, "", relayGatewayChatChunk(relayGatewayPartial, false, ""))
						if mode == "after_done_read_failure" {
							writeRelayGatewaySSE(w, "", relayGatewayChatChunk("", true, ""))
							writeRelayGatewaySSE(w, "", "[DONE]")
						}
					}
					select {
					case <-release:
					case <-r.Context().Done():
						return
					}
					if mode == "client_write_failure" {
						writeRelayGatewaySSE(w, "", relayGatewayChatChunk("second-fixture", false, ""))
						return
					}
					conn, _, err := w.(http.Hijacker).Hijack()
					if err == nil {
						_ = conn.Close()
					}
				}))
				fixture.addFallback(t, fixture.upstream.URL)
				writer := newRecordingRelayWriter(0)
				cancel, done := fixture.startStreamRequest(t, format, writer)
				waitRelayGatewayFlush(t, writer, relayGatewayPartial)
				snapshot := writer.body()
				switch mode {
				case "after_done_read_failure":
					terminal := "[DONE]"
					if format == llm.APIFormatOpenAIResponse {
						terminal = "response.completed"
					}
					snapshot = waitRelayGatewayFlush(t, writer, terminal)
				case "client_disconnect":
					cancel()
				case "client_write_failure":
					writer.mu.Lock()
					writer.failAt = writer.writes + 1
					writer.mu.Unlock()
				}
				if mode == "client_disconnect" {
					waitRelayGatewayDone(t, done)
					unblock()
				} else {
					unblock()
					waitRelayGatewayDone(t, done)
				}
				if failures := relayGatewayFailureFrames(relayGatewaySSEFrames(t, writer.body())); len(failures) != 0 {
					t.Errorf("terminated/disconnected stream got an extra failure: %#v", failures)
				}
				if mode == "client_disconnect" && writer.body() != snapshot {
					t.Errorf("canceled client received extra bytes: %q", writer.body())
				}
				relayLog := fixture.finalLog(t)
				if len(relayLog.Attempts) != 1 || hits.Load() != 1 || relayLog.UpstreamStatusCode != 200 || relayLog.Attempts[0].UpstreamStatusCode != 200 {
					t.Errorf("terminated stream retried/lost source code: hits=%d log=%#v", hits.Load(), relayLog)
				}
				if mode == "after_done_read_failure" {
					if writer.body() != snapshot || writer.recorder.Code != http.StatusOK {
						t.Errorf("post-terminal read failure changed committed response: status=%d body=%q", writer.recorder.Code, writer.body())
					}
					if relayLog.State != op.RelayLogStateFailed || !strings.Contains(relayLog.Error, "EOF") {
						t.Errorf("post-terminal read failure lost its actual diagnostic: %#v", relayLog)
					}
				}
				if mode == "client_disconnect" && (relayLog.State != op.RelayLogStateCanceled || !strings.Contains(relayLog.Error, context.Canceled.Error())) {
					t.Errorf("disconnect lost cancellation: %#v", relayLog)
				}
				if mode == "client_write_failure" && (relayLog.State != op.RelayLogStateFailed || !strings.Contains(relayLog.Error, errRelayWriterSentinel.Error())) {
					t.Errorf("write failure lost diagnostic: %#v", relayLog)
				}
			})
		}
	}
}

func TestRelayAttemptStreamCancellationRemainsIdentifiable(t *testing.T) {
	writer := newRecordingRelayWriter(0)
	attempt := newTestRelayAttempt(t, writer)
	if err := attempt.prepareSSEStreamResponse(); err != nil {
		t.Fatal(err)
	}
	ticker := newManualRelaySSEHeartbeatTicker()
	stream := newControllableRelayStream()
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), relayAttemptContextKey{}, true))
	done := make(chan error, 1)
	go func() { done <- attempt.writeStreamWithHeartbeatTicker(ctx, stream, ticker) }()
	waitClosed(t, stream.entered, "attempt-owned stream read")
	cancel()
	if err := <-done; !errors.Is(attempt.normalizeUpstreamError(err), context.Canceled) {
		t.Fatalf("attempt cancellation is no longer errors.Is-identifiable: %v", err)
	}
	waitClosed(t, stream.closed, "canceled stream close")
	waitClosed(t, ticker.stopped, "canceled ticker stop")
}

func TestRelayAttemptStreamFailurePreservesMetadata(t *testing.T) {
	for _, sourceCode := range []string{"fixture_stream", ""} {
		for _, kind := range []string{"error", "response.failed"} {
			t.Run(kind+"/source_code="+sourceCode, func(t *testing.T) {
				writer := newRecordingRelayWriter(0)
				attempt := newTestRelayAttempt(t, writer)
				attempt.inboundType = llm.APIFormatOpenAIResponse
				attempt.upstreamStatusCode.Store(200)
				attempt.captureUpstreamError(&relayUpstreamError{
					statusCode: 200, stream: true,
					detail: llm.ErrorDetail{Message: "source detail", Type: "rate_limit_error", Code: sourceCode, Param: "model"},
				})
				payload := `{"type":"error","sequence_number":7,"message":"adapter detail","code":"stream_error","extra":{"retained":true}}`
				if kind == "response.failed" {
					payload = `{"type":"response.failed","sequence_number":7,"extra":{"retained":true},"response":{"id":"resp-retained","object":"response","created_at":123,"status":"failed","output":[{"id":"msg-retained","type":"message","content":[{"type":"output_text","text":"partial-fixture"}]}],"error":{"type":"server_error","message":"adapter detail","code":"stream_error","extra":"retained"},"metadata":{"retained":true}}}`
				}
				event := &httpclient.StreamEvent{Data: []byte(payload)} // payload.type 明确失败，Type 刻意为空。
				repaired, err := attempt.streamFailure(event)
				if err == nil || repaired.Type != kind || !strings.Contains(err.Error(), "source detail") {
					t.Fatalf("payload-only failure not recognized: event=%#v err=%v", repaired, err)
				}
				original := decodeRelayGatewayJSON(t, payload)
				result := decodeRelayGatewayJSON(t, string(repaired.Data))
				if result["sequence_number"] != json.Number("7") || fmt.Sprint(result["extra"]) != fmt.Sprint(original["extra"]) {
					t.Errorf("repair lost event metadata: %#v", result)
				}
				detail := result
				if kind == "response.failed" {
					response, _ := result["response"].(map[string]any)
					originalResponse, _ := original["response"].(map[string]any)
					for _, key := range []string{"id", "object", "created_at", "status", "output", "metadata"} {
						actualJSON, _ := json.Marshal(response[key])
						expectedJSON, _ := json.Marshal(originalResponse[key])
						if string(actualJSON) != string(expectedJSON) {
							t.Errorf("repair changed response.%s: got=%s want=%s", key, actualJSON, expectedJSON)
						}
					}
					detail, _ = response["error"].(map[string]any)
					if detail["extra"] != "retained" || detail["type"] != "rate_limit_error" || detail["message"] != "source detail [param=model]" {
						t.Errorf("repair lost source error fields/extensions: %#v", detail)
					}
				} else if detail["message"] != "source detail" || detail["param"] != "model" {
					t.Errorf("explicit Responses error lost source detail: %#v", detail)
				}
				wantCode := sourceCode
				if wantCode == "" {
					wantCode = "stream_error"
				}
				if detail["code"] != wantCode {
					t.Errorf("repair code=%v, want %q", detail["code"], wantCode)
				}
				if string(event.Data) != payload || event.Type != "" {
					t.Error("repair mutated original event")
				}
				stream := newControllableRelayStream()
				stream.steps <- relayStreamStep{event: event}
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				writeErr := attempt.writeStreamWithHeartbeatTicker(ctx, stream, newManualRelaySSEHeartbeatTicker())
				if writeErr == nil || !strings.Contains(writeErr.Error(), "source detail") {
					t.Fatalf("terminal event writer did not report source failure: %v", writeErr)
				}
				failures := relayGatewayFailureFrames(relayGatewaySSEFrames(t, writer.body()))
				if len(failures) != 1 || failures[0].data != string(repaired.Data) {
					t.Fatalf("terminal event writer changed the complete repaired payload: %q", writer.body())
				}
				finalLog := attempt.metrics.buildRelayLog(writeErr, 0, nil, 0, "", 0, "")
				if finalLog.ResponseContent != failures[0].data || finalLog.ResponseContentTruncated {
					t.Error("final response content simplified the complete failure event")
				}
				if !attempt.streamErrorWritten || !attempt.streamTerminated {
					t.Error("failure event did not terminate the stream")
				}
			})
		}
	}
}

func TestRelayAttemptManualHeartbeatHTTPFailureRemainsRetryable(t *testing.T) {
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"message":"manual fixture rejected"}}`)
	}))
	t.Cleanup(upstream.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	writer := newRecordingRelayWriter(0)
	attempt := newTestRelayAttempt(t, writer)
	ticker := newManualRelaySSEHeartbeatTicker()
	done := make(chan error, 1)
	go func() {
		_, _, _, err := attempt.processStreamWithHeartbeat(ctx, ticker, func(ctx context.Context) (*pipeline.Result, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, upstream.URL, nil)
			if err != nil {
				return nil, err
			}
			response, err := upstream.Client().Do(req)
			if err != nil {
				return nil, err
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil {
				return nil, err
			}
			return nil, &httpclient.Error{StatusCode: response.StatusCode, Body: body}
		})
		done <- err
	}()
	writer.waitFlush(t)
	ticker.ticks <- time.Now()
	if got := writer.waitFlush(t); strings.Count(got, ": ping\n\n") != 2 || strings.Contains(got, "data:") {
		t.Fatalf("manual pre-content heartbeat = %q", got)
	}
	unblock()
	var source *httpclient.Error
	if err := <-done; !errors.As(err, &source) || source.StatusCode != 429 || !strings.Contains(string(source.Body), "manual fixture rejected") {
		t.Fatalf("manual heartbeat lost actual HTTP failure: %v", err)
	}
	if attempt.responseFinalized() || attempt.streamEventWritten || attempt.clientStreamWriteFailed {
		t.Fatal("manual heartbeat closed retry before any model event")
	}
	waitClosed(t, ticker.stopped, "manual failed attempt ticker stop")
}
