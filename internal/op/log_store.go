package op

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/bestruirui/octopus/internal/model"
)

const relayLogStoreMaxCompleted = 100

// RelayLogState describes the in-memory lifecycle of one relay request.
type RelayLogState string

const (
	RelayLogStateRunning   RelayLogState = "running"
	RelayLogStateCommitted RelayLogState = "committed"
	RelayLogStateSuccess   RelayLogState = "success"
	RelayLogStateFailed    RelayLogState = "failed"
	RelayLogStateCanceled  RelayLogState = "canceled"
)

// RelayLogAttempt is the stream/detail representation of a channel decision.
// Duration is kept in milliseconds for compatibility with model.ChannelAttempt.
type RelayLogAttempt struct {
	AttemptIndex   int                 `json:"attempt_index"`
	ChannelID      int                 `json:"channel_id,omitempty"`
	ChannelKeyID   int                 `json:"channel_key_id,omitempty"`
	ChannelName    string              `json:"channel_name"`
	ModelName      string              `json:"model_name"`
	Error          string              `json:"error,omitempty"`
	Status         model.AttemptStatus `json:"status"`
	Duration       int                 `json:"duration"`
	RateMultiplier float64             `json:"rate_multiplier"`
	Sticky         bool                `json:"sticky,omitempty"`
	AttemptNum     int                 `json:"attempt_num,omitempty"`
	Msg            string              `json:"msg,omitempty"`
}

// RelayLogOverview combines the current RelayLog JSON contract with the
// fields needed by the live log UI. The embedded RelayLog intentionally keeps
// all legacy field names (cached_tokens, rate_multiplier, attempts, and body
// fields) available to old clients.
type RelayLogOverview struct {
	model.RelayLog

	State       RelayLogState `json:"state"`
	StartedAt   time.Time     `json:"started_at"`
	CompletedAt *time.Time    `json:"completed_at,omitempty"`
	Duration    time.Duration `json:"duration"`

	RequestModel string `json:"request_model"`
	ActualModel  string `json:"actual_model"`
	APIKeyName   string `json:"api_key_name,omitempty"`

	ClientProtocol string `json:"client_protocol"`
	Stream         bool   `json:"stream"`

	FinalChannelName    string  `json:"final_channel_name"`
	FinalRateMultiplier float64 `json:"final_rate_multiplier"`

	CacheReadTokens  int64   `json:"cache_read_tokens"`
	CacheWriteTokens int64   `json:"cache_write_tokens"`
	TotalCost        float64 `json:"total_cost"`

	// Body aliases make the on-demand body contract usable without removing
	// request_content/response_content consumed by the current frontend.
	RequestBody  string `json:"request_body,omitempty"`
	ResponseBody string `json:"response_body,omitempty"`

	ResponseCommitted   bool              `json:"response_committed"`
	History             []RelayLogAttempt `json:"history"`
	CurrentAttemptIndex int               `json:"current_attempt_index"`
}

// MarshalJSON keeps request/response bodies on the authenticated on-demand
// endpoints instead of copying them into SSE payloads and access logs.
func (overview RelayLogOverview) MarshalJSON() ([]byte, error) {
	type relayLogOverview RelayLogOverview
	sanitized := relayLogOverview(cloneRelayLogOverview(overview))
	sanitized.RelayLog.RequestContent = ""
	sanitized.RelayLog.ResponseContent = ""
	sanitized.RequestBody = ""
	sanitized.ResponseBody = ""
	return json.Marshal(sanitized)
}

// RelayLogDetailEvent is the payload sent to a per-request SSE stream.
type RelayLogDetailEvent struct {
	Type     string
	ID       int64
	Attempt  *RelayLogAttempt
	Overview *RelayLogOverview
}

const (
	RelayLogEventAttemptStarted    = "attempt.started"
	RelayLogEventAttemptFinished   = "attempt.finished"
	RelayLogEventResponseCommitted = "response.committed"
	RelayLogEventOverview          = "log"
)

type relayLogStoreRecord struct {
	overview    RelayLogOverview
	cancel      context.CancelFunc
	cancelIndex int
	committed   bool
}

