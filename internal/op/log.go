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

const relayLogMaxSize = 20
const relayLogMaxSizeNoDB = 100 // 当不保存到数据库时，允许更大的缓存用于实时查询

var relayLogCache = make([]model.RelayLog, 0, relayLogMaxSize)
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
	if len(relayLogCache) == 0 {
		relayLogCacheLock.Unlock()
		return nil
	}
	batch := make([]model.RelayLog, len(relayLogCache))
	copy(batch, relayLogCache)
	flushedUpto := len(batch)
	relayLogCacheLock.Unlock()

	result := db.GetDB().WithContext(ctx).Create(&batch)
	if result.Error != nil {
		return result.Error
	}

	relayLogCacheLock.Lock()
	if len(relayLogCache) >= flushedUpto {
		relayLogCache = relayLogCache[flushedUpto:]
	} else {
		relayLogCache = relayLogCache[:0]
	}
	if len(relayLogCache) == 0 {
		relayLogCache = make([]model.RelayLog, 0, relayLogMaxSize)
	}
	relayLogCacheLock.Unlock()

	return nil
}

func RelayLogAdd(ctx context.Context, relayLog model.RelayLog) error {
	enabled, err := SettingGetBool(model.SettingKeyRelayLogKeepEnabled)
	if err != nil {
		return err
	}
	maxSize := relayLogMaxSize
	if !enabled {
		maxSize = relayLogMaxSizeNoDB
	}
	if relayLog.ID == 0 {
		relayLog.ID = snowflake.GenerateID()
	}
	go notifySubscribers(relayLog)

	relayLogCacheLock.Lock()
	relayLogCache = append(relayLogCache, relayLog)
	if len(relayLogCache) >= maxSize {
		if enabled {
			relayLogCacheLock.Unlock()
			return relayLogFlushToDB(ctx)
		}
		// 如果未启用日志保存，移除最旧的日志，保留最新的日志用于实时查询
		// 重建底层数组而不是 reslice，避免数组持续引用旧日志的 Request/ResponseContent 导致内存无法回收
		keepSize := maxSize / 2
		if len(relayLogCache) > keepSize {
			newCache := make([]model.RelayLog, keepSize, maxSize)
			copy(newCache, relayLogCache[len(relayLogCache)-keepSize:])
			relayLogCache = newCache
		}
	}
	relayLogCacheLock.Unlock()
	return nil
}

func RelayLogSaveDBTask(ctx context.Context) error {
	log.Debugf("relay log save db task started")
	startTime := time.Now()
	defer func() {
		log.Debugf("relay log save db task finished, save time: %s", time.Since(startTime))
	}()
	enabled, err := SettingGetBool(model.SettingKeyRelayLogKeepEnabled)
	if err != nil {
		return err
	}

	if enabled {
		if err := relayLogFlushToDB(ctx); err != nil {
			return err
		}
		return relayLogCleanup(ctx)
	}

	// 如果未启用日志保存，检查缓存大小，如果超过限制则清理旧日志
	relayLogCacheLock.Lock()
	if len(relayLogCache) > relayLogMaxSizeNoDB {
		keepSize := relayLogMaxSizeNoDB / 2
		newCache := make([]model.RelayLog, keepSize, relayLogMaxSizeNoDB)
		copy(newCache, relayLogCache[len(relayLogCache)-keepSize:])
		relayLogCache = newCache
	}
	relayLogCacheLock.Unlock()

	return nil
}

func relayLogCleanup(ctx context.Context) error {
	keepPeriod, err := SettingGetInt(model.SettingKeyRelayLogKeepPeriod)
	if err != nil {
		return err
	}

	if keepPeriod <= 0 {
		return nil
	}

	cutoffTime := time.Now().Add(-time.Duration(keepPeriod) * 24 * time.Hour).Unix()
	relayLogFlushLock.Lock()
	defer relayLogFlushLock.Unlock()
	return db.GetDB().WithContext(ctx).Where("time < ?", cutoffTime).Delete(&model.RelayLog{}).Error
}

// RelayLogList 查询日志列表，支持可选的时间范围过滤
// startTime 和 endTime 为 nil 时表示不限制时间范围
func RelayLogList(ctx context.Context, startTime, endTime *int, page, pageSize int) ([]model.RelayLog, error) {
	enabled, err := SettingGetBool(model.SettingKeyRelayLogKeepEnabled)
	if err != nil {
		return nil, err
	}
	hasTimeFilter := startTime != nil && endTime != nil

	// 获取缓存中符合条件的日志
	relayLogCacheLock.Lock()
	var cachedLogs []model.RelayLog
	for _, log := range relayLogCache {
		if hasTimeFilter {
			if log.Time >= int64(*startTime) && log.Time <= int64(*endTime) {
				cachedLogs = append(cachedLogs, log)
			}
		} else {
			cachedLogs = append(cachedLogs, log)
		}
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

	if err := db.GetDB().WithContext(ctx).Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&model.RelayLog{}).Error; err != nil {
		return err
	}

	relayLogCacheLock.Lock()
	relayLogCache = make([]model.RelayLog, 0, relayLogMaxSize)
	relayLogCacheLock.Unlock()
	return nil
}
