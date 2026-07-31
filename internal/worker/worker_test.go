package worker

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/neurun-io/neurun/internal/artifact"
	"github.com/neurun-io/neurun/internal/deployment"
)

type fixtureBuilder struct{ archive string }

func (builder fixtureBuilder) Build(context.Context, deployment.BuildRequest) (deployment.BuildResult, error) {
	return deployment.BuildResult{Artifacts: []deployment.BuiltArtifact{{Kind: deployment.ArtifactCodeLayer, Name: "code-layer.zip", MediaType: "application/zip", Path: builder.archive}}}, nil
}

type fixtureRunner struct{}

func (fixtureRunner) Execute(_ context.Context, request ExecuteRequest) (ExecuteResult, error) {
	return ExecuteResult{
		Output: append(json.RawMessage(nil), request.Input...),
		Logs:   "scraper completed",
	}, nil
}

func TestWorkerExecutesPinnedBuildAndFinalizesRun(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.zip")
	layer := filepath.Join(root, "layer.zip")
	writeArchive(t, source, "main.py", "def handler(event):\n    return event\n")
	writeArchive(t, layer, "main.py", "def handler(event):\n    return event\n")
	store := deployment.NewMemoryStore()
	blobs := &artifact.MemoryStore{}
	service, err := deployment.NewService(store, blobs, fixtureBuilder{archive: layer}, deployment.ServiceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.EnsureProject(context.Background(), "project", "project"); err != nil {
		t.Fatal(err)
	}
	app, err := service.CreateApp(context.Background(), deployment.CreateAppRequest{
		ProjectID: "project",
		Name:      "worker-fixture",
	})
	if err != nil {
		t.Fatal(err)
	}
	sourceFile, err := os.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	record, err := service.Create(context.Background(), deployment.CreateRequest{ProjectID: "project", AppID: app.ID, Runtime: deployment.RuntimePython, EntryPoint: "main.py:handler", SourceName: "source.zip", Source: sourceFile})
	sourceFile.Close()
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.CreateRun(context.Background(), deployment.CreateRunRequest{ProjectID: "project", DeploymentID: record.ID, Input: json.RawMessage(`{"ok":true}`)})
	if err != nil {
		t.Fatal(err)
	}
	executor, err := New(store, blobs, fixtureRunner{}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := executor.ProcessOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	finished, err := store.GetRun(context.Background(), "project", run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finished.Status != deployment.RunSucceeded ||
		!bytes.Equal(finished.Output, run.Input) ||
		finished.Logs != "scraper completed" {
		t.Fatalf("unexpected finished run: %#v", finished)
	}
}

func TestRecoverMarksInterruptedRunsFailed(t *testing.T) {
	store := deployment.NewMemoryStore()
	now := time.Now().UTC()
	run := deployment.Run{ID: "run_one", ProjectID: "project", DeploymentID: "dep_one", BuildID: "bld_one", Status: deployment.RunQueued, Input: json.RawMessage(`null`), CreatedAt: now}
	if err := store.CreateRun(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ClaimQueuedRun(context.Background(), now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	executor, err := New(store, &artifact.MemoryStore{}, fixtureRunner{}, Options{Now: func() time.Time { return now.Add(2 * time.Second) }})
	if err != nil {
		t.Fatal(err)
	}
	count, err := executor.Recover(context.Background())
	if err != nil || count != 1 {
		t.Fatalf("recover: count=%d err=%v", count, err)
	}
	finished, _ := store.GetRun(context.Background(), "project", "run_one")
	if finished.Status != deployment.RunFailed || finished.Failure == nil || finished.Failure.Code != "worker_restarted" {
		t.Fatalf("unexpected recovered run: %#v", finished)
	}
}

func writeArchive(t *testing.T, target, name, contents string) {
	t.Helper()
	output, err := os.Create(target)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(output)
	writer, err := archive.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte(contents)); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
}
