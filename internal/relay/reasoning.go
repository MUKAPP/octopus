package relay

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/httpclient"
)

// ensureDeepSeekChatReasoningContent preserves the required assistant field on
// DeepSeek Chat Completions requests. AxonHub's Responses adapter can lose an
// explicitly empty reasoning item while converting formats; DeepSeek still
// requires the field when the assistant message contains tool calls.
//
// The patch is deliberately limited to the OpenAI Chat Completions format and
// only adds a missing field. Existing reasoning_content values, including an
// explicit null or empty string, are left unchanged. httpclient.BuildHttpRequest
// rebuilds the native request from Body with bytes.NewReader, so its
// ContentLength is recalculated from the patched body.
func ensureDeepSeekChatReasoningContent(
	request *httpclient.Request,
	channelType llm.APIFormat,
	thinkingEnabled bool,
) (bool, error) {
	if request == nil || channelType != llm.APIFormatOpenAIChatCompletion || !thinkingEnabled {
		return false, nil
	}
	if !isJSONRequest(request) || request.Headers.Get("Content-Encoding") != "" {
		return false, nil
	}

	var body map[string]json.RawMessage
	if err := json.Unmarshal(request.Body, &body); err != nil {
		return false, fmt.Errorf("decode chat request body: %w", err)
	}

	modelJSON, ok := body["model"]
	if !ok {
		return false, nil
	}
	var model string
	if err := json.Unmarshal(modelJSON, &model); err != nil || !isDeepSeekModel(model) {
		return false, nil
	}

	var messages []map[string]json.RawMessage
	messagesJSON, ok := body["messages"]
	if !ok {
		return false, nil
	}
	if err := json.Unmarshal(messagesJSON, &messages); err != nil {
		return false, fmt.Errorf("decode chat messages: %w", err)
	}

	changed := false
	for _, message := range messages {
		var role string
		if err := json.Unmarshal(message["role"], &role); err != nil || role != "assistant" {
			continue
		}

		var toolCalls []json.RawMessage
		if err := json.Unmarshal(message["tool_calls"], &toolCalls); err != nil || len(toolCalls) == 0 {
			continue
		}
		if _, exists := message["reasoning_content"]; exists {
			continue
		}

		message["reasoning_content"] = json.RawMessage(`""`)
		changed = true
	}

	if !changed {
		return false, nil
	}

	updatedMessages, err := json.Marshal(messages)
	if err != nil {
		return false, fmt.Errorf("encode chat messages: %w", err)
	}
	body["messages"] = updatedMessages

	updatedBody, err := json.Marshal(body)
	if err != nil {
		return false, fmt.Errorf("encode chat request body: %w", err)
	}
	request.Body = updatedBody
	return true, nil
}

func isDeepSeekModel(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	return model == "deepseek" || strings.HasPrefix(model, "deepseek-")
}

func isJSONRequest(request *httpclient.Request) bool {
	contentType := request.ContentType
	if request.Headers != nil {
		contentType += " " + request.Headers.Get("Content-Type")
	}
	return strings.Contains(strings.ToLower(contentType), "application/json")
}