var relayLogStore = struct {
	sync.RWMutex
	records      map[int64]*relayLogStoreRecord
	completedIDs []int64
	overviewSubs map[chan RelayLogOverview]struct{}
	detailSubs   map[int64]map[chan RelayLogDetailEvent]struct{}
}{
	records:      make(map[int64]*relayLogStoreRecord),
	overviewSubs: make(map[chan RelayLogOverview]struct{}),
	detailSubs:   make(map[int64]map[chan RelayLogDetailEvent]struct{}),
}

func cloneChannelAttempts(attempts []model.ChannelAttempt) []model.ChannelAttempt {
	if len(attempts) == 0 {
		return nil
	}
	return append([]model.ChannelAttempt(nil), attempts...)
}

func cloneRelayLog(log model.RelayLog) model.RelayLog {
	log.Attempts = cloneChannelAttempts(log.Attempts)
	if log.CachedTokens != nil {
		cached := *log.CachedTokens
		log.CachedTokens = &cached
	}
	return log
}

func cloneRelayLogAttempt(attempt RelayLogAttempt) RelayLogAttempt {
	return attempt
}

func relayLogAttemptFromModel(index int, attempt model.ChannelAttempt) RelayLogAttempt {
	return RelayLogAttempt{
		AttemptIndex:   index,
		ChannelID:      attempt.ChannelID,
		ChannelKeyID:   attempt.ChannelKeyID,
		ChannelName:    attempt.ChannelName,
		ModelName:      attempt.ModelName,
		Error:          attempt.Msg,
		Status:         attempt.Status,
		Duration:       attempt.Duration,
		RateMultiplier: attempt.RateMultiplier,
		Sticky:         attempt.Sticky,
		AttemptNum:     attempt.AttemptNum,
		Msg:            attempt.Msg,
	}
}

func modelAttemptFromRelay(attempt RelayLogAttempt) model.ChannelAttempt {
	return model.ChannelAttempt{
		ChannelID:      attempt.ChannelID,
		ChannelKeyID:   attempt.ChannelKeyID,
		ChannelName:    attempt.ChannelName,
		ModelName:      attempt.ModelName,
		RateMultiplier: attempt.RateMultiplier,
		AttemptNum:     attempt.AttemptNum,
		Status:         attempt.Status,
		Duration:       attempt.Duration,
		Sticky:         attempt.Sticky,
		Msg:            attempt.Msg,
	}
}

func cloneRelayLogOverview(overview RelayLogOverview) RelayLogOverview {
	overview.RelayLog = cloneRelayLog(overview.RelayLog)
	if overview.CompletedAt != nil {
		completed := *overview.CompletedAt
		overview.CompletedAt = &completed
	}
	if len(overview.History) > 0 {
		overview.History = append([]RelayLogAttempt(nil), overview.History...)
	}
	return overview
}

func relayLogOverviewFromModel(log model.RelayLog, state RelayLogState, startedAt time.Time, clientProtocol string, stream bool) RelayLogOverview {
	log = cloneRelayLog(log)
	if startedAt.IsZero() {
		if log.Time > 0 {
			startedAt = time.Unix(log.Time, 0)
		} else {
			startedAt = time.Now()
		}
	}
	overview := RelayLogOverview{
		RelayLog:            log,
		State:               state,
		StartedAt:           startedAt,
		Duration:            0,
		RequestModel:        log.RequestModelName,
		ActualModel:         log.ActualModelName,
		APIKeyName:          log.RequestAPIKeyName,
		ClientProtocol:      clientProtocol,
		Stream:              stream,
		FinalChannelName:    log.ChannelName,
		FinalRateMultiplier: log.RateMultiplier,
		TotalCost:           log.Cost,
		RequestBody:         log.RequestContent,
		ResponseBody:        log.ResponseContent,
		CurrentAttemptIndex: -1,
	}
	if log.CachedTokens != nil {
		overview.CacheReadTokens = int64(*log.CachedTokens)
	}
	for index, attempt := range log.Attempts {
		overview.History = append(overview.History, relayLogAttemptFromModel(index, attempt))
	}
	return overview
}

func relayLogStoreSnapshotLocked(record *relayLogStoreRecord) RelayLogOverview {
	snapshot := cloneRelayLogOverview(record.overview)
	if snapshot.State == RelayLogStateRunning || snapshot.State == RelayLogStateCommitted {
		snapshot.Duration = time.Since(snapshot.StartedAt)
		if snapshot.Duration < 0 {
			snapshot.Duration = 0
		}
	}
	return snapshot
}

