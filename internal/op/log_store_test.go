package op

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bestruirui/octopus/internal/model"
)

func resetRelayLogStoreForTest() {
	relayLogStore.Lock()
	for ch := range relayLogStore.overviewSubs {
		delete(relayLogStore.overviewSubs, ch)
		close(ch)
	}
	for id, subscribers := range relayLogStore.detailSubs {
		for ch := range subscribers {
			delete(subscribers, ch)
			close(ch)
		}
		delete(relayLogStore.detailSubs, id)
	}
	relayLogStore.records = make(map[int64]*relayLogStoreRecord)
	relayLogStore.completedIDs = nil
	relayLogStore.Unlock()
}

func TestRelayLogStoreLifecycleAndDetailReplay(t *testing.T) {
	resetRelayLogStoreForTest()
	t.Cleanup(resetRelayLogStoreForTest)

	startedAt := time.Unix(1_700_000_000, 0)
	id := int64(7101)
	RelayLogStoreStart(model.RelayLog{
		ID:               id,
		Time:             startedAt.Unix(),
		RequestModelName: "request-model",
	}, startedAt, "openai", true)

	snapshot, ok := RelayLogStoreGet(id)
	if !ok || snapshot.State != RelayLogStateRunning {
		t.Fatalf("running snapshot = %+v, found=%t", snapshot, ok)
	}

	detail, ok := RelayLogStoreSubscribeDetail(id)
	if !ok {
		t.Fatal("detail subscription failed")
	}
	defer RelayLogStoreUnsubscribeDetail(id, detail)

	attempt := model.ChannelAttempt{
		ChannelID:      4,
		ChannelName:    "channel-a",
		ModelName:      "actual-model",
		RateMultiplier: 1.25,
		AttemptNum:     1,
		Status:         model.AttemptFailed,
		Duration:       42,
		Msg:            "upstream failed",
	}
	RelayLogStoreAttemptStarted(id, 0, attempt)
	select {
	case event := <-detail:
		if event.Type != RelayLogEventAttemptStarted || event.Attempt == nil || event.Attempt.ChannelName != "channel-a" {
			t.Fatalf("attempt start event = %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for attempt start")
	}

	RelayLogStoreAttemptFinished(id, 0, attempt)
	select {
	case event := <-detail:
		if event.Type != RelayLogEventAttemptFinished || event.Attempt == nil || event.Attempt.Error != "upstream failed" {
			t.Fatalf("attempt finish event = %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for attempt finish")
	}

	RelayLogStoreComplete(id, RelayLogStateFailed, model.RelayLog{
		ID:               id,
		Time:             startedAt.Unix(),
		RequestModelName: "request-model",
		ActualModelName:  "actual-model",
		Attempts:         []model.ChannelAttempt{attempt},
		Error:            "request failed",
	}, errors.New("request failed"), 12, 3)

	completed, ok := RelayLogStoreGet(id)
	if !ok || completed.State != RelayLogStateFailed || len(completed.History) != 1 {
		t.Fatalf("completed snapshot = %+v, found=%t", completed, ok)
	}
	if completed.CacheReadTokens != 12 || completed.CacheWriteTokens != 3 {
		t.Fatalf("cache usage = read %d/write %d", completed.CacheReadTokens, completed.CacheWriteTokens)
	}
}

func TestRelayLogStoreStopAttempt(t *testing.T) {
	resetRelayLogStoreForTest()
	t.Cleanup(resetRelayLogStoreForTest)

	id := int64(7102)
	RelayLogStoreStart(model.RelayLog{ID: id, Time: time.Now().Unix()}, time.Now(), "openai", false)
	stopped := make(chan struct{})
	RelayLogStoreRegisterAttemptCancel(id, 2, func() { close(stopped) })
	if !RelayLogStoreStopAttempt(id, 2) {
		t.Fatal("stop attempt returned false")
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("cancel function was not called")
	}
	RelayLogStoreClearAttemptCancel(id, 2)
	if RelayLogStoreStopAttempt(id, 2) {
		t.Fatal("stopped attempt remained cancellable")
	}
}

func TestRelayLogOverviewJSONOmitsBodies(t *testing.T) {
	overview := RelayLogOverview{
		RelayLog: model.RelayLog{
			ID:              7103,
			RequestContent:  "request-secret",
			ResponseContent: "response-secret",
		},
		RequestBody:  "request-secret",
		ResponseBody: "response-secret",
	}
	payload, err := json.Marshal(overview)
	if err != nil {
		t.Fatalf("marshal overview: %v", err)
	}
	if strings.Contains(string(payload), "request-secret") || strings.Contains(string(payload), "response-secret") {
		t.Fatalf("overview leaked request/response body: %s", payload)
	}
}

func TestRelayLogStreamTokenConsumedOnce(t *testing.T) {
	token, err := RelayLogStreamTokenCreate()
	if err != nil {
		t.Fatalf("create stream token: %v", err)
	}
	if !RelayLogStreamTokenConsume(token) {
		t.Fatal("first stream token consume failed")
	}
	if RelayLogStreamTokenConsume(token) {
		t.Fatal("stream token was reusable")
	}
}
