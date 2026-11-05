// Package memory 读取当前进程可使用的物理内存。
package memory

// Status 的单位为字节；Available 不大于 Total。
type Status struct {
	Total     uint64
	Available uint64
}