func sendLatestRelayLog[T any](ch chan T, value T) {
	select {
	case ch <- value:
		return
	default:
	}
	select {
	case <-ch:
	default:
	}
	select {
	case ch <- value:
	default:
	}
}

func notifyRelayLogOverview(snapshot RelayLogOverview) {
	relayLogStore.RLock()
	defer relayLogStore.RUnlock()
	for ch := range relayLogStore.overviewSubs {
		sendLatestRelayLog(ch, cloneRelayLogOverview(snapshot))
	}
}

func notifyRelayLogDetail(id int64, event RelayLogDetailEvent) {
	relayLogStore.RLock()
	defer relayLogStore.RUnlock()
	for ch := range relayLogStore.detailSubs[id] {
		eventCopy := event
		if event.Attempt != nil {
			attempt := cloneRelayLogAttempt(*event.Attempt)
			eventCopy.Attempt = &attempt
		}
		if event.Overview != nil {
			overview := cloneRelayLogOverview(*event.Overview)
			eventCopy.Overview = &overview
		}
		sendLatestRelayLog(ch, eventCopy)
	}
}

// RelayLogStoreStart starts a request in memory and publishes its running snapshot.
func RelayLogStoreStart(log model.RelayLog, startedAt time.Time, clientProtocol string, stream bool) {
	if log.ID == 0 {
		return
	}
	overview := relayLogOverviewFromModel(log, RelayLogStateRunning, startedAt, clientProtocol, stream)
	record := &relayLogStoreRecord{overview: overview, cancelIndex: -1}
	relayLogStore.Lock()
	relayLogStore.records[log.ID] = record
	relayLogStore.Unlock()
	notifyRelayLogOverview(overview)
}

// RelayLogStoreGet returns a defensive snapshot of a request.
func RelayLogStoreGet(id int64) (RelayLogOverview, bool) {
	relayLogStore.RLock()
	record, ok := relayLogStore.records[id]
	if !ok {
		relayLogStore.RUnlock()
		return RelayLogOverview{}, false
	}
	snapshot := relayLogStoreSnapshotLocked(record)
	relayLogStore.RUnlock()
	return snapshot, true
}

// RelayLogStoreList returns newest request snapshots with optional Unix-second filters.
func RelayLogStoreList(startTime, endTime *int, page, pageSize int) []RelayLogOverview {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	relayLogStore.RLock()
	snapshots := make([]RelayLogOverview, 0, len(relayLogStore.records))
	for _, record := range relayLogStore.records {
		snapshot := relayLogStoreSnapshotLocked(record)
		if startTime != nil && snapshot.StartedAt.Unix() < int64(*startTime) {
			continue
		}
		if endTime != nil && snapshot.StartedAt.Unix() > int64(*endTime) {
			continue
		}
		snapshots = append(snapshots, snapshot)
	}
	relayLogStore.RUnlock()
	sort.SliceStable(snapshots, func(i, j int) bool {
		if snapshots[i].StartedAt.Equal(snapshots[j].StartedAt) {
			return snapshots[i].ID > snapshots[j].ID
		}
		return snapshots[i].StartedAt.After(snapshots[j].StartedAt)
	})
	offset := (page - 1) * pageSize
	if offset >= len(snapshots) {
		return []RelayLogOverview{}
	}
	end := offset + pageSize
	if end > len(snapshots) {
		end = len(snapshots)
	}
	return snapshots[offset:end]
}

// RelayLogStoreSubscribeOverview subscribes to snapshots for the overview stream.
func RelayLogStoreSubscribeOverview() chan RelayLogOverview {
	ch := make(chan RelayLogOverview, 32)
	relayLogStore.Lock()
	relayLogStore.overviewSubs[ch] = struct{}{}
	relayLogStore.Unlock()
	return ch
}

func RelayLogStoreUnsubscribeOverview(ch chan RelayLogOverview) {
	relayLogStore.Lock()
	delete(relayLogStore.overviewSubs, ch)
	relayLogStore.Unlock()
	close(ch)
}

