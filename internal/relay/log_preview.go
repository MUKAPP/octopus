package relay

import (
	"bytes"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/bestruirui/octopus/internal/model"
)

// 响应正文保存上限（字节）。请求正文不做大小限制。
const responseLogMaxBytes = 262144

const (
	previewFormatJSON = "json"
	previewFormatText = "text"

	previewReasonSizeLimit   = "size_limit"
	previewReasonInvalidUTF8 = "invalid_utf8"

	previewKindString = "string"
	previewKindArray  = "array"
	previewKindObject = "object"
	previewKindBinary = "binary"
)

// 结构化预览的裁剪参数：保证正文与 omissions 元数据本身有界。
const (
	previewMaxOmissions   = 32
	previewMaxPathBytes   = 512
	previewMaxStringBytes = 16384
	previewMaxBinaryBytes = 64
	previewMaxItems       = 256
)

// previewDiagnosticKeys 是无论出现在哪个对象、哪个位置都优先保留的小型排障字段。
// 它们不依赖源字段顺序，避免超出预算的大数组/大字段把 id、model、usage 等挤出预览。
var previewDiagnosticKeys = map[string]struct{}{
	"id":            {},
	"model":         {},
	"status":        {},
	"object":        {},
	"usage":         {},
	"finish_reason": {},
}

var errPreviewJSONShape = errors.New("relay: malformed JSON during preview")

// buildRelayLogContent 构造响应正文字段及其保存元数据。
// 小响应按原字节保存；超限合法 JSON 保存仍可解析的结构化预览；非法 UTF-8 保存有效前缀并标记原因。
func buildRelayLogContent(content []byte, maxBytes int) (string, bool, *model.RelayLogContentPreview) {
	if len(content) == 0 {
		return "", false, nil
	}

	validUTF8 := utf8.Valid(content)
	if validUTF8 && len(content) <= maxBytes {
		return string(content), false, &model.RelayLogContentPreview{
			OriginalBytes: len(content),
			StoredBytes:   len(content),
			Format:        previewFormat(content),
		}
	}

	// 非法 UTF-8 优先：保留有效前缀，不做结构化解析，避免把替换字符当成响应内容。
	if !validUTF8 {
		prefix, _ := truncateLogContent(content, maxBytes)
		return prefix, true, &model.RelayLogContentPreview{
			OriginalBytes: len(content),
			StoredBytes:   len(prefix),
			Format:        previewFormat([]byte(prefix)),
			Reason:        previewReasonInvalidUTF8,
		}
	}

	// 超限且是合法 JSON：生成结构化预览；任何无法保持合法 JSON 的情况都退回文本前缀。
	if json.Valid(content) {
		if stored, preview, ok := buildJSONStructurePreview(content, maxBytes); ok {
			return stored, true, preview
		}
	}

	prefix, _ := truncateLogContent(content, maxBytes)
	return prefix, true, &model.RelayLogContentPreview{
		OriginalBytes: len(content),
		StoredBytes:   len(prefix),
		Format:        previewFormat([]byte(prefix)),
		Reason:        previewReasonSizeLimit,
	}
}

// previewFormat 依据内容是否为合法 JSON 返回格式标记。
func previewFormat(content []byte) string {
	if json.Valid(content) {
		return previewFormatJSON
	}
	return previewFormatText
}

// buildJSONStructurePreview 完整解码合法 JSON（保留数字字面量），再在字节预算内重建结构：
// 每个对象先写小型排障字段，其余字段按源顺序写出；放不下的单个字段或键被跳过并记录有损信息，
// 不会把预算耗尽的判定扩散成整份预览失效。
func buildJSONStructurePreview(content []byte, maxBytes int) (string, *model.RelayLogContentPreview, bool) {
	root, err := decodePreviewValue(content)
	if err != nil {
		return "", nil, false
	}
	writer := &previewWriter{budget: maxBytes, buf: make([]byte, 0, min(maxBytes, len(content)))}
	writer.writeNode(root, "", true, "")
	stored := string(writer.buf)
	if stored == "" || !json.Valid([]byte(stored)) {
		return "", nil, false
	}
	return stored, &model.RelayLogContentPreview{
		OriginalBytes:  len(content),
		StoredBytes:    len(stored),
		Format:         previewFormatJSON,
		Reason:         previewReasonSizeLimit,
		Omissions:      writer.omissions,
		OmittedDetails: writer.omittedDetails,
	}, true
}

type previewValueKind uint8

const (
	previewValueObject previewValueKind = iota
	previewValueArray
	previewValueString
	previewValueNumber
	previewValueBool
	previewValueNull
)

// previewValue 是解码后的 JSON 值；对象保留键的源顺序。
type previewValue struct {
	kind    previewValueKind
	members []previewMember
	items   []*previewValue
	text    string
	number  json.Number
	flag    bool
}

type previewMember struct {
	key   string
	value *previewValue
}

// decodePreviewValue 用 UseNumber 完整解码 JSON，数字保持原始字面量以免精度丢失。
func decodePreviewValue(content []byte) (*previewValue, error) {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.UseNumber()
	return readPreviewValue(decoder)
}

