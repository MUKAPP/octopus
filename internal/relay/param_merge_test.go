package relay

import (
	"reflect"
	"testing"
)

func TestMergeParamAppend(t *testing.T) {
	t.Run("concat arrays", func(t *testing.T) {
		body := map[string]any{
			"tools": []any{
				map[string]any{"type": "function", "function": map[string]any{"name": "get_weather"}},
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

	// 客户端 function.name 为 web_search 时，不追加内置 web_search，仍可追加 x_search
	t.Run("dedup tools keep client when name conflicts", func(t *testing.T) {
		body := map[string]any{
			"tools": []any{
				map[string]any{
					"type": "function",
					"function": map[string]any{
						"name":        "web_search",
						"description": "client anysearch",
					},
				},
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
		if len(tools) != 2 {
			t.Fatalf("tools len = %d, want 2 (client web_search + x_search)", len(tools))
		}
		// 第一项仍是客户端 function
		first, ok := tools[0].(map[string]any)
		if !ok || first["type"] != "function" {
			t.Fatalf("first tool should remain client function, got %#v", tools[0])
		}
		// 第二项为 x_search
		second, ok := tools[1].(map[string]any)
		if !ok || second["type"] != "x_search" {
			t.Fatalf("second tool should be x_search, got %#v", tools[1])
		}
	})

	// 客户端已有 user-anysearch-search 时，内置 web_search / x_search 正常追加
	t.Run("dedup tools append when names differ", func(t *testing.T) {
		body := map[string]any{
			"tools": []any{
				map[string]any{
					"type": "function",
					"function": map[string]any{
						"name": "user-anysearch-search",
					},
				},
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

	// 客户端已有内置 web_search 时，不再追加同 type
	t.Run("dedup built-in tools by type", func(t *testing.T) {
		body := map[string]any{
			"tools": []any{
				map[string]any{"type": "web_search"},
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
		if len(tools) != 2 {
			t.Fatalf("tools len = %d, want 2", len(tools))
		}
	})
}

func TestToolIdentityName(t *testing.T) {
	tests := []struct {
		name string
		tool any
		want string
	}{
		{
			name: "function name",
			tool: map[string]any{"type": "function", "function": map[string]any{"name": "web_search"}},
			want: "web_search",
		},
		{
			name: "built-in type",
			tool: map[string]any{"type": "web_search"},
			want: "web_search",
		},
		{
			name: "x_search",
			tool: map[string]any{"type": "x_search"},
			want: "x_search",
		},
		{
			name: "invalid",
			tool: "not-a-map",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := toolIdentityName(tt.tool); got != tt.want {
				t.Fatalf("toolIdentityName() = %q, want %q", got, tt.want)
			}
		})
	}
}
