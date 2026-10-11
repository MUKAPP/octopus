package relay

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/bestruirui/octopus/internal/model"
)

// buildPreview 以生产预算生成结构化预览，并断言结果落在预算内且为合法 UTF-8。
func buildPreview(t *testing.T, content string) (string, *model.RelayLogContentPreview) {
	t.Helper()
	stored, truncated, preview := buildRelayLogContent([]byte(content), responseLogMaxBytes)
	if len(stored) > responseLogMaxBytes {
		t.Fatalf("stored preview exceeds budget: %d", len(stored))
	}
	if !utf8.ValidString(stored) {
		t.Fatal("stored preview is not valid UTF-8")
	}
	if !truncated {
		t.Fatalf("oversized content was not marked truncated")
	}
	if preview == nil {
		t.Fatal("oversized content has no preview metadata")
	}
	return stored, preview
}

func decodePreviewObject(t *testing.T, stored string) map[string]any {
	t.Helper()
	if !json.Valid([]byte(stored)) {
		t.Fatalf("structured preview is not parseable JSON: %.200q", stored)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(stored), &decoded); err != nil {
		t.Fatalf("decode structured preview: %v", err)
	}
	return decoded
}

func TestBuildRelayLogContentSmallResponseUnchanged(t *testing.T) {
	content := `{"id":"chatcmpl-1","model":"fixture","choices":[{"index":0,"finish_reason":"stop"}],"usage":{"total_tokens":7}}`
	stored, truncated, preview := buildRelayLogContent([]byte(content), responseLogMaxBytes)
	if truncated || stored != content {
		t.Fatalf("small response was rewritten: truncated=%t stored=%q", truncated, stored)
	}
	if preview == nil || preview.Format != previewFormatJSON || preview.OriginalBytes != len(content) || preview.StoredBytes != len(content) {
		t.Fatalf("small response metadata = %+v", preview)
	}
	if preview.Reason != "" || preview.Omissions != nil || preview.OmittedDetails != 0 {
		t.Fatalf("small response reported loss: %+v", preview)
	}
}

func TestBuildRelayLogContentOversizedJSONKeepsSmallFields(t *testing.T) {
	content := fmt.Sprintf(
		`{"id":"chatcmpl-1","model":"fixture","choices":[{"index":0,"message":{"role":"assistant","content":"%s"},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":11,"completion_tokens":22,"total_tokens":33}}`,
		strings.Repeat("x", 400000),
	)
	stored, preview := buildPreview(t, content)
	if preview.Format != previewFormatJSON || preview.Reason != previewReasonSizeLimit {
		t.Fatalf("oversized JSON metadata = %+v", preview)
	}
	decoded := decodePreviewObject(t, stored)
	if decoded["id"] != "chatcmpl-1" || decoded["model"] != "fixture" {
		t.Errorf("small identity fields lost: %#v", decoded)
	}
	choices, _ := decoded["choices"].([]any)
	if len(choices) != 1 {
		t.Fatalf("choices structure lost: %#v", decoded["choices"])
	}
	choice, _ := choices[0].(map[string]any)
	if choice["finish_reason"] != "tool_calls" {
		t.Errorf("finish_reason lost: %#v", choice)
	}
	usage, _ := decoded["usage"].(map[string]any)
	if usage["total_tokens"] != float64(33) || usage["prompt_tokens"] != float64(11) {
		t.Errorf("usage lost: %#v", usage)
	}
	message, _ := choice["message"].(map[string]any)
	contentValue, _ := message["content"].(string)
	if len(contentValue) == 0 || len(contentValue) > previewMaxStringBytes {
		t.Errorf("oversized content not trimmed to field budget: %d", len(contentValue))
	}
	if len(preview.Omissions) != 1 {
		t.Fatalf("expected one omission, got %+v", preview.Omissions)
	}
	omission := preview.Omissions[0]
	if omission.Path != "/choices/0/message/content" || omission.Kind != previewKindString || omission.OriginalBytes != 400000 || omission.StoredBytes != len(contentValue) {
		t.Errorf("string omission = %+v", omission)
	}
}

