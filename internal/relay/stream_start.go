package relay

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/pipeline"
	"github.com/looplj/axonhub/llm/streams"
)

// 旧版 axonhub 的预读会在若干启动事件后直接回放。这里在统一响应层等待真实输出，
// 使本地固定依赖与 CI 的新版依赖保持同一提交边界，不复制各协议的转换逻辑。
func (m *relayPipelineMiddleware) OnOutboundLlmStream(ctx context.Context, source streams.Stream[*llm.Response]) (streams.Stream[*llm.Response], error) {
	const maxStartupEvents = 1024
	const maxStartupBytes = 8 * 1024 * 1024
	var buffered []*llm.Response
	bufferedBytes := 0
	for source.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		event := source.Current()
		if event == nil {
			continue
		}
		if event.Error != nil {
			return nil, pipeline.WrapUpstreamError(event.Error)
		}
		if relayStreamHasOutput(event) {
			// 大图片等真实输出不计入启动元数据预算，立即移交消费方。
			buffered = append(buffered, event)
			return streams.PrependStream(source, buffered...), nil
		}
		if event == llm.DoneResponse || event.Object == "[DONE]" {
			return nil, pipeline.ErrEmptyResponse
		}
		for _, choice := range event.Choices {
			if choice.FinishReason != nil && *choice.FinishReason != "" {
				return nil, pipeline.ErrEmptyResponse
			}
		}
		encoded, err := json.Marshal(event)
		if err != nil {
			return nil, fmt.Errorf("encode stream startup event: %w", err)
		}
		if len(buffered) >= maxStartupEvents || len(encoded) > maxStartupBytes-bufferedBytes {
			return nil, fmt.Errorf("stream startup buffer exceeded (%d events / %d bytes)", maxStartupEvents, maxStartupBytes)
		}
		bufferedBytes += len(encoded)
		buffered = append(buffered, event)
	}
	if err := source.Err(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("upstream stream ended before first content")
}

// 当前 relay 支持 Chat/Messages/Responses 及图片协议；文本、思考、工具、拒绝、音频和图片均属于输出。
func relayStreamHasOutput(response *llm.Response) bool {
	if response.Image != nil && len(response.Image.Data) > 0 {
		return true
	}
	for _, choice := range response.Choices {
		for _, message := range []*llm.Message{choice.Delta, choice.Message} {
			if message == nil {
				continue
			}
			if (message.Content.Content != nil && *message.Content.Content != "") ||
				len(message.Content.MultipleContent) > 0 || len(message.ToolCalls) > 0 || message.Refusal != "" ||
				(message.ReasoningContent != nil && *message.ReasoningContent != "") ||
				(message.Reasoning != nil && *message.Reasoning != "") ||
				(message.ReasoningSignature != nil && *message.ReasoningSignature != "") ||
				(message.Audio != nil && (message.Audio.Data != "" || message.Audio.Transcript != "")) {
				return true
			}
		}
	}
	return false
}
