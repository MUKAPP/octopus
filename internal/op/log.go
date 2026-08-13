package op

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/utils/log"
	"github.com/bestruirui/octopus/internal/utils/snowflake"
	"gorm.io/gorm"
)

const relayLogLiveMaxSize = 100     // 实时展示缓存：最新完成日志 ring，无论是否持久化
const relayLogPendingFlushSize = 20 // 待落库队列达到该条数时触发一次 flush

// relayLogLiveCache 保存最新完成的日志用于实时查询；relayLogPending 保存接收时
// 历史保存开关为 true 的待持久化日志。两者都由 relayLogCacheLock 保护。
var relayLogLiveCache = make([]model.RelayLog, 0, relayLogLiveMaxSize)
var relayLogPending = make([]model.RelayLog, 0, relayLogPendingFlushSize)
var relayLogCacheLock sync.Mutex

var relayLogFlushLock sync.Mutex

var relayLogSubscribers = make(map[chan model.RelayLog]struct{})
var relayLogSubscribersLock sync.RWMutex

var relayLogStreamTokens = make(map[string]struct{})
var relayLogStreamTokensLock sync.RWMutex

func RelayLogStreamTokenCreate() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	token := hex.EncodeToString(bytes)

	relayLogStreamTokensLock.Lock()
	relayLogStreamTokens[token] = struct{}{}
	relayLogStreamTokensLock.Unlock()

	return token, nil
}

func RelayLogStreamTokenVerify(token string) bool {
	relayLogStreamTokensLock.RLock()
	_, ok := relayLogStreamTokens[token]
	relayLogStreamTokensLock.RUnlock()
	return ok
}

func RelayLogStreamTokenRevoke(token string) {
	relayLogStreamTokensLock.Lock()
	delete(relayLogStreamTokens, token)
	relayLogStreamTokensLock.Unlock()
}

func RelayLogSubscribe() chan model.RelayLog {
	ch := make(chan model.RelayLog, 10)
	relayLogSubscribersLock.Lock()
	relayLogSubscribers[ch] = struct{}{}
	relayLogSubscribersLock.Unlock()
	return ch
}

func RelayLogUnsubscribe(ch chan model.RelayLog) {
	relayLogSubscribersLock.Lock()
	delete(relayLogSubscribers, ch)
	relayLogSubscribersLock.Unlock()
	close(ch)
}

func notifySubscribers(relayLog model.RelayLog) {
	relayLogSubscribersLock.RLock()
	defer relayLogSubscribersLock.RUnlock()

	for ch := range relayLogSubscribers {
		select {
		case ch <- relayLog:
		default:
		}
	}
}

// ActiveRelayRequest 正在进行的请求
type ActiveRelayRequest struct {
	ID                int64  `json:"id"`                             // Snowflake ID，与完成后的 RelayLog.ID 一致
	Time              int64  `json:"time"`                           // 开始时间戳（秒）
	RequestModelName  string `json:"request_model_name"`             // 请求模型名称
	RequestAPIKeyName string `json:"request_api_key_name,omitempty"` // 请求使用的 API Key 名称
	ChannelName       string `json:"channel_name,omitempty"`         // 当前尝试的渠道名称
	ActualModelName   string `json:"actual_model_name,omitempty"`    // 当前尝试的实际上游模型名称
}

// RelayActiveEventType 进行中请求事件类型
type RelayActiveEventType string

const (
	RelayActiveEventStart  RelayActiveEventType = "start"
	RelayActiveEventUpdate RelayActiveEventType = "update"
	RelayActiveEventEnd    RelayActiveEventType = "end"
)

// RelayActiveEvent 进行中请求的状态变化事件
type RelayActiveEvent struct {
	Type    RelayActiveEventType `json:"type"`
	Request ActiveRelayRequest   `json:"request,omitempty"`
	ID      int64                `json:"id,omitempty"`
}

var activeRelayRequests = make(map[int64]*ActiveRelayRequest)
var activeRelayRequestsLock sync.RWMutex

var relayActiveSubscribers = make(map[chan RelayActiveEvent]struct{})
var relayActiveSubscribersLock sync.RWMutex

func RelayActiveSubscribe() chan RelayActiveEvent {
	ch := make(chan RelayActiveEvent, 10)
	relayActiveSubscribersLock.Lock()
	relayActiveSubscribers[ch] = struct{}{}
	relayActiveSubscribersLock.Unlock()
	return ch
}

func RelayActiveUnsubscribe(ch chan RelayActiveEvent) {
	relayActiveSubscribersLock.Lock()
	delete(relayActiveSubscribers, ch)
	relayActiveSubscribersLock.Unlock()
	close(ch)
}

func notifyActiveSubscribers(event RelayActiveEvent) {
	relayActiveSubscribersLock.RLock()
	defer relayActiveSubscribersLock.RUnlock()

	for ch := range relayActiveSubscribers {
		select {
		case ch <- event:
		default:
		}
	}
}

