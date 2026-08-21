package relay

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/looplj/axonhub/llm"
)

func TestTruncateLogContent(t *testing.T) {
	// ASCII 恰好达到上限：不截断
	exact := strings.Repeat("a", 10)
	if got, truncated := truncateLogContent([]byte(exact), 10); got != exact || truncated {
		t.Fatalf("ASCII exact limit: got %q truncated=%t, want %q truncated=false", got, truncated, exact)
	}

	// ASCII 超限 1 字节：截断到上限
	over := strings.Repeat("a", 11)
	if got, truncated := truncateLogContent([]byte(over), 10); got != exact || !truncated {
		t.Fatalf("ASCII over by 1: got %q truncated=%t, want %q truncated=true", got, truncated, exact)
	}

	// 三字节 UTF-8 跨边界："abcdefgh"+"中" = 11 字节，上限 10 → 保留 8 字节，不切断码点
	three := "abcdefgh" + "中"
	if got, truncated := truncateLogContent([]byte(three), 10); got != "abcdefgh" || !truncated {
		t.Fatalf("3-byte rune across boundary: got %q truncated=%t", got, truncated)
	}

	// 三字节 UTF-8 恰好落在边界内："abcdef"+"中" = 9 字节 ≤ 9 → 完整
	okThree := "abcdef" + "中"
	if got, truncated := truncateLogContent([]byte(okThree), 9); got != okThree || truncated {
		t.Fatalf("3-byte rune within limit: got %q truncated=%t", got, truncated)
	}

	// 四字节 UTF-8 跨边界："abcdefgh"+"😀" = 12 字节，上限 10 → 保留 8 字节
	four := "abcdefgh" + "😀"
	if got, truncated := truncateLogContent([]byte(four), 10); got != "abcdefgh" || !truncated {
		t.Fatalf("4-byte rune across boundary: got %q truncated=%t", got, truncated)
	}

	// 无效 UTF-8：只保留其前面的有效前缀
	invalid := []byte("ab\xff\xfecd")
	if got, truncated := truncateLogContent(invalid, 100); got != "ab" || !truncated {
		t.Fatalf("invalid utf8: got %q truncated=%t", got, truncated)
	}

	// 空内容：不截断且为空
	if got, truncated := truncateLogContent(nil, 1024); got != "" || truncated {
		t.Fatalf("empty content: got %q truncated=%t", got, truncated)
	}

	// 结果永不超限且始终是有效 UTF-8
	fuzz := []byte("a中😀\xffb中" + strings.Repeat("x", 5000))
	got, truncated := truncateLogContent(fuzz, 1024)
	if len(got) > 1024 || !utf8.ValidString(got) {
		t.Fatalf("fuzz: len=%d valid=%t", len(got), utf8.ValidString(got))
	}
	if !truncated {
		t.Fatalf("fuzz over limit should be truncated")
	}
}

func TestRelayMetricsLogContentPreservesRequest(t *testing.T) {
	text := strings.Repeat("中", 100000) // 300,000 bytes; exceeds the fixed response limit.
	metrics := &RelayMetrics{
		InternalRequest: &llm.Request{
			Model: "gpt-4o",
			Messages: []llm.Message{{
				Role:    "user",
				Content: llm.MessageContent{Content: &text},
			}},
		},
		InternalResponse: []byte(strings.Repeat("r", 300000)),
	}

	req, reqTrunc, resp, respTrunc := metrics.logContent()
	if reqTrunc || !strings.Contains(req, text) || len(req) <= 262144 || !utf8.ValidString(req) {
		t.Fatalf("request content was truncated: truncated=%t len=%d valid=%t", reqTrunc, len(req), utf8.ValidString(req))
	}
	if !respTrunc || len(resp) > 262144 {
		t.Fatalf("response content limit not applied: truncated=%t len=%d", respTrunc, len(resp))
	}
}
