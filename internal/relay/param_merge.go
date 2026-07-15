package relay

import (
	"github.com/bestruirui/octopus/internal/utils/log"
)

// mergeParamAppend 将 appendParams 合并进 body：
// - 双方同 key 且均为数组：拼接
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
			body[key] = append(existingSlice, appendSlice...)
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
