package task

import (
	"reflect"
	"testing"
)

func TestSelectModelsForSyncSkipsEmptyUpstreamResult(t *testing.T) {
	oldModels := []string{"gpt-4o", "claude-3-5-sonnet"}

	models, shouldUpdate := selectModelsForSync(oldModels, []string{"", "  "})

	if shouldUpdate {
		t.Fatal("上游返回空模型列表时不应更新渠道")
	}
	if !reflect.DeepEqual(models, oldModels) {
		t.Fatalf("上游返回空列表时应保留原模型：got %v, want %v", models, oldModels)
	}
}

func TestAppendUniqueModelNamesPreservesExistingModels(t *testing.T) {
	seen := make(map[string]struct{})
	models := appendUniqueModelNames(nil, []string{" GPT-4O ", "claude-3-5-sonnet", "gpt-4o", ""}, seen)

	want := []string{"gpt-4o", "claude-3-5-sonnet"}
	if !reflect.DeepEqual(models, want) {
		t.Fatalf("模型列表不符合预期：got %v, want %v", models, want)
	}
}
