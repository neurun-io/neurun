package repository

import (
	"context"
	"strings"
	"time"

	"github.com/jellydator/ttlcache/v3"
)

// CacheRepository is a keyed byte store with per-entry expiry, backed by an
// in-process ttlcache. Entries do not survive a restart and are not shared
// between replicas.
//
// Every method maps onto a single Redis command, so moving to a shared server
// later is a change to this file and nothing else. Values are opaque bytes —
// encoding is the caller's business.
type CacheRepository struct {
	cache  *ttlcache.Cache[string, []byte]
	prefix string
}

func NewCacheRepository(prefix string) *CacheRepository {
	// WithDisableTouchOnHit keeps a TTL absolute. Without it, reading an entry
	// extends its life, which would quietly turn a fixed session lifetime into
	// a sliding one that never ends while the holder keeps browsing.
	cache := ttlcache.New(ttlcache.WithDisableTouchOnHit[string, []byte]())
	// Start runs the eviction loop and blocks until Stop.
	go cache.Start()
	return &CacheRepository{cache: cache, prefix: prefix}
}

// Close stops the eviction loop.
func (repository *CacheRepository) Close() {
	repository.cache.Stop()
}

// PrefixedKey namespaces a key, so two deployments sharing one cache server do
// not read each other's entries.
func (repository *CacheRepository) PrefixedKey(key string) string {
	if repository.prefix == "" {
		return key
	}
	return repository.prefix + ":" + key
}

// Get reports found=false for an expired entry, which keeps a stale token from
// being probed for existence.
func (repository *CacheRepository) Get(
	ctx context.Context,
	key string,
) ([]byte, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	item := repository.cache.Get(repository.PrefixedKey(key))
	if item == nil || item.IsExpired() {
		return nil, false, nil
	}
	return append([]byte(nil), item.Value()...), true, nil
}

// Set replaces any existing entry. A ttl of zero or less stores it without
// expiry.
func (repository *CacheRepository) Set(
	ctx context.Context,
	key string,
	value []byte,
	ttl time.Duration,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if ttl <= 0 {
		ttl = ttlcache.NoTTL
	}
	repository.cache.Set(
		repository.PrefixedKey(key), append([]byte(nil), value...), ttl,
	)
	return nil
}

// Delete is not an error for an absent key, so signing out an already-expired
// session needs no special case.
func (repository *CacheRepository) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	repository.cache.Delete(repository.PrefixedKey(key))
	return nil
}

// Keys returns every live key under prefix.
//
// The result is a snapshot taken without holding a lock over the caller's
// subsequent work — Redis answers this with SCAN MATCH, which makes no
// atomicity promise either. Treat it as advisory.
func (repository *CacheRepository) Keys(
	ctx context.Context,
	prefix string,
) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	prefix = repository.PrefixedKey(prefix)
	keys := make([]string, 0, repository.cache.Len())
	repository.cache.Range(func(item *ttlcache.Item[string, []byte]) bool {
		if strings.HasPrefix(item.Key(), prefix) && !item.IsExpired() {
			keys = append(keys, item.Key())
		}
		return true
	})
	return keys, nil
}

// DeleteExpired drops entries the eviction loop has not reached yet and reports
// how many it removed.
func (repository *CacheRepository) DeleteExpired() int {
	before := repository.cache.Len()
	repository.cache.DeleteExpired()
	removed := before - repository.cache.Len()
	if removed < 0 {
		return 0
	}
	return removed
}

func (repository *CacheRepository) Len() int {
	return repository.cache.Len()
}
