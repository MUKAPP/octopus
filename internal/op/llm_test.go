package op

import (
	"context"
	"testing"

	"github.com/bestruirui/octopus/internal/model"
)

func TestLLMGetNormalizesModelName(t *testing.T) {
	previous := llmModelCache.GetAll()
	llmModelCache.Clear()
	t.Cleanup(func() {
		llmModelCache.Clear()
		for name, modelPrice := range previous {
			llmModelCache.Set(name, modelPrice)
		}
	})

	want := model.LLMPrice{Input: 1.25, Output: 5}
	llmModelCache.Set("gpt-test", want)
	got, err := LLMGet("  GPT-TEST ")
	if err != nil {
		t.Fatalf("normalized model lookup failed: %v", err)
	}
	if got != want {
		t.Fatalf("normalized model lookup returned %+v, want %+v", got, want)
	}
}

func TestLLMBatchSaveRejectsCanonicalDuplicates(t *testing.T) {
	err := LLMBatchSave([]model.LLMInfo{
		{Name: "GPT-Test", LLMPrice: model.LLMPrice{Input: 1}},
		{Name: " gpt-test ", LLMPrice: model.LLMPrice{Input: 2}},
	}, context.Background())
	if err == nil {
		t.Fatal("expected duplicate canonical model names to be rejected")
	}
}

func TestLLMCleanupGhostsKeepsPricedUnreferencedModels(t *testing.T) {
	previousModels := llmModelCache.GetAll()
	previousChannels := channelCache.GetAll()
	llmModelCache.Clear()
	channelCache.Clear()
	t.Cleanup(func() {
		llmModelCache.Clear()
		for name, modelPrice := range previousModels {
			llmModelCache.Set(name, modelPrice)
		}
		channelCache.Clear()
		for id, channel := range previousChannels {
			channelCache.Set(id, channel)
		}
	})

	llmModelCache.Set("manual-ghost", model.LLMPrice{Input: 0.1})
	if err := LLMCleanupGhosts(context.Background()); err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}
	if _, ok := llmModelCache.Get("manual-ghost"); !ok {
		t.Fatal("cleanup removed an unreferenced priced model")
	}
}
