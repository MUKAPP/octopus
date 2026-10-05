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

func TestRelayLogStorePrune(t *testing.T) {
	resetRelayLogStoreForTest()
	t.Cleanup(resetRelayLogStoreForTest)

	cutoff := time.Unix(1_700_000_000, 0)
	oldCompletedAt := cutoff.Add(-time.Hour)

	complete := func(id int64, completedAt time.Time) {
		RelayLogStoreStart(model.RelayLog{ID: id, Time: completedAt.Unix()}, completedAt, "openai", false)
		RelayLogStoreComplete(id, RelayLogStateFailed, model.RelayLog{ID: id, Time: completedAt.Unix()}, errors.New("failed"), 0, 0)
		relayLogStore.Lock()
		relayLogStore.records[id].overview.CompletedAt = &completedAt
		relayLogStore.Unlock()
	}

	complete(7201, oldCompletedAt)
	complete(7202, cutoff.Add(time.Hour))
	RelayLogStoreStart(model.RelayLog{ID: 7203, Time: oldCompletedAt.Unix()}, oldCompletedAt, "openai", false)
	RelayLogStoreStart(model.RelayLog{ID: 7204, Time: oldCompletedAt.Unix()}, oldCompletedAt, "openai", false)
	RelayLogStoreResponseCommitted(7204)
	complete(7205, oldCompletedAt)
	detail, ok := RelayLogStoreSubscribeDetail(7205)
	if !ok {
		t.Fatal("detail subscription failed")
	}

	if removed := RelayLogStorePrune(cutoff); removed != 1 {
		t.Fatalf("pruned records = %d, want 1", removed)
	}
	if _, ok := RelayLogStoreGet(7201); ok {
		t.Fatal("old terminal record was not pruned")
	}
	for _, id := range []int64{7202, 7203, 7204, 7205} {
		if _, ok := RelayLogStoreGet(id); !ok {
			t.Fatalf("record %d should be retained", id)
		}
	}

	RelayLogStoreUnsubscribeDetail(7205, detail)
	if removed := RelayLogStorePrune(cutoff); removed != 1 {
		t.Fatalf("pruned subscribed record = %d, want 1", removed)
	}
	if _, ok := RelayLogStoreGet(7205); ok {
		t.Fatal("closed detail record was not pruned")
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

func TestRelayLogStoreErrorCompletion(t *testing.T) {
	const diagnostic = "upstream HTTP 520: fixture upstream detail\n请减少输入长度"
	const streamDiagnostic = "upstream HTTP 200: fixture stream failed"
	const degraded = "Request failed: , , type: api_error"
	for _, test := range []struct {
		name       string
		committed  bool
		state      RelayLogState
		statusCode int
		logError   string
		cause      error
		wantError  string
	}{
		{"uncommitted diagnostic", false, RelayLogStateFailed, 520, diagnostic, errors.New(degraded), diagnostic},
		{"committed diagnostic", true, RelayLogStateFailed, 200, streamDiagnostic, errors.New(degraded), streamDiagnostic},
		{"uncommitted fallback", false, RelayLogStateFailed, 0, "", errors.New("connection refused"), "connection refused"},
		{"committed canceled fallback", true, RelayLogStateCanceled, 0, "", errors.New("context canceled"), "context canceled"},
		{"diagnostic without cause", false, RelayLogStateFailed, 520, diagnostic, nil, diagnostic},
		{"success", true, RelayLogStateSuccess, 200, "", nil, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			resetRelayLogStoreForTest()
			t.Cleanup(resetRelayLogStoreForTest)
			const id int64 = 7301
			startedAt := time.Now().Add(-time.Second)
			RelayLogStoreStart(model.RelayLog{ID: id, Time: startedAt.Unix(), RequestModelName: "fixture-model"}, startedAt, "openai", test.committed)
			detail, ok := RelayLogStoreSubscribeDetail(id)
			if !ok {
				t.Fatal("detail subscription failed")
			}
			defer RelayLogStoreUnsubscribeDetail(id, detail)
			receiveDetail := func(eventType string) RelayLogDetailEvent {
				t.Helper()
				select {
				case event := <-detail:
					if event.Type != eventType || event.ID != id {
						t.Fatalf("detail event = %+v, want type %q for %d", event, eventType, id)
					}
					return event
				case <-time.After(time.Second):
					t.Fatalf("timed out waiting for detail %q", eventType)
					return RelayLogDetailEvent{}
				}
			}
			attempts := []model.ChannelAttempt{
				{ChannelID: 4, ChannelName: "fixture-first", ModelName: "fixture-model", AttemptNum: 1, Status: model.AttemptFailed, Msg: "upstream HTTP 429: fixture rate limit", UpstreamStatusCode: 429},
				{ChannelID: 5, ChannelName: "fixture-final", ModelName: "fixture-model", AttemptNum: 2, Status: model.AttemptFailed, Msg: test.wantError, UpstreamStatusCode: test.statusCode},
			}
			if test.state == RelayLogStateSuccess {
				attempts[1].Status = model.AttemptSuccess
			}
			for index, attempt := range attempts {
				started := attempt
				started.Msg = ""
				started.UpstreamStatusCode = 0
				RelayLogStoreAttemptStarted(id, index, started)
				event := receiveDetail(RelayLogEventAttemptStarted)
				if event.Attempt == nil || event.Attempt.AttemptIndex != index || event.Attempt.UpstreamStatusCode != 0 {
					t.Fatalf("started attempt = %+v", event.Attempt)
				}
				RelayLogStoreAttemptFinished(id, index, attempt)
				event = receiveDetail(RelayLogEventAttemptFinished)
				if event.Attempt == nil || event.Attempt.AttemptIndex != index || event.Attempt.UpstreamStatusCode != attempt.UpstreamStatusCode || event.Attempt.Error != attempt.Msg || event.Attempt.Msg != attempt.Msg {
					t.Fatalf("finished attempt = %+v, want %+v", event.Attempt, attempt)
				}
			}
			live, ok := RelayLogStoreGet(id)
			if !ok || len(live.Attempts) != len(attempts) || len(live.History) != len(attempts) {
				t.Fatalf("live attempt history = %+v, found=%t", live, ok)
			}
			for index, attempt := range attempts {
				if live.Attempts[index] != attempt || live.History[index].UpstreamStatusCode != attempt.UpstreamStatusCode {
					t.Fatalf("live attempt %d = %+v / %+v, want %+v", index, live.Attempts[index], live.History[index], attempt)
				}
			}
			overviews := RelayLogStoreSubscribeOverview()
			defer RelayLogStoreUnsubscribeOverview(overviews)
			legacy := RelayLogSubscribe()
			defer RelayLogUnsubscribe(legacy)
			if test.committed {
				RelayLogStoreResponseCommitted(id)
				event := receiveDetail(RelayLogEventResponseCommitted)
				if event.Overview == nil || event.Overview.State != RelayLogStateCommitted || !event.Overview.ResponseCommitted {
					t.Fatalf("committed event = %+v", event.Overview)
				}
				select {
				case overview := <-overviews:
					if overview.State != RelayLogStateCommitted {
						t.Fatalf("committed overview = %+v", overview)
					}
				case <-time.After(time.Second):
					t.Fatal("timed out waiting for committed overview")
				}
			}
			finalLog := model.RelayLog{
				ID:                 id,
				Time:               startedAt.Unix(),
				RequestModelName:   "fixture-model",
				ActualModelName:    "fixture-model",
				UpstreamStatusCode: test.statusCode,
				Attempts:           attempts,
				TotalAttempts:      len(attempts),
				Error:              test.logError,
				RequestContent:     "fixture private request",
				ResponseContent:    "fixture downstream response",
			}
			RelayLogStoreComplete(id, test.state, finalLog, test.cause, 0, 0)
			assertFinal := func(overview RelayLogOverview) {
				t.Helper()
				if overview.ID != id || overview.State != test.state || overview.Error != test.wantError || overview.UpstreamStatusCode != test.statusCode || overview.ResponseCommitted != test.committed || overview.CompletedAt == nil || overview.CurrentAttemptIndex != -1 {
					t.Fatalf("terminal snapshot = %+v", overview)
				}
				if len(overview.History) != len(attempts) || len(overview.Attempts) != len(attempts) {
					t.Fatalf("terminal history = %+v / attempts = %+v", overview.History, overview.Attempts)
				}
				for index, attempt := range attempts {
					entry := overview.History[index]
					if entry.AttemptIndex != index || entry.Status != attempt.Status || entry.UpstreamStatusCode != attempt.UpstreamStatusCode || entry.Msg != attempt.Msg || entry.Error != attempt.Msg || overview.Attempts[index] != attempt {
						t.Fatalf("terminal attempt %d = %+v / %+v, want %+v", index, entry, overview.Attempts[index], attempt)
					}
				}
				payload, err := json.Marshal(overview)
				if err != nil {
					t.Fatalf("marshal terminal overview: %v", err)
				}
				var decoded RelayLogOverview
				if err := json.Unmarshal(payload, &decoded); err != nil {
					t.Fatalf("decode terminal overview: %v", err)
				}
				if decoded.State != test.state || decoded.Error != test.wantError || decoded.UpstreamStatusCode != test.statusCode || len(decoded.History) != len(attempts) || decoded.History[0].UpstreamStatusCode != 429 || decoded.History[1].UpstreamStatusCode != test.statusCode || decoded.History[1].Error != test.wantError {
					t.Fatalf("serialized terminal overview = %s", payload)
				}
				if decoded.RequestContent != "" || decoded.ResponseContent != "" || decoded.RequestBody != "" || decoded.ResponseBody != "" || strings.Contains(string(payload), finalLog.RequestContent) || strings.Contains(string(payload), finalLog.ResponseContent) {
					t.Fatalf("terminal overview leaked body: %s", payload)
				}
			}
			if test.committed {
				event := receiveDetail(RelayLogEventResponseCommitted)
				if event.Overview == nil {
					t.Fatal("final committed event has no snapshot")
				}
				assertFinal(*event.Overview)
			}
			event := receiveDetail(RelayLogEventOverview)
			if event.Overview == nil {
				t.Fatal("terminal log event has no snapshot")
			}
			assertFinal(*event.Overview)
			select {
			case event := <-detail:
				t.Fatalf("unexpected additional detail event: %+v", event)
			default:
			}
			select {
			case overview := <-overviews:
				assertFinal(overview)
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for terminal overview")
			}
			select {
			case log := <-legacy:
				if log.Error != test.wantError || log.UpstreamStatusCode != test.statusCode || len(log.Attempts) != len(attempts) || log.Attempts[0].UpstreamStatusCode != 429 || log.Attempts[1].UpstreamStatusCode != test.statusCode {
					t.Fatalf("legacy final log = %+v", log)
				}
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for legacy final log")
			}
			stored, ok := RelayLogStoreGet(id)
			if !ok {
				t.Fatal("terminal record not found")
			}
			assertFinal(stored)
			for _, response := range []bool{false, true} {
				wantBody := finalLog.RequestContent
				if response {
					wantBody = finalLog.ResponseContent
				}
				if body, truncated, found := RelayLogStoreBody(id, response); !found || truncated || body != wantBody {
					t.Fatalf("stored body response=%t: %q, truncated=%t, found=%t", response, body, truncated, found)
				}
			}
		})
	}
}
