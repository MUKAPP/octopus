package relay

import (
	"encoding/json"
	"net/url"
	"strings"

	"github.com/bestruirui/octopus/internal/op"
	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/httpclient"
)

type modelNameEnvelope struct {
	Model string `json:"model"`
}

type geminiModelNameEnvelope struct {
	ModelVersion string `json:"modelVersion"`
}

type anthropicModelNameEnvelope struct {
	Message modelNameEnvelope `json:"message"`
}

type responsesModelNameEnvelope struct {
	Response modelNameEnvelope `json:"response"`
}

// extractUpstreamModelName 读取参数覆盖和协议修补之后真正发送的模型，不回退到候选名。
func extractUpstreamModelName(request *httpclient.Request, format llm.APIFormat) string {
	if format == llm.APIFormatGeminiContents {
		requestURL, err := url.Parse(request.URL)
		if err != nil {
			return ""
		}
		modelStart := strings.LastIndex(requestURL.Path, "/models/")
		if modelStart < 0 {
			return ""
		}
		modelStart += len("/models/")
		actionStart := strings.LastIndex(requestURL.Path, ":")
		if actionStart <= modelStart || actionStart == len(requestURL.Path)-1 {
			return ""
		}
		return requestURL.Path[modelStart:actionStart]
	}

	var data []byte
	if isJSONRequest(request) {
		data = request.Body
	} else if strings.Contains(strings.ToLower(request.ContentType+" "+request.Headers.Get("Content-Type")), "multipart/") {
		// transformer 同时构造 multipart 表单和 JSONBody；无需再次解析或复制二进制正文。
		data = request.JSONBody
	} else {
		return ""
	}
	var envelope modelNameEnvelope
	if json.Unmarshal(data, &envelope) != nil {
		return ""
	}
	return envelope.Model
}

// extractResponseModelName 仅采集原始协议声明，忽略工具参数和转换器补造的模型。
func extractResponseModelName(data []byte, format llm.APIFormat, streaming bool) string {
	if format == llm.APIFormatGeminiContents {
		var envelope geminiModelNameEnvelope
		if json.Unmarshal(data, &envelope) != nil {
			return ""
		}
		return envelope.ModelVersion
	}
	if streaming {
		switch format {
		case llm.APIFormatAnthropicMessage:
			var envelope anthropicModelNameEnvelope
			if json.Unmarshal(data, &envelope) != nil {
				return ""
			}
			return envelope.Message.Model
		case llm.APIFormatOpenAIResponse:
			var envelope responsesModelNameEnvelope
			if json.Unmarshal(data, &envelope) != nil {
				return ""
			}
			return envelope.Response.Model
		}
	}
	var envelope modelNameEnvelope
	if json.Unmarshal(data, &envelope) != nil {
		return ""
	}
	return envelope.Model
}

func (ra *relayAttempt) recordUpstreamModelName(name string) {
	ra.modelNamesMu.Lock()
	if ra.upstreamModelName == name {
		ra.modelNamesMu.Unlock()
		return
	}
	ra.upstreamModelName = name
	upstreamModel, responseModel := ra.upstreamModelName, ra.responseModelName
	ra.modelNamesMu.Unlock()
	op.RelayLogStoreModelNames(ra.metrics.ID, ra.attemptIndex, upstreamModel, responseModel)
}

func (ra *relayAttempt) recordResponseModelName(data []byte, streaming bool) {
	ra.modelNamesMu.Lock()
	if ra.responseModelName != "" {
		ra.modelNamesMu.Unlock()
		return
	}
	name := extractResponseModelName(data, ra.channel.Type, streaming)
	if name == "" {
		ra.modelNamesMu.Unlock()
		return
	}
	ra.responseModelName = name
	upstreamModel, responseModel := ra.upstreamModelName, ra.responseModelName
	ra.modelNamesMu.Unlock()
	// store 在自己的锁下核对当前尝试和活态；晚到事件只更新本 attempt，不会覆盖下一次 metrics。
	op.RelayLogStoreModelNames(ra.metrics.ID, ra.attemptIndex, upstreamModel, responseModel)
}
