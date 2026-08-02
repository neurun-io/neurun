package deployment

import (
	"errors"
	"strings"
	"testing"
	"time"
)

var fixtureTime = time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)

func uploadedFixture(t *testing.T) Deployment {
	t.Helper()
	digest := strings.Repeat("a", 64)
	record, err := New(
		"dep_fixture", "prj_fixture", "app_fixture",
		RuntimePython, "main.py:handler",
		Artifact{
			ID: "art_source", Kind: ArtifactSource, Name: "source.zip",
			MediaType: "application/zip", SizeBytes: 12, SHA256: digest,
			StorageKey: "objects/sha256/aa/" + digest, CreatedAt: fixtureTime,
		},
		fixtureTime,
	)
	if err != nil {
		t.Fatalf("build fixture: %v", err)
	}
	return record
}

func codeLayer() []Artifact {
	digest := strings.Repeat("b", 64)
	return []Artifact{{
		ID: "art_code", Kind: ArtifactCodeLayer, Name: "code.zip",
		MediaType: "application/zip", SizeBytes: 24, SHA256: digest,
		StorageKey: "objects/sha256/bb/" + digest, CreatedAt: fixtureTime,
	}}
}

// A deployment's own status mirrors its newest build, and build numbers stay
// contiguous. Both are invariants Validate enforces, so the transitions have to
// maintain them without the caller helping.
func TestBuildTransitionsKeepDeploymentConsistent(t *testing.T) {
	t.Parallel()
	record := uploadedFixture(t)
	if record.Status != StatusUploaded || len(record.Builds) != 0 {
		t.Fatalf("new deployment = %#v", record)
	}

	build, err := record.StartBuild("bld_one", fixtureTime.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if build.Number != 1 || build.Status != StatusBuilding ||
		record.Status != StatusBuilding {
		t.Fatalf("after StartBuild: build=%#v status=%s", build, record.Status)
	}
	if err := record.Validate(); err != nil {
		t.Fatalf("building deployment invalid: %v", err)
	}

	if err := record.MarkBuildReady(
		"bld_one", codeLayer(), fixtureTime.Add(2*time.Second),
	); err != nil {
		t.Fatal(err)
	}
	if record.Status != StatusReady {
		t.Fatalf("after MarkBuildReady status = %s", record.Status)
	}
	if err := record.Validate(); err != nil {
		t.Fatalf("ready deployment invalid: %v", err)
	}
	ready, ok := record.ReadyBuild()
	if !ok || ready.ID != "bld_one" {
		t.Fatalf("ReadyBuild = %#v, %v", ready, ok)
	}

	second, err := record.StartBuild("bld_two", fixtureTime.Add(3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if second.Number != 2 {
		t.Fatalf("second build number = %d", second.Number)
	}
	if err := record.FailBuild(
		"bld_two",
		Failure{Code: "build_failed", Message: "toolchain exploded"},
		fixtureTime.Add(4*time.Second),
	); err != nil {
		t.Fatal(err)
	}
	if record.Status != StatusFailed {
		t.Fatalf("after FailBuild status = %s", record.Status)
	}
	if err := record.Validate(); err != nil {
		t.Fatalf("failed deployment invalid: %v", err)
	}
	// The earlier ready build is still the one an execution would pin to.
	if ready, ok := record.ReadyBuild(); !ok || ready.ID != "bld_one" {
		t.Fatalf("ReadyBuild after failure = %#v, %v", ready, ok)
	}
}

// Finishing a build twice must not be possible: the second attempt would
// overwrite a terminal record that an execution may already be pinned to.
func TestFinishedBuildCannotBeFinishedAgain(t *testing.T) {
	t.Parallel()
	record := uploadedFixture(t)
	if _, err := record.StartBuild("bld_one", fixtureTime.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := record.MarkBuildReady(
		"bld_one", codeLayer(), fixtureTime.Add(2*time.Second),
	); err != nil {
		t.Fatal(err)
	}
	if err := record.MarkBuildReady(
		"bld_one", codeLayer(), fixtureTime.Add(3*time.Second),
	); !errors.Is(err, ErrInvalid) {
		t.Fatalf("second MarkBuildReady error = %v", err)
	}
	if err := record.FailBuild(
		"bld_one", Failure{Code: "x", Message: "y"}, fixtureTime.Add(3*time.Second),
	); !errors.Is(err, ErrInvalid) {
		t.Fatalf("FailBuild on ready build error = %v", err)
	}
	if err := record.FailBuild(
		"bld_missing", Failure{Code: "x", Message: "y"}, fixtureTime,
	); !errors.Is(err, ErrNotFound) {
		t.Fatalf("FailBuild on unknown build error = %v", err)
	}
}

// Recovery fails a build a crash left running, and leaves everything else be.
func TestFailInterruptedBuildOnlyTouchesBuildingDeployments(t *testing.T) {
	t.Parallel()
	failure := Failure{Code: "build_interrupted", Message: "restarted"}
	now := fixtureTime.Add(time.Minute)

	uploaded := uploadedFixture(t)
	if uploaded.FailInterruptedBuild(now, failure) {
		t.Fatal("uploaded deployment reported as recovered")
	}

	building := uploadedFixture(t)
	if _, err := building.StartBuild("bld_one", fixtureTime.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if !building.FailInterruptedBuild(now, failure) {
		t.Fatal("building deployment was not recovered")
	}
	if building.Status != StatusFailed || building.Builds[0].Failure == nil {
		t.Fatalf("recovered deployment = %#v", building)
	}
	if err := building.Validate(); err != nil {
		t.Fatalf("recovered deployment invalid: %v", err)
	}
	// Recovery is idempotent: a second pass has nothing left to do.
	if building.FailInterruptedBuild(now, failure) {
		t.Fatal("already-failed deployment reported as recovered again")
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
