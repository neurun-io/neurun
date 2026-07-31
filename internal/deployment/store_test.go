package deployment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestArtifactStorageHandleIsPrivateButPersistsInInternalSnapshot(t *testing.T) {
	t.Parallel()
	record := readyDeploymentFixture(time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC))
	publicJSON, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(publicJSON, []byte("storage_key")) ||
		bytes.Contains(publicJSON, []byte(record.Source.StorageKey())) {
		t.Fatalf("public JSON leaked storage handle: %s", publicJSON)
	}
	privateJSON, err := json.Marshal(deploymentToSnapshot(record))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(privateJSON, []byte(`"storage_key"`)) ||
		!bytes.Contains(privateJSON, []byte(record.Source.StorageKey())) {
		t.Fatalf("private snapshot omitted storage handle: %s", privateJSON)
	}
	roundTrip := deploymentFromSnapshot(deploymentToSnapshot(record).Record)
	if roundTrip.Source.StorageKey() != record.Source.StorageKey() ||
		roundTrip.Builds[0].Artifacts[0].StorageKey() !=
			record.Builds[0].Artifacts[0].StorageKey() {
		t.Fatalf("storage handles did not survive snapshot conversion: %#v", roundTrip)
	}
}

func TestMemoryStoreProjectAndBuildScoping(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryStore()
	now := time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)
	project := Project{ID: "prj_fixture", Name: "Fixture", CreatedAt: now, UpdatedAt: now}
	ensured, err := store.EnsureProject(ctx, project)
	if err != nil {
		t.Fatal(err)
	}
	if ensured != project {
		t.Fatalf("ensured project = %#v", ensured)
	}
	project.Name = "Renamed"
	project.UpdatedAt = now.Add(time.Second)
	updated, err := store.UpdateProject(ctx, project)
	if err != nil || updated.Name != "Renamed" {
		t.Fatalf("update project = %#v, %v", updated, err)
	}
	listedProjects, err := store.ListProjects(ctx, project.ID, 100)
	if err != nil || len(listedProjects) != 1 || listedProjects[0].ID != project.ID {
		t.Fatalf("list projects = %#v, %v", listedProjects, err)
	}
	if projects, err := store.ListProjects(ctx, "prj_other", 100); err != nil || len(projects) != 0 {
		t.Fatalf("other principal project list = %#v, %v", projects, err)
	}
	app := App{
		ID: "app_fixture", ProjectID: project.ID, Name: "Fixture App",
		CreatedAt: now, UpdatedAt: now,
	}
	if _, err := store.CreateApp(ctx, app); err != nil {
		t.Fatal(err)
	}

	record := readyDeploymentFixture(now)
	if err := store.SaveDeployment(ctx, record); err != nil {
		t.Fatal(err)
	}
	listedDeployments, err := store.ListDeployments(ctx, project.ID, "", 100)
	if err != nil || len(listedDeployments) != 1 || listedDeployments[0].ID != record.ID {
		t.Fatalf("list deployments = %#v, %v", listedDeployments, err)
	}
	listedDeployments, err = store.ListDeployments(ctx, project.ID, record.AppID, 100)
	if err != nil || len(listedDeployments) != 1 || listedDeployments[0].ID != record.ID {
		t.Fatalf("filter deployments by app = %#v, %v", listedDeployments, err)
	}
	listedDeployments, err = store.ListDeployments(ctx, project.ID, "app_missing", 100)
	if err != nil || len(listedDeployments) != 0 {
		t.Fatalf("filter deployments by missing app = %#v, %v", listedDeployments, err)
	}
	if _, err := store.ListDeployments(ctx, project.ID, "bad app", 100); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid deployment app filter error = %v", err)
	}
	build, err := store.GetBuild(ctx, project.ID, record.Builds[0].ID)
	if err != nil || build.ProjectID != project.ID || build.DeploymentID != record.ID {
		t.Fatalf("get build = %#v, %v", build, err)
	}
	builds, err := store.ListBuilds(ctx, project.ID, "", 100)
	if err != nil || len(builds) != 1 || builds[0].ID != build.ID {
		t.Fatalf("list project builds = %#v, %v", builds, err)
	}
	if _, err := store.GetBuild(ctx, "prj_other", build.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-project build error = %v", err)
	}
}