func TestBuildRelayLogContentOversizedArrayTrimmed(t *testing.T) {
	var builder strings.Builder
	builder.WriteString(`{"data":[`)
	for index := range 2000 {
		if index > 0 {
			builder.WriteByte(',')
		}
		fmt.Fprintf(&builder, `{"index":%d,"value":"%s"}`, index, strings.Repeat("y", 300))
	}
	builder.WriteString(`],"usage":{"total_tokens":7}}`)
	stored, preview := buildPreview(t, builder.String())
	decoded := decodePreviewObject(t, stored)
	items, _ := decoded["data"].([]any)
	if len(items) != previewMaxItems {
		t.Fatalf("array kept %d items, want %d", len(items), previewMaxItems)
	}
	usage, _ := decoded["usage"].(map[string]any)
	if usage["total_tokens"] != float64(7) {
		t.Errorf("usage after trimmed array lost: %#v", usage)
	}
	if len(preview.Omissions) != 1 {
		t.Fatalf("expected one array omission, got %+v", preview.Omissions)
	}
	omission := preview.Omissions[0]
	if omission.Path != "/data" || omission.Kind != previewKindArray || omission.OriginalItems != 2000 || omission.StoredItems != previewMaxItems {
		t.Errorf("array omission = %+v", omission)
	}
}

func TestBuildRelayLogContentOversizedBinaryTrimmed(t *testing.T) {
	content := fmt.Sprintf(`{"created":1,"data":[{"b64_json":"%s"}],"usage":{"total_tokens":1}}`, strings.Repeat("QUJD", 200000))
	stored, preview := buildPreview(t, content)
	decodePreviewObject(t, stored)
	if len(preview.Omissions) != 1 {
		t.Fatalf("expected one binary omission, got %+v", preview.Omissions)
	}
	omission := preview.Omissions[0]
	if omission.Path != "/data/0/b64_json" || omission.Kind != previewKindBinary {
		t.Errorf("binary omission = %+v", omission)
	}
	if !strings.Contains(stored, `"usage":{"total_tokens":1}`) {
		t.Errorf("usage after binary field lost: %.200q", stored)
	}
}

func TestBuildRelayLogContentOmissionsBounded(t *testing.T) {
	// 40 个小数组各自超出条目上限，再加一个大字符串保证整体超预算；
	// 有损条目应被上限截断并汇总其余数量。
	var builder strings.Builder
	builder.WriteString("{")
	for field := range 40 {
		if field > 0 {
			builder.WriteByte(',')
		}
		fmt.Fprintf(&builder, `"field%d":[`, field)
		for item := range 300 {
			if item > 0 {
				builder.WriteByte(',')
			}
			builder.WriteString(strconv.Itoa(item))
		}
		builder.WriteString("]")
	}
	fmt.Fprintf(&builder, `,"filler":"%s"}`, strings.Repeat("z", 400000))
	stored, preview := buildPreview(t, builder.String())
	decodePreviewObject(t, stored)
	if len(preview.Omissions) != previewMaxOmissions {
		t.Fatalf("omissions = %d, want cap %d", len(preview.Omissions), previewMaxOmissions)
	}
	if preview.OmittedDetails != 41-previewMaxOmissions {
		t.Errorf("omitted_details = %d, want %d", preview.OmittedDetails, 41-previewMaxOmissions)
	}
	for _, omission := range preview.Omissions {
		if len(omission.Path) > previewMaxPathBytes {
			t.Errorf("omission path exceeds limit: %d", len(omission.Path))
		}
	}
}

