//go:build windows

package memory

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var globalMemoryStatusEx = windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")

// Read 每次查询当前物理内存，不缓存总量或可用量。
func Read() (Status, error) {
	var info struct {
		Length               uint32
		MemoryLoad           uint32
		TotalPhys            uint64
		AvailPhys            uint64
		TotalPageFile        uint64
		AvailPageFile        uint64
		TotalVirtual         uint64
		AvailVirtual         uint64
		AvailExtendedVirtual uint64
	}
	info.Length = uint32(unsafe.Sizeof(info))
	result, _, err := globalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&info)))
	if result == 0 {
		return Status{}, fmt.Errorf("GlobalMemoryStatusEx failed: %w", err)
	}
	if info.TotalPhys == 0 {
		return Status{}, fmt.Errorf("GlobalMemoryStatusEx returned zero physical memory")
	}
	return Status{Total: info.TotalPhys, Available: min(info.AvailPhys, info.TotalPhys)}, nil
}
