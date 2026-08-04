package relay

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

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
	}

	run.writeFinalError(context.Background(), errors.New("dial upstream: connection refused"))

	if recorder.Code != http.StatusFailedDependency {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusFailedDependency)
	}
}

func TestWriteFinalErrorSkipsCommittedSSEStream(t *testing.T) {
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

	run.writeFinalError(context.Background(), errors.New("stream failed"))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Body.String(); got != body {
		t.Fatalf("body = %q, want unchanged %q", got, body)
	}
}