func TestBuildRelayLogContentOversizedKeyBounded(t *testing.T) {
	// 超长键名不得产生不可寻址的伪造路径，只计入汇总数量。
	content := fmt.Sprintf(`{"%s":"%s","usage":{"total_tokens":5}}`, strings.Repeat("k", previewMaxPathBytes+50), strings.Repeat("z", 400000))
	stored, preview := buildPreview(t, content)
	decoded := decodePreviewObject(t, stored)
	if decoded["usage"] == nil {
		t.Errorf("usage after oversized key lost: %.200q", stored)
	}
	for _, omission := range preview.Omissions {
		if len(omission.Path) > previewMaxPathBytes {
			t.Errorf("oversized key produced an over-long path: %d", len(omission.Path))
		}
	}
	if preview.OmittedDetails == 0 {
		t.Errorf("oversized key omission was not summarised: %+v", preview)
	}
}

func TestBuildRelayLogContentHugeObjectBounded(t *testing.T) {
	// 超大对象（数千个字段）必须有界裁剪并保留结构可解析。
	var builder strings.Builder
	builder.WriteString(`{"usage":{"total_tokens":9},"blob":{`)
	for field := range 5000 {
		if field > 0 {
			builder.WriteByte(',')
		}
		fmt.Fprintf(&builder, `"k%d":"%s"`, field, strings.Repeat("q", 200))
	}
	builder.WriteString("}}")
	stored, preview := buildPreview(t, builder.String())
	decoded := decodePreviewObject(t, stored)
	usage, _ := decoded["usage"].(map[string]any)
	if usage["total_tokens"] != float64(9) {
		t.Errorf("usage after huge object lost: %#v", usage)
	}
	blob, _ := decoded["blob"].(map[string]any)
	if len(blob) > previewMaxItems {
		t.Errorf("huge object kept %d entries, want at most %d", len(blob), previewMaxItems)
	}
	if len(preview.Omissions) == 0 {
		t.Fatalf("huge object loss was not recorded: %+v", preview)
	}
}

func TestBuildRelayLogContentNumberPrecision(t *testing.T) {
	content := fmt.Sprintf(`{"big":9007199254740993,"huge":123456789012345678901234567890,"ratio":0.1,"pad":"%s"}`, strings.Repeat("p", 400000))
	stored, _ := buildPreview(t, content)
	for _, literal := range []string{"9007199254740993", "123456789012345678901234567890", "0.1"} {
		if !strings.Contains(stored, literal) {
			t.Errorf("number literal %s lost during preview:\n%.200q", literal, stored)
		}
	}
}

func TestBuildRelayLogContentUTF8ByteBudget(t *testing.T) {
	content := fmt.Sprintf(`{"message":{"content":"%s"}}`, strings.Repeat("中", 150000))
	stored, preview := buildPreview(t, content)
	if !utf8.ValidString(stored) || !strings.HasSuffix(stored, `"}}`) {
		t.Fatalf("multibyte budget handling broke structure: %.120q", stored)
	}
	if preview.Format != previewFormatJSON {
		t.Fatalf("multibyte JSON preview format = %q", preview.Format)
	}
	if !json.Valid([]byte(stored)) {
		t.Error("multibyte structured preview is not parseable")
	}
}

func TestBuildRelayLogContentInvalidUTF8KeepsValidPrefix(t *testing.T) {
	raw := append([]byte(`{"error":"ab`), 0xff, 0xfe)
	raw = append(raw, []byte("cd\"}")...)
	stored, truncated, preview := buildRelayLogContent(raw, responseLogMaxBytes)
	if !truncated {
		t.Fatal("invalid UTF-8 content was not marked truncated")
	}
	if preview == nil || preview.Reason != previewReasonInvalidUTF8 || preview.Format != previewFormatText {
		t.Fatalf("invalid UTF-8 metadata = %+v", preview)
	}
	if stored != `{"error":"ab` || preview.OriginalBytes != len(raw) || preview.StoredBytes != len(stored) {
		t.Fatalf("invalid UTF-8 prefix = %q (original=%d stored=%d)", stored, preview.OriginalBytes, preview.StoredBytes)
	}
	if !utf8.ValidString(stored) {
		t.Fatal("invalid UTF-8 prefix is not valid UTF-8")
	}
}

