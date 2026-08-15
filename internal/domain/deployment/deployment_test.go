package deployment

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/neurun-io/neurun/internal/domain/build"
)

var fixtureTime = time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)

// sourceDigest is what the deployment service hashes off the archive it
// spooled, and hands to the build it starts.
var sourceDigest = strings.Repeat("a", 64)

func queuedFixture(t *testing.T) Deployment {
	t.Helper()
	record, err := New(
		"dep_fixture", "prj_fixture", "app_fixture",
		build.RuntimePython, fixtureTime,
	)
	if err != nil {
		t.Fatalf("build fixture: %v", err)
	}
	return record
}

func codeLayer() []build.Artifact {
	digest := strings.Repeat("b", 64)
	return []build.Artifact{{
		ID: "art_code", Name: build.LayerCode,
		SizeBytes: 24, SHA256: digest,
		StorageKey: "objects/sha256/bb/" + digest, CreatedAt: fixtureTime,
	}}
}

// sealed is what the deployment service hands over: a build already made.
func sealed(t *testing.T, record Deployment, buildID string, now time.Time) build.Build {
	t.Helper()
	produced, err := build.New(
		buildID, record.AppID, record.ID, record.Runtime,
		sourceDigest, codeLayer(), now,
	)
	if err != nil {
		t.Fatal(err)
	}
	return produced
}

