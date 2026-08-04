package relay

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bestruirui/octopus/internal/model"
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
		flushed:  make(chan string, 16),
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
	waitClosed(t, ticker.stopped, "ticker stop")
	waitClosed(t, stream.closed, "stream close")
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
