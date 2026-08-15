package database

import (
	"strings"
	"testing"
	"time"

	"github.com/neurun-io/neurun/internal/ids"
)

// PostgreSQL rejects a NUL byte in any text value, so an advisory-lock key that
// embedded one failed the enclosing transaction with SQLSTATE 22021 instead of
// taking a lock. That once made every deployment save fail.
func TestAdvisoryKeyIsValidPostgresText(t *testing.T) {
	t.Parallel()
	key := advisoryKey("deployment", "prj_local", "dep_01HXQ8F2")
	if strings.ContainsRune(key, '\x00') {
		t.Fatalf("advisory key contains NUL: %q", key)
	}
}

// The separator has to be a character ids.Validate rejects, otherwise two
// different part sequences could hash to the same lock.
func TestAdvisoryKeySeparatorCannotAppearInIdentifiers(t *testing.T) {
	t.Parallel()
	left := advisoryKey("deployment", "prj_a", "b_dep")
	right := advisoryKey("deployment", "prj_a_b", "dep")
	if left == right {
		t.Fatalf("distinct part sequences collided: %q", left)
	}
	if err := ids.Validate("separator", advisoryKey("", "")); err == nil {
		t.Fatal("the separator is a legal identifier character")
	}
}

// A minted timestamp must already carry the precision PostgreSQL keeps.
// Claiming an execution with a nanosecond StartedAt stored a truncated value
// but returned the untruncated one, so ValidateTransitionTo saw the two differ
// and rejected every finalization — which killed the worker, and with it the
// server.
func TestPostgresTimeMatchesStoredPrecision(t *testing.T) {
	t.Parallel()
	minted := time.Date(2026, 7, 31, 19, 28, 58, 149550610, time.UTC)
	normalized := postgresTime(minted)
	if normalized.Nanosecond()%1000 != 0 {
		t.Fatalf("normalized time keeps sub-microsecond precision: %d", normalized.Nanosecond())
	}
	// The driver truncates rather than rounds, so .149550610 must become
	// .149550, never .149551.
	want := time.Date(2026, 7, 31, 19, 28, 58, 149550000, time.UTC)
	if !normalized.Equal(want) {
		t.Fatalf("normalized = %s, want %s",
			normalized.Format(time.RFC3339Nano), want.Format(time.RFC3339Nano))
	}
	// Normalizing is idempotent, so a value read back and re-normalized still
	// compares equal.
	if again := postgresTime(normalized); !again.Equal(normalized) {
		t.Fatalf("postgresTime is not idempotent: %s", again.Format(time.RFC3339Nano))
	}
}