// RelayActiveAdd 记录一个正在进行的请求并推送 start 事件
func RelayActiveAdd(request ActiveRelayRequest) {
	activeRelayRequestsLock.Lock()
	activeRelayRequests[request.ID] = &request
	activeRelayRequestsLock.Unlock()
	notifyActiveSubscribers(RelayActiveEvent{Type: RelayActiveEventStart, Request: request})
}

// RelayActiveUpdate 更新进行中请求的当前渠道与实际上游模型并推送 update 事件
func RelayActiveUpdate(id int64, channelName, actualModelName string) {
	activeRelayRequestsLock.Lock()
	request, ok := activeRelayRequests[id]
	if !ok {
		activeRelayRequestsLock.Unlock()
		return
	}
	request.ChannelName = channelName
	request.ActualModelName = actualModelName
	requestCopy := *request
	activeRelayRequestsLock.Unlock()
	notifyActiveSubscribers(RelayActiveEvent{Type: RelayActiveEventUpdate, Request: requestCopy})
}

// RelayActiveRemove 移除一个已结束的请求并推送 end 事件
func RelayActiveRemove(id int64) {
	activeRelayRequestsLock.Lock()
	_, ok := activeRelayRequests[id]
	delete(activeRelayRequests, id)
	activeRelayRequestsLock.Unlock()
	if ok {
		notifyActiveSubscribers(RelayActiveEvent{Type: RelayActiveEventEnd, ID: id})
	}
}

// RelayActiveList 返回当前所有进行中的请求
func RelayActiveList() []ActiveRelayRequest {
	activeRelayRequestsLock.RLock()
	defer activeRelayRequestsLock.RUnlock()

	result := make([]ActiveRelayRequest, 0, len(activeRelayRequests))
	for _, request := range activeRelayRequests {
		result = append(result, *request)
	}
	return result
}

func relayLogFlushToDB(ctx context.Context) error {
	relayLogFlushLock.Lock()
	defer relayLogFlushLock.Unlock()

	relayLogCacheLock.Lock()
	if len(relayLogPending) == 0 {
		relayLogCacheLock.Unlock()
		return nil
	}
	batch := make([]model.RelayLog, len(relayLogPending))
	copy(batch, relayLogPending)
	pendingIDs := make(map[int64]struct{}, len(batch))
	for _, l := range batch {
		pendingIDs[l.ID] = struct{}{}
	}
	relayLogCacheLock.Unlock()

	// 无论当前开关值如何，都排空已接受的 pending；失败时原批次原序保留以供下次重试。
	result := db.GetDB().WithContext(ctx).Create(&batch)
	if result.Error != nil {
		return result.Error
	}

	relayLogCacheLock.Lock()
	// 按快照中的稳定 ID 删除对应 pending，不能按过期切片长度裁掉并发追加项。
	kept := relayLogPending[:0]
	for _, l := range relayLogPending {
		if _, ok := pendingIDs[l.ID]; ok {
			continue
		}
		kept = append(kept, l)
	}
	if len(kept) == 0 {
		relayLogPending = make([]model.RelayLog, 0, relayLogPendingFlushSize)
	} else {
		relayLogPending = kept
	}
	relayLogCacheLock.Unlock()

	return nil
}

func RelayLogAdd(ctx context.Context, relayLog model.RelayLog) error {
	// 只读取一次开关并固定本次决策：始终写入 live ring，仅当时启用时追加 pending。
	enabled, err := SettingGetBool(model.SettingKeyRelayLogKeepEnabled)
	if err != nil {
		return err
	}
	if relayLog.ID == 0 {
		relayLog.ID = snowflake.GenerateID()
	}
	go notifySubscribers(relayLog)

	relayLogCacheLock.Lock()
	relayLogLiveCache = append(relayLogLiveCache, relayLog)
	if len(relayLogLiveCache) > relayLogLiveMaxSize {
		// 重建底层数组而不是 reslice，避免数组持续引用旧日志的 Request/ResponseContent 导致内存无法回收
		keepSize := relayLogLiveMaxSize / 2
		newCache := make([]model.RelayLog, keepSize, relayLogLiveMaxSize)
		copy(newCache, relayLogLiveCache[len(relayLogLiveCache)-keepSize:])
		relayLogLiveCache = newCache
	}
	if !enabled {
		relayLogCacheLock.Unlock()
		return nil
	}
	relayLogPending = append(relayLogPending, relayLog)
	if len(relayLogPending) < relayLogPendingFlushSize {
		relayLogCacheLock.Unlock()
		return nil
	}
	// 触发 flush 前释放缓存锁，避免与 flush 的锁序（flush 锁后缓存锁）冲突。
	relayLogCacheLock.Unlock()
	return relayLogFlushToDB(ctx)
}

func RelayLogSaveDBTask(ctx context.Context) error {
	log.Debugf("relay log save db task started")
	startTime := time.Now()
	defer func() {
		log.Debugf("relay log save db task finished, save time: %s", time.Since(startTime))
	}()

	// 先排空已接受的 pending，无论当前开关值如何。
	if err := relayLogFlushToDB(ctx); err != nil {
		return err
	}

	enabled, err := SettingGetBool(model.SettingKeyRelayLogKeepEnabled)
	if err != nil {
		return err
	}
	return relayLogCleanup(ctx, enabled)
}

