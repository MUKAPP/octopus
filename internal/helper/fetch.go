package helper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/dlclark/regexp2"
	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/transformer"
)

var ErrRateMultiplierUnsupported = errors.New("上游不支持倍率查询")

type rateMultiplierResponse struct {
	Object                  string  `json:"object"`
	SchemaVersion           int     `json:"schema_version"`
	EffectiveRateMultiplier float64 `json:"effective_rate_multiplier"`
}

func FetchRateMultiplier(ctx context.Context, request model.Channel) (float64, error) {
	client, err := ChannelHttpClient(&request)
	if err != nil {
		return 0, err
	}
	baseURL := transformer.NormalizeBaseURL(NormalizeLegacyBaseURL(request.GetBaseUrl()), "v1")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/sub2api/billing", nil)
	if err != nil {
		return 0, err
	}
	key := request.GetChannelKey().ChannelKey
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("X-Api-Key", key)
	applyCustomHeaders(req, request)

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return 0, ErrRateMultiplierUnsupported
	}
	if err := validateModelResponse(resp); err != nil {
		return 0, err
	}

	var result rateMultiplierResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, err
	}
	if result.Object != "sub2api.key_billing" || result.SchemaVersion != 1 {
		return 0, ErrRateMultiplierUnsupported
	}
	if result.EffectiveRateMultiplier <= 0 || math.IsNaN(result.EffectiveRateMultiplier) || math.IsInf(result.EffectiveRateMultiplier, 0) {
		return 0, fmt.Errorf("上游返回了无效倍率 %v", result.EffectiveRateMultiplier)
	}
	return result.EffectiveRateMultiplier, nil
}

func FetchModels(ctx context.Context, request model.Channel) ([]string, error) {
	client, err := ChannelHttpClient(&request)
	if err != nil {
		return nil, err
	}
	fetchModel := make([]string, 0)
	switch request.Type {
	case llm.APIFormatAnthropicMessage:
		fetchModel, err = fetchAnthropicModels(client, ctx, request)
	case llm.APIFormatGeminiContents:
		fetchModel, err = fetchGeminiModels(client, ctx, request)
	default:
		fetchModel, err = fetchOpenAIModels(client, ctx, request)
	}
	if err != nil {
		return nil, err
	}
	if request.MatchRegex != nil && *request.MatchRegex != "" {
		matchModel := make([]string, 0)
		re, err := regexp2.Compile(*request.MatchRegex, regexp2.ECMAScript)
		if err != nil {
			return nil, err
		}
		for _, model := range fetchModel {
			matched, err := re.MatchString(model)
			if err != nil {
				return nil, err
			}
			if matched {
				matchModel = append(matchModel, model)
			}
		}
		return matchModel, nil
	}
	return fetchModel, nil
}

// refer: https://platform.openai.com/docs/api-reference/models/list
func fetchOpenAIModels(client *http.Client, ctx context.Context, request model.Channel) ([]string, error) {
	baseURL := transformer.NormalizeBaseURL(NormalizeLegacyBaseURL(request.GetBaseUrl()), "v1")
	if request.Type == model.ChannelTypeDoubao {
		baseURL = transformer.NormalizeBaseURL(NormalizeLegacyBaseURL(request.GetBaseUrl()), "v3")
	}
	req, _ := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		baseURL+"/models",
		nil,
	)
	req.Header.Set("Authorization", "Bearer "+request.GetChannelKey().ChannelKey)
	applyCustomHeaders(req, request)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := validateModelResponse(resp); err != nil {
		return nil, err
	}

	var result model.OpenAIModelList

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	models := make([]string, 0, len(result.Data))
	for _, m := range result.Data {
		models = append(models, m.ID)
	}
	return models, nil
}

// refer: https://ai.google.dev/api/models
func fetchGeminiModels(client *http.Client, ctx context.Context, request model.Channel) ([]string, error) {
	var allModels []string
	pageToken := ""
	baseURL := transformer.NormalizeBaseURL(NormalizeLegacyBaseURL(request.GetBaseUrl()), "v1beta")
	// Gemini transformer 会保留用户显式填写的 /v1；这里同样处理，避免把 /v1 拼成 /v1/v1beta。
	if strings.HasSuffix(strings.TrimRight(request.GetBaseUrl(), "/"), "/v1") {
		baseURL = transformer.NormalizeBaseURL(NormalizeLegacyBaseURL(request.GetBaseUrl()), "")
	}

	for {
		req, _ := http.NewRequestWithContext(
			ctx,
			http.MethodGet,
			baseURL+"/models",
			nil,
		)
		req.Header.Set("X-Goog-Api-Key", request.GetChannelKey().ChannelKey)
		applyCustomHeaders(req, request)
		if pageToken != "" {
			q := req.URL.Query()
			q.Add("pageToken", pageToken)
			req.URL.RawQuery = q.Encode()
		}

		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if err := validateModelResponse(resp); err != nil {
			return nil, err
		}

		var result model.GeminiModelList

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, err
		}

		for _, m := range result.Models {
			name := strings.TrimPrefix(m.Name, "models/")
			allModels = append(allModels, name)
		}

		if result.NextPageToken == "" {
			break
		}
		pageToken = result.NextPageToken
	}
	if len(allModels) == 0 {
		return fetchOpenAIModels(client, ctx, request)
	}
	return allModels, nil
}

// refer: https://platform.claude.com/docs
func fetchAnthropicModels(client *http.Client, ctx context.Context, request model.Channel) ([]string, error) {

	var allModels []string
	var afterID string
	baseURL := transformer.NormalizeBaseURL(NormalizeLegacyBaseURL(request.GetBaseUrl()), "v1")
	for {

		req, _ := http.NewRequestWithContext(
			ctx,
			http.MethodGet,
			baseURL+"/models",
			nil,
		)
		req.Header.Set("X-Api-Key", request.GetChannelKey().ChannelKey)
		req.Header.Set("Anthropic-Version", "2023-06-01")
		applyCustomHeaders(req, request)
		// 设置多页参数
		q := req.URL.Query()

		if afterID != "" {
			q.Set("after_id", afterID)
		}
		req.URL.RawQuery = q.Encode()

		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if err := validateModelResponse(resp); err != nil {
			return nil, err
		}

		var result model.AnthropicModelList

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, err
		}

		for _, m := range result.Data {
			allModels = append(allModels, m.ID)
		}

		if !result.HasMore {
			break
		}

		afterID = result.LastID
	}
	if len(allModels) == 0 {
		return fetchOpenAIModels(client, ctx, request)
	}
	return allModels, nil
}

func validateModelResponse(resp *http.Response) error {
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("获取模型失败，上游返回状态码 %d", resp.StatusCode)
	}
	return nil
}

func applyCustomHeaders(req *http.Request, channel model.Channel) {
	for _, header := range channel.CustomHeader {
		if header.HeaderKey != "" {
			req.Header.Set(header.HeaderKey, header.HeaderValue)
		}
	}
}
