package helper

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bestruirui/octopus/internal/model"
)

func TestFetchRateMultiplierReturnsSub2APIEffectiveRate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/sub2api/billing" {
			t.Fatalf("请求路径不符合预期：%s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Fatalf("Authorization 不符合预期：%s", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"sub2api.key_billing","schema_version":1,"effective_rate_multiplier":0.75}`))
	}))
	defer server.Close()

	channel := model.Channel{
		BaseUrls: []model.BaseUrl{{URL: server.URL}},
		Keys:     []model.ChannelKey{{Enabled: true, ChannelKey: "sk-test"}},
	}
	rate, err := FetchRateMultiplier(context.Background(), channel)
	if err != nil {
		t.Fatalf("获取倍率失败：%v", err)
	}
	if rate != 0.75 {
		t.Fatalf("倍率不符合预期：got %v, want 0.75", rate)
	}
}

func TestFetchRateMultiplierTreatsNotFoundAsUnsupported(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	channel := model.Channel{BaseUrls: []model.BaseUrl{{URL: server.URL}}}
	_, err := FetchRateMultiplier(context.Background(), channel)
	if !errors.Is(err, ErrRateMultiplierUnsupported) {
		t.Fatalf("404 应视为不支持倍率查询，实际为：%v", err)
	}
}

func TestFetchOpenAIModelsRejectsUpstreamErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"temporary failure"}}`))
	}))
	defer server.Close()

	channel := model.Channel{BaseUrls: []model.BaseUrl{{URL: server.URL}}}
	models, err := fetchOpenAIModels(server.Client(), context.Background(), channel)

	if err == nil {
		t.Fatal("上游返回错误状态码时应返回错误")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("错误信息应包含上游状态码，实际为：%v", err)
	}
	if models != nil {
		t.Fatalf("上游异常时不应返回模型，实际为：%v", models)
	}
}
