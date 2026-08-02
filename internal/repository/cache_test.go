package repository

import (
	"context"
	"testing"
	"time"
)

// ttlcache extends an entry's life on read by default. For a session store that
// would turn a fixed lifetime into a sliding one that never ends while the
// holder keeps browsing, so the cache disables it — and this is the guard.
func TestReadingAnEntryDoesNotExtendItsLife(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cache := NewCacheRepository("test")
	defer cache.Close()

	if err := cache.Set(ctx, "session:one", []byte("payload"), 150*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	// Keep reading across the lifetime; a touch-on-hit cache would never expire.
	deadline := time.Now().Add(400 * time.Millisecond)
	for time.Now().Before(deadline) {
		if _, _, err := cache.Get(ctx, "session:one"); err != nil {
			t.Fatal(err)
		}
		time.Sleep(25 * time.Millisecond)
	}
	value, found, err := cache.Get(ctx, "session:one")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatalf("entry survived its TTL under repeated reads: %q", value)
	}
}

func TestCacheRoundTripsAndNamespaces(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cache := NewCacheRepository("neurun")
	defer cache.Close()

	if err := cache.Set(ctx, "session:one", []byte("first"), time.Minute); err != nil {
		t.Fatal(err)
	}
	value, found, err := cache.Get(ctx, "session:one")
	if err != nil || !found || string(value) != "first" {
		t.Fatalf("get = %q, %v, %v", value, found, err)
	}
	if key := cache.PrefixedKey("session:one"); key != "neurun:session:one" {
		t.Fatalf("PrefixedKey = %q", key)
	}
	keys, err := cache.Keys(ctx, "session:")
	if err != nil || len(keys) != 1 || keys[0] != "neurun:session:one" {
		t.Fatalf("Keys = %#v, %v", keys, err)
	}

	// A returned value is a copy: mutating it must not corrupt the entry.
	value[0] = 'X'
	again, _, err := cache.Get(ctx, "session:one")
	if err != nil || string(again) != "first" {
		t.Fatalf("entry was mutated through the returned slice: %q (%v)", again, err)
	}

	if err := cache.Delete(ctx, "session:one"); err != nil {
		t.Fatal(err)
	}
	if _, found, _ = cache.Get(ctx, "session:one"); found {
		t.Fatal("entry survived deletion")
	}
	// Deleting an absent key is not an error.
	if err := cache.Delete(ctx, "session:missing"); err != nil {
		t.Fatalf("deleting an absent key errored: %v", err)
	}
}