// The stages are the deployment's whole life, and every one of them has to hold
// the invariants Validate enforces without the caller helping.
func TestStagesCarryTheDeploymentToItsBuild(t *testing.T) {
	t.Parallel()
	record := queuedFixture(t)
	if record.Status != StatusQueued || record.Build != nil || record.StartedAt != nil {
		t.Fatalf("new deployment = %#v", record)
	}

	if err := record.Advance(fixtureTime.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if record.Status != StatusBuilding || record.StartedAt == nil {
		t.Fatalf("after first advance = %#v", record)
	}
	if err := record.Validate(); err != nil {
		t.Fatalf("building deployment invalid: %v", err)
	}

	if err := record.Advance(fixtureTime.Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if record.Status != StatusPublishing {
		t.Fatalf("after second advance status = %s", record.Status)
	}
	if err := record.Validate(); err != nil {
		t.Fatalf("publishing deployment invalid: %v", err)
	}

	if err := record.MarkReady(
		sealed(t, record, "bld_one", fixtureTime.Add(3*time.Second)), fixtureTime.Add(3*time.Second),
	); err != nil {
		t.Fatal(err)
	}
	if record.Status != StatusReady || record.Build == nil ||
		record.Build.ID != "bld_one" || record.FinishedAt == nil {
		t.Fatalf("after MarkReady = %#v", record)
	}
	if err := record.Validate(); err != nil {
		t.Fatalf("ready deployment invalid: %v", err)
	}
	// The build carries what it takes to run the artifacts, and nothing about
	// how the deployment went.
	if record.Build.SourceSHA256 != sourceDigest ||
		record.Build.AppID != record.AppID ||
		record.Build.DeploymentID != record.ID {
		t.Fatalf("build = %#v", record.Build)
	}
}

// A build is something that came out. A deployment that dies before the
// toolchain ran produced nothing, so it has a failure and no build.
func TestFailureLeavesNoBuild(t *testing.T) {
	t.Parallel()
	record := queuedFixture(t)
	if err := record.Fail(
		Failure{Code: "source_unavailable", Message: "archive was gone"},
		fixtureTime.Add(time.Second),
	); err != nil {
		t.Fatal(err)
	}
	if record.Status != StatusFailed || record.Build != nil ||
		record.Failure == nil || record.FinishedAt == nil {
		t.Fatalf("failed deployment = %#v", record)
	}
	if record.StartedAt != nil {
		t.Fatal("a deployment that never built reported a start time")
	}
	if err := record.Validate(); err != nil {
		t.Fatalf("failed deployment invalid: %v", err)
	}
}

// Finishing twice must not be possible: the second attempt would overwrite a
// terminal record an execution may already be pinned to.
func TestFinishedDeploymentCannotFinishAgain(t *testing.T) {
	t.Parallel()
	record := queuedFixture(t)
	if err := record.Advance(fixtureTime.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := record.MarkReady(
		sealed(t, record, "bld_one", fixtureTime.Add(2*time.Second)), fixtureTime.Add(2*time.Second),
	); err != nil {
		t.Fatal(err)
	}
	if err := record.MarkReady(
		sealed(t, record, "bld_two", fixtureTime.Add(3*time.Second)), fixtureTime.Add(3*time.Second),
	); !errors.Is(err, ErrInvalid) {
		t.Fatalf("second MarkReady error = %v", err)
	}
	if err := record.Fail(
		Failure{Code: "x", Message: "y"}, fixtureTime.Add(3*time.Second),
	); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Fail on a ready deployment error = %v", err)
	}
	if err := record.Advance(fixtureTime.Add(3 * time.Second)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Advance on a ready deployment error = %v", err)
	}
}

// Recovery fails what a crash left running, and leaves everything else be.
func TestFailInterruptedOnlyTouchesRunningDeployments(t *testing.T) {
	t.Parallel()
	failure := Failure{Code: "build_interrupted", Message: "restarted"}
	now := fixtureTime.Add(time.Minute)

	building := queuedFixture(t)
	if err := building.Advance(fixtureTime.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if !building.FailInterrupted(now, failure) {
		t.Fatal("building deployment was not recovered")
	}
	if building.Status != StatusFailed || building.Failure == nil {
		t.Fatalf("recovered deployment = %#v", building)
	}
	if err := building.Validate(); err != nil {
		t.Fatalf("recovered deployment invalid: %v", err)
	}
	// Recovery is idempotent: a second pass has nothing left to do.
	if building.FailInterrupted(now, failure) {
		t.Fatal("already-failed deployment reported as recovered again")
	}

	ready := queuedFixture(t)
	if err := ready.Advance(fixtureTime.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := ready.MarkReady(
		sealed(t, ready, "bld_one", fixtureTime.Add(2*time.Second)), fixtureTime.Add(2*time.Second),
	); err != nil {
		t.Fatal(err)
	}
	if ready.FailInterrupted(now, failure) {
		t.Fatal("ready deployment reported as recovered")
	}
}

// The end of a build is where the error is, so that is the end that survives.
func TestLogKeepsTheTail(t *testing.T) {
	t.Parallel()
	record := queuedFixture(t)
	record.Log(strings.Repeat("a", MaxLogBytes))
	record.Log("error[E0433]: failed to resolve\n")

	if len(record.Logs) != MaxLogBytes {
		t.Fatalf("logs = %d bytes", len(record.Logs))
	}
	if !strings.HasSuffix(record.Logs, "error[E0433]: failed to resolve\n") {
		t.Fatal("the newest output was trimmed instead of the oldest")
	}
	if err := record.Validate(); err != nil {
		t.Fatalf("deployment with capped logs invalid: %v", err)
	}
}

// Identifiers reach filesystem paths, where the legacy DOS device names resolve
// to a device rather than a file regardless of directory or suffix.
func TestIdentifiersRejectWindowsReservedDeviceNames(t *testing.T) {
	t.Parallel()
	for _, value := range []string{
		"CON", "con.txt", "PrN", "AUX.json", "NUL", "COM1", "com9.log",
		"LPT1", "lpt9.bin",
	} {
		if err := ValidateIdentifier("id", value); !errors.Is(err, ErrInvalid) {
			t.Fatalf("ValidateIdentifier(%q) = %v", value, err)
		}
	}
	for _, value := range []string{"CONSOLE", "COM10", "LPT0", "prn_job"} {
		if err := ValidateIdentifier("id", value); err != nil {
			t.Fatalf("valid identifier %q rejected: %v", value, err)
		}
	}
}
