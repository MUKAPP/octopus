package relay

import (
	"strconv"
	"sync"

	"github.com/looplj/axonhub/llm"
	"github.com/tidwall/gjson"
)

// These pointers describe fields actually received from the upstream, including explicit
// zeroes. Normalized llm details can drop zero fields or synthesize absent details.
// The collector belongs to one attempt, so late frames never affect a subsequent attempt.
type relayUsageMetadata struct {
	Present         bool
	CacheReadToken  *int64
	CacheWriteToken *int64
	ReasoningToken  *int64
}

type relayUsageCollector struct {
	mu    sync.Mutex
	value relayUsageMetadata
}

func rawUsageInt(usage gjson.Result, path string) *int64 {
	value := usage.Get(path)
	if value.Type != gjson.Number {
		return nil
	}
	count, err := strconv.ParseInt(value.Raw, 10, 64)
	if err != nil || count < 0 {
		return nil
	}
	return &count
}

// extractRawUsageMetadata reads only documented usage locations of the active outbound
// protocol. It never searches arbitrary response content, tool results or converted bodies.
func extractRawUsageMetadata(data []byte, format llm.APIFormat) relayUsageMetadata {
	var out relayUsageMetadata
	usage := gjson.GetBytes(data, "usage")
	switch format {
	case llm.APIFormatAnthropicMessage:
		if messageUsage := gjson.GetBytes(data, "message.usage"); messageUsage.IsObject() {
			usage = messageUsage
		}
	case llm.APIFormatOpenAIResponse, llm.APIFormatOpenAIResponseCompact:
		if responseUsage := gjson.GetBytes(data, "response.usage"); responseUsage.IsObject() {
			usage = responseUsage
		}
	case llm.APIFormatGeminiContents:
		usage = gjson.GetBytes(data, "usageMetadata")
	case llm.APIFormatOpenAIChatCompletion, llm.APIFormatOpenAIEmbedding,
		llm.APIFormatOpenAIImageGeneration, llm.APIFormatOpenAIImageEdit, llm.APIFormatOpenAIImageVariation:
	default:
		return out
	}
	if !usage.IsObject() {
		return out
	}
	for _, field := range []string{"prompt_tokens", "completion_tokens", "total_tokens", "input_tokens", "output_tokens", "promptTokenCount", "candidatesTokenCount", "totalTokenCount"} {
		if rawUsageInt(usage, field) != nil {
			out.Present = true
			break
		}
	}
	switch format {
	case llm.APIFormatAnthropicMessage:
		out.CacheReadToken = rawUsageInt(usage, "cache_read_input_tokens")
		out.CacheWriteToken = rawUsageInt(usage, "cache_creation_input_tokens")
		// Moonshot's Anthropic endpoint reports cache reads through cached_tokens.
		if cached := rawUsageInt(usage, "cached_tokens"); cached != nil {
			if out.CacheReadToken == nil || *cached > 0 && (out.CacheWriteToken == nil || *out.CacheWriteToken == 0) {
				out.CacheReadToken = cached
			}
		}
		// Anthropic has no reasoning-token count field. Its converter's empty details are not evidence.
	case llm.APIFormatGeminiContents:
		out.CacheReadToken = rawUsageInt(usage, "cachedContentTokenCount")
		out.ReasoningToken = rawUsageInt(usage, "thoughtsTokenCount")
	case llm.APIFormatOpenAIResponse, llm.APIFormatOpenAIResponseCompact:
		out.CacheReadToken = rawUsageInt(usage, "input_tokens_details.cached_tokens")
		out.CacheWriteToken = rawUsageInt(usage, "input_tokens_details.cache_write_tokens")
		out.ReasoningToken = rawUsageInt(usage, "output_tokens_details.reasoning_tokens")
	default:
		out.CacheReadToken = rawUsageInt(usage, "prompt_tokens_details.cached_tokens")
		out.CacheWriteToken = rawUsageInt(usage, "prompt_tokens_details.write_cached_tokens")
		out.ReasoningToken = rawUsageInt(usage, "completion_tokens_details.reasoning_tokens")
		if cached := rawUsageInt(usage, "cached_tokens"); cached != nil {
			if out.CacheReadToken == nil || *out.CacheReadToken == 0 && *cached > 0 {
				out.CacheReadToken = cached
			}
		}
		// The OpenAI outbound adapter also handles image responses, whose usage fields use input/output names.
		if out.CacheReadToken == nil {
			out.CacheReadToken = rawUsageInt(usage, "input_tokens_details.cached_tokens")
		}
		if out.ReasoningToken == nil {
			out.ReasoningToken = rawUsageInt(usage, "output_tokens_details.reasoning_tokens")
		}
	}
	return out
}

func (ra *relayAttempt) recordUsageMetadata(data []byte) {
	value := extractRawUsageMetadata(data, ra.outAdapter.APIFormat())
	ra.usageMetadata.mu.Lock()
	defer ra.usageMetadata.mu.Unlock()
	ra.usageMetadata.value.Present = ra.usageMetadata.value.Present || value.Present
	// Streaming usage is cumulative/partial: update provided fields, retain earlier fields
	// omitted by later frames (e.g. Anthropic message_start input and message_delta output).
	if value.CacheReadToken != nil {
		ra.usageMetadata.value.CacheReadToken = value.CacheReadToken
	}
	if value.CacheWriteToken != nil {
		ra.usageMetadata.value.CacheWriteToken = value.CacheWriteToken
	}
	if value.ReasoningToken != nil {
		ra.usageMetadata.value.ReasoningToken = value.ReasoningToken
	}
}

func (ra *relayAttempt) snapshotUsageMetadata() relayUsageMetadata {
	ra.usageMetadata.mu.Lock()
	defer ra.usageMetadata.mu.Unlock()
	return ra.usageMetadata.value
}
