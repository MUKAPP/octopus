//go:build !linux && !windows

package memory

import (
	"fmt"
	"runtime"
)

// Read 在未实现的平台返回错误，由调用方选择保守预算。
func Read() (Status, error) {
	return Status{}, fmt.Errorf("memory status is unsupported on %s", runtime.GOOS)
}