func TestMemoryStoreAppsAreProjectScopedDeploymentOwners(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryStore()
	now := time.Date(2026, 7, 30, 11, 0, 0, 0, time.UTC)
	for _, project := range []Project{
		{ID: "prj_one", Name: "One", CreatedAt: now, UpdatedAt: now},
		{ID: "prj_two", Name: "Two", CreatedAt: now, UpdatedAt: now},
	} {
		if _, err := store.EnsureProject(ctx, project); err != nil {
			t.Fatal(err)
		}
	}
	app := App{
		ID: "app_one", ProjectID: "prj_one", Name: "First",
		CreatedAt: now, UpdatedAt: now,
	}
	created, err := store.CreateApp(ctx, app)
	if err != nil || created != app {
		t.Fatalf("create app = %#v, %v", created, err)
	}
	if _, err := store.GetApp(ctx, "prj_two", app.ID); !errors.Is(err, ErrAppNotFound) {
		t.Fatalf("cross-project get app error = %v", err)
	}
	second := App{
		ID: "app_second", ProjectID: "prj_one", Name: "Second",
		CreatedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second),
	}
	if _, err := store.CreateApp(ctx, second); err != nil {
		t.Fatal(err)
	}
	listed, err := store.ListApps(ctx, "prj_one", "", 1)
	if err != nil || len(listed) != 1 || listed[0].ID != second.ID {
		t.Fatalf("list apps = %#v, %v", listed, err)
	}
	listed, err = store.ListApps(ctx, "prj_one", app.Name, 100)
	if err != nil || len(listed) != 1 || listed[0].ID != app.ID {
		t.Fatalf("filter apps by exact name = %#v, %v", listed, err)
	}
	if _, err := store.ListApps(ctx, "prj_one", " First ", 100); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unnormalized app name filter error = %v", err)
	}
	newName := "Renamed"
	app.Name = newName
	app.UpdatedAt = now.Add(2 * time.Second)
	updated, err := store.UpdateApp(ctx, app)
	if err != nil || updated.Name != newName {
		t.Fatalf("update app = %#v, %v", updated, err)
	}

	record := readyDeploymentFixture(now)
	record.ProjectID = "prj_two"
	record.AppID = app.ID
	record.Builds[0].ProjectID = record.ProjectID
	if err := store.SaveDeployment(ctx, record); !errors.Is(err, ErrAppNotFound) {
		t.Fatalf("cross-project deployment owner error = %v", err)
	}
}

func TestRunClaimAndFinalizeUsesCompareAndSet(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryStore()
	created := runFixture("exe_one", RunQueued)
	if err := store.CreateRun(ctx, created); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateRun(ctx, created); !errors.Is(err, ErrRunConflict) {
		t.Fatalf("duplicate create error = %v", err)
	}
	claimed, err := store.ClaimQueuedRun(ctx, created.CreatedAt.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if claimed.Status != RunRunning || claimed.StartedAt == nil {
		t.Fatalf("unexpected claimed execution: %#v", claimed)
	}
	first := cloneRun(claimed)
	first.Status = RunSucceeded
	first.Output = json.RawMessage(`{"winner":1}`)
	first.Logs = "one line\n"
	finished := created.CreatedAt.Add(3 * time.Second)
	first.FinishedAt = &finished
	second := cloneRun(first)
	second.Output = json.RawMessage(`{"winner":2}`)

	start := make(chan struct{})
	results := make(chan error, 2)
	var group sync.WaitGroup
	for _, candidate := range []Run{first, second} {
		candidate := candidate
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			results <- store.FinalizeRun(ctx, candidate)
		}()
	}
	close(start)
	group.Wait()
	close(results)
	var successes, conflicts int
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrRunConflict):
			conflicts++
		default:
			t.Fatalf("unexpected finalization error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
	}
	if err := store.FinalizeRun(ctx, first); !errors.Is(err, ErrRunConflict) {
		t.Fatalf("stale terminal overwrite error = %v", err)
	}
	persisted, err := store.GetRun(ctx, created.ProjectID, created.ID)
	if err != nil || persisted.Status != RunSucceeded || persisted.Logs == "" {
		t.Fatalf("terminal execution = %#v, %v", persisted, err)
	}
}

