package ids

import (
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
}

func TestNewRejectsInvalidPrefix(t *testing.T) {
	t.Parallel()

	for _, prefix := range []string{"", "Job", "too_long_prefix", "job-"} {
		if _, err := New(prefix); err == nil {
			t.Errorf("New(%q) should fail", prefix)
		}
	}
}
