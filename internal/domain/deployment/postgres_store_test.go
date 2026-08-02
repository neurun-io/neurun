package deployment

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// PostgreSQL rejects a NUL byte in any text value, so an advisory-lock key that
// embeds one fails the enclosing transaction with SQLSTATE 22021 instead of
// taking a lock. That once made every SaveDeployment fail.
func TestAdvisoryKeyIsValidPostgresText(t *testing.T) {
	t.Parallel()
	key := advisoryKey("deployment", "prj_local", "dep_01HXQ8F2")
	if strings.ContainsRune(key, '\x00') {
		t.Fatalf("advisory key contains NUL: %q", key)
	}
}

// A minted timestamp must already carry the precision PostgreSQL will keep.
// Claiming an execution with a nanosecond StartedAt stored a truncated value but
// returned the untruncated one, so ValidateTransitionTo saw the two differ and
// rejected every finalization — which killed the worker, and with it the server.
func TestPostgresTimeMatchesStoredPrecision(t *testing.T) {
	t.Parallel()
	minted := time.Date(2026, 7, 31, 19, 28, 58, 149550610, time.UTC)
	normalized := postgresTime(minted)
	if normalized.Nanosecond()%1000 != 0 {
		t.Fatalf("normalized time keeps sub-microsecond precision: %d", normalized.Nanosecond())
	}
	// The driver truncates rather than rounds, so .149550610 must become
	// .149550, never .149551.
	if want := time.Date(2026, 7, 31, 19, 28, 58, 149550000, time.UTC); !normalized.Equal(want) {
		t.Fatalf("normalized = %s, want %s", normalized.Format(time.RFC3339Nano), want.Format(time.RFC3339Nano))
	}
	// Normalizing is idempotent, so a value read back and re-normalized still
	// compares equal.
	if again := postgresTime(normalized); !again.Equal(normalized) {
		t.Fatalf("postgresTime is not idempotent: %s", again.Format(time.RFC3339Nano))
	}
}

// An absent failure has to reach PostgreSQL as SQL NULL. Storing the JSON
// literal null instead read back as a non-nil, empty failure, which failed
// validateBuild and made every built deployment unreadable.
func TestNullableJSONMapsAbsentFailureToSQLNull(t *testing.T) {
	t.Parallel()
	var absent *Failure
	encoded, err := nullableJSON(absent)
	if err != nil {
		t.Fatalf("encode absent failure: %v", err)
	}
	if encoded != nil {
		t.Fatalf("absent failure encoded as %#v, want untyped nil", encoded)
	}

	present, err := nullableJSON(&Failure{Code: "build_failed", Message: "boom"})
	if err != nil {
		t.Fatalf("encode present failure: %v", err)
	}
	body, ok := present.([]byte)
	if !ok {
		t.Fatalf("present failure encoded as %T, want []byte", present)
	}
	if string(body) == "null" {
		t.Fatal("present failure encoded as the JSON literal null")
	}
}

// Rows written before the fix hold the literal null, so decoding must still
// yield an absent failure rather than an empty one.
func TestBuildFailureDecodesJSONNullAsAbsent(t *testing.T) {
	t.Parallel()
	var failure *Failure
	if err := json.Unmarshal([]byte("null"), &failure); err != nil {
		t.Fatalf("decode null failure: %v", err)
	}
	if failure != nil {
		t.Fatalf("JSON null decoded to %#v, want nil", failure)
	}
	if err := json.Unmarshal([]byte(`{"code":"build_failed"}`), &failure); err != nil {
		t.Fatalf("decode real failure: %v", err)
	}
	if failure == nil || failure.Code != "build_failed" {
		t.Fatalf("real failure decoded to %#v", failure)
	}
}

// The separator must be a character ValidateIdentifier rejects, otherwise two
// different part sequences could hash to the same lock.
func TestAdvisoryKeySeparatorCannotAppearInIdentifiers(t *testing.T) {
	t.Parallel()
	left := advisoryKey("deployment", "prj_a", "b_dep")
	right := advisoryKey("deployment", "prj_a_b", "dep")
	if left == right {
		t.Fatalf("distinct part sequences collided: %q", left)
	}
	// Joining two empty parts yields the separator on its own.
	separator := advisoryKey("", "")
	if err := ValidateIdentifier("separator", separator); err == nil {
		t.Fatalf("separator %q is a legal identifier character", separator)
	}
}
