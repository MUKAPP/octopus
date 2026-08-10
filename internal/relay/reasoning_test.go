package relay

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/httpclient"
)

func TestEnsureDeepSeekChatReasoningContentInjectsMissingField(t *testing.T) {
	request := &httpclient.Request{
		Body:        []byte(`{"model":"deepseek-v4-flash","messages":[{"role":"assistant","tool_calls":[{"id":"call-1"}]}]}`),
		Headers:     http.Header{"Content-Type": []string{"application/json"}},
		ContentType: "application/json",
	}

	changed, err := ensureDeepSeekChatReasoningContent(
		request,
		llm.APIFormatOpenAIChatCompletion,
		true,
	)
	if err != nil {
		t.Fatalf("ensureDeepSeekChatReasoningContent: %v", err)
	}
	if !changed {
		t.Fatal("changed = false, want true")
	}

	var body struct {
		Messages []struct {
			ReasoningContent *string `json:"reasoning_content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(request.Body, &body); err != nil {
		t.Fatalf("decode patched body: %v", err)
	}
	if len(body.Messages) != 1 || body.Messages[0].ReasoningContent == nil || *body.Messages[0].ReasoningContent != "" {
		t.Fatalf("patched reasoning_content = %#v, want pointer to empty string", body.Messages)
	}

	nativeRequest, err := httpclient.BuildHttpRequest(t.Context(), request)
	if err != nil {
		t.Fatalf("BuildHttpRequest: %v", err)
	}
	if nativeRequest.ContentLength != int64(len(request.Body)) {
		t.Fatalf("ContentLength = %d, want %d", nativeRequest.ContentLength, len(request.Body))
	}
}

func TestEnsureDeepSeekChatReasoningContentDoesNotOverwriteExisting(t *testing.T) {
	for _, reasoning := range []string{`"existing"`, `""`, `null`} {
		t.Run(reasoning, func(t *testing.T) {
			request := &httpclient.Request{
				Body: []byte(`{"model":"deepseek-v4-flash","messages":[{"role":"assistant","tool_calls":[{"id":"call-1"}],"reasoning_content":` + reasoning + `}]}`),
				Headers: http.Header{
					"Content-Type": []string{"application/json"},
				},
			}

			changed, err := ensureDeepSeekChatReasoningContent(request, llm.APIFormatOpenAIChatCompletion, true)

			if err != nil {
				t.Fatalf("ensureDeepSeekChatReasoningContent: %v", err)
			}
			if changed {
				t.Fatal("changed = true, want false")
			}
		})
	}
}

func TestEnsureDeepSeekChatReasoningContentSkipsNonToolCalls(t *testing.T) {
	cases := []struct {
		name        string
		channelType llm.APIFormat
		model       string
		thinking    bool
		messages    string
	}{
		{
			name:        "no tool calls",
			channelType: llm.APIFormatOpenAIChatCompletion,
			model:       "deepseek-v4-flash",
			thinking:    true,
			messages:    `[{"role":"assistant","content":"answer"}]`,
		},
		{
			name:        "non DeepSeek model",
			channelType: llm.APIFormatOpenAIChatCompletion,
			model:       "gpt-5.6-terra",
			thinking:    true,
			messages:    `[{"role":"assistant","tool_calls":[{"id":"call-1"}]}]`,
		},
		{
			name:        "Responses channel",
			channelType: llm.APIFormatOpenAIResponse,
			model:       "deepseek-v4-flash",
			thinking:    true,
			messages:    `[{"role":"assistant","tool_calls":[{"id":"call-1"}]}]`,
		},
		{
			name:        "thinking disabled",
			channelType: llm.APIFormatOpenAIChatCompletion,
			model:       "deepseek-v4-flash",
			thinking:    false,
			messages:    `[{"role":"assistant","tool_calls":[{"id":"call-1"}]}]`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := &httpclient.Request{
				Body:        []byte(`{"model":"` + tc.model + `","messages":` + tc.messages + `}`),
				Headers:     http.Header{"Content-Type": []string{"application/json"}},
				ContentType: "application/json",
			}
			before := string(request.Body)

			changed, err := ensureDeepSeekChatReasoningContent(request, tc.channelType, tc.thinking)
			if err != nil {
				t.Fatalf("ensureDeepSeekChatReasoningContent: %v", err)
			}
			if changed {
				t.Fatal("changed = true, want false")
			}
			if string(request.Body) != before {
				t.Fatalf("request body changed from %s to %s", before, request.Body)
			}
		})
	}
}

func TestRelayMiddlewareRepairsFinalDeepSeekChatRequest(t *testing.T) {
	attempt := &relayAttempt{
		relayRun: &relayRun{
			internalRequest: &llm.Request{},
			metrics:         &RelayMetrics{ActualModel: "deepseek-v4-flash"},
		},
		channel: &model.Channel{Type: llm.APIFormatOpenAIChatCompletion},
	}
	request := &httpclient.Request{
		Body:        []byte(`{"model":"deepseek-v4-flash","messages":[{"role":"assistant","tool_calls":[{"id":"call-1"}]}]}`),
		Headers:     http.Header{"Content-Type": []string{"application/json"}},
		ContentType: "application/json",
	}

	if _, err := (&relayPipelineMiddleware{attempt: attempt}).OnOutboundRawRequest(context.Background(), request); err != nil {
		t.Fatalf("OnOutboundRawRequest: %v", err)
	}

	var body map[string]any
	if err := json.Unmarshal(request.Body, &body); err != nil {
		t.Fatalf("decode patched body: %v", err)
	}
	messages := body["messages"].([]any)
	message := messages[0].(map[string]any)
	if reasoning, ok := message["reasoning_content"]; !ok || reasoning != "" {
		t.Fatalf("reasoning_content = %#v, want empty string", reasoning)
	}
}

func TestRelayMiddlewareUsesFinalModelAfterOverrides(t *testing.T) {
	tests := []struct {
		name        string
		model       string
		actualModel string
		override    string
		wantPatched bool
		wantModel   string
	}{
		{
			name:        "DeepSeek candidate overridden to non DeepSeek",
			model:       "deepseek-v4-flash",
			actualModel: "deepseek-v4-flash",
			override:    `{"model":"gpt-5.6-terra"}`,
			wantModel:   "gpt-5.6-terra",
		},
		{
			name:        "non DeepSeek candidate overridden to DeepSeek",
			model:       "gpt-5.6-terra",
			actualModel: "gpt-5.6-terra",
			override:    `{"model":"deepseek-v4-flash"}`,
			wantPatched: true,
			wantModel:   "deepseek-v4-flash",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attempt := &relayAttempt{
				relayRun: &relayRun{
					internalRequest: &llm.Request{},
					metrics:         &RelayMetrics{ActualModel: tt.actualModel},
				},
				channel: &model.Channel{
					Type:          llm.APIFormatOpenAIChatCompletion,
					ParamOverride: &tt.override,
				},
			}
			request := &httpclient.Request{
				Body:        []byte(`{"model":"` + tt.model + `","messages":[{"role":"assistant","tool_calls":[{"id":"call-1"}]}]}`),
				Headers:     http.Header{"Content-Type": []string{"application/json"}},
				ContentType: "application/json",
			}

			if _, err := (&relayPipelineMiddleware{attempt: attempt}).OnOutboundRawRequest(context.Background(), request); err != nil {
				t.Fatalf("OnOutboundRawRequest: %v", err)
			}

			var body struct {
				Model    string `json:"model"`
				Messages []struct {
					ReasoningContent *string `json:"reasoning_content"`
				} `json:"messages"`
			}
			if err := json.Unmarshal(request.Body, &body); err != nil {
				t.Fatalf("decode patched body: %v", err)
			}
			if body.Model != tt.wantModel {
				t.Fatalf("model = %q, want %q", body.Model, tt.wantModel)
			}
			gotPatched := len(body.Messages) == 1 && body.Messages[0].ReasoningContent != nil && *body.Messages[0].ReasoningContent == ""
			if gotPatched != tt.wantPatched {
				t.Fatalf("patched = %t, want %t", gotPatched, tt.wantPatched)
			}
		})
	}
}