func TestStoreRecoveryFailsInterruptedWorkWithoutReexecution(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryStore()
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	if _, err := store.EnsureProject(ctx, Project{
		ID: "prj_fixture", Name: "Fixture", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateApp(ctx, App{
		ID: "app_fixture", ProjectID: "prj_fixture", Name: "Fixture App",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	building := buildingDeploymentFixture(now)
	if err := store.SaveDeployment(ctx, building); err != nil {
		t.Fatal(err)
	}
	running := runFixture("exe_running", RunQueued)
	if err := store.CreateRun(ctx, running); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ClaimQueuedRun(ctx, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	queued := runFixture("exe_queued", RunQueued)
	queued.CreatedAt = now.Add(2 * time.Second)
	if err := store.CreateRun(ctx, queued); err != nil {
		t.Fatal(err)
	}
	recoveredBuilds, err := store.RecoverBuildingDeployments(
		ctx, now.Add(3*time.Second),
		Failure{Code: "build_interrupted", Message: "build interrupted by restart"},
	)
	if err != nil || recoveredBuilds != 1 {
		t.Fatalf("recover builds = %d, %v", recoveredBuilds, err)
	}
	recoveredRuns, err := store.RecoverRunningRuns(
		ctx, now.Add(4*time.Second),
		Failure{Code: "worker_restarted", Message: "worker restarted"},
	)
	if err != nil || recoveredRuns != 1 {
		t.Fatalf("recover executions = %d, %v", recoveredRuns, err)
	}
	loaded, err := store.GetRun(ctx, running.ProjectID, running.ID)
	if err != nil || loaded.Status != RunFailed || loaded.Failure == nil {
		t.Fatalf("recovered execution = %#v, %v", loaded, err)
	}
	stillQueued, err := store.GetRun(ctx, queued.ProjectID, queued.ID)
	if err != nil || stillQueued.Status != RunQueued {
		t.Fatalf("queued execution changed = %#v, %v", stillQueued, err)
	}
}

func TestIdentifiersRejectWindowsReservedDeviceNames(t *testing.T) {
	t.Parallel()
	for _, value := range []string{
		"CON", "con.txt", "PrN", "AUX.json", "NUL", "COM1", "com9.log",
		"LPT1", "lpt9.bin",
	} {
		if err := validateIdentifier("id", value); !errors.Is(err, ErrInvalid) {
			t.Fatalf("validateIdentifier(%q) = %v", value, err)
		}
	}
	for _, value := range []string{"CONSOLE", "COM10", "LPT0", "prn_job"} {
		if err := validateIdentifier("id", value); err != nil {
			t.Fatalf("valid identifier %q rejected: %v", value, err)
		}
	}
}

func TestPostgresStoreRequiresDatabase(t *testing.T) {
	t.Parallel()
	if _, err := NewPostgresStore(nil); err == nil {
		t.Fatal("NewPostgresStore(nil) succeeded")
	}
}

func readyDeploymentFixture(now time.Time) Deployment {
	sourceDigest := strings.Repeat("a", 64)
	codeDigest := strings.Repeat("b", 64)
	finished := now.Add(2 * time.Second)
	return Deployment{
		ID: "dep_fixture", ProjectID: "prj_fixture", AppID: "app_fixture",
		Runtime: RuntimePython, EntryPoint: "main.py:handler", Status: StatusReady,
		Source: newArtifact(
			"art_source", ArtifactSource, "source.zip", "application/zip",
			12, sourceDigest, "objects/sha256/aa/"+sourceDigest, now,
		),
		Builds: []Build{{
			ID: "bld_fixture", ProjectID: "prj_fixture",
			DeploymentID: "dep_fixture", Number: 1, Status: StatusReady,
			Runtime: RuntimePython, EntryPoint: "main.py:handler",
			SourceSHA256: sourceDigest,
			Artifacts: []Artifact{newArtifact(
				"art_code", ArtifactCodeLayer, "code.zip", "application/zip",
				24, codeDigest, "objects/sha256/bb/"+codeDigest, now.Add(time.Second),
			)},
			StartedAt: now.Add(time.Second), FinishedAt: &finished,
		}},
		CreatedAt: now, UpdatedAt: finished,
	}
}

func buildingDeploymentFixture(now time.Time) Deployment {
	record := readyDeploymentFixture(now)
	record.Status = StatusBuilding
	record.UpdatedAt = now.Add(time.Second)
	record.Builds[0].Status = StatusBuilding
	record.Builds[0].Artifacts = []Artifact{}
	record.Builds[0].FinishedAt = nil
	return record
}

func runFixture(id string, status RunStatus) Run {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	record := Run{
		ID: id, ProjectID: "prj_fixture", DeploymentID: "dep_fixture",
		BuildID: "bld_fixture", Status: status,
		Input: json.RawMessage(`{"message":"hello"}`), CreatedAt: now,
	}
	if status == RunRunning {
		started := now.Add(time.Second)
		record.StartedAt = &started
	}
	return record
}
