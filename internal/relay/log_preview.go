package relay

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"unicode/utf16"
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
	// previewMaxDepth 限制预览下钻深度：仅靠每层 256 条目上限无法约束深树，超过此深度的值整体跳过。
	previewMaxDepth = 64
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

// buildJSONStructurePreview 直接在已通过 json.Valid 的原始字节上按预算重建预览。
// 不再完整解码成 AST：丢弃的超大数组/标量只扫描计数，不物化每个无用节点；
// 对象先写小型排障字段，其余字段按源顺序写出，放不下的字段跳过并记录有损信息。
func buildJSONStructurePreview(content []byte, maxBytes int) (string, *model.RelayLogContentPreview, bool) {
	scanner := &previewScanner{src: content}
	scanner.skipWS()
	if scanner.pos >= len(content) {
		return "", nil, false
	}
	writer := &previewWriter{budget: maxBytes, buf: make([]byte, 0, min(maxBytes, len(content)))}
	writer.writeValue(scanner, "", true, "", 0)
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

// previewScanner 在原始 JSON 字节上前进，不物化 AST；扫描/跳过/计数路径均不分配。
type previewScanner struct {
	src []byte
	pos int
}

// skipWS 跳过 JSON 空白。
func (s *previewScanner) skipWS() {
	for s.pos < len(s.src) {
		switch s.src[s.pos] {
		case ' ', '\t', '\n', '\r':
			s.pos++
		default:
			return
		}
	}
}

// skipString 跳到当前字符串闭合引号之后。
func (s *previewScanner) skipString() {
	s.pos++
	for s.pos < len(s.src) {
		switch s.src[s.pos] {
		case '\\':
			s.pos += 2
		case '"':
			s.pos++
			return
		default:
			s.pos++
		}
	}
}

// scanRawString 返回字符串未解码的原始内容（不含两侧引号），并前进到闭合引号之后。
func (s *previewScanner) scanRawString() []byte {
	s.pos++
	start := s.pos
	for s.pos < len(s.src) {
		switch s.src[s.pos] {
		case '\\':
			s.pos += 2
		case '"':
			raw := s.src[start:s.pos]
			s.pos++
			return raw
		default:
			s.pos++
		}
	}
	return s.src[start:]
}

// skipScalar 跳到 true/false/null/数字之后。
func (s *previewScanner) skipScalar() {
	for s.pos < len(s.src) {
		switch s.src[s.pos] {
		case ' ', '\t', '\n', '\r', ',', '}', ']':
			return
		default:
			s.pos++
		}
	}
}

// skipValue 迭代跳过任意 JSON 值；容器用显式深度计数，深树不会增长调用栈。
func (s *previewScanner) skipValue() {
	s.skipWS()
	if s.pos >= len(s.src) {
		return
	}
	switch s.src[s.pos] {
	case '{', '[':
		depth := 0
		for s.pos < len(s.src) {
			switch s.src[s.pos] {
			case '{', '[':
				depth++
				s.pos++
			case '}', ']':
				depth--
				s.pos++
				if depth == 0 {
					return
				}
			case '"':
				s.skipString()
			case ',', ':':
				s.pos++
			case ' ', '\t', '\n', '\r':
				s.pos++
			default:
				s.skipScalar()
			}
		}
	case '"':
		s.skipString()
	default:
		s.skipScalar()
	}
}

// isPreviewDiagnosticKey 判断对象键是否为优先保留的小型排障字段。
// 常见无转义键直接查表；含转义键只解码到固定栈缓冲，丢弃字段不分配字符串。
func isPreviewDiagnosticKey(raw []byte) bool {
	// 最长排障键 finish_reason 即使每字节写成 Unicode 转义也不超过 78 字节。
	if len(raw) > 6*len("finish_reason") {
		return false
	}
	if bytes.IndexByte(raw, '\\') < 0 {
		_, ok := previewDiagnosticKeys[string(raw)]
		return ok
	}
	var decoded [len("finish_reason")]byte
	used := 0
	for index := 0; index < len(raw); {
		r, next := decodeJSONStringRune(raw, index)
		if r >= utf8.RuneSelf || used == len(decoded) {
			return false
		}
		decoded[used] = byte(r)
		used++
		index = next
	}
	_, ok := previewDiagnosticKeys[string(decoded[:used])]
	return ok
}

// decodeJSONString 把 JSON 字符串原始内容解码为 Go 字符串，仅用于含转义的键。
func decodeJSONString(raw []byte) string {
	if bytes.IndexByte(raw, '\\') < 0 {
		return string(raw)
	}
	var builder strings.Builder
	builder.Grow(len(raw))
	for index := 0; index < len(raw); {
		r, next := decodeJSONStringRune(raw, index)
		builder.WriteRune(r)
		index = next
	}
	return builder.String()
}

// measureJSONString 扫描字符串原始内容，返回解码后的字节数及是否为 Base64 data URL。
// 全程不分配，也不复制整段内容；无转义的常见情形直接按字节判定。
func measureJSONString(raw []byte) (int, bool) {
	if bytes.IndexByte(raw, '\\') < 0 {
		return len(raw), bytes.HasPrefix(raw, jsonDataURLPrefix) && bytes.Contains(raw, jsonBase64Marker)
	}
	decodedLen := 0
	prefixMatched := 0
	prefixLive := true
	matched := 0
	dataURL := false
	for index := 0; index < len(raw); {
		r, next := decodeJSONStringRune(raw, index)
		index = next
		decodedLen += utf8.RuneLen(r)
		if prefixLive && prefixMatched < len(jsonDataURLPrefix) {
			if r == rune(jsonDataURLPrefix[prefixMatched]) {
				prefixMatched++
			} else {
				prefixLive = false
			}
		}
		if !dataURL {
			if r == rune(jsonBase64Marker[matched]) {
				matched++
				if matched == len(jsonBase64Marker) {
					dataURL = true
				}
			} else {
				matched = 0
				if r == rune(jsonBase64Marker[0]) {
					matched = 1
				}
			}
		}
	}
	return decodedLen, dataURL && prefixLive && prefixMatched == len(jsonDataURLPrefix)
}

var (
	jsonDataURLPrefix = []byte("data:")
	jsonBase64Marker  = []byte(";base64,")
)

// decodeJSONStringRune 解出 raw 中从 i 开始的一个码点，返回码点与下一位置。
// 处理 \uXXXX、代理对与孤立代理；孤立代理按 encoding/json 的约定退化为 U+FFFD。
func decodeJSONStringRune(raw []byte, i int) (rune, int) {
	if raw[i] != '\\' {
		if raw[i] < utf8.RuneSelf {
			return rune(raw[i]), i + 1
		}
		r, size := utf8.DecodeRune(raw[i:])
		return r, i + size
	}
	switch raw[i+1] {
	case '"':
		return '"', i + 2
	case '\\':
		return '\\', i + 2
	case '/':
		return '/', i + 2
	case 'b':
		return '\b', i + 2
	case 'f':
		return '\f', i + 2
	case 'n':
		return '\n', i + 2
	case 'r':
		return '\r', i + 2
	case 't':
		return '\t', i + 2
	default: // json.Valid 保证剩余转义只能是完整的 \uXXXX。
		r := decodeJSONHex4(raw, i+2)
		if utf16.IsSurrogate(r) {
			if i+12 <= len(raw) && raw[i+6] == '\\' && raw[i+7] == 'u' {
				if dec := utf16.DecodeRune(r, decodeJSONHex4(raw, i+8)); dec != utf8.RuneError {
					return dec, i + 12
				}
			}
			return utf8.RuneError, i + 6
		}
		return r, i + 6
	}
}

// decodeJSONHex4 解析 raw 中 i 起的 4 位十六进制。
func decodeJSONHex4(raw []byte, i int) rune {
	var r rune
	for offset := range 4 {
		c := raw[i+offset]
		switch {
		case c >= '0' && c <= '9':
			r = r<<4 | rune(c-'0')
		case c >= 'a' && c <= 'f':
			r = r<<4 | rune(c-'a'+10)
		default: // json.Valid 保证其余字符是 A-F。
			r = r<<4 | rune(c-'A'+10)
		}
	}
	return r
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

// writeRawLiteral 直接写出源字节片段（数字字面量、true/false/null），不做字符串转换。
func (w *previewWriter) writeRawLiteral(raw []byte) bool {
	if !w.room(len(raw)) {
		return false
	}
	w.buf = append(w.buf, raw...)
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

// writeValue 写出当前位置的 JSON 值；无论是否写出，都会前进到该值之后。
// 超过 previewMaxDepth 的容器整体跳过，避免深树无界增长。
func (w *previewWriter) writeValue(s *previewScanner, path string, pathValid bool, key string, depth int) bool {
	switch s.src[s.pos] {
	case '{', '[':
		if depth >= previewMaxDepth {
			s.skipValue()
			return false
		}
		if s.src[s.pos] == '{' {
			return w.writeObject(s, path, pathValid, depth)
		}
		return w.writeArray(s, path, pathValid, depth)
	case '"':
		return w.writeString(s, path, pathValid, key)
	default:
		return w.writeScalar(s)
	}
}

// writeScalar 写出 true/false/null/数字；数字保留源字面量以免精度丢失。
func (w *previewWriter) writeScalar(s *previewScanner) bool {
	start := s.pos
	s.skipScalar()
	return w.writeRawLiteral(s.src[start:s.pos])
}

// writeObject 先写小型排障字段，再按源顺序写其余字段。
// 单个条目放不下时回滚该条目并继续，绝不因此放弃后续字段。
func (w *previewWriter) writeObject(s *previewScanner, path string, pathValid bool, depth int) bool {
	if !w.openContainer('{') {
		s.skipValue()
		return false
	}
	s.pos++
	membersStart := s.pos
	written, attempted, total := 0, 0, 0
	for pass := range 2 {
		s.pos = membersStart
		first := true
		for {
			s.skipWS()
			if s.src[s.pos] == '}' {
				break
			}
			if !first {
				s.pos++ // ','
				s.skipWS()
			}
			first = false
			keyRaw := s.scanRawString()
			if pass == 0 {
				total++
			}
			s.skipWS()
			s.pos++ // ':'
			s.skipWS()
			if isPreviewDiagnosticKey(keyRaw) != (pass == 0) || attempted >= previewMaxItems {
				s.skipValue()
				continue
			}
			attempted++
			key := ""
			childPath, childValid := path, false
			if keyBytes, _ := measureJSONString(keyRaw); keyBytes <= previewMaxPathBytes {
				key = decodeJSONString(keyRaw)
				childPath, childValid = extendPreviewPointer(path, pathValid, key)
			}
			entryMark, entryOpen := len(w.buf), w.open
			entryOmissions, entryDetails := len(w.omissions), w.omittedDetails
			ok := w.writeLiteral(previewComma(written))
			if ok {
				ok = w.writeKey(keyRaw)
			}
			if ok {
				ok = w.writeLiteral(":")
			}
			if !ok {
				s.skipValue()
			} else {
				ok = w.writeValue(s, childPath, childValid, key, depth+1)
			}
			if ok {
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
	s.pos++ // '}'
	w.buf = append(w.buf, '}')
	w.open--
	if written < total {
		w.recordOmission(path, pathValid, previewKindObject, 0, 0, total, written)
	}
	return true
}

// writeArray 只保留前缀：首个放不下的条目之后全部跳过，但仍扫描计数以记录损失规模。
func (w *previewWriter) writeArray(s *previewScanner, path string, pathValid bool, depth int) bool {
	if !w.openContainer('[') {
		s.skipValue()
		return false
	}
	s.pos++
	written, attempted, total := 0, 0, 0
	broken := false
	for {
		s.skipWS()
		if s.src[s.pos] == ']' {
			break
		}
		if total > 0 {
			s.pos++ // ','
			s.skipWS()
		}
		total++
		if broken || attempted >= previewMaxItems {
			s.skipValue()
			continue
		}
		attempted++
		childPath, childValid := extendPreviewPointer(path, pathValid, strconv.Itoa(total-1))
		entryMark, entryOpen := len(w.buf), w.open
		entryOmissions, entryDetails := len(w.omissions), w.omittedDetails
		ok := w.writeLiteral(previewComma(written))
		if !ok {
			s.skipValue()
		} else {
			ok = w.writeValue(s, childPath, childValid, "", depth+1)
		}
		if ok {
			written++
			continue
		}
		w.buf = w.buf[:entryMark]
		w.open = entryOpen
		w.omissions = w.omissions[:entryOmissions]
		w.omittedDetails = entryDetails
		// 数组只能保留前缀；跳过中间项会改变后续索引，使损失路径指向错误字段。
		broken = true
	}
	s.pos++ // ']'
	w.buf = append(w.buf, ']')
	w.open--
	if written < total {
		w.recordOmission(path, pathValid, previewKindArray, 0, 0, total, written)
	}
	return true
}

// writeString 扫描字符串原始内容并在预算内重建；超长或二进制字段有界写入，不整体解码复制。
// 二进制字段只保留极短值，超出时保存空值并记录独立损失信息，避免占用普通文本的预览容量。
// 原始/保存字节数均按解码后的 UTF-8 值计算，与转义后长度无关。
func (w *previewWriter) writeString(s *previewScanner, path string, pathValid bool, key string) bool {
	raw := s.scanRawString()
	decodedLen, dataURL := measureJSONString(raw)
	binary := key == "b64_json" || dataURL
	limit := previewMaxStringBytes
	if binary {
		limit = previewMaxBinaryBytes
	}
	if !w.room(2) {
		return false
	}
	w.buf = append(w.buf, '"')
	used := 0
	truncated := false
	if binary && decodedLen > limit {
		truncated = true
	} else {
		for index := 0; index < len(raw); {
			r, next := decodeJSONStringRune(raw, index)
			size := utf8.RuneLen(r)
			if used+size > limit || !w.room(escapedRuneLen(r)+1) {
				truncated = true
				break
			}
			w.buf = appendJSONEscapedRune(w.buf, r)
			index = next
			used += size
		}
	}
	w.buf = append(w.buf, '"')
	if truncated {
		kind := previewKindString
		if binary {
			kind = previewKindBinary
		}
		w.recordOmission(path, pathValid, kind, decodedLen, used, 0, 0)
	}
	return true
}

// writeKey 写入对象键；键名同样受严格预算限制，放不下时由调用方整体丢弃该条目。
func (w *previewWriter) writeKey(raw []byte) bool {
	needed := 2
	for index := 0; index < len(raw); {
		r, next := decodeJSONStringRune(raw, index)
		needed += escapedRuneLen(r)
		index = next
	}
	if !w.room(needed) {
		return false
	}
	w.buf = append(w.buf, '"')
	for index := 0; index < len(raw); {
		r, next := decodeJSONStringRune(raw, index)
		w.buf = appendJSONEscapedRune(w.buf, r)
		index = next
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
