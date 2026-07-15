package relay

import (
	"fmt"
	"strings"

	"github.com/bestruirui/octopus/internal/utils/log"
)

// mergeParamAppend 将 appendParams 合并进 body：
// - 双方同 key 且均为数组：拼接；tools 按工具名去重，冲突时保留 body（客户端）侧
// - body 无该 key：直接写入
// - 非数组：仅当 body 不存在该 key 时写入
// - 类型冲突：跳过并打 warn
func mergeParamAppend(body map[string]any, appendParams map[string]any) {
	if body == nil || appendParams == nil {
		return
	}
	for key, appendValue := range appendParams {
		existingValue, exists := body[key]
		if !exists {
			body[key] = appendValue
			continue
		}

		existingSlice, existingIsSlice := existingValue.([]any)
		appendSlice, appendIsSlice := appendValue.([]any)
		if existingIsSlice && appendIsSlice {
			if key == "tools" {
				body[key] = appendToolsDedup(existingSlice, appendSlice)
			} else {
				body[key] = append(existingSlice, appendSlice...)
			}
			continue
		}

		// 非数组：不覆盖已有值，避免与 override 职责重叠
		if !existingIsSlice && !appendIsSlice {
			// body 已有该 key，跳过
			continue
		}

		log.Warnf("param_append type conflict for key %q: body=%T append=%T, skipping", key, existingValue, appendValue)
	}
}

// appendToolsDedup 将 appendTools 拼到 existingTools 后；工具名冲突时保留客户端（existing）项，跳过追加项。
// 命名规则：type 为 function/custom 时用 function.name；否则用 type（如 web_search、x_search）。
func appendToolsDedup(existingTools []any, appendTools []any) []any {
	seenNames := make(map[string]struct{}, len(existingTools)+len(appendTools))
	for _, tool := range existingTools {
		if name := toolIdentityName(tool); name != "" {
			seenNames[name] = struct{}{}
		}
	}

	result := make([]any, 0, len(existingTools)+len(appendTools))
	result = append(result, existingTools...)

	for _, tool := range appendTools {
		name := toolIdentityName(tool)
		if name != "" {
			if _, exists := seenNames[name]; exists {
				log.Warnf("param_append skip duplicate tool %q (keep client tool)", name)
				continue
			}
			seenNames[name] = struct{}{}
		}
		result = append(result, tool)
	}
	return result
}

// toolIdentityName 提取上游用于判重的工具名。
// - function / custom：function.name
// - 内置 tool（web_search、x_search 等）：type
func toolIdentityName(tool any) string {
	toolMap, ok := tool.(map[string]any)
	if !ok || toolMap == nil {
		return ""
	}

	toolType := strings.TrimSpace(fmt.Sprint(toolMap["type"]))
	if toolType == "<nil>" {
		toolType = ""
	}
	toolType = strings.ToLower(toolType)

	// OpenAI function / custom tools：以 function.name 为准
	if toolType == "function" || toolType == "custom" || toolType == "" {
		if functionValue, hasFunction := toolMap["function"]; hasFunction {
			if functionMap, isMap := functionValue.(map[string]any); isMap {
				if nameValue, hasName := functionMap["name"]; hasName {
					name := strings.TrimSpace(fmt.Sprint(nameValue))
					if name != "" && name != "<nil>" {
						return name
					}
				}
			}
		}
		// type 为空且无 function.name 时无法可靠判重
		if toolType == "" {
			return ""
		}
	}

	// 内置 tool：type 即名称（web_search、x_search 等）
	return toolType
}