func readPreviewValue(decoder *json.Decoder) (*previewValue, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	switch value := token.(type) {
	case json.Delim:
		switch value {
		case '{':
			node := &previewValue{kind: previewValueObject}
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return nil, err
				}
				key, ok := keyToken.(string)
				if !ok {
					return nil, errPreviewJSONShape
				}
				child, err := readPreviewValue(decoder)
				if err != nil {
					return nil, err
				}
				node.members = append(node.members, previewMember{key: key, value: child})
			}
			if _, err := decoder.Token(); err != nil {
				return nil, err
			}
			return node, nil
		case '[':
			node := &previewValue{kind: previewValueArray}
			for decoder.More() {
				child, err := readPreviewValue(decoder)
				if err != nil {
					return nil, err
				}
				node.items = append(node.items, child)
			}
			if _, err := decoder.Token(); err != nil {
				return nil, err
			}
			return node, nil
		}
		return nil, errPreviewJSONShape
	case string:
		return &previewValue{kind: previewValueString, text: value}, nil
	case json.Number:
		return &previewValue{kind: previewValueNumber, number: value}, nil
	case bool:
		return &previewValue{kind: previewValueBool, flag: value}, nil
	case nil:
		return &previewValue{kind: previewValueNull}, nil
	default:
		return nil, errPreviewJSONShape
	}
}

// previewWriter 在固定字节预算内重建 JSON，并记录被裁剪的路径。
type previewWriter struct {
	buf            []byte
	budget         int
	open           int
	omissions      []model.RelayLogContentOmission
	omittedDetails int
}

// room 判断再写入 n 字节、并最终补齐所有已打开容器后仍不超预算。
func (w *previewWriter) room(n int) bool {
	return len(w.buf)+n+w.open <= w.budget
}

func (w *previewWriter) writeLiteral(literal string) bool {
	if !w.room(len(literal)) {
		return false
	}
	w.buf = append(w.buf, literal...)
	return true
}

// openContainer 写入容器起始符并登记其闭合字节的预算占用。
func (w *previewWriter) openContainer(delim byte) bool {
	w.open++
	if !w.room(1) {
		w.open--
		return false
	}
	w.buf = append(w.buf, delim)
	return true
}

func (w *previewWriter) writeNode(node *previewValue, path string, pathValid bool, key string) bool {
	switch node.kind {
	case previewValueObject:
		return w.writeObject(node, path, pathValid)
	case previewValueArray:
		return w.writeArray(node, path, pathValid)
	case previewValueString:
		return w.writeString(node.text, path, pathValid, key)
	case previewValueNumber:
		return w.writeLiteral(string(node.number))
	case previewValueBool:
		if node.flag {
			return w.writeLiteral("true")
		}
		return w.writeLiteral("false")
	default:
		return w.writeLiteral("null")
	}
}

// writeObject 先写小型排障字段，再按源顺序写其余字段。
// 单个条目放不下时回滚该条目并继续，绝不因此放弃后续字段。
func (w *previewWriter) writeObject(node *previewValue, path string, pathValid bool) bool {
	if !w.openContainer('{') {
		return false
	}
	written, attempted := 0, 0
	for pass := range 2 {
		for index := range node.members {
			member := &node.members[index]
			_, diagnostic := previewDiagnosticKeys[member.key]
			if (pass == 0) != diagnostic || attempted >= previewMaxItems {
				continue
			}
			attempted++
			childPath, childValid := extendPreviewPointer(path, pathValid, member.key)
			entryMark, entryOpen := len(w.buf), w.open
			entryOmissions, entryDetails := len(w.omissions), w.omittedDetails
			if w.writeLiteral(previewComma(written)) &&
				w.writeKey(member.key) &&
				w.writeLiteral(":") &&
				w.writeNode(member.value, childPath, childValid, member.key) {
				written++
				continue
			}
			w.buf = w.buf[:entryMark]
			w.open = entryOpen
			w.omissions = w.omissions[:entryOmissions]
			w.omittedDetails = entryDetails
			if !childValid {
				// 键名/路径过长时无法给出可寻址的 JSON Pointer，只计入汇总数量。
				w.omittedDetails++
			}
		}
	}
	w.buf = append(w.buf, '}')
	w.open--
	if written < len(node.members) {
		w.recordOmission(path, pathValid, previewKindObject, 0, 0, len(node.members), written)
	}
	return true
}

func (w *previewWriter) writeArray(node *previewValue, path string, pathValid bool) bool {
	if !w.openContainer('[') {
		return false
	}
	written, attempted := 0, 0
	for index := range node.items {
		if attempted >= previewMaxItems {
			break
		}
		attempted++
		childPath, childValid := extendPreviewPointer(path, pathValid, strconv.Itoa(index))
		entryMark, entryOpen := len(w.buf), w.open
		entryOmissions, entryDetails := len(w.omissions), w.omittedDetails
		if w.writeLiteral(previewComma(written)) &&
			w.writeNode(node.items[index], childPath, childValid, "") {
			written++
			continue
		}
		w.buf = w.buf[:entryMark]
		w.open = entryOpen
		w.omissions = w.omissions[:entryOmissions]
		w.omittedDetails = entryDetails
		// 数组只能保留前缀；跳过中间项会改变后续索引，使损失路径指向错误字段。
		break
	}
	w.buf = append(w.buf, ']')
	w.open--
	if written < len(node.items) {
		w.recordOmission(path, pathValid, previewKindArray, 0, 0, len(node.items), written)
	}
	return true
}

