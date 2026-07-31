package deployment

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/neurun-io/neurun/internal/artifact"
)

type serviceFixtureBuilder struct {
	failure error
}

func (builder serviceFixtureBuilder) Build(
	_ context.Context,
	request BuildRequest,
) (BuildResult, error) {
	if builder.failure != nil {
		return BuildResult{}, builder.failure
	}
	if _, err := os.Stat(request.SourceArchivePath); err != nil {
		return BuildResult{}, err
	}
	target := filepath.Join(request.WorkDirectory, "fixture-code.zip")
	file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return BuildResult{}, err
	}
	archive := zip.NewWriter(file)
	entry, err := archive.Create("main.py")
	if err == nil {
		_, err = entry.Write([]byte("def handler(event):\n    return event\n"))
	}
	err = errors.Join(err, archive.Close(), file.Close())
	if err != nil {
		return BuildResult{}, err
	}
	return BuildResult{Artifacts: []BuiltArtifact{{
		Kind: ArtifactCodeLayer, Name: "code.zip",
		MediaType: "application/zip", Path: target,
	}}}, nil
}

func TestServiceCreatesBuildAndExactExecutionRerun(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryStore()
	clock := newFixtureClock(time.Date(2026, 7, 30, 8, 0, 0, 0, time.UTC))
	service, err := NewService(
		store,
		&artifact.MemoryStore{},
		serviceFixtureBuilder{},
		ServiceOptions{Now: clock.Now, NewID: fixtureIDGenerator()},
	)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.EnsureProject(ctx, "prj_fixture", "Fixture")
	if err != nil {
		t.Fatal(err)
	}
	newName := "Renamed Fixture"
	project, err = service.UpdateProject(ctx, project.ID, UpdateProjectRequest{Name: &newName})
	if err != nil || project.Name != newName {
		t.Fatalf("updated project = %#v, %v", project, err)
	}
	app, err := service.CreateApp(ctx, CreateAppRequest{
		ProjectID: project.ID, Name: "Fixture App",
	})
	if err != nil {
		t.Fatal(err)
	}
	apps, err := service.ListApps(ctx, project.ID, app.Name, 100)
	if err != nil || len(apps) != 1 || apps[0].ID != app.ID {
		t.Fatalf("filter apps = %#v, %v", apps, err)
	}
	if _, err := service.ListApps(ctx, project.ID, " Fixture App ", 100); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unnormalized app name filter error = %v", err)
	}

	source := zipSourceFixture(t, map[string]string{
		"main.py": "def handler(event):\n    return event\n",
	})
	created, err := service.Create(ctx, CreateRequest{
		ProjectID: project.ID, AppID: app.ID, Runtime: RuntimePython,
		EntryPoint: "main.py:handler", SourceName: "source.zip",
		Source: bytes.NewReader(source),
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != StatusReady || len(created.Builds) != 1 {
		t.Fatalf("created deployment = %#v", created)
	}
	deployments, err := service.List(ctx, project.ID, app.ID, 100)
	if err != nil || len(deployments) != 1 || deployments[0].ID != created.ID {
		t.Fatalf("filter deployments by app = %#v, %v", deployments, err)
	}
	originalBuild := created.Builds[0]
	if originalBuild.ProjectID != project.ID ||
		originalBuild.DeploymentID != created.ID ||
		!strings.HasPrefix(created.Source.StorageKey(), "objects/sha256/") {
		t.Fatalf("unexpected build/source ownership: %#v", created)
	}
	listedBuilds, err := service.ListBuilds(ctx, project.ID, "", 100)
	if err != nil || len(listedBuilds) != 1 || listedBuilds[0].ID != originalBuild.ID {
		t.Fatalf("list builds = %#v, %v", listedBuilds, err)
	}

	execution, err := service.CreateExecution(ctx, CreateExecutionRequest{
		ProjectID: project.ID, DeploymentID: created.ID,
		Input: json.RawMessage(" { \"b\" : 2, \"a\" : 1 } "),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(execution.ID, "exe_") ||
		string(execution.Input) != `{"a":1,"b":2}` ||
		execution.BuildID != originalBuild.ID {
		t.Fatalf("queued execution = %#v", execution)
	}
	nullExecution, err := service.CreateExecution(ctx, CreateExecutionRequest{
		ProjectID: project.ID, DeploymentID: created.ID,
		Input: json.RawMessage(`null`),
	})
	if err != nil || string(nullExecution.Input) != "null" {
		t.Fatalf("null execution = %#v, %v", nullExecution, err)
	}

	claimed, err := store.ClaimQueuedRun(ctx, clock.Now())
	if err != nil || claimed.ID != execution.ID {
		t.Fatalf("claimed execution = %#v, %v", claimed, err)
	}
	claimed.Status = RunSucceeded
	claimed.Output = json.RawMessage(`{"ok":true}`)
	claimed.Logs = "worker log\n"
	finished := clock.Now()
	claimed.FinishedAt = &finished
	if err := store.FinalizeRun(ctx, claimed); err != nil {
		t.Fatal(err)
	}

	// A newer ready build must not change rerun pinning.
	newer := cloneBuild(originalBuild)
	newer.ID = "bld_newer"
	newer.Number = 2
	newer.StartedAt = clock.Now()
	newerFinished := clock.Now()
	newer.FinishedAt = &newerFinished
	created.Builds = append(created.Builds, newer)
	created.UpdatedAt = newerFinished
	if err := store.SaveDeployment(ctx, created); err != nil {
		t.Fatal(err)
	}
	rerun, err := service.RerunExecution(ctx, project.ID, execution.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rerun.BuildID != execution.BuildID ||
		!bytes.Equal(rerun.Input, execution.Input) ||
		rerun.RerunOfRunID != execution.ID || rerun.Status != RunQueued {
		t.Fatalf("rerun changed immutable execution inputs: %#v", rerun)
	}
	publicJSON, err := json.Marshal(rerun)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(publicJSON, []byte(`"rerun_of_execution_id"`)) ||
		bytes.Contains(publicJSON, []byte("rerun_of_run_id")) {
		t.Fatalf("execution JSON uses stale vocabulary: %s", publicJSON)
	}
	executions, err := service.ListExecutions(ctx, project.ID, "", 100)
	if err != nil || len(executions) != 3 {
		t.Fatalf("project executions = %#v, %v", executions, err)
	}
	if _, err := service.GetExecution(ctx, "prj_other", execution.ID); !errors.Is(err, ErrExecutionNotFound) {
		t.Fatalf("cross-project execution error = %v", err)
	}
}

func TestServicePersistsBuildFailuresAndBoundsInputs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryStore()
	clock := newFixtureClock(time.Date(2026, 7, 30, 9, 0, 0, 0, time.UTC))
	service, err := NewService(
		store,
		&artifact.MemoryStore{},
		serviceFixtureBuilder{failure: errors.New("toolchain failed")},
		ServiceOptions{
			Now: clock.Now, NewID: fixtureIDGenerator(), MaxRunInputBytes: 8,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.EnsureProject(ctx, "prj_fixture", "Fixture"); err != nil {
		t.Fatal(err)
	}
	app, err := service.CreateApp(ctx, CreateAppRequest{
		ProjectID: "prj_fixture", Name: "Fixture App",
	})
	if err != nil {
		t.Fatal(err)
	}
	created, err := service.Create(ctx, CreateRequest{
		ProjectID: "prj_fixture", AppID: app.ID, Runtime: RuntimePython,
		SourceName: "source.zip", Source: bytes.NewReader(zipSourceFixture(t, map[string]string{
			"main.py": "def handler(event): return event\n",
		})),
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != StatusFailed || created.Builds[0].Failure == nil ||
		created.Builds[0].Failure.Code != "build_failed" {
		t.Fatalf("failed build was not persisted: %#v", created)
	}
	if _, err := service.CreateExecution(ctx, CreateExecutionRequest{
		ProjectID: created.ProjectID, DeploymentID: created.ID,
		Input: json.RawMessage(`null`),
	}); !errors.Is(err, ErrNoReadyBuild) {
		t.Fatalf("execution against failed build error = %v", err)
	}
	if _, err := service.CreateExecution(ctx, CreateExecutionRequest{
		ProjectID: created.ProjectID, DeploymentID: created.ID,
		Input: json.RawMessage(`{"long":1}`),
	}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("oversized execution input error = %v", err)
	}
}

func TestServiceRequiresProjectScopedAppForDeployment(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryStore()
	service, err := NewService(
		store,
		&artifact.MemoryStore{},
		serviceFixtureBuilder{},
		ServiceOptions{NewID: fixtureIDGenerator()},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.EnsureProject(ctx, "prj_one", "One"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.EnsureProject(ctx, "prj_two", "Two"); err != nil {
		t.Fatal(err)
	}
	app, err := service.CreateApp(ctx, CreateAppRequest{
		ProjectID: "prj_one", Name: "Fixture App",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(ctx, CreateRequest{
		ProjectID: "prj_one", Runtime: RuntimePython,
	}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing app id error = %v", err)
	}
	if _, err := service.Create(ctx, CreateRequest{
		ProjectID: "prj_two", AppID: app.ID, Runtime: RuntimePython,
	}); !errors.Is(err, ErrAppNotFound) {
		t.Fatalf("cross-project app error = %v", err)
	}
	if _, err := service.List(ctx, "prj_two", app.ID, 100); !errors.Is(err, ErrAppNotFound) {
		t.Fatalf("cross-project app filter error = %v", err)
	}
	if _, err := service.List(ctx, "prj_one", "app_missing", 100); !errors.Is(err, ErrAppNotFound) {
		t.Fatalf("missing app filter error = %v", err)
	}
}

type fixtureClock struct {
	mu      sync.Mutex
	current time.Time
}

func newFixtureClock(start time.Time) *fixtureClock {
	return &fixtureClock{current: start.Add(-time.Second)}
}

func (clock *fixtureClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	clock.current = clock.current.Add(time.Second)
	return clock.current
}

func fixtureIDGenerator() func(string) (string, error) {
	var mutex sync.Mutex
	counters := make(map[string]int)
	return func(prefix string) (string, error) {
		mutex.Lock()
		defer mutex.Unlock()
		counters[prefix]++
		return fmt.Sprintf("%s_%02d", prefix, counters[prefix]), nil
	}
}

func zipSourceFixture(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var payload bytes.Buffer
	archive := zip.NewWriter(&payload)
	for name, contents := range files {
		entry, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(contents)); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return payload.Bytes()
}
