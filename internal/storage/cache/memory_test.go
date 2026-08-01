package cache

import (
	"context"
	"testing"
	"time"
)

func TestMemoryStoresAndReadsBackAValue(t *testing.T) {
	t.Parallel()
	memory := NewMemory()

	if err := memory.Set(context.Background(), "session:a", []byte("payload"), time.Minute); err != nil {
		t.Fatal(err)
	}
	value, found, err := memory.Get(context.Background(), "session:a")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("stored key reported as absent")
	}
	if string(value) != "payload" {
		t.Fatalf("value = %q, want %q", value, "payload")
	}
}

func TestMemoryReportsExpiredEntriesAsAbsentAndDropsThem(t *testing.T) {
	t.Parallel()
	now := time.Now()
	memory := NewMemoryWithClock(func() time.Time { return now })

	if err := memory.Set(context.Background(), "session:a", []byte("payload"), time.Minute); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)

	if _, found, err := memory.Get(context.Background(), "session:a"); err != nil || found {
		t.Fatalf("Get after expiry = (found %v, err %v), want (false, nil)", found, err)
	}
	// An expired read must evict, so a stale token cannot be probed repeatedly.
	if memory.Len() != 0 {
		t.Fatalf("live entries = %d, want 0", memory.Len())
	}
}

func TestMemoryTreatsNonPositiveTTLAsNoExpiry(t *testing.T) {
	t.Parallel()
	now := time.Now()
	memory := NewMemoryWithClock(func() time.Time { return now })

	if err := memory.Set(context.Background(), "key", []byte("value"), 0); err != nil {
		t.Fatal(err)
	}
	now = now.Add(100 * time.Hour)

	if _, found, err := memory.Get(context.Background(), "key"); err != nil || !found {
		t.Fatalf("Get = (found %v, err %v), want (true, nil)", found, err)
	}
}

func TestMemoryDoesNotAliasStoredBytes(t *testing.T) {
	t.Parallel()
	memory := NewMemory()
	original := []byte("payload")

	if err := memory.Set(context.Background(), "key", original, time.Minute); err != nil {
		t.Fatal(err)
	}
	original[0] = 'X'

	value, _, err := memory.Get(context.Background(), "key")
	if err != nil {
		t.Fatal(err)
	}
	if string(value) != "payload" {
		t.Fatalf("value = %q, want %q: caller mutation reached the cache", value, "payload")
	}

	value[0] = 'Y'
	again, _, err := memory.Get(context.Background(), "key")
	if err != nil {
		t.Fatal(err)
	}
	if string(again) != "payload" {
		t.Fatalf("value = %q, want %q: returned slice aliases the cache", again, "payload")
	}
}

func TestMemoryDeleteIsIdempotent(t *testing.T) {
	t.Parallel()
	memory := NewMemory()

	if err := memory.Delete(context.Background(), "absent"); err != nil {
		t.Fatalf("deleting an absent key returned %v, want nil", err)
	}
}

func TestMemoryKeysFiltersByPrefixAndSkipsExpired(t *testing.T) {
	t.Parallel()
	now := time.Now()
	memory := NewMemoryWithClock(func() time.Time { return now })
	ctx := context.Background()

	for key, ttl := range map[string]time.Duration{
		"session:live":    time.Hour,
		"session:expired": time.Minute,
		"other:live":      time.Hour,
	} {
		if err := memory.Set(ctx, key, []byte("v"), ttl); err != nil {
			t.Fatal(err)
		}
	}
	now = now.Add(30 * time.Minute)

	keys, err := memory.Keys(ctx, "session:")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0] != "session:live" {
		t.Fatalf("Keys = %v, want [session:live]", keys)
	}
}

func TestMemorySweepReclaimsExpiredEntries(t *testing.T) {
	t.Parallel()
	now := time.Now()
	memory := NewMemoryWithClock(func() time.Time { return now })
	ctx := context.Background()

	if err := memory.Set(ctx, "a", []byte("v"), time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := memory.Set(ctx, "b", []byte("v"), time.Hour); err != nil {
		t.Fatal(err)
	}
	now = now.Add(30 * time.Minute)

	if removed := memory.Sweep(); removed != 1 {
		t.Fatalf("Sweep removed %d, want 1", removed)
	}
	if memory.Len() != 1 {
		t.Fatalf("live entries = %d, want 1", memory.Len())
	}
}

func TestMemoryRespectsCancelledContext(t *testing.T) {
	t.Parallel()
	memory := NewMemory()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := memory.Set(ctx, "key", []byte("v"), time.Minute); err == nil {
		t.Fatal("Set with a cancelled context returned nil, want an error")
	}
	if _, _, err := memory.Get(ctx, "key"); err == nil {
		t.Fatal("Get with a cancelled context returned nil, want an error")
	}
}
