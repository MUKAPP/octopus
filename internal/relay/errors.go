package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/httpclient"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// relayStatusTransport 只观察当前尝试的真实 HTTP 响应，不读取正文或改变原 transport 的行为。
type relayStatusTransport struct {
	base       http.RoundTripper
	statusCode *atomic.Int32
}

func (t *relayStatusTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := t.base.RoundTrip(request)
	if response != nil {
		t.statusCode.Store(int32(response.StatusCode))
	} else if err != nil {
		t.statusCode.Store(0)
	}
	return response, err
}

// relayUpstreamError 保存不可变的上游错误事实；正文和 headers 引用已读取完成的输入。
// cause 保留 pipeline 错误链，供取消、超时等 errors.Is/As 判断使用。
type relayUpstreamError struct {
	cause      error
	protocol   llm.APIFormat
	statusCode int
	body       []byte
	headers    http.Header
	detail     llm.ErrorDetail
	stream     bool
}

func (e *relayUpstreamError) Error() string {
	if body := strings.TrimSpace(string(e.body)); body != "" {
		if e.statusCode != 0 {
			return fmt.Sprintf("upstream HTTP %d: %s", e.statusCode, body)
		}
		return "upstream error: " + body
	}

	if e.statusCode == 0 && e.detail == (llm.ErrorDetail{}) && e.cause != nil {
		return e.cause.Error()
	}

	message := e.detail.Message
	if strings.TrimSpace(message) == "" {
		message = http.StatusText(e.statusCode)
	}
	var diagnostic strings.Builder
	if e.statusCode != 0 {
		fmt.Fprintf(&diagnostic, "upstream HTTP %d", e.statusCode)
		if message != "" {
			diagnostic.WriteString(": ")
			diagnostic.WriteString(message)
		}
	} else if message != "" {
		diagnostic.WriteString("upstream error: ")
		diagnostic.WriteString(message)
	} else {
		diagnostic.WriteString("upstream HTTP 0")
	}
	if e.detail.Type != "" {
		diagnostic.WriteString(", type: ")
		diagnostic.WriteString(e.detail.Type)
	}
	if e.detail.Code != "" {
		diagnostic.WriteString(", code: ")
		diagnostic.WriteString(e.detail.Code)
	}
	if e.detail.Param != "" {
		diagnostic.WriteString(", param: ")
		diagnostic.WriteString(e.detail.Param)
	}
	if e.detail.RequestID != "" {
		diagnostic.WriteString(", request_id: ")
		diagnostic.WriteString(e.detail.RequestID)
	}
	return diagnostic.String()
}

func (e *relayUpstreamError) Unwrap() error {
	return e.cause
}

// relayErrorEnvelope 只在错误候选正文上解码，不复制正常响应对象。
type relayErrorEnvelope struct {
	Error     json.RawMessage `json:"error"`
	Errors    json.RawMessage `json:"errors"`
	Type      string          `json:"type"`
	Event     string          `json:"event"`
	Message   string          `json:"message"`
	Code      json.RawMessage `json:"code"`
	Param     string          `json:"param"`
	RequestID string          `json:"request_id"`
	Data      struct {
		Error     json.RawMessage `json:"error"`
		RequestID string          `json:"request_id"`
	} `json:"data"`
	Response struct {
		Error json.RawMessage `json:"error"`
	} `json:"response"`
}

func relayErrorCode(raw json.RawMessage) string {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	// RawMessage 保留 JSON 数字的原始十进制表示，避免 float64 丢失精度。
	raw = bytes.TrimSpace(raw)
	if len(raw) > 0 && (raw[0] == '-' || raw[0] >= '0' && raw[0] <= '9') {
		return string(raw)
	}
	return ""
}

