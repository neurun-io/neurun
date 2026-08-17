package execution

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

var fixtureTime = time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)

func queuedFixture(t *testing.T) Execution {
	t.Helper()
	record, err := New(
		"exe_one", "prj_fixture", "dep_fixture", "bld_fixture",
		json.RawMessage(`{"message":"hello"}`), fixtureTime,
	)
	if err != nil {
		t.Fatalf("build fixture: %v", err)
	}
	return record
}

// The queued → running → terminal walk is the whole state machine, and each
// step has to leave a record Validate accepts.
func TestExecutionWalksQueuedToTerminal(t *testing.T) {
	t.Parallel()
	record := queuedFixture(t)
	if record.Status != StatusQueued || record.StartedAt != nil {
		t.Fatalf("new execution = %#v", record)
	}

	if err := record.Claim(fixtureTime.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if record.Status != StatusRunning || record.StartedAt == nil ||
		record.FinishedAt != nil {
		t.Fatalf("after Claim = %#v", record)
	}
	// A second claim must not restart work already in flight.
	if err := record.Claim(fixtureTime.Add(2 * time.Second)); !errors.Is(err, ErrNotQueued) {
		t.Fatalf("second Claim error = %v", err)
	}

	if err := record.Succeed(
		json.RawMessage(`{"ok":true}`), "one line\n", fixtureTime.Add(3*time.Second),
	); err != nil {
		t.Fatal(err)
	}
	if record.Status != StatusSucceeded || !record.Status.Terminal() ||
		record.FinishedAt == nil || record.Logs == "" {
		t.Fatalf("after Succeed = %#v", record)
	}
	if err := record.Succeed(
		json.RawMessage(`{"ok":false}`), "", fixtureTime.Add(4*time.Second),
	); !errors.Is(err, ErrConflict) {
		t.Fatalf("Succeed on terminal execution error = %v", err)
	}
}

// Failing carries the reason and drops any output, so a failed record can never
// be read as if it produced a result.
func TestFailClearsOutputAndRequiresRunning(t *testing.T) {
	t.Parallel()
	record := queuedFixture(t)
	if err := record.Fail(
		Failure{Code: "boom", Message: "nope"}, "", fixtureTime,
	); !errors.Is(err, ErrConflict) {
		t.Fatalf("Fail on queued execution error = %v", err)
	}
	if err := record.Claim(fixtureTime.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := record.Fail(
		Failure{Code: "failed", Message: "traceback"},
		"stderr\n", fixtureTime.Add(2*time.Second),
	); err != nil {
		t.Fatal(err)
	}
	if record.Output != nil || record.Failure == nil || record.Status != StatusFailed {
		t.Fatalf("after Fail = %#v", record)
	}
}

// A rerun repeats the exact build and input. That pinning is the only reason a
// rerun means anything, so a newer build must not be able to change it.
func TestRerunPinsSameBuildAndInput(t *testing.T) {
	t.Parallel()
	record := queuedFixture(t)
	if _, err := record.Rerun("exe_two", fixtureTime); !errors.Is(err, ErrInvalid) {
		t.Fatalf("rerun of unfinished execution error = %v", err)
	}
	if err := record.Claim(fixtureTime.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := record.Succeed(
		json.RawMessage(`1`), "", fixtureTime.Add(2*time.Second),
	); err != nil {
		t.Fatal(err)
	}
	rerun, err := record.Rerun("exe_two", fixtureTime.Add(3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if rerun.BuildID != record.BuildID ||
		string(rerun.Input) != string(record.Input) ||
		rerun.RerunOfExecutionID != record.ID ||
		rerun.Status != StatusQueued {
		t.Fatalf("rerun = %#v", rerun)
	}
	if rerun.Output != nil || rerun.FinishedAt != nil {
		t.Fatalf("rerun carried terminal state: %#v", rerun)
	}
}

// Finalizing is a compare-and-set: the caller has to be finishing the same
// running record it started from.
func TestValidateTransitionToRejectsChangedMetadata(t *testing.T) {
	t.Parallel()
	running := queuedFixture(t)
	if err := running.Claim(fixtureTime.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	finished := Clone(running)
	if err := finished.Succeed(
		json.RawMessage(`{}`), "", fixtureTime.Add(2*time.Second),
	); err != nil {
		t.Fatal(err)
	}
	if err := running.ValidateTransitionTo(finished); err != nil {
		t.Fatalf("clean transition rejected: %v", err)
	}

	tampered := Clone(finished)
	tampered.Input = json.RawMessage(`{"other":1}`)
	if err := running.ValidateTransitionTo(tampered); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed input accepted: %v", err)
	}
	tampered = Clone(finished)
	tampered.BuildID = "bld_other"
	if err := running.ValidateTransitionTo(tampered); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed build accepted: %v", err)
	}
	if err := finished.ValidateTransitionTo(finished); !errors.Is(err, ErrConflict) {
		t.Fatalf("terminal-to-terminal transition accepted: %v", err)
	}
}

// UseNumber keeps a large integer exact. Decoding into float64 would round it,
// and the handler would be invoked with a different number than was submitted.
func TestNormalizeInputIsCanonicalAndLossless(t *testing.T) {
	t.Parallel()
	normalized, err := NormalizeInput(
		json.RawMessage(` { "b" : 2, "a" : 1 } `), 1_048_576,
	)
	if err != nil {
		t.Fatal(err)
	}
	if string(normalized) != `{"a":1,"b":2}` {
		t.Fatalf("normalized = %s", normalized)
	}

	big := json.RawMessage(`9007199254740993`)
	normalized, err = NormalizeInput(big, 1_048_576)
	if err != nil || string(normalized) != string(big) {
		t.Fatalf("large integer normalized to %s (%v)", normalized, err)
	}

	if _, err := NormalizeInput(json.RawMessage(`{} {}`), 1_048_576); !errors.Is(err, ErrInvalid) {
		t.Fatalf("two values accepted: %v", err)
	}
	if _, err := NormalizeInput(json.RawMessage(`{"a":1}`), 4); !errors.Is(err, ErrInvalid) {
		t.Fatalf("oversized input accepted: %v", err)
	}
	if _, err := NormalizeInput(json.RawMessage(`null`), 1_048_576); err != nil {
		t.Fatalf("null input rejected: %v", err)
	}
}