// RelayLogStoreSubscribeDetail subscribes to one request's attempt stream.
func RelayLogStoreSubscribeDetail(id int64) (chan RelayLogDetailEvent, bool) {
	relayLogStore.Lock()
	if _, ok := relayLogStore.records[id]; !ok {
		relayLogStore.Unlock()
		return nil, false
	}
	ch := make(chan RelayLogDetailEvent, 32)
	if relayLogStore.detailSubs[id] == nil {
		relayLogStore.detailSubs[id] = make(map[chan RelayLogDetailEvent]struct{})
	}
	relayLogStore.detailSubs[id][ch] = struct{}{}
	relayLogStore.Unlock()
	return ch, true
}

func RelayLogStoreUnsubscribeDetail(id int64, ch chan RelayLogDetailEvent) {
	relayLogStore.Lock()
	if subscribers := relayLogStore.detailSubs[id]; subscribers != nil {
		delete(subscribers, ch)
		if len(subscribers) == 0 {
			delete(relayLogStore.detailSubs, id)
		}
	}
	relayLogStore.Unlock()
	close(ch)
}

func RelayLogStoreAttemptStarted(id int64, attemptIndex int, attempt model.ChannelAttempt) {
	relayLogStore.Lock()
	record, ok := relayLogStore.records[id]
	if !ok {
		relayLogStore.Unlock()
		return
	}
	entry := relayLogAttemptFromModel(attemptIndex, attempt)
	entry.Status = model.AttemptStatus("running")
	if attemptIndex < 0 {
		attemptIndex = 0
		entry.AttemptIndex = 0
	}
	for len(record.overview.History) <= attemptIndex {
		record.overview.History = append(record.overview.History, RelayLogAttempt{})
	}
	record.overview.History[attemptIndex] = entry
	record.overview.CurrentAttemptIndex = attemptIndex
	record.overview.Attempts = appendModelAttempts(record.overview.History)
	record.overview.TotalAttempts = len(record.overview.History)
	record.overview.ChannelId = entry.ChannelID
	record.overview.ChannelName = entry.ChannelName
	record.overview.RateMultiplier = entry.RateMultiplier
	record.overview.ActualModelName = entry.ModelName
	record.overview.ActualModel = entry.ModelName
	record.overview.FinalChannelName = entry.ChannelName
	record.overview.FinalRateMultiplier = entry.RateMultiplier
	snapshot := relayLogStoreSnapshotLocked(record)
	relayLogStore.Unlock()
	attemptCopy := entry
	notifyRelayLogDetail(id, RelayLogDetailEvent{Type: RelayLogEventAttemptStarted, ID: id, Attempt: &attemptCopy})
	notifyRelayLogOverview(snapshot)
}

func appendModelAttempts(history []RelayLogAttempt) []model.ChannelAttempt {
	if len(history) == 0 {
		return nil
	}
	attempts := make([]model.ChannelAttempt, len(history))
	for index, attempt := range history {
		attempts[index] = modelAttemptFromRelay(attempt)
	}
	return attempts
}

func RelayLogStoreAttemptFinished(id int64, attemptIndex int, attempt model.ChannelAttempt) {
	relayLogStore.Lock()
	record, ok := relayLogStore.records[id]
	if !ok {
		relayLogStore.Unlock()
		return
	}
	entry := relayLogAttemptFromModel(attemptIndex, attempt)
	if attemptIndex < 0 {
		attemptIndex = 0
		entry.AttemptIndex = 0
	}
	for len(record.overview.History) <= attemptIndex {
		record.overview.History = append(record.overview.History, RelayLogAttempt{})
	}
	record.overview.History[attemptIndex] = entry
	record.overview.Attempts = appendModelAttempts(record.overview.History)
	record.overview.TotalAttempts = len(record.overview.History)
	if record.overview.CurrentAttemptIndex == attemptIndex {
		record.overview.CurrentAttemptIndex = -1
	}
	record.overview.ChannelId = entry.ChannelID
	record.overview.ChannelName = entry.ChannelName
	record.overview.RateMultiplier = entry.RateMultiplier
	record.overview.ActualModelName = entry.ModelName
	record.overview.ActualModel = entry.ModelName
	record.overview.FinalChannelName = entry.ChannelName
	record.overview.FinalRateMultiplier = entry.RateMultiplier
	snapshot := relayLogStoreSnapshotLocked(record)
	relayLogStore.Unlock()
	attemptCopy := entry
	notifyRelayLogDetail(id, RelayLogDetailEvent{Type: RelayLogEventAttemptFinished, ID: id, Attempt: &attemptCopy})
	notifyRelayLogOverview(snapshot)
}