func relayErrorObject(raw json.RawMessage) (llm.ErrorDetail, bool) {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return llm.ErrorDetail{Message: text}, strings.TrimSpace(text) != ""
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil || len(object) == 0 {
		return llm.ErrorDetail{}, false
	}
	var detail llm.ErrorDetail
	_ = json.Unmarshal(object["message"], &detail.Message)
	_ = json.Unmarshal(object["type"], &detail.Type)
	_ = json.Unmarshal(object["param"], &detail.Param)
	_ = json.Unmarshal(object["request_id"], &detail.RequestID)
	detail.Code = relayErrorCode(object["code"])
	if detail.Type == "" {
		_ = json.Unmarshal(object["status"], &detail.Type)
	}
	return detail, true
}

func decodeRelayErrorDetail(body []byte, headers http.Header, eventType string) (llm.ErrorDetail, bool) {
	var envelope relayErrorEnvelope
	var detail llm.ErrorDetail
	explicit := false
	if json.Unmarshal(body, &envelope) == nil {
		for _, raw := range [...]json.RawMessage{envelope.Error, envelope.Errors, envelope.Data.Error, envelope.Response.Error} {
			if candidate, ok := relayErrorObject(raw); ok {
				detail, explicit = candidate, true
				break
			}
		}
		if !explicit && (eventType == "error" || eventType == "response.failed" || envelope.Type == "error" || envelope.Event == "error" || envelope.Type == "response.failed" || envelope.Event == "response.failed") {
			explicit = true
			detail.Message = envelope.Message
			detail.Code = relayErrorCode(envelope.Code)
			detail.Param = envelope.Param
			if envelope.Type != "error" && envelope.Type != "response.failed" {
				detail.Type = envelope.Type
			}
		}
		if detail.RequestID == "" {
			detail.RequestID = envelope.RequestID
		}
		if detail.RequestID == "" {
			detail.RequestID = envelope.Data.RequestID
		}
	}
	if detail.RequestID == "" {
		detail.RequestID = headers.Get("X-Request-Id")
	}
	if detail.RequestID == "" {
		detail.RequestID = headers.Get("Request-Id")
	}
	if strings.TrimSpace(detail.Message) == "" && len(bytes.TrimSpace(body)) != 0 {
		detail.Message = string(body)
	}
	if detail.Type == "" {
		detail.Type = "api_error"
	}
	return detail, explicit
}

func parseRelayErrorDetail(body []byte, headers http.Header) llm.ErrorDetail {
	detail, _ := decodeRelayErrorDetail(body, headers, "")
	return detail
}

// relayBodyWithoutEmptyError 仅移除已确认无错误的顶层字段，保留其余正文原始字节。
// 固定版 OpenAI 流 converter 按字段存在判错；非流式也无法解码空字符串，空对象会生成 ResponseError。
func relayBodyWithoutEmptyError(body []byte) ([]byte, bool) {
	field := gjson.GetBytes(body, "error")
	if !field.Exists() || !json.Valid(body) {
		return body, false
	}
	empty := field.Type == gjson.Null || field.Type == gjson.String && strings.TrimSpace(field.Str) == ""
	if field.IsObject() {
		raw := strings.TrimSpace(field.Raw)
		empty = strings.TrimSpace(raw[1:len(raw)-1]) == ""
	}
	if !empty {
		return body, false
	}
	converterBody, err := sjson.DeleteBytes(body, "error")
	if err != nil {
		return body, false
	}
	return converterBody, true
}

func (ra *relayAttempt) captureUpstreamError(source *relayUpstreamError) {
	ra.upstreamErrorMu.Lock()
	ra.upstreamError = source
	ra.upstreamErrorMu.Unlock()
}

func (ra *relayAttempt) normalizeUpstreamError(err error) error {
	if err == nil {
		return nil
	}
	ra.upstreamErrorMu.Lock()
	source := ra.upstreamError
	ra.upstreamErrorMu.Unlock()
	if source == nil {
		var existing *relayUpstreamError
		if errors.As(err, &existing) {
			source = existing
		} else if responseErr := relayResponseError(err); responseErr != nil {
			source = &relayUpstreamError{statusCode: responseErr.StatusCode, detail: responseErr.Detail}
		}
	}
	if source == nil {
		return err
	}
	// 复制小型事实结构，不复制正文；cause 指向本次真实返回链，保留取消/超时语义。
	normalized := *source
	normalized.cause = err
	return &normalized
}

