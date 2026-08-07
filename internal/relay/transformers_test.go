package relay

import (
	"context"
	"testing"

	"github.com/looplj/axonhub/llm"
)

func TestNewOutboundKeepsExistingVersionPath(t *testing.T) {
	// 腾讯 codebuddy 渠道：base URL 自带 /v2，必须请求 /v2/chat/completions 而不是 /v2/v1/chat/completions。
	out, err := newOutbound(llm.APIFormatOpenAIChatCompletion, chatRequest("hy3"), "https://copilot.tencent.com/v2", "ck_test")
	if err != nil {
		t.Fatalf("newOutbound: %v", err)
	}

	req, err := out.TransformRequest(context.Background(), chatRequest("hy3"))
	if err != nil {
		t.Fatalf("TransformRequest: %v", err)
	}
	if got, want := req.URL, "https://copilot.tencent.com/v2/chat/completions"; got != want {
		t.Errorf("URL = %q, want %q", got, want)
	}
}

func TestNewOutboundV1BaseURLUnchanged(t *testing.T) {
	out, err := newOutbound(llm.APIFormatOpenAIChatCompletion, chatRequest("gpt-4o"), "https://api.openai.com/v1", "sk_test")
	if err != nil {
		t.Fatalf("newOutbound: %v", err)
	}

	req, err := out.TransformRequest(context.Background(), chatRequest("gpt-4o"))
	if err != nil {
		t.Fatalf("TransformRequest: %v", err)
	}
	if got, want := req.URL, "https://api.openai.com/v1/chat/completions"; got != want {
		t.Errorf("URL = %q, want %q", got, want)
	}
}

func chatRequest(model string) *llm.Request {
	text := "hi"
	return &llm.Request{
		Model: model,
		Messages: []llm.Message{{
			Role:    "user",
			Content: llm.MessageContent{Content: &text},
		}},
	}
}
