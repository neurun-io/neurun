package memory

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCache is the shared implementation of Cache.
//
// It is what lets the control plane run more than one replica: sessions live in
// the server rather than in a process, so a request answered by a second replica
// sees the session the first one issued.
type RedisCache struct {
	client *redis.Client
	prefix string
}

// NewRedisCache dials a server from a redis:// or rediss:// URL.
//
// The connection is not proved here. A cache that is briefly unreachable should
// fail the requests that need it, not the boot that would otherwise have served
// everything else; readiness is where that belongs.
func NewRedisCache(url, prefix string) (*RedisCache, error) {
	options, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parse redis URL: %w", err)
	}
	return &RedisCache{client: redis.NewClient(options), prefix: prefix}, nil
}

func (cache *RedisCache) Close() {
	_ = cache.client.Close()
}

// PrefixedKey namespaces a key, so two deployments sharing one server do not
// read each other's entries.
func (cache *RedisCache) PrefixedKey(key string) string {
	if cache.prefix == "" {
		return key
	}
	return cache.prefix + ":" + key
}

func (cache *RedisCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	value, err := cache.client.Get(ctx, cache.PrefixedKey(key)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read cache entry: %w", err)
	}
	return value, true, nil
}

// Set replaces any existing entry. A ttl of zero or less stores it without
// expiry, which is also how go-redis reads a zero expiration.
func (cache *RedisCache) Set(
	ctx context.Context,
	key string,
	value []byte,
	ttl time.Duration,
) error {
	if ttl < 0 {
		ttl = 0
	}
	if err := cache.client.Set(
		ctx, cache.PrefixedKey(key), value, ttl,
	).Err(); err != nil {
		return fmt.Errorf("write cache entry: %w", err)
	}
	return nil
}

// Delete is not an error for an absent key, so signing out an already-expired
// session needs no special case.
func (cache *RedisCache) Delete(ctx context.Context, key string) error {
	if err := cache.client.Del(ctx, cache.PrefixedKey(key)).Err(); err != nil {
		return fmt.Errorf("delete cache entry: %w", err)
	}
	return nil
}

// Keys scans for live keys under prefix.
//
// SCAN makes no atomicity promise and may return a key twice across a cursor
// walk, so the result is advisory — the same contract the in-process store
// documents. KEYS is not used: it blocks the server for the length of the walk.
func (cache *RedisCache) Keys(ctx context.Context, prefix string) ([]string, error) {
	pattern := cache.PrefixedKey(prefix) + "*"
	var (
		keys   []string
		cursor uint64
	)
	for {
		page, next, err := cache.client.Scan(ctx, cursor, pattern, 256).Result()
		if err != nil {
			return nil, fmt.Errorf("scan cache keys: %w", err)
		}
		keys = append(keys, page...)
		if next == 0 {
			return keys, nil
		}
		cursor = next
	}
}

// DeleteExpired does nothing: Redis expires keys itself, so there is no sweep to
// run and nothing to report.
func (cache *RedisCache) DeleteExpired() int { return 0 }

// Check proves the server answers, which is what readiness is asking.
func (cache *RedisCache) Check(ctx context.Context) error {
	if err := cache.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis is not reachable: %w", err)
	}
	return nil
}

var _ Cache = (*RedisCache)(nil)
