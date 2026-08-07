package helper

import (
	"net/url"
	"path"
	"strings"
)

// NormalizeLegacyBaseURL 兼容迁移前的渠道 BaseURL 语义。
// 旧版 octopus 直接把渠道 BaseURL 拼上端点路径（如 "/chat/completions"），从不自动补版本号；
// axonhub/llm 的 NormalizeBaseURL 只在 URL 已含目标版本号（如 /v1）时跳过追加，
// 遇到已带其他版本路径的渠道（如腾讯 codebuddy 的 "https://copilot.tencent.com/v2"）
// 会错误地拼出 "/v2/v1/chat/completions" 导致 404。
// 这里对这类 URL 追加 "#" 标记，让 NormalizeBaseURL 跳过默认版本补全，还原旧版行为；
// 根路径与 /v1 结尾的 URL 保持不变，保留自动补版本的改进。
func NormalizeLegacyBaseURL(baseURL string) string {
	if baseURL == "" {
		return ""
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return baseURL
	}
	last := path.Base(u.Path)
	if last == "v1" || len(last) < 2 || last[0] != 'v' || last[1] < '0' || last[1] > '9' {
		return baseURL
	}
	return strings.TrimRight(baseURL, "/") + "#"
}
