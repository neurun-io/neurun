package dto

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/neurun-io/neurun/internal/domain/build"
	"github.com/neurun-io/neurun/internal/domain/deployment"
)

// The blob handle names internal storage topology and must never reach a
// client. It used to be unexported, which made leaking it impossible; it is now
// an ordinary field, so this projection is the only thing keeping it off the
// wire. Marshal the response, not the domain record, and prove the handle is
// absent.
func TestResponsesNeverCarryTheStorageHandle(t *testing.T) {
	t.Parallel()
	sourceDigest := strings.Repeat("a", 64)
	codeDigest := strings.Repeat("b", 64)
	now := time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)
	finished := now.Add(time.Second)
	sourceKey := "objects/sha256/aa/" + sourceDigest
	codeKey := "objects/sha256/bb/" + codeDigest

	record := deployment.Deployment{
		ID: "dep_fixture", ProjectID: "prj_fixture", AppID: "app_fixture",
		Runtime: build.RuntimePython, EntryPoint: "main.py:handler",
		Status: deployment.StatusReady,
		Source: build.Artifact{
			ID: "art_source", Name: "source",
			SizeBytes: 12, SHA256: sourceDigest,
			StorageKey: sourceKey, CreatedAt: now,
		},
		Build: &build.Build{
			ID: "bld_fixture", Runtime: build.RuntimePython,
			EntryPoint: "main.py:handler", SourceSHA256: sourceDigest,
			Artifacts: []build.Artifact{{
				ID: "art_code", Name: build.LayerCode,
				SizeBytes: 24, SHA256: codeDigest,
				StorageKey: codeKey, CreatedAt: now,
			}},
			CreatedAt: finished,
		},
		StartedAt: &now, FinishedAt: &finished,
		CreatedAt: now, UpdatedAt: finished,
	}

	encoded, err := json.Marshal(NewDeploymentResponse(record))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"storage_key", sourceKey, codeKey} {
		if bytes.Contains(encoded, []byte(forbidden)) {
			t.Fatalf("response leaked %q: %s", forbidden, encoded)
		}
	}
	// The digest is public — it is how a caller verifies what it uploaded.
	if !bytes.Contains(encoded, []byte(sourceDigest)) {
		t.Fatalf("response dropped the source digest: %s", encoded)
	}

	// The domain record itself does serialize the handle, which is what makes
	// projecting mandatory rather than merely tidy.
	direct, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(direct, []byte("storage_key")) {
		t.Fatal("domain record no longer carries storage_key; this guard is now testing nothing")
	}

	if encoded, err = json.Marshal(NewBuildResponse(*record.Build)); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte(codeKey)) {
		t.Fatalf("build response leaked the artifact handle: %s", encoded)
	}
}
