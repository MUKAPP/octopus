package relay

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"testing"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/httpclient"
)

func logImageTestPNG(t *testing.T, size int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	state := uint32(0x9e3779b9)
	for y := range size {
		for x := range size {
			state ^= state << 13
			state ^= state >> 17
			state ^= state << 5
			img.SetRGBA(x, y, color.RGBA{R: uint8(state), G: uint8(state >> 8), B: uint8(state >> 16), A: 0xff})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func TestExtractRelayLogImagesResponsesOutputKinds(t *testing.T) {
	b64 := logImageTestPNG(t, 4)
	content, err := json.Marshal(map[string]any{"output": []any{
		map[string]any{"type": "message", "status": "completed", "result": b64},
		map[string]any{"type": "image_generation_call", "status": "in_progress", "result": b64},
		map[string]any{"type": "image_generation_call", "status": "completed", "result": "not-an-image"},
		map[string]any{"type": "image_generation_call", "status": "completed", "result": b64},
	}})
	if err != nil {
		t.Fatal(err)
	}
	images := extractRelayLogImages(&llm.Request{APIFormat: llm.APIFormatOpenAIResponse}, content)
	if len(images) != 1 || images[0].Src != "data:image/png;base64,"+b64 {
		t.Fatalf("images = %+v, want only the completed image result", images)
	}
	if images := extractRelayLogImages(&llm.Request{APIFormat: llm.APIFormatOpenAIChatCompletion}, content); len(images) != 0 {
		t.Fatalf("non-Responses output was scanned: %+v", images)
	}
}

func TestRelayLogImagesSurviveChannelInjectedTools(t *testing.T) {
	injectedTools := `{"tools":[{"type":"image_generation"}]}`
	b64 := logImageTestPNG(t, 384)
	if len(b64) <= responseLogMaxBytes {
		t.Fatalf("image fixture must exceed the response body budget: %d", len(b64))
	}
	content, err := json.Marshal(map[string]any{"id": "resp-image", "output": []any{
		map[string]any{"type": "image_generation_call", "status": "completed", "result": b64},
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		appendTo *string
		override *string
	}{
		{name: "param_append", appendTo: &injectedTools},
		{name: "param_override", override: &injectedTools},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metrics := &RelayMetrics{
				InternalRequest:  &llm.Request{APIFormat: llm.APIFormatOpenAIResponse, Model: "fixture"},
				InternalResponse: content,
			}
			attempt := &relayAttempt{
				relayRun: &relayRun{metrics: metrics},
				channel:  &model.Channel{Type: llm.APIFormatOpenAIResponse, ParamAppend: tc.appendTo, ParamOverride: tc.override},
			}
			outbound := &httpclient.Request{
				Headers: http.Header{"Content-Type": []string{"application/json"}},
				Body:    []byte(`{"model":"fixture","input":"draw an image"}`),
			}
			attempt.applyChannelRequestOptions(outbound)
			var effective struct {
				Tools []struct {
					Type string `json:"type"`
				} `json:"tools"`
			}
			if err := json.Unmarshal(outbound.Body, &effective); err != nil {
				t.Fatal(err)
			}
			if len(effective.Tools) != 1 || effective.Tools[0].Type != "image_generation" {
				t.Fatalf("effective tools = %+v", effective.Tools)
			}
			entry := metrics.buildRelayLog(nil, 0, nil, 0, "", 0, "")
			if len(entry.ResponseImages) != 1 || entry.ResponseImages[0].Src != "data:image/png;base64,"+b64 {
				t.Fatal("channel-generated image was not retained in full")
			}
			if !entry.ResponseContentTruncated || len(entry.ResponseContent) > responseLogMaxBytes {
				t.Fatalf("body truncation = %t, saved bytes = %d", entry.ResponseContentTruncated, len(entry.ResponseContent))
			}
			var saved struct {
				Output []struct {
					Result string `json:"result"`
				} `json:"output"`
			}
			if err := json.Unmarshal([]byte(entry.ResponseContent), &saved); err != nil {
				t.Fatal(err)
			}
			if len(saved.Output) != 1 || saved.Output[0].Result != b64[:previewMaxStringBytes] {
				t.Fatal("saved JSON must contain only the cropped image result")
			}
			omissions := entry.ResponseContentPreview.Omissions
			if len(omissions) != 1 || omissions[0].Path != "/output/0/result" || omissions[0].OriginalBytes != len(b64) || omissions[0].StoredBytes != previewMaxStringBytes {
				t.Fatalf("image result omission = %+v", omissions)
			}
		})
	}
}
