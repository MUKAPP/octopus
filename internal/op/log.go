package op

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/bestruirui/octopus/internal/model"
)

var relayLogSubscribers = make(map[chan model.RelayLog]struct{})
var relayLogSubscribersLock sync.RWMutex

var relayLogStreamTokens = make(map[string]time.Time)
var relayLogStreamTokensLock sync.RWMutex

const relayLogStreamTokenTTL = 1 * time.Minute

func RelayLogStreamTokenCreate() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	token := hex.EncodeToString(bytes)
	expiresAt := time.Now().Add(relayLogStreamTokenTTL)

	relayLogStreamTokensLock.Lock()
	now := time.Now()
	for existing, expires := range relayLogStreamTokens {
		if !expires.After(now) {
			delete(relayLogStreamTokens, existing)
		}
	}
	relayLogStreamTokens[token] = expiresAt
	relayLogStreamTokensLock.Unlock()

	return token, nil
}

func RelayLogStreamTokenVerify(token string) bool {
	relayLogStreamTokensLock.Lock()
	expiresAt, ok := relayLogStreamTokens[token]
	if ok && !expiresAt.After(time.Now()) {
		delete(relayLogStreamTokens, token)
		ok = false
	}
	relayLogStreamTokensLock.Unlock()
	return ok
}

// RelayLogStreamTokenConsume verifies and revokes a stream token atomically.
func RelayLogStreamTokenConsume(token string) bool {
	if token == "" {
		return false
	}
	relayLogStreamTokensLock.Lock()
	expiresAt, ok := relayLogStreamTokens[token]
	if ok {
		delete(relayLogStreamTokens, token)
		ok = expiresAt.After(time.Now())
	}
	relayLogStreamTokensLock.Unlock()
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
