package op

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bestruirui/octopus/internal/model"
)

type apiKeyConcurrencyWaiter struct {
	ctx     context.Context
	ready   chan struct{}
	prev    *apiKeyConcurrencyWaiter
	next    *apiKeyConcurrencyWaiter
	queued  bool
	granted bool
	err     error
}

type apiKeyConcurrencyState struct {
	active int
	head   *apiKeyConcurrencyWaiter
	tail   *apiKeyConcurrencyWaiter
}

var apiKeyConcurrencyLock sync.Mutex
var apiKeyConcurrencyStates = make(map[int]*apiKeyConcurrencyState)

func APIKeyValidateAccess(key model.APIKey) error {
	if !key.Enabled {
		return fmt.Errorf("API key is disabled")
	}
	if key.ExpireAt > 0 && key.ExpireAt < time.Now().Unix() {
		return fmt.Errorf("API key has expired")
	}
	stats := StatsAPIKeyGet(key.ID)
	if key.MaxCost > 0 && key.MaxCost < stats.StatsMetrics.OutputCost+stats.StatsMetrics.InputCost {
		return fmt.Errorf("API key has reached the max cost")
	}
	return nil
}

func apiKeyCurrentAccess(id int) (model.APIKey, error) {
	key, ok := apiKeyCache.Get(id)
	if !ok {
		return model.APIKey{}, fmt.Errorf("API key not found")
	}
	return key, APIKeyValidateAccess(key)
}

func (s *apiKeyConcurrencyState) remove(w *apiKeyConcurrencyWaiter) {
	if w.prev != nil {
		w.prev.next = w.next
	} else {
		s.head = w.next
	}
	if w.next != nil {
		w.next.prev = w.prev
	} else {
		s.tail = w.prev
	}
	w.prev, w.next = nil, nil
	w.queued = false
}

// APIKeyAcquire reserves one slot for the entire relay lifecycle, including retries.
func APIKeyAcquire(id int, ctx context.Context) (model.APIKey, error) {
	apiKeyConcurrencyLock.Lock()
	if err := ctx.Err(); err != nil {
		apiKeyConcurrencyLock.Unlock()
		return model.APIKey{}, err
	}
	key, err := apiKeyCurrentAccess(id)
	if err != nil {
		apiKeyConcurrencyLock.Unlock()
		return model.APIKey{}, err
	}
	s := apiKeyConcurrencyStates[id]
	if s == nil {
		s = &apiKeyConcurrencyState{}
		apiKeyConcurrencyStates[id] = s
	}
	if s.head == nil && (key.MaxConcurrency == 0 || s.active < key.MaxConcurrency) {
		s.active++
		apiKeyConcurrencyLock.Unlock()
		return key, nil
	}
	w := &apiKeyConcurrencyWaiter{ctx: ctx, ready: make(chan struct{}), prev: s.tail, queued: true}
	if s.tail != nil {
		s.tail.next = w
	} else {
		s.head = w
	}
	s.tail = w
	apiKeyConcurrencyLock.Unlock()

	select {
	case <-w.ready:
	case <-ctx.Done():
	}

	apiKeyConcurrencyLock.Lock()
	defer apiKeyConcurrencyLock.Unlock()
	if err := ctx.Err(); err != nil {
		if w.queued {
			s.remove(w)
		} else if w.granted {
			s.active--
			w.granted = false
		}
		apiKeyDrainWaitersLocked(id)
		return model.APIKey{}, err
	}
	// Re-enabling or recreating a key must not resurrect a rejected waiter.
	if w.err != nil {
		return model.APIKey{}, w.err
	}
	key, err = apiKeyCurrentAccess(id)
	if err != nil {
		s.active--
		w.granted = false
		apiKeyDrainWaitersLocked(id)
		return model.APIKey{}, err
	}
	return key, nil
}

func APIKeyRelease(id int) {
	apiKeyConcurrencyLock.Lock()
	defer apiKeyConcurrencyLock.Unlock()
	apiKeyConcurrencyStates[id].active--
	apiKeyDrainWaitersLocked(id)
}

func apiKeyDrainWaitersLocked(id int) {
	s := apiKeyConcurrencyStates[id]
	if s == nil {
		return
	}
	key, err := apiKeyCurrentAccess(id)
	for s.head != nil {
		w := s.head
		waitErr := err
		if waitErr == nil {
			waitErr = w.ctx.Err()
		}
		if waitErr == nil && key.MaxConcurrency > 0 && s.active >= key.MaxConcurrency {
			break
		}
		s.remove(w)
		w.err = waitErr
		if waitErr == nil {
			s.active++
			w.granted = true
		}
		close(w.ready)
	}
	if _, exists := apiKeyCache.Get(id); !exists && s.active == 0 && s.head == nil {
		delete(apiKeyConcurrencyStates, id)
	}
}

func apiKeyPublish(key model.APIKey) {
	apiKeyConcurrencyLock.Lock()
	defer apiKeyConcurrencyLock.Unlock()
	apiKeyCache.Set(key.ID, key)
	apiKeyDrainWaitersLocked(key.ID)
}

func apiKeyForget(id int) {
	apiKeyConcurrencyLock.Lock()
	defer apiKeyConcurrencyLock.Unlock()
	apiKeyCache.Del(id)
	apiKeyDrainWaitersLocked(id)
}
