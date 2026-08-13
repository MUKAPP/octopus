package relay

import (
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
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

func TestRelayMetricsLogContentIndependenceAndSwitch(t *testing.T) {
	if err := db.InitDB("sqlite", filepath.Join(t.TempDir(), "content.db"), false); err != nil {
		t.Fatalf("init db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := op.InitCache(); err != nil {
		t.Fatalf("init cache: %v", err)
	}

	// 上限设为最小 1 KiB；构造请求、响应两侧都超限的内容
	if err := op.SettingSetString(model.SettingKeyRelayLogContentMaxBytes, "1024"); err != nil {
		t.Fatalf("set max bytes: %v", err)
	}
	if err := op.SettingSetString(model.SettingKeyRelayLogContentEnabled, "true"); err != nil {
		t.Fatalf("enable content: %v", err)
	}

	text := strings.Repeat("中", 400) // 400*3 = 1200 字节 > 1024
	metrics := &RelayMetrics{
		InternalRequest: &llm.Request{
			Model: "gpt-4o",
			Messages: []llm.Message{{
				Role:    "user",
				Content: llm.MessageContent{Content: &text},
			}},
		},
		InternalResponse: []byte(strings.Repeat("r", 2048)),
	}

	req, reqTrunc, resp, respTrunc := metrics.logContent()
	if !reqTrunc || len(req) > 1024 || !utf8.ValidString(req) {
		t.Fatalf("request content: truncated=%t len=%d valid=%t", reqTrunc, len(req), utf8.ValidString(req))
	}
	if !respTrunc || len(resp) > 1024 {
		t.Fatalf("response content: truncated=%t len=%d", respTrunc, len(resp))
	}

	// 关闭正文：两个正文字段为空且不标截断
	if err := op.SettingSetString(model.SettingKeyRelayLogContentEnabled, "false"); err != nil {
		t.Fatalf("disable content: %v", err)
	}
	req, reqTrunc, resp, respTrunc = metrics.logContent()
	if req != "" || resp != "" || reqTrunc || respTrunc {
		t.Fatalf("content disabled: req=%q reqTrunc=%t resp=%q respTrunc=%t", req, reqTrunc, resp, respTrunc)
	}
}
