package price

import (
	"testing"

	"github.com/bestruirui/octopus/internal/model"
)

func TestGetLLMPriceNormalizesAndMatchesPresetSegments(t *testing.T) {
	previous := llmPrice
	llmPriceLock.Lock()
	llmPrice = map[string]model.LLMPrice{
		"gpt-4o":   {Input: 2.5, Output: 10},
		"claude-3": {Input: 3, Output: 15},
	}
	llmPriceLock.Unlock()
	t.Cleanup(func() {
		llmPriceLock.Lock()
		llmPrice = previous
		llmPriceLock.Unlock()
	})

	got := GetLLMPrice(" OpenAI/GPT-4O:deployment ")
	if got == nil {
		t.Fatal("expected provider-prefixed model to match preset")
	}
	want := model.LLMPrice{Input: 2.5, Output: 10}
	if *got != want {
		t.Fatalf("preset lookup returned %+v, want %+v", *got, want)
	}
}