func relayResponseError(err error) *llm.ResponseError {
	var pointer *llm.ResponseError
	if errors.As(err, &pointer) {
		return pointer
	}
	var value llm.ResponseError
	if errors.As(err, &value) {
		return &value
	}
	return nil
}

func relayErrorMessage(source *relayUpstreamError) string {
	if strings.TrimSpace(source.detail.Message) != "" {
		return source.detail.Message
	}
	if len(bytes.TrimSpace(source.body)) != 0 {
		return string(source.body)
	}
	if source.statusCode != 0 {
		if message := http.StatusText(source.statusCode); message != "" {
			return message
		}
		return fmt.Sprintf("upstream HTTP %d", source.statusCode)
	}
	if source.cause != nil {
		return source.cause.Error()
	}
	return "upstream error"
}

func relayOpenAIErrorFamily(protocol llm.APIFormat) bool {
	return strings.HasPrefix(string(protocol), "openai/") || protocol == "doubao"
}

func relayNativeErrorBody(source *relayUpstreamError, target llm.APIFormat) bool {
	if source.stream || len(source.body) == 0 {
		return false
	}
	sameOpenAI := relayOpenAIErrorFamily(source.protocol) && relayOpenAIErrorFamily(target)
	sameAnthropic := source.protocol == llm.APIFormatAnthropicMessage && target == llm.APIFormatAnthropicMessage
	if !sameOpenAI && !sameAnthropic {
		return false
	}
	var envelope struct {
		Type  string `json:"type"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	return json.Unmarshal(source.body, &envelope) == nil && strings.TrimSpace(envelope.Error.Message) != "" && (sameOpenAI || envelope.Type == "error")
}

func relayErrorHeaders(source http.Header) http.Header {
	headers := make(http.Header)
	for key, values := range source {
		lower := strings.ToLower(key)
		if lower == "retry-after" || lower == "x-request-id" || lower == "request-id" || strings.HasPrefix(lower, "x-ratelimit-") || strings.HasPrefix(lower, "ratelimit-") {
			headers[http.CanonicalHeaderKey(key)] = values
		}
	}
	return headers
}

func relayErrorMessageSuffix(message string, detail llm.ErrorDetail, includeCode bool) string {
	if includeCode && detail.Code != "" {
		message += " [code=" + detail.Code + "]"
	}
	if detail.Param != "" {
		message += " [param=" + detail.Param + "]"
	}
	return message
}

func relayErrorSource(err error) *relayUpstreamError {
	var source *relayUpstreamError
	if !errors.As(err, &source) {
		if responseErr := relayResponseError(err); responseErr != nil {
			source = &relayUpstreamError{cause: err, statusCode: responseErr.StatusCode, detail: responseErr.Detail}
		} else {
			var raw *httpclient.Error
			if errors.As(err, &raw) {
				source = &relayUpstreamError{cause: err, statusCode: raw.StatusCode, body: raw.Body, headers: raw.Headers, detail: parseRelayErrorDetail(raw.Body, raw.Headers)}
			}
		}
	}
	return source
}

func relayClientErrorDetail(source *relayUpstreamError) llm.ErrorDetail {
	detail := source.detail
	detail.Message = relayErrorMessage(source)
	if detail.Type == "" {
		detail.Type = "api_error"
	}
	return detail
}

// clientError 仅由源 HTTP/结构化错误事实决定协议和状态；pipeline 包裹不改变本地 424 契约。
func (r *relayRun) clientError(ctx context.Context, err error) *httpclient.Error {
	source := relayErrorSource(err)
	if source == nil {
		body, _ := json.Marshal(struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}{http.StatusFailedDependency, err.Error()})
		return &httpclient.Error{StatusCode: http.StatusFailedDependency, Headers: http.Header{"Content-Type": {"application/json"}}, Body: body}
	}
	statusCode := source.statusCode
	if statusCode < http.StatusBadRequest || statusCode >= 600 {
		statusCode = http.StatusInternalServerError
	}
	detail := relayClientErrorDetail(source)
	headers := relayErrorHeaders(source.headers)
	if detail.RequestID != "" && headers.Get("X-Request-Id") == "" {
		headers.Set("X-Request-Id", detail.RequestID)
	}
	clientErr := &httpclient.Error{StatusCode: statusCode, Status: http.StatusText(statusCode), Headers: headers}
	if relayNativeErrorBody(source, r.inboundType) {
		clientErr.Body = source.body
		contentType := source.headers.Get("Content-Type")
		if contentType == "" {
			contentType = "application/json"
		}
		headers.Set("Content-Type", contentType)
		return clientErr
	}
	headers.Set("Content-Type", "application/json")
	switch r.inboundType {
	case llm.APIFormatAnthropicMessage:
		var body struct {
			Type  string `json:"type"`
			Error struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"error"`
			RequestID string `json:"request_id,omitempty"`
		}
		body.Type = "error"
		body.Error.Type = detail.Type
		body.Error.Message = relayErrorMessageSuffix(detail.Message, detail, true)
		body.RequestID = detail.RequestID
		clientErr.Body, _ = json.Marshal(body)
	case llm.APIFormatOpenAIResponse, llm.APIFormatOpenAIResponseCompact:
		var body struct {
			Error struct {
				Message string `json:"message"`
				Type    string `json:"type"`
				Code    string `json:"code,omitempty"`
			} `json:"error"`
		}
		body.Error.Message = relayErrorMessageSuffix(detail.Message, detail, false)
		body.Error.Type = detail.Type
		body.Error.Code = detail.Code
		clientErr.Body, _ = json.Marshal(body)
	default:
		// 没有额外原始信息的既有 ResponseError 仍走现有入站适配器。
		if source.protocol == "" && len(source.body) == 0 && source.headers == nil && r.inAdapter != nil {
			if adapted := r.inAdapter.TransformError(ctx, &llm.ResponseError{StatusCode: statusCode, Detail: detail}); adapted != nil {
				clientErr.Body = adapted.Body
			}
		}
		if len(clientErr.Body) == 0 {
			clientErr.Body, _ = json.Marshal(struct {
				Error llm.ErrorDetail `json:"error"`
			}{detail})
		}
	}
	return clientErr
}