func RelayLogStoreResponseCommitted(id int64) {
	relayLogStore.Lock()
	record, ok := relayLogStore.records[id]
	if !ok || record.committed {
		relayLogStore.Unlock()
		return
	}
	record.committed = true
	record.overview.ResponseCommitted = true
	if record.overview.State == RelayLogStateRunning {
		record.overview.State = RelayLogStateCommitted
	}
	snapshot := relayLogStoreSnapshotLocked(record)
	relayLogStore.Unlock()
	notifyRelayLogDetail(id, RelayLogDetailEvent{Type: RelayLogEventResponseCommitted, ID: id, Overview: &snapshot})
	notifyRelayLogOverview(snapshot)
}

// RelayLogStoreComplete stores the final compatibility record without touching the database.
func RelayLogStoreComplete(id int64, state RelayLogState, log model.RelayLog, err error, cacheReadTokens, cacheWriteTokens int64) {
	if id == 0 {
		return
	}
	if state == "" {
		state = RelayLogStateFailed
	}
	relayLogStore.Lock()
	record, ok := relayLogStore.records[id]
	if !ok {
		started := time.Now()
		record = &relayLogStoreRecord{overview: relayLogOverviewFromModel(log, state, started, "", false), cancelIndex: -1}
		relayLogStore.records[id] = record
	}
	previousState := record.overview.State
	log = cloneRelayLog(log)
	overview := &record.overview
	overview.RelayLog = log
	overview.State = state
	overview.RequestModel = log.RequestModelName
	overview.ActualModel = log.ActualModelName
	overview.APIKeyName = log.RequestAPIKeyName
	overview.FinalChannelName = log.ChannelName
	overview.FinalRateMultiplier = log.RateMultiplier
	overview.CacheReadTokens = cacheReadTokens
	if cacheReadTokens == 0 && log.CachedTokens != nil {
		overview.CacheReadTokens = int64(*log.CachedTokens)
	}
	overview.CacheWriteTokens = cacheWriteTokens
	overview.TotalCost = log.Cost
	overview.RequestBody = log.RequestContent
	overview.ResponseBody = log.ResponseContent
	overview.History = overview.History[:0]
	for index, attempt := range log.Attempts {
		overview.History = append(overview.History, relayLogAttemptFromModel(index, attempt))
	}
	if err != nil {
		overview.Error = err.Error()
	} else {
		overview.Error = log.Error
	}
	if overview.StartedAt.IsZero() {
		if log.Time > 0 {
			overview.StartedAt = time.Unix(log.Time, 0)
		} else {
			overview.StartedAt = time.Now()
		}
	}
	completed := time.Now()
	overview.CompletedAt = &completed
	if log.UseTime > 0 {
		overview.Duration = time.Duration(log.UseTime) * time.Millisecond
	} else {
		overview.Duration = completed.Sub(overview.StartedAt)
		if overview.Duration < 0 {
			overview.Duration = 0
		}
	}
	overview.CurrentAttemptIndex = -1
	record.cancel = nil
	record.cancelIndex = -1
	wasTerminal := previousState == RelayLogStateSuccess || previousState == RelayLogStateFailed || previousState == RelayLogStateCanceled
	if !record.committed && (state == RelayLogStateSuccess || state == RelayLogStateCommitted) {
		record.committed = true
	}
	overview.ResponseCommitted = record.committed
	isNewCompleted := !wasTerminal && state != RelayLogStateRunning && state != RelayLogStateCommitted
	if state == RelayLogStateCommitted {
		// A committed response remains live until relayRun reports its final result.
		isNewCompleted = false
	}
	if state == RelayLogStateSuccess || state == RelayLogStateFailed || state == RelayLogStateCanceled {
		isNewCompleted = !wasTerminal
	}
	if isNewCompleted {
		relayLogStore.completedIDs = append(relayLogStore.completedIDs, id)
		for len(relayLogStore.completedIDs) > relayLogStoreMaxCompleted {
			oldest := relayLogStore.completedIDs[0]
			relayLogStore.completedIDs = relayLogStore.completedIDs[1:]
			if oldest == id {
				continue
			}
			if oldRecord, exists := relayLogStore.records[oldest]; exists && oldRecord.overview.State != RelayLogStateRunning && oldRecord.overview.State != RelayLogStateCommitted {
				delete(relayLogStore.records, oldest)
			}
		}
	}
	snapshot := relayLogStoreSnapshotLocked(record)
	committed := record.committed
	relayLogStore.Unlock()
	notifyRelayLogOverview(snapshot)
	notifySubscribers(snapshot.RelayLog)
	if committed {
		notifyRelayLogDetail(id, RelayLogDetailEvent{Type: RelayLogEventResponseCommitted, ID: id, Overview: &snapshot})
	}
}