func relayLogCleanup(ctx context.Context, enabled bool) error {
	relayLogFlushLock.Lock()
	defer relayLogFlushLock.Unlock()

	// 在同一物理连接上完成删除并提交后执行有界增量回收。
	return db.GetDB().WithContext(ctx).Connection(func(tx *gorm.DB) error {
		if enabled {
			keepPeriod, err := SettingGetInt(model.SettingKeyRelayLogKeepPeriod)
			if err != nil {
				return err
			}
			if keepPeriod <= 0 {
				return nil
			}
			cutoffTime := time.Now().Add(-time.Duration(keepPeriod) * 24 * time.Hour).Unix()
			if err := tx.Where("time < ?", cutoffTime).Delete(&model.RelayLog{}).Error; err != nil {
				return err
			}
			return db.ReclaimSQLiteFreePages(tx)
		}
		// 保存关闭时删除全部已持久化日志，而不是保留关闭前的旧行。
		if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&model.RelayLog{}).Error; err != nil {
			return err
		}
		return db.ReclaimSQLiteFreePages(tx)
	})
}

// RelayLogList 查询日志列表，支持可选的时间范围过滤
// startTime 和 endTime 为 nil 时表示不限制时间范围
func RelayLogList(ctx context.Context, startTime, endTime *int, page, pageSize int) ([]model.RelayLog, error) {
	enabled, err := SettingGetBool(model.SettingKeyRelayLogKeepEnabled)
	if err != nil {
		return nil, err
	}
	hasTimeFilter := startTime != nil && endTime != nil

	// 获取 live ring 中符合条件的日志
	relayLogCacheLock.Lock()
	var cachedLogs []model.RelayLog
	for _, log := range relayLogLiveCache {
		if hasTimeFilter {
			if log.Time >= int64(*startTime) && log.Time <= int64(*endTime) {
				cachedLogs = append(cachedLogs, log)
			}
		} else {
			cachedLogs = append(cachedLogs, log)
		}
	}
	// 已落库但仍在 live ring 中的日志需要从数据库查询中排除，避免重复
	liveIDs := make(map[int64]struct{}, len(relayLogLiveCache))
	for _, log := range relayLogLiveCache {
		liveIDs[log.ID] = struct{}{}
	}
	liveIDList := make([]int64, 0, len(liveIDs))
	for id := range liveIDs {
		liveIDList = append(liveIDList, id)
	}
	relayLogCacheLock.Unlock()

	// 反转缓存日志顺序（原本新的在末尾，反转后新的在前面，方便分页）
	for i, j := 0, len(cachedLogs)-1; i < j; i, j = i+1, j-1 {
		cachedLogs[i], cachedLogs[j] = cachedLogs[j], cachedLogs[i]
	}

	cacheCount := len(cachedLogs)
	offset := (page - 1) * pageSize

	var result []model.RelayLog

	// 先从缓存中取（缓存是最新的日志）
	if offset < cacheCount {
		cacheEnd := offset + pageSize
		if cacheEnd > cacheCount {
			cacheEnd = cacheCount
		}
		result = append(result, cachedLogs[offset:cacheEnd]...)
	}

	// 如果启用了日志保存，缓存不够时从数据库补充
	if enabled {
		remaining := pageSize - len(result)
		if remaining > 0 {
			dbOffset := 0
			if offset > cacheCount {
				dbOffset = offset - cacheCount
			}

			query := db.GetDB().WithContext(ctx)
			if hasTimeFilter {
				query = query.Where("time >= ? AND time <= ?", *startTime, *endTime)
			}
			if len(liveIDList) > 0 {
				query = query.Where("id NOT IN ?", liveIDList)
			}

			var dbLogs []model.RelayLog
			if err := query.Order("id DESC").Offset(dbOffset).Limit(remaining).Find(&dbLogs).Error; err != nil {
				return nil, err
			}
			result = append(result, dbLogs...)
		}
	}

	return result, nil
}

func RelayLogClear(ctx context.Context) error {
	relayLogFlushLock.Lock()
	defer relayLogFlushLock.Unlock()

	// 持缓存锁跨过整个数据库删除：期间到达的日志要么完整位于 clear 之前被删除，
	// 要么完整位于 clear 之后按当时开关重新入队，不会留下半清状态；
	// 删除失败时不触碰内存队列，留待重试。
	relayLogCacheLock.Lock()
	defer relayLogCacheLock.Unlock()

	err := db.GetDB().WithContext(ctx).Connection(func(tx *gorm.DB) error {
		if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&model.RelayLog{}).Error; err != nil {
			return err
		}
		return db.ReclaimSQLiteFreePages(tx)
	})
	if err != nil {
		return err
	}

	relayLogLiveCache = make([]model.RelayLog, 0, relayLogLiveMaxSize)
	relayLogPending = make([]model.RelayLog, 0, relayLogPendingFlushSize)
	return nil
}
