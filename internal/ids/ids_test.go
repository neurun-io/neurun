package ids

import (
	"bytes"
	"regexp"
	"testing"
)

func TestNew(t *testing.T) {
	t.Parallel()

	first, err := New("job")
	if err != nil {
		t.Fatal(err)
	}
	second, err := New("job")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("generated duplicate IDs")
	}
	if !regexp.MustCompile(`^job_[0-9A-HJKMNP-TV-Z]{26}$`).MatchString(first) {
		t.Fatalf("unexpected ID %q", first)
	}
	if Prefix(first) != "job" {
		t.Fatalf("Prefix(%q) = %q", first, Prefix(first))
	}
}

func TestNewRejectsInvalidPrefix(t *testing.T) {
	t.Parallel()

	for _, prefix := range []string{"", "Job", "too_long_prefix", "job-"} {
		if _, err := New(prefix); err == nil {
			t.Errorf("New(%q) should fail", prefix)
		}
	}
}

func TestTraceAndSpan(t *testing.T) {
	t.Parallel()

	trace, err := Trace()
	if err != nil {
		t.Fatal(err)
	}
	span, err := Span()
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(trace) {
		t.Fatalf("invalid trace %q", trace)
	}
	if !regexp.MustCompile(`^[0-9a-f]{16}$`).MatchString(span) {
		t.Fatalf("invalid span %q", span)
	}
}

func TestEnsureNonZero(t *testing.T) {
	t.Parallel()

	zero := make([]byte, 16)
	ensureNonZero(zero)
	if bytes.Equal(zero, make([]byte, 16)) {
		t.Fatal("all-zero trace identifier was not repaired")
	}
	if zero[len(zero)-1] != 1 {
		t.Fatalf("repaired identifier = %x", zero)
	}

	nonzero := []byte{3, 0, 0}
	ensureNonZero(nonzero)
	if !bytes.Equal(nonzero, []byte{3, 0, 0}) {
		t.Fatalf("nonzero identifier was changed to %x", nonzero)
	}
}
