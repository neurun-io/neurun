// Package cache holds the keyed, expiring stores the runtime needs but must not
// keep in a domain package.
//
// The interface is deliberately narrow: every method maps onto a single Redis
// command, so the in-process implementation can be replaced by a shared one
// without any caller changing. Values are opaque bytes — encoding belongs to the
// caller, not here.
package storage

import (
	"context"
	"time"
)

// Cache is a keyed byte store with per-entry expiry. Implementations must be
// safe for concurrent use.
type Cache interface {
	// Get returns the value stored under key. found reports whether a live
	// entry existed; an expired entry reports false and is indistinguishable
	// from an absent one, which is what keeps an expired token from being
	// probed for existence.
	Get(ctx context.Context, key string) (value []byte, found bool, err error)

	// Set stores value under key, replacing any existing entry. A ttl of zero
	// or less stores the entry without expiry.
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error

	// Delete removes key. Deleting an absent key is not an error, so callers
	// signing out an already-expired session do not have to special-case it.
	Delete(ctx context.Context, key string) error

	// Keys returns every live key carrying prefix.
	//
	// The result is a snapshot taken without holding a lock over the caller's
	// subsequent work — a Redis implementation answers this with SCAN MATCH,
	// which makes no atomicity promise either. Treat it as advisory.
	Keys(ctx context.Context, prefix string) ([]string, error)
}
