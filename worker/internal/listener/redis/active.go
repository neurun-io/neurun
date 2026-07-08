package redis

import "sync"

type activeTracker struct {
	mu  sync.Mutex
	ids map[string]struct{}
}

func (t *activeTracker) Mark(id string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.ids == nil {
		t.ids = map[string]struct{}{}
	}
	if _, exists := t.ids[id]; exists {
		return false
	}
	t.ids[id] = struct{}{}
	return true
}

func (t *activeTracker) Unmark(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.ids, id)
}