// writeString 按码点写入字符串，逐字符遵守预算与单字段上限。
// 二进制字段只保留极短值，超出时保存空值并记录独立损失信息，避免占用普通文本的预览容量。
// 原始/保存字节数均按解码后的 UTF-8 值计算，与转义后长度无关。
func (w *previewWriter) writeString(value, path string, pathValid bool, key string) bool {
	if !w.room(2) {
		return false
	}
	w.buf = append(w.buf, '"')
	binary := key == "b64_json" || isBase64DataURL(value)
	limit := previewMaxStringBytes
	if binary {
		limit = previewMaxBinaryBytes
	}
	used := 0
	truncated := false
	if binary && len(value) > limit {
		truncated = true
	} else {
		for index := 0; index < len(value); {
			r, size := utf8.DecodeRuneInString(value[index:])
			if used+size > limit || !w.room(escapedRuneLen(r)+1) {
				truncated = true
				break
			}
			w.buf = appendJSONEscapedRune(w.buf, r)
			index += size
			used += size
		}
	}
	w.buf = append(w.buf, '"')
	if truncated {
		kind := previewKindString
		if binary {
			kind = previewKindBinary
		}
		w.recordOmission(path, pathValid, kind, len(value), used, 0, 0)
	}
	return true
}

// writeKey 写入对象键；键名同样受严格预算限制，放不下时由调用方整体丢弃该条目。
func (w *previewWriter) writeKey(key string) bool {
	needed := 2
	for _, r := range key {
		needed += escapedRuneLen(r)
	}
	if !w.room(needed) {
		return false
	}
	w.buf = append(w.buf, '"')
	for _, r := range key {
		w.buf = appendJSONEscapedRune(w.buf, r)
	}
	w.buf = append(w.buf, '"')
	return true
}

// recordOmission 记录一处有损裁剪；路径过长或超过条数上限时只累加计数，不伪造可寻址路径。
func (w *previewWriter) recordOmission(path string, pathValid bool, kind string, originalBytes, storedBytes, originalItems, storedItems int) {
	if !pathValid || len(path) > previewMaxPathBytes || len(w.omissions) >= previewMaxOmissions {
		w.omittedDetails++
		return
	}
	w.omissions = append(w.omissions, model.RelayLogContentOmission{
		Path:          path,
		Kind:          kind,
		OriginalBytes: originalBytes,
		StoredBytes:   storedBytes,
		OriginalItems: originalItems,
		StoredItems:   storedItems,
	})
}

func extendPreviewPointer(parent string, parentValid bool, token string) (string, bool) {
	if !parentValid {
		return parent, false
	}
	escaped := escapePreviewPointerToken(token)
	if len(parent)+len(escaped)+1 > previewMaxPathBytes {
		return parent, false
	}
	if parent == "" {
		return "/" + escaped, true
	}
	return parent + "/" + escaped, true
}

// escapePreviewPointerToken 按 RFC 6901 转义 JSON Pointer 片段。
func escapePreviewPointerToken(token string) string {
	if !strings.ContainsAny(token, "~/") {
		return token
	}
	token = strings.ReplaceAll(token, "~", "~0")
	return strings.ReplaceAll(token, "/", "~1")
}

func isBase64DataURL(value string) bool {
	return strings.HasPrefix(value, "data:") && strings.Contains(value, ";base64,")
}

func previewComma(written int) string {
	if written > 0 {
		return ","
	}
	return ""
}

// escapedRuneLen 返回 r 作为 JSON 字符串内容所需的最大字节数（含转义）。
func escapedRuneLen(r rune) int {
	switch r {
	case '"', '\\', '\n', '\r', '\t', '\b', '\f':
		return 2
	}
	if r < 0x20 {
		return 6
	}
	if r == utf8.RuneError {
		return 3
	}
	return utf8.RuneLen(r)
}

func appendJSONEscapedRune(dst []byte, r rune) []byte {
	switch r {
	case '"':
		return append(dst, '\\', '"')
	case '\\':
		return append(dst, '\\', '\\')
	case '\n':
		return append(dst, '\\', 'n')
	case '\r':
		return append(dst, '\\', 'r')
	case '\t':
		return append(dst, '\\', 't')
	case '\b':
		return append(dst, '\\', 'b')
	case '\f':
		return append(dst, '\\', 'f')
	}
	if r < 0x20 {
		const hexDigits = "0123456789abcdef"
		return append(dst, '\\', 'u', '0', '0', hexDigits[byte(r)>>4], hexDigits[byte(r)&0xf])
	}
	return utf8.AppendRune(dst, r)
}
