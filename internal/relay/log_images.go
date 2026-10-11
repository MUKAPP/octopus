package relay

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/looplj/axonhub/llm"
)

// extractRelayLogImages 仅从最终客户端生图响应提取图片，不解析普通文本响应。
func extractRelayLogImages(request *llm.Request, content []byte) []model.RelayLogImage {
	if request == nil || len(content) == 0 {
		return nil
	}
	var images []model.RelayLogImage
	if request.RequestType == llm.RequestTypeImage {
		var envelope struct {
			Data []llm.ImageData `json:"data"`
		}
		if json.Unmarshal(content, &envelope) != nil {
			return nil
		}
		for _, item := range envelope.Data {
			if src := logImageSource(item.B64JSON, item.URL); src != "" {
				images = append(images, model.RelayLogImage{Src: src})
			}
		}
		return images
	}
	if request.APIFormat != llm.APIFormatOpenAIResponse {
		return nil
	}
	hasImageTool := false
	for _, tool := range request.Tools {
		if tool.Type == "image_generation" {
			hasImageTool = true
			break
		}
	}
	if !hasImageTool {
		return nil
	}
	var envelope struct {
		Output []struct {
			Type   string `json:"type"`
			Status string `json:"status"`
			Result string `json:"result"`
		} `json:"output"`
	}
	if json.Unmarshal(content, &envelope) != nil {
		return nil
	}
	for _, item := range envelope.Output {
		if item.Type == "image_generation_call" && item.Status == "completed" {
			if src := inlineLogImageSource(item.Result); src != "" {
				images = append(images, model.RelayLogImage{Src: src})
			}
		}
	}
	return images
}

func logImageSource(b64JSON, imageURL string) string {
	if src := inlineLogImageSource(b64JSON); src != "" {
		return src
	}
	if strings.HasPrefix(imageURL, "data:") {
		return inlineLogImageSource(imageURL)
	}
	parsed, err := url.Parse(imageURL)
	if err == nil && parsed.Host != "" && (parsed.Scheme == "http" || parsed.Scheme == "https") {
		return imageURL
	}
	return ""
}

// inlineLogImageSource 只解码至多 512 字节用于嗅探；完整编码保留，后续损坏由浏览器呈现。
func inlineLogImageSource(value string) string {
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "data:") {
		header, encoded, found := strings.Cut(value, ",")
		if !found || !strings.HasPrefix(header, "data:image/") || !strings.HasSuffix(header, ";base64") {
			return ""
		}
		value = encoded
	}
	var prefix [512]byte
	n, err := io.ReadFull(base64.NewDecoder(base64.StdEncoding, strings.NewReader(value)), prefix[:])
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return ""
	}
	mime := http.DetectContentType(prefix[:n])
	switch mime {
	case "image/png", "image/jpeg", "image/webp":
		return "data:" + mime + ";base64," + value
	default:
		return ""
	}
}