func TestBuildRelayLogContentNonJSONPrefix(t *testing.T) {
	content := strings.Repeat("plain text line\n", 30000)
	stored, preview := buildPreview(t, content)
	if preview.Format != previewFormatText || preview.Reason != previewReasonSizeLimit {
		t.Fatalf("non-JSON preview metadata = %+v", preview)
	}
	if len(stored) > responseLogMaxBytes || !utf8.ValidString(stored) || !strings.HasPrefix(content, stored) {
		t.Fatalf("non-JSON prefix invalid: len=%d", len(stored))
	}
}

func TestBuildRelayLogErrorUnchanged(t *testing.T) {
	metrics := &RelayMetrics{
		InternalResponse: []byte(`{"error":"boom"}`),
	}
	err := errors.New("upstream HTTP 520: fixture upstream detail")
	relayLog := metrics.buildRelayLog(err, 0, nil, 0, "", 0, "")
	if relayLog.Error != err.Error() {
		t.Fatalf("error text was rewritten: %q", relayLog.Error)
	}
}

func TestBuildRelayLogContentKeepsTrailingMetadata(t *testing.T) {
	t.Run("large array", func(t *testing.T) {
		var builder strings.Builder
		builder.WriteString(`{"choices":[`)
		for index := range 40 {
			if index > 0 {
				builder.WriteByte(',')
			}
			fmt.Fprintf(&builder, `{"message":{"content":"%s"},"finish_reason":"stop"}`, strings.Repeat("x", 18000))
		}
		builder.WriteString(`],"id":"chatcmpl-tail","model":"fixture","usage":{"total_tokens":23}}`)
		stored, _ := buildPreview(t, builder.String())
		decoded := decodePreviewObject(t, stored)
		usage, _ := decoded["usage"].(map[string]any)
		if usage["total_tokens"] != float64(23) || decoded["id"] != "chatcmpl-tail" || decoded["model"] != "fixture" {
			t.Fatalf("trailing metadata lost: usage=%v id=%v model=%v", usage, decoded["id"], decoded["model"])
		}
	})
	t.Run("oversized key", func(t *testing.T) {
		content := fmt.Sprintf(`{"%s":"payload","usage":{"total_tokens":23},"model":"fixture"}`, strings.Repeat("k", responseLogMaxBytes+1))
		stored, _ := buildPreview(t, content)
		decoded := decodePreviewObject(t, stored)
		usage, _ := decoded["usage"].(map[string]any)
		if usage["total_tokens"] != float64(23) || decoded["model"] != "fixture" {
			t.Fatalf("metadata after oversized key lost: usage=%v model=%v", usage, decoded["model"])
		}
	})
}

func TestBuildRelayLogContentEscapedStringSizes(t *testing.T) {
	value := strings.Repeat("\n", 150000)
	content, err := json.Marshal(map[string]string{"content": value})
	if err != nil {
		t.Fatal(err)
	}
	stored, preview := buildPreview(t, string(content))
	decoded := decodePreviewObject(t, stored)
	saved, _ := decoded["content"].(string)
	if len(preview.Omissions) != 1 || preview.Omissions[0].StoredBytes != len(saved) {
		t.Fatalf("stored field byte count does not match saved value: omissions=%+v decoded bytes=%d", preview.Omissions, len(saved))
	}
}

