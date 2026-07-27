package relay

import (
	"testing"

	"github.com/looplj/axonhub/llm"
)

func TestRelayMetricsRecordUsageCapturesCachedTokens(t *testing.T) {
	metrics := &RelayMetrics{}

	metrics.RecordUsage(&llm.Usage{
		PromptTokens:     1_000,
		CompletionTokens: 100,
		PromptTokensDetails: &llm.PromptTokensDetails{
			CachedTokens:      750,
			WriteCachedTokens: 100,
		},
	})

	if metrics.CachedTokens != 750 {
		t.Errorf("CachedTokens = %d, want 750", metrics.CachedTokens)
	}
	if metrics.Stats.InputToken != 1_000 || metrics.Stats.OutputToken != 100 {
		t.Errorf("token usage = (%d, %d), want (1000, 100)", metrics.Stats.InputToken, metrics.Stats.OutputToken)
	}
}
