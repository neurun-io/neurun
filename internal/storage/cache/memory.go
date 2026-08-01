package cache

import (
	"context"
	"strings"
	"sync"
	"time"
)

// Memory is the process-local Cache.
//
// Entries live only in this process, so a restart empties it and every operator
// session ends. That is the honest consequence of having no shared adapter yet,
// and it is the reason the Cache interface exists: a Redis implementation
// replaces this type without any caller changing.
type Memory struct {
	mu      sync.RWMutex
	entries map[string]entry
	now     func() time.Time
}

type entry struct {
	value []byte
	// expiresAt is zero for an entry that never expires.
	expiresAt time.Time
}

func (e entry) expired(now time.Time) bool {
	return !e.expiresAt.IsZero() && !now.Before(e.expiresAt)
}

var _ Cache = (*Memory)(nil)

func NewMemory() *Memory {
	return &Memory{entries: make(map[string]entry), now: time.Now}
}

// NewMemoryWithClock builds a Memory reading time from now, so expiry can be
// tested without sleeping.
func NewMemoryWithClock(now func() time.Time) *Memory {
	if now == nil {
		now = time.Now
	}
	return &Memory{entries: make(map[string]entry), now: now}
}

func (m *Memory) Get(ctx context.Context, key string) ([]byte, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}

	// Written under the write lock because a read that finds an expired entry
	// drops it, so a stale token cannot be probed repeatedly.
	m.mu.Lock()
	defer m.mu.Unlock()

	found, ok := m.entries[key]
	if !ok {
		return nil, false, nil
	}
	if found.expired(m.now()) {
		delete(m.entries, key)
		return nil, false, nil
	}
	return append([]byte(nil), found.value...), true, nil
}

func (m *Memory) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	stored := entry{value: append([]byte(nil), value...)}
	if ttl > 0 {
		stored.expiresAt = m.now().Add(ttl)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[key] = stored
	return nil
}

func (m *Memory) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.entries, key)
	return nil
}

func (m *Memory) Keys(ctx context.Context, prefix string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	now := m.now()
	m.mu.RLock()
	defer m.mu.RUnlock()

	keys := make([]string, 0, len(m.entries))
	for key, stored := range m.entries {
		if strings.HasPrefix(key, prefix) && !stored.expired(now) {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

// Sweep drops every expired entry and reports how many it removed.
//
// Get evicts lazily, which is enough for correctness but leaves the map holding
// entries nobody reads again. A caller on a ticker reclaims that memory. Redis
// expires keys on its own, so its implementation of this is a no-op.
func (m *Memory) Sweep() int {
	now := m.now()

	m.mu.Lock()
	defer m.mu.Unlock()

	removed := 0
	for key, stored := range m.entries {
		if stored.expired(now) {
			delete(m.entries, key)
			removed++
		}
	}
	return removed
}

// Len reports the number of live entries, for tests and operational logging.
func (m *Memory) Len() int {
	now := m.now()

	m.mu.RLock()
	defer m.mu.RUnlock()

	live := 0
	for _, stored := range m.entries {
		if !stored.expired(now) {
			live++
		}
	}
	return live
}
