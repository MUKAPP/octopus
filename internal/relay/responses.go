package relay

import (
	"context"
	"fmt"

	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/httpclient"
	"github.com/looplj/axonhub/llm/streams"
	"github.com/looplj/axonhub/llm/transformer"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// responsesInbound 修正 AxonHub 转换后的完成事件，保留其他事件及未知字段。
type responsesInbound struct {
	transformer.Inbound
}

func (t *responsesInbound) TransformStream(ctx context.Context, source streams.Stream[*llm.Response]) (streams.Stream[*httpclient.StreamEvent], error) {
	stream, err := t.Inbound.TransformStream(ctx, source)
	if err != nil {
		return nil, err
	}
	return streams.MapErr(stream, normalizeResponsesStreamEvent), nil
}

func normalizeResponsesStreamEvent(event *httpclient.StreamEvent) (*httpclient.StreamEvent, error) {
	data := event.Data
	var err error
	switch event.Type {
	case "response.function_call_arguments.done":
		// 空参数在库中会被 omitempty 省略；只在完成时补为有效 JSON 对象。
		arguments := gjson.GetBytes(data, "arguments")
		if !arguments.Exists() || arguments.Type == gjson.String && arguments.Str == "" {
			data, err = sjson.SetBytes(data, "arguments", "{}")
		}
	case "response.output_item.done":
		data, err = normalizeResponsesCompletedItem(data, "item")
	case "response.completed":
		// 库内部的聚合快照早于本层修正，终态 output 需要同步处理。
		for index := range gjson.GetBytes(data, "response.output").Array() {
			data, err = normalizeResponsesCompletedItem(data, fmt.Sprintf("response.output.%d", index))
			if err != nil {
				break
			}
		}
	default:
		return event, nil
	}
	if err != nil {
		return nil, fmt.Errorf("normalize Responses completion event: %w", err)
	}
	result := *event
	result.Data = data
	return &result, nil
}

func normalizeResponsesCompletedItem(data []byte, path string) ([]byte, error) {
	item := gjson.GetBytes(data, path)
	switch item.Get("type").Str {
	case "function_call":
		arguments := item.Get("arguments")
		if !arguments.Exists() || arguments.Type == gjson.String && arguments.Str == "" {
			return sjson.SetBytes(data, path+".arguments", "{}")
		}
	case "reasoning":
		status := item.Get("status").Str
		if status == "" || status == "in_progress" {
			return sjson.SetBytes(data, path+".status", "completed")
		}
	}
	return data, nil
}