func TestBuildRelayLogContentKeepsMetadataAfterOrderIndependent(t *testing.T) {
	t.Run("large object then metadata", func(t *testing.T) {
		// usage/model 位于一个超过条目上限的对象字段之后：小型排障字段的保留不依赖源字段顺序。
		var builder strings.Builder
		builder.WriteString(`{"blob":{`)
		for field := range previewMaxItems + 8 {
			if field > 0 {
				builder.WriteByte(',')
			}
			fmt.Fprintf(&builder, `"k%d":"%s"`, field, strings.Repeat("q", 20000))
		}
		builder.WriteString(`},"usage":{"total_tokens":41},"model":"fixture"}`)
		stored, _ := buildPreview(t, builder.String())
		decoded := decodePreviewObject(t, stored)
		usage, _ := decoded["usage"].(map[string]any)
		if usage["total_tokens"] != float64(41) || decoded["model"] != "fixture" {
			t.Fatalf("metadata after large object lost: usage=%v model=%v", usage, decoded["model"])
		}
	})
	t.Run("omitted pointer is escaped", func(t *testing.T) {
		// 被省略字段的键含 "/" 与 "~"：JSON Pointer 应遵循 RFC 6901 转义。
		content := fmt.Sprintf(`{"a/b~c":"%s","usage":{"total_tokens":3}}`, strings.Repeat("z", 400000))
		stored, preview := buildPreview(t, content)
		decoded := decodePreviewObject(t, stored)
		usage, _ := decoded["usage"].(map[string]any)
		if usage["total_tokens"] != float64(3) {
			t.Errorf("usage lost: %#v", usage)
		}
		if len(preview.Omissions) != 1 {
			t.Fatalf("expected one omission, got %+v", preview.Omissions)
		}
		if omission := preview.Omissions[0]; omission.Path != "/a~1b~0c" || omission.Kind != previewKindString {
			t.Errorf("escaped pointer omission = %+v", omission)
		}
	})
}

func TestBuildRelayLogContentArrayRetainsPrefix(t *testing.T) {
	content := "[" + strings.Repeat("9", responseLogMaxBytes+1) + `,"` + strings.Repeat("x", 400000) + `"]`
	stored, preview := buildPreview(t, content)
	var items []any
	if err := json.Unmarshal([]byte(stored), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("unrepresentable first item was skipped, changing later array indices: kept %d items", len(items))
	}
	if len(preview.Omissions) != 1 || preview.Omissions[0].Path != "" || preview.Omissions[0].OriginalItems != 2 || preview.Omissions[0].StoredItems != 0 {
		t.Fatalf("array omission metadata = %+v", preview.Omissions)
	}
}

// TestBuildJSONStructurePreviewEmbeddingKeepsPrefixAndUsage 覆盖 embedding 类响应：
// 超大数值数组只保留前缀，扫描丢弃的标量不得把尾部 usage 与身份字段挤出预览。
func TestBuildJSONStructurePreviewEmbeddingKeepsPrefixAndUsage(t *testing.T) {
	var builder strings.Builder
	builder.WriteString(`{"object":"list","model":"text-embedding-3-small","data":[{"object":"embedding","index":0,"embedding":[`)
	for index := range 150000 {
		if index > 0 {
			builder.WriteByte(',')
		}
		builder.WriteString("0.5")
	}
	builder.WriteString(`]}],"usage":{"prompt_tokens":7,"total_tokens":7}}`)
	stored, preview := buildPreview(t, builder.String())
	decoded := decodePreviewObject(t, stored)
	if decoded["object"] != "list" || decoded["model"] != "text-embedding-3-small" {
		t.Fatalf("embedding identity lost: %#v", decoded)
	}
	data, _ := decoded["data"].([]any)
	if len(data) != 1 {
		t.Fatalf("embedding data structure lost: %#v", decoded["data"])
	}
	entry, _ := data[0].(map[string]any)
	if entry["object"] != "embedding" || entry["index"] != float64(0) {
		t.Errorf("embedding entry lost: %#v", entry)
	}
	embedding, _ := entry["embedding"].([]any)
	if len(embedding) != previewMaxItems {
		t.Fatalf("embedding prefix length = %d, want %d", len(embedding), previewMaxItems)
	}
	for index, value := range embedding {
		if value != float64(0.5) {
			t.Fatalf("embedding[%d] = %v, want 0.5", index, value)
		}
	}
	usage, _ := decoded["usage"].(map[string]any)
	if usage["prompt_tokens"] != float64(7) || usage["total_tokens"] != float64(7) {
		t.Fatalf("trailing usage lost after numeric array: %#v", usage)
	}
	if len(preview.Omissions) != 1 {
		t.Fatalf("expected one array omission, got %+v", preview.Omissions)
	}
	omission := preview.Omissions[0]
	if omission.Path != "/data/0/embedding" || omission.Kind != previewKindArray || omission.OriginalItems != 150000 || omission.StoredItems != previewMaxItems {
		t.Errorf("embedding omission = %+v", omission)
	}
}

