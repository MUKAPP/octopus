package helper

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bestruirui/octopus/internal/model"
)

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