// relaySSEErrorBody 使用客户端协议的原生流错误外壳，不复用本地非流式 424 响应。
func relaySSEErrorBody(protocol llm.APIFormat, detail llm.ErrorDetail) []byte {
	var body []byte
	switch protocol {
	case llm.APIFormatAnthropicMessage:
		body, _ = json.Marshal(struct {
			Type  string `json:"type"`
			Error struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"error"`
			RequestID string `json:"request_id,omitempty"`
		}{
			Type: "error",
			Error: struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			}{detail.Type, relayErrorMessageSuffix(detail.Message, detail, true)},
			RequestID: detail.RequestID,
		})
	case llm.APIFormatOpenAIResponse, llm.APIFormatOpenAIResponseCompact:
		code := detail.Code
		if code == "" {
			code = "stream_error"
		}
		body, _ = json.Marshal(struct {
			Type    string `json:"type"`
			Code    string `json:"code"`
			Message string `json:"message"`
			Param   string `json:"param,omitempty"`
		}{"error", code, detail.Message, detail.Param})
	default:
		body, _ = json.Marshal(struct {
			Error llm.ErrorDetail `json:"error"`
		}{detail})
	}
	return body
}

func relayStreamFailureType(event *httpclient.StreamEvent) string {
	if event.Type == "error" || event.Type == "response.failed" {
		return event.Type
	}
	if event.Type == "" && bytes.Contains(event.Data, []byte(`"type"`)) {
		var payload struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(event.Data, &payload) == nil && (payload.Type == "error" || payload.Type == "response.failed") {
			return payload.Type
		}
	}
	return ""
}

func relaySetErrorField(object map[string]json.RawMessage, key, value string) {
	object[key], _ = json.Marshal(value)
}

// streamFailure 仅处理明确失败事件；RawMessage 保留 response、output、sequence 等非错误元数据。
func (ra *relayAttempt) streamFailure(event *httpclient.StreamEvent) (*httpclient.StreamEvent, error) {
	failureType := relayStreamFailureType(event)
	if failureType == "" {
		return event, nil
	}
	ra.upstreamErrorMu.Lock()
	source := ra.upstreamError
	ra.upstreamErrorMu.Unlock()
	detail, _ := decodeRelayErrorDetail(event.Data, nil, failureType)
	if source != nil {
		detail = relayClientErrorDetail(source)
	}
	if strings.TrimSpace(detail.Message) == "" {
		detail.Message = "upstream stream error"
		detail.Type = "stream_error"
	}
	failureErr := ra.normalizeUpstreamError(errors.New(detail.Message))
	repaired := *event
	repaired.Type = failureType
	if source == nil && len(bytes.TrimSpace(event.Data)) != 0 {
		return &repaired, failureErr
	}
	var payload map[string]json.RawMessage
	if json.Unmarshal(event.Data, &payload) != nil || payload == nil {
		repaired.Data = relaySSEErrorBody(ra.inboundType, detail)
		return &repaired, failureErr
	}
	if failureType == "response.failed" {
		var response map[string]json.RawMessage
		_ = json.Unmarshal(payload["response"], &response)
		if response == nil {
			response = make(map[string]json.RawMessage)
		}
		var responseError map[string]json.RawMessage
		_ = json.Unmarshal(response["error"], &responseError)
		if responseError == nil {
			responseError = make(map[string]json.RawMessage)
		}
		relaySetErrorField(responseError, "message", relayErrorMessageSuffix(detail.Message, detail, false))
		relaySetErrorField(responseError, "type", detail.Type)
		if detail.Code != "" {
			relaySetErrorField(responseError, "code", detail.Code)
		}
		response["error"], _ = json.Marshal(responseError)
		payload["response"], _ = json.Marshal(response)
	} else if ra.inboundType == llm.APIFormatOpenAIResponse || ra.inboundType == llm.APIFormatOpenAIResponseCompact {
		relaySetErrorField(payload, "message", detail.Message)
		if detail.Code != "" {
			relaySetErrorField(payload, "code", detail.Code)
		}
		if detail.Param != "" {
			relaySetErrorField(payload, "param", detail.Param)
		}
	} else {
		var eventError map[string]json.RawMessage
		_ = json.Unmarshal(payload["error"], &eventError)
		if eventError == nil {
			eventError = make(map[string]json.RawMessage)
		}
		message := detail.Message
		if ra.inboundType == llm.APIFormatAnthropicMessage {
			message = relayErrorMessageSuffix(message, detail, true)
			if detail.RequestID != "" {
				relaySetErrorField(payload, "request_id", detail.RequestID)
			}
		} else {
			if detail.Code != "" {
				relaySetErrorField(eventError, "code", detail.Code)
			}
			if detail.Param != "" {
				relaySetErrorField(eventError, "param", detail.Param)
			}
			if detail.RequestID != "" {
				relaySetErrorField(eventError, "request_id", detail.RequestID)
			}
		}
		relaySetErrorField(eventError, "message", message)
		relaySetErrorField(eventError, "type", detail.Type)
		payload["error"], _ = json.Marshal(eventError)
	}
	repaired.Data, _ = json.Marshal(payload)
	return &repaired, failureErr
}