// TestBuildJSONStructurePreviewAllocationsDecoupledFromDroppedScalars 断言分配不随丢弃的标量线性增长：
// 数值数组从三万余项增到十三万余项时，分配次数仍受 previewMaxItems 上限约束（旧 AST 路径会为每个
// 丢弃的标量各建一个节点，约等于丢弃项数），而不是随丢弃项数增长。
func TestBuildJSONStructurePreviewAllocationsDecoupledFromDroppedScalars(t *testing.T) {
	fixture := func(count int) []byte {
		var builder strings.Builder
		builder.WriteString(`{"object":"list","data":[`)
		for index := range count {
			if index > 0 {
				builder.WriteByte(',')
			}
			fmt.Fprintf(&builder, "%d.%08d", index%10, index)
		}
		builder.WriteString(`],"usage":{"total_tokens":3}}`)
		return []byte(builder.String())
	}
	small := fixture(1 << 15)
	large := fixture(1 << 17)
	if len(small) <= responseLogMaxBytes || len(large) <= responseLogMaxBytes {
		t.Fatalf("fixtures must exceed budget: small=%d large=%d", len(small), len(large))
	}
	if _, preview, ok := buildJSONStructurePreview(large, responseLogMaxBytes); !ok || preview == nil {
		t.Fatalf("large embedding preview failed")
	}
	measure := func(content []byte) float64 {
		return testing.AllocsPerRun(50, func() {
			buildJSONStructurePreview(content, responseLogMaxBytes)
		})
	}
	smallAllocs, largeAllocs := measure(small), measure(large)
	// 两条路径丢弃的标量数量相差 4 倍，但都只保留有界的 previewMaxItems 项：
	// 分配次数应基本持平，且受 previewMaxItems 上限约束（旧 AST 路径约等于丢弃项数）。
	budget := 8*float64(previewMaxItems) + 256
	if largeAllocs > budget {
		t.Fatalf("allocations grew with dropped scalars: large=%.0f budget=%.0f (dropped %d scalars)", largeAllocs, budget, 1<<17)
	}
	if largeAllocs > smallAllocs+64 {
		t.Fatalf("allocations not decoupled from dropped scalar count: small=%.0f large=%.0f", smallAllocs, largeAllocs)
	}
}

func TestBuildJSONStructurePreviewAllocationsDecoupledFromEscapedKeys(t *testing.T) {
	fixture := func(count int) []byte {
		var builder strings.Builder
		builder.WriteByte('{')
		for index := range count {
			fmt.Fprintf(&builder, `"k%06d\u0061":0,`, index)
		}
		builder.WriteString(`"m\u006fdel":"fixture","\u0075sage":{"total_tokens":9}}`)
		return []byte(builder.String())
	}
	small, large := fixture(1<<15), fixture(1<<17)
	stored, preview, ok := buildJSONStructurePreview(large, responseLogMaxBytes)
	if !ok {
		t.Fatal("escaped-key preview failed")
	}
	decoded := decodePreviewObject(t, stored)
	usage, _ := decoded["usage"].(map[string]any)
	if decoded["model"] != "fixture" || usage["total_tokens"] != float64(9) || decoded["k000000a"] != float64(0) {
		t.Fatalf("escaped keys lost or decoded incorrectly: %#v", decoded)
	}
	if len(decoded) != previewMaxItems || len(preview.Omissions) != 1 || preview.Omissions[0].OriginalItems != (1<<17)+2 || preview.Omissions[0].StoredItems != previewMaxItems {
		t.Fatalf("escaped object omission = %+v, stored keys = %d", preview, len(decoded))
	}
	measure := func(content []byte) float64 {
		return testing.AllocsPerRun(3, func() {
			buildJSONStructurePreview(content, responseLogMaxBytes)
		})
	}
	smallAllocs, largeAllocs := measure(small), measure(large)
	if largeAllocs > 8*float64(previewMaxItems)+256 || largeAllocs > smallAllocs+64 {
		t.Fatalf("allocations grew with discarded escaped keys: small=%.0f large=%.0f", smallAllocs, largeAllocs)
	}
}