// RelayLogStorePrune removes old terminal records while preserving active requests
// and requests whose detail stream is still open.
func RelayLogStorePrune(cutoff time.Time) int {
	relayLogStore.Lock()
	defer relayLogStore.Unlock()

	removed := 0
	for id, record := range relayLogStore.records {
		state := record.overview.State
		if state == RelayLogStateRunning || state == RelayLogStateCommitted {
			continue
		}
		if len(relayLogStore.detailSubs[id]) > 0 {
			continue
		}
		completedAt := record.overview.CompletedAt
		if completedAt == nil || !completedAt.Before(cutoff) {
			continue
		}
		delete(relayLogStore.records, id)
		removed++
	}

	keptIDs := relayLogStore.completedIDs[:0]
	for _, id := range relayLogStore.completedIDs {
		if _, ok := relayLogStore.records[id]; ok {
			keptIDs = append(keptIDs, id)
		}
	}
	relayLogStore.completedIDs = keptIDs
	return removed
}

// RelayLogStoreRegisterAttemptCancel associates a cancel function with the current attempt.
func RelayLogStoreRegisterAttemptCancel(id int64, attemptIndex int, cancel context.CancelFunc) {
	relayLogStore.Lock()
	if record, ok := relayLogStore.records[id]; ok && (record.overview.State == RelayLogStateRunning || record.overview.State == RelayLogStateCommitted) {
		record.cancel = cancel
		record.cancelIndex = attemptIndex
	}
	relayLogStore.Unlock()
}

func RelayLogStoreClearAttemptCancel(id int64, attemptIndex int) {
	relayLogStore.Lock()
	if record, ok := relayLogStore.records[id]; ok && record.cancelIndex == attemptIndex {
		record.cancel = nil
		record.cancelIndex = -1
	}
	relayLogStore.Unlock()
}

// RelayLogStoreStopAttempt cancels only the current attempt. The relay's request context remains intact.
func RelayLogStoreStopAttempt(id int64, attemptIndex int) bool {
	relayLogStore.RLock()
	record, ok := relayLogStore.records[id]
	if !ok || (record.overview.State != RelayLogStateRunning && record.overview.State != RelayLogStateCommitted) || record.cancel == nil || record.cancelIndex != attemptIndex {
		relayLogStore.RUnlock()
		return false
	}
	cancel := record.cancel
	relayLogStore.RUnlock()
	cancel()
	return true
}

func RelayLogStoreBody(id int64, response bool) (string, bool, bool) {
	relayLogStore.RLock()
	record, ok := relayLogStore.records[id]
	if !ok {
		relayLogStore.RUnlock()
		return "", false, false
	}
	if response {
		body := record.overview.ResponseContent
		truncated := record.overview.ResponseContentTruncated
		relayLogStore.RUnlock()
		return body, truncated, true
	}
	body := record.overview.RequestContent
	truncated := record.overview.RequestContentTruncated
	relayLogStore.RUnlock()
	return body, truncated, true
}

// RelayLogStoreClear removes completed records but never interrupts running requests.
func RelayLogStoreClear() {
	relayLogStore.Lock()
	for id, record := range relayLogStore.records {
		if record.overview.State == RelayLogStateRunning || record.overview.State == RelayLogStateCommitted {
			continue
		}
		delete(relayLogStore.records, id)
	}
	relayLogStore.completedIDs = relayLogStore.completedIDs[:0]
	relayLogStore.Unlock()
}

func relayLogStateForResult(success bool, err error) RelayLogState {
	if errors.Is(err, context.Canceled) {
		return RelayLogStateCanceled
	}
	if success {
		return RelayLogStateSuccess
	}
	return RelayLogStateFailed
}

// RelayLogStateForResult maps a relay result to its terminal in-memory state.
func RelayLogStateForResult(success bool, err error) RelayLogState {
	return relayLogStateForResult(success, err)
}
