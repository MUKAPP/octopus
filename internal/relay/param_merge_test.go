package relay

import (
	"reflect"
	"testing"
)

func TestMergeParamAppend(t *testing.T) {
	t.Run("concat arrays", func(t *testing.T) {
		body := map[string]any{
			"tools": []any{
				map[string]any{"type": "function"},
			},
		}
		appendParams := map[string]any{
			"tools": []any{
				map[string]any{"type": "web_search"},
				map[string]any{"type": "x_search"},
			},
		}
		mergeParamAppend(body, appendParams)
		tools, ok := body["tools"].([]any)
		if !ok {
			t.Fatalf("tools type = %T", body["tools"])
		}
		if len(tools) != 3 {
			t.Fatalf("tools len = %d, want 3", len(tools))
		}
	})

	t.Run("write missing key", func(t *testing.T) {
		body := map[string]any{}
		appendParams := map[string]any{
			"tools": []any{map[string]any{"type": "web_search"}},
		}
		mergeParamAppend(body, appendParams)
		if _, ok := body["tools"]; !ok {
			t.Fatal("expected tools key")
		}
	})

	t.Run("skip existing scalar", func(t *testing.T) {
		body := map[string]any{"temperature": 0.2}
		appendParams := map[string]any{"temperature": 0.9}
		mergeParamAppend(body, appendParams)
		if body["temperature"] != 0.2 {
			t.Fatalf("temperature = %v, want 0.2", body["temperature"])
		}
	})

	t.Run("type conflict keeps body", func(t *testing.T) {
		body := map[string]any{"tools": []any{map[string]any{"type": "function"}}}
		appendParams := map[string]any{"tools": map[string]any{"type": "web_search"}}
		mergeParamAppend(body, appendParams)
		tools, ok := body["tools"].([]any)
		if !ok || len(tools) != 1 {
			t.Fatalf("unexpected tools: %#v", body["tools"])
		}
	})

	t.Run("nil maps are no-op", func(t *testing.T) {
		mergeParamAppend(nil, map[string]any{"a": 1})
		body := map[string]any{"a": 1}
		mergeParamAppend(body, nil)
		if !reflect.DeepEqual(body, map[string]any{"a": 1}) {
			t.Fatalf("unexpected body: %#v", body)
		}
	})
}