// TestBuildJSONStructurePreviewScansEscapedStrings 覆盖新扫描器的易错字符串：
// 控制字符、转义引号/反斜杠、多字节与替换字符；原始字节数按解码值计，与转义后长度无关。
func TestBuildJSONStructurePreviewScansEscapedStrings(t *testing.T) {
	tricky := "esc:\u0000\u001f\\\"\n\r\t/\u00e9\u4e2d\U0001F600\ufffd"
	repeated := strings.Repeat(tricky, 10000)
	content, err := json.Marshal(map[string]string{"content": repeated, "id": "chatcmpl-tricky"})
	if err != nil {
		t.Fatal(err)
	}
	if len(content) <= responseLogMaxBytes {
		t.Fatalf("fixture below budget: %d", len(content))
	}
	stored, preview := buildPreview(t, string(content))
	decoded := decodePreviewObject(t, stored)
	if decoded["id"] != "chatcmpl-tricky" {
		t.Fatalf("id lost: %#v", decoded["id"])
	}
	saved, _ := decoded["content"].(string)
	want, _ := truncateLogContent([]byte(repeated), previewMaxStringBytes)
	if saved != want {
		t.Fatalf("decoded content prefix differs: saved %d bytes, want %d", len(saved), len(want))
	}
	if len(preview.Omissions) != 1 {
		t.Fatalf("expected one string omission, got %+v", preview.Omissions)
	}
	omission := preview.Omissions[0]
	if omission.Path != "/content" || omission.Kind != previewKindString {
		t.Fatalf("tricky string omission = %+v", omission)
	}
	if omission.OriginalBytes != len(repeated) {
		t.Errorf("original bytes = %d, want %d", omission.OriginalBytes, len(repeated))
	}
	if omission.StoredBytes != len(saved) {
		t.Errorf("stored bytes = %d, want %d", omission.StoredBytes, len(saved))
	}
}

// TestBuildJSONStructurePreviewScansSurrogateEscapes 覆盖手写 JSON 中的代理对与孤立代理：
// 解码结果、原始字节计数与 encoding/json 的约定一致，且只保留有界前缀。
func TestBuildJSONStructurePreviewScansSurrogateEscapes(t *testing.T) {
	t.Run("surrogate pair", func(t *testing.T) {
		content := `{"content":"` + strings.Repeat(`\ud83d\ude00`, 25000) + `","id":"m"}`
		if len(content) <= responseLogMaxBytes {
			t.Fatalf("fixture below budget: %d", len(content))
		}
		stored, preview := buildPreview(t, content)
		decoded := decodePreviewObject(t, stored)
		saved, _ := decoded["content"].(string)
		want := strings.Repeat("\U0001F600", 25000)
		if saved != strings.Repeat("\U0001F600", previewMaxStringBytes/4) {
			t.Fatalf("surrogate pair decoded incorrectly")
		}
		if len(preview.Omissions) != 1 || preview.Omissions[0].OriginalBytes != len(want) {
			t.Fatalf("surrogate pair omission = %+v, want original %d", preview.Omissions, len(want))
		}
	})
	t.Run("isolated surrogate", func(t *testing.T) {
		content := `{"content":"` + strings.Repeat(`\ud800`, 50000) + `","id":"m"}`
		if len(content) <= responseLogMaxBytes {
			t.Fatalf("fixture below budget: %d", len(content))
		}
		stored, preview := buildPreview(t, content)
		decoded := decodePreviewObject(t, stored)
		saved, _ := decoded["content"].(string)
		want := strings.Repeat("\uFFFD", 50000)
		if saved != strings.Repeat("\uFFFD", previewMaxStringBytes/3) {
			t.Fatalf("isolated surrogate did not decode to the expected prefix")
		}
		if len(preview.Omissions) != 1 || preview.Omissions[0].OriginalBytes != len(want) {
			t.Fatalf("isolated surrogate omission = %+v, want original %d", preview.Omissions, len(want))
		}
	})
}

