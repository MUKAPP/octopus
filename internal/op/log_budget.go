package op

import (
	"runtime/debug"
	"sync"
	"time"

	"github.com/bestruirui/octopus/internal/utils/log"
	"github.com/bestruirui/octopus/internal/utils/memory"
)

const (
	relayLogBodyBudgetFallback = 64 << 20
	relayLogBodyBudgetMaximum  = 512 << 20
)

var relayLogMemoryProbe = struct {
	sync.Mutex
	checkedAt time.Time
	status    memory.Status
	warned    bool
}{}

// relayLogMemoryStatus 在 store 锁外采样，一秒内复用结果，避免每次请求读取系统文件。
func relayLogMemoryStatus() memory.Status {
	relayLogMemoryProbe.Lock()
	defer relayLogMemoryProbe.Unlock()
	if !relayLogMemoryProbe.checkedAt.IsZero() && time.Since(relayLogMemoryProbe.checkedAt) < time.Second {
		return relayLogMemoryProbe.status
	}
	status, err := memory.Read()
	// 只查询当前软限额，不修改 Go GC 配置；显式小进程预算不能被宿主容量放大。
	if limit := debug.SetMemoryLimit(-1); limit > 0 && status.Total > uint64(limit) {
		status.Total = uint64(limit)
		status.Available = min(status.Available, status.Total)
	}
	if err != nil && !relayLogMemoryProbe.warned {
		log.Warnf("relay log memory detection failed: %v", err)
	}
	relayLogMemoryProbe.warned = err != nil
	relayLogMemoryProbe.checkedAt = time.Now()
	// 有限容器余量无法读取时，探测器返回 Available=0 的部分结果，不能改用宿主余量。
	relayLogMemoryProbe.status = status
	return status
}

// relayLogCompletedBodyBudget 限制日志占用总内存的比例，并给系统和转发请求留足余量。
// 只按实际 Available 计算压力预算，不把逻辑缓存大小加回物理余量，避免淘汰后
// RSS 尚未回落时下一轮再次无意义缩减。它不是进程 RSS 上限，也不会强制 GC。
func relayLogCompletedBodyBudget(status memory.Status) int64 {
	if status.Total == 0 {
		return relayLogBodyBudgetFallback
	}
	available := min(status.Available, status.Total)
	reserve := max(status.Total/10, uint64(64<<20))
	if available <= reserve {
		return 0
	}
	return int64(min(status.Total/16, uint64(relayLogBodyBudgetMaximum), (available-reserve)/4))
}

// RelayLogMemoryTrimTask 在没有新请求完成时也响应可用内存下降，不中断活动请求。
func RelayLogMemoryTrimTask() {
	relayLogStore.RLock()
	hasCompleted := len(relayLogStore.completedIDs) > 1
	relayLogStore.RUnlock()
	if !hasCompleted {
		return
	}
	status := relayLogMemoryStatus()
	relayLogStore.Lock()
	retired := trimCompletedRelayLogsLocked(status)
	relayLogStore.Unlock()
	retireRelayLogStoredImages(retired)
}
