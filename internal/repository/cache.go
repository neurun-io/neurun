package repository

import (
	"context"
	"time"
)

// Cache is a keyed byte store with per-entry expiry. Values are opaque bytes —
// encoding is the caller's business.
//
// Every method maps onto a single Redis command. The interface remains because
// the repositories are written against behaviour rather than against Redis, not
// because a second implementation is coming — the in-process one was removed
// once sessions became something a restart must not destroy.
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Keys(ctx context.Context, prefix string) ([]string, error)
	// DeleteExpired reports how many entries it removed. Redis expires keys
	// itself, so it answers zero and does nothing; the sweep remains because the
	// caller schedules one either way.
	DeleteExpired() int
	Close()
}