// TestBuildJSONStructurePreviewDepthBounded 覆盖嵌套边界：
// 依赖每层条目上限无法约束深树；下钻深度必须有界，且深层尾部元数据仍被保留。
func TestBuildJSONStructurePreviewDepthBounded(t *testing.T) {
	const depth = 4 * previewMaxDepth
	var builder strings.Builder
	builder.WriteString(`{"model":"fixture","deep":`)
	builder.WriteString(strings.Repeat("[", depth))
	builder.WriteString("7")
	builder.WriteString(strings.Repeat("]", depth))
	builder.WriteString(`,"usage":{"total_tokens":5},"padding":"`)
	builder.WriteString(strings.Repeat("z", 400000))
	builder.WriteString(`"}`)
	stored, preview := buildPreview(t, builder.String())
	decoded := decodePreviewObject(t, stored)
	usage, _ := decoded["usage"].(map[string]any)
	if usage["total_tokens"] != float64(5) || decoded["model"] != "fixture" {
		t.Fatalf("metadata after deep tree lost: usage=%#v model=%#v", usage, decoded["model"])
	}
	represented := 0
	for node := decoded["deep"]; ; {
		items, ok := node.([]any)
		if !ok || len(items) == 0 {
			break
		}
		represented++
		node = items[0]
	}
	if represented == 0 || represented > previewMaxDepth {
		t.Fatalf("deep tree depth %d not bounded by %d", represented, previewMaxDepth)
	}
	foundDeepLoss := false
	for _, omission := range preview.Omissions {
		if strings.HasPrefix(omission.Path, "/deep/") && omission.Kind == previewKindArray && omission.OriginalItems == 1 && omission.StoredItems == 0 {
			foundDeepLoss = true
		}
	}
	if !foundDeepLoss {
		t.Fatalf("deep tree omission does not describe the dropped child: %+v", preview)
	}
}

func TestBuildJSONStructurePreviewScansEscapedKeys(t *testing.T) {
	content := `{"x\/~\u0061":"` + strings.Repeat("z", 400000) + `","m\u006fdel":"fixture","\u0075sage":{"total_tokens":9}}`
	stored, preview := buildPreview(t, content)
	decoded := decodePreviewObject(t, stored)
	usage, _ := decoded["usage"].(map[string]any)
	if decoded["model"] != "fixture" || usage["total_tokens"] != float64(9) {
		t.Fatalf("escaped diagnostic keys lost: %#v", decoded)
	}
	if decoded["x/~a"] != strings.Repeat("z", previewMaxStringBytes) {
		t.Fatal("escaped ordinary key or value prefix changed")
	}
	if len(preview.Omissions) != 1 || preview.Omissions[0].Path != "/x~1~0a" || preview.Omissions[0].OriginalBytes != 400000 {
		t.Fatalf("escaped key omission = %+v", preview.Omissions)
	}
}

// BenchmarkBuildJSONStructurePreviewEmbedding 度量 embedding 类大响应的结构化预览开销；
// 输入在测量前构造，不计入 b.N 循环。
func BenchmarkBuildJSONStructurePreviewEmbedding(b *testing.B) {
	var builder strings.Builder
	builder.WriteString(`{"object":"list","model":"text-embedding-3-small","data":[{"object":"embedding","index":0,"embedding":[`)
	for index := range 150000 {
		if index > 0 {
			builder.WriteByte(',')
		}
		builder.WriteString("0.5")
	}
	builder.WriteString(`]}],"usage":{"prompt_tokens":7,"total_tokens":7}}`)
	content := []byte(builder.String())
	b.ReportAllocs()
	b.SetBytes(int64(len(content)))
	b.ResetTimer()
	for range b.N {
		if _, preview, ok := buildJSONStructurePreview(content, responseLogMaxBytes); !ok || preview == nil {
			b.Fatal("structured preview failed")
		}
	}
}
