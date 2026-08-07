package helper

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bestruirui/octopus/internal/model"
)

func TestNormalizeLegacyBaseURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"root URL keeps default version append", "https://api.openai.com", "https://api.openai.com"},
		{"v1 keeps default behavior", "https://api.openai.com/v1", "https://api.openai.com/v1"},
		{"v1 with trailing slash keeps default", "https://api.openai.com/v1/", "https://api.openai.com/v1/"},
		{"non-version path keeps default", "https://custom.example.com/api", "https://custom.example.com/api"},
		{"v2 version path skips append", "https://copilot.tencent.com/v2", "https://copilot.tencent.com/v2#"},
		{"v2 with trailing slash skips append", "https://copilot.tencent.com/v2/", "https://copilot.tencent.com/v2#"},
		{"v1beta version path skips append", "https://generativelanguage.googleapis.com/v1beta", "https://generativelanguage.googleapis.com/v1beta#"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeLegacyBaseURL(tt.in); got != tt.want {
				t.Errorf("NormalizeLegacyBaseURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestFetchOpenAIModelsKeepsExistingVersionPath 验证 codebuddy 这类自带版本路径的渠道
// 拉取模型时请求落在 /v2/models 而不是 /v2/v1/models。
func TestFetchOpenAIModelsKeepsExistingVersionPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/models" {
			t.Fatalf("请求路径不符合预期：%s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"hy3"},{"id":"glm-5.2"}]}`))
	}))
	defer server.Close()

	channel := model.Channel{
		BaseUrls: []model.BaseUrl{{URL: server.URL + "/v2"}},
		Keys:     []model.ChannelKey{{Enabled: true, ChannelKey: "ck-test"}},
	}
	models, err := fetchOpenAIModels(server.Client(), context.Background(), channel)
	if err != nil {
		t.Fatalf("拉取模型失败：%v", err)
	}
	if len(models) != 2 || models[0] != "hy3" || models[1] != "glm-5.2" {
		t.Fatalf("模型列表不符合预期：%v", models)
	}
}

// TestFetchOpenAIModelsV1BaseURLKeepsDefault 验证 /v1 结尾的渠道行为不变。
func TestFetchOpenAIModelsV1BaseURLKeepsDefault(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("请求路径不符合预期：%s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"gpt-4o"}]}`))
	}))
	defer server.Close()

	channel := model.Channel{
		BaseUrls: []model.BaseUrl{{URL: server.URL + "/v1"}},
		Keys:     []model.ChannelKey{{Enabled: true, ChannelKey: "sk-test"}},
	}
	models, err := fetchOpenAIModels(server.Client(), context.Background(), channel)
	if err != nil {
		t.Fatalf("拉取模型失败：%v", err)
	}
	if len(models) != 1 || models[0] != "gpt-4o" {
		t.Fatalf("模型列表不符合预期：%v", models)
	}
}
