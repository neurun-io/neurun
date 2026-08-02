package deployment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/neurun-io/neurun/internal/artifact"
)

// Store persists projects, deployments, builds, and executions. ClaimQueuedRun
// and RecoverRunningRuns retain internal worker names but are atomic.
type Store interface {
	Check(context.Context) error
	EnsureProject(context.Context, Project) (Project, error)
	GetProject(context.Context, string) (Project, error)
	ListProjects(context.Context, string, int) ([]Project, error)
	UpdateProject(context.Context, Project) (Project, error)
	CreateApp(context.Context, App) (App, error)
	GetApp(context.Context, string, string) (App, error)
	ListApps(context.Context, string, string, int) ([]App, error)
	UpdateApp(context.Context, App) (App, error)

	SaveDeployment(context.Context, Deployment) error
	GetDeployment(context.Context, string, string) (Deployment, error)
	ListDeployments(context.Context, string, string, int) ([]Deployment, error)
	GetBuild(context.Context, string, string) (Build, error)
	ListBuilds(context.Context, string, string, int) ([]Build, error)
	RecoverBuildingDeployments(context.Context, time.Time, Failure) (int, error)

	CreateRun(context.Context, Run) error
	FinalizeRun(context.Context, Run) error
	GetRun(context.Context, string, string) (Run, error)
	ListRuns(context.Context, string, string, int) ([]Run, error)
	ClaimQueuedRun(context.Context, time.Time) (Run, error)
	RecoverRunningRuns(context.Context, time.Time, Failure) (int, error)
}

// MemoryStore is a concurrency-safe Store for tests and ephemeral use. Its
// zero value is ready for use.
type MemoryStore struct {
	mu          sync.RWMutex
	projects    map[string]Project
	apps        map[string]App
	deployments map[string]Deployment
	runs        map[string]Run
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		projects:    make(map[string]Project),
		apps:        make(map[string]App),
		deployments: make(map[string]Deployment),
		runs:        make(map[string]Run),
	}
}

func (store *MemoryStore) EnsureProject(
	ctx context.Context,
	record Project,
) (Project, error) {
	if err := contextError(ctx); err != nil {
		return Project{}, err
	}
	if err := validateProject(record); err != nil {
		return Project{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.projects == nil {
		store.projects = make(map[string]Project)
	}
	if existing, found := store.projects[record.ID]; found {
		return cloneProject(existing), nil
	}
	for _, existing := range store.projects {
		if strings.EqualFold(existing.Name, record.Name) {
			return Project{}, fmt.Errorf("%w: project name already exists", ErrProjectConflict)
		}
	}
	store.projects[record.ID] = cloneProject(record)
	return cloneProject(record), nil
}

func (store *MemoryStore) GetProject(
	ctx context.Context,
	projectID string,
) (Project, error) {
	if err := contextError(ctx); err != nil {
		return Project{}, err
	}
	if err := validateIdentifier("project_id", projectID); err != nil {
		return Project{}, err
	}
	store.mu.RLock()
	record, found := store.projects[projectID]
	store.mu.RUnlock()
	if !found {
		return Project{}, fmt.Errorf("%w: %s", ErrProjectNotFound, projectID)
	}
	return cloneProject(record), nil
}

func (store *MemoryStore) ListProjects(
	ctx context.Context,
	principalProjectID string,
	limit int,
) ([]Project, error) {
	record, err := store.GetProject(ctx, principalProjectID)
	if errors.Is(err, ErrProjectNotFound) {
		return []Project{}, nil
	}
	if err != nil {
		return nil, err
	}
	if limit < 1 {
		return []Project{}, nil
	}
	return []Project{record}, nil
}

func (store *MemoryStore) UpdateProject(
	ctx context.Context,
	record Project,
) (Project, error) {
	if err := contextError(ctx); err != nil {
		return Project{}, err
	}
	if err := validateProject(record); err != nil {
		return Project{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	existing, found := store.projects[record.ID]
	if !found {
		return Project{}, fmt.Errorf("%w: %s", ErrProjectNotFound, record.ID)
	}
	if !existing.CreatedAt.Equal(record.CreatedAt) || record.UpdatedAt.Before(existing.UpdatedAt) {
		return Project{}, fmt.Errorf("%w: immutable project metadata changed", ErrProjectConflict)
	}
	for id, other := range store.projects {
		if id != record.ID && strings.EqualFold(other.Name, record.Name) {
			return Project{}, fmt.Errorf("%w: project name already exists", ErrProjectConflict)
		}
	}
	store.projects[record.ID] = cloneProject(record)
	return cloneProject(record), nil
}

func (store *MemoryStore) Check(ctx context.Context) error {
	return contextError(ctx)
}

func (store *MemoryStore) CreateApp(ctx context.Context, record App) (App, error) {
	if err := contextError(ctx); err != nil {
		return App{}, err
	}
	if err := validateApp(record); err != nil {
		return App{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if _, found := store.projects[record.ProjectID]; !found {
		return App{}, fmt.Errorf("%w: %s", ErrProjectNotFound, record.ProjectID)
	}
	if store.apps == nil {
		store.apps = make(map[string]App)
	}
	key := recordKey(record.ProjectID, record.ID)
	if _, found := store.apps[key]; found {
		return App{}, fmt.Errorf("%w: app %s already exists", ErrAppConflict, record.ID)
	}
	for _, existing := range store.apps {
		if existing.ProjectID == record.ProjectID && strings.EqualFold(existing.Name, record.Name) {
			return App{}, fmt.Errorf("%w: app name already exists", ErrAppConflict)
		}
	}
	store.apps[key] = cloneApp(record)
	return cloneApp(record), nil
}

func (store *MemoryStore) GetApp(
	ctx context.Context,
	projectID string,
	appID string,
) (App, error) {
	if err := contextError(ctx); err != nil {
		return App{}, err
	}
	if err := validateIdentifier("project_id", projectID); err != nil {
		return App{}, err
	}
	if err := validateIdentifier("app_id", appID); err != nil {
		return App{}, err
	}
	store.mu.RLock()
	record, found := store.apps[recordKey(projectID, appID)]
	store.mu.RUnlock()
	if !found {
		return App{}, fmt.Errorf("%w: %s", ErrAppNotFound, appID)
	}
	return cloneApp(record), nil
}

func (store *MemoryStore) ListApps(
	ctx context.Context,
	projectID string,
	name string,
	limit int,
) ([]App, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	if err := validateIdentifier("project_id", projectID); err != nil {
		return nil, err
	}
	if err := validateOptionalAppNameFilter(name); err != nil {
		return nil, err
	}
	store.mu.RLock()
	records := make([]App, 0)
	for _, record := range store.apps {
		if record.ProjectID == projectID && (name == "" || record.Name == name) {
			records = append(records, cloneApp(record))
		}
	}
	store.mu.RUnlock()
	sort.Slice(records, func(left, right int) bool {
		if records[left].CreatedAt.Equal(records[right].CreatedAt) {
			return records[left].ID > records[right].ID
		}
		return records[left].CreatedAt.After(records[right].CreatedAt)
	})
	if limit > 0 && len(records) > limit {
		records = records[:limit]
	}
	return records, nil
}

func (store *MemoryStore) UpdateApp(ctx context.Context, record App) (App, error) {
	if err := contextError(ctx); err != nil {
		return App{}, err
	}
	if err := validateApp(record); err != nil {
		return App{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	key := recordKey(record.ProjectID, record.ID)
	existing, found := store.apps[key]
	if !found {
		return App{}, fmt.Errorf("%w: %s", ErrAppNotFound, record.ID)
	}
	if !existing.CreatedAt.Equal(record.CreatedAt) || record.UpdatedAt.Before(existing.UpdatedAt) {
		return App{}, fmt.Errorf("%w: immutable app metadata changed", ErrAppConflict)
	}
	for otherKey, other := range store.apps {
		if otherKey != key && other.ProjectID == record.ProjectID &&
			strings.EqualFold(other.Name, record.Name) {
			return App{}, fmt.Errorf("%w: app name already exists", ErrAppConflict)
		}
	}
	store.apps[key] = cloneApp(record)
	return cloneApp(record), nil
}

func (store *MemoryStore) SaveDeployment(ctx context.Context, record Deployment) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if err := validateDeploymentRecord(record); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if _, found := store.apps[recordKey(record.ProjectID, record.AppID)]; !found {
		return fmt.Errorf("%w: %s", ErrAppNotFound, record.AppID)
	}
	if store.deployments == nil {
		store.deployments = make(map[string]Deployment)
	}
	store.deployments[recordKey(record.ProjectID, record.ID)] = cloneDeployment(record)
	return nil
}

func (store *MemoryStore) GetDeployment(
	ctx context.Context,
	projectID string,
	deploymentID string,
) (Deployment, error) {
	if err := contextError(ctx); err != nil {
		return Deployment{}, err
	}
	if err := validateIdentifier("project_id", projectID); err != nil {
		return Deployment{}, err
	}
	if err := validateIdentifier("deployment_id", deploymentID); err != nil {
		return Deployment{}, err
	}
	store.mu.RLock()
	record, exists := store.deployments[recordKey(projectID, deploymentID)]
	store.mu.RUnlock()
	if !exists {
		return Deployment{}, fmt.Errorf("%w: %s", ErrNotFound, deploymentID)
	}
	return cloneDeployment(record), nil
}

func (store *MemoryStore) ListDeployments(
	ctx context.Context,
	projectID string,
	appID string,
	limit int,
) ([]Deployment, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	if err := validateIdentifier("project_id", projectID); err != nil {
		return nil, err
	}
	if appID != "" {
		if err := validateIdentifier("app_id", appID); err != nil {
			return nil, err
		}
	}
	store.mu.RLock()
	records := make([]Deployment, 0)
	for _, record := range store.deployments {
		if record.ProjectID == projectID && (appID == "" || record.AppID == appID) {
			records = append(records, cloneDeployment(record))
		}
	}
	store.mu.RUnlock()
	sortDeployments(records)
	if limit > 0 && len(records) > limit {
		records = records[:limit]
	}
	return records, nil
}

func (store *MemoryStore) GetBuild(
	ctx context.Context,
	projectID string,
	buildID string,
) (Build, error) {
	if err := contextError(ctx); err != nil {
		return Build{}, err
	}
	if err := validateIdentifier("project_id", projectID); err != nil {
		return Build{}, err
	}
	if err := validateIdentifier("build_id", buildID); err != nil {
		return Build{}, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	for _, record := range store.deployments {
		if record.ProjectID != projectID {
			continue
		}
		if build, found := record.BuildByID(buildID); found {
			return build, nil
		}
	}
	return Build{}, fmt.Errorf("%w: build %s", ErrNotFound, buildID)
}

func (store *MemoryStore) ListBuilds(
	ctx context.Context,
	projectID string,
	deploymentID string,
	limit int,
) ([]Build, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	if err := validateIdentifier("project_id", projectID); err != nil {
		return nil, err
	}
	if deploymentID != "" {
		if err := validateIdentifier("deployment_id", deploymentID); err != nil {
			return nil, err
		}
	}
	store.mu.RLock()
	builds := make([]Build, 0)
	for _, record := range store.deployments {
		if record.ProjectID != projectID ||
			(deploymentID != "" && record.ID != deploymentID) {
			continue
		}
		for _, build := range record.Builds {
			builds = append(builds, cloneBuild(build))
		}
	}
	store.mu.RUnlock()
	sortBuilds(builds)
	if limit > 0 && len(builds) > limit {
		builds = builds[:limit]
	}
	return builds, nil
}

func (store *MemoryStore) RecoverBuildingDeployments(
	ctx context.Context,
	now time.Time,
	failure Failure,
) (int, error) {
	if err := contextError(ctx); err != nil {
		return 0, err
	}
	if err := validateRecovery(now, failure); err != nil {
		return 0, err
	}
	now = now.UTC().Round(0)
	store.mu.Lock()
	defer store.mu.Unlock()
	recovered := 0
	for key, record := range store.deployments {
		if !failInterruptedBuild(&record, now, failure) {
			continue
		}
		if err := validateDeploymentRecord(record); err != nil {
			return recovered, err
		}
		store.deployments[key] = cloneDeployment(record)
		recovered++
	}
	return recovered, nil
}

func (store *MemoryStore) CreateRun(ctx context.Context, record Run) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if err := validateRunRecord(record); err != nil {
		return err
	}
	if record.Status != RunQueued {
		return fmt.Errorf("%w: a new execution must be queued", ErrRunConflict)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.runs == nil {
		store.runs = make(map[string]Run)
	}
	key := recordKey(record.ProjectID, record.ID)
	if _, exists := store.runs[key]; exists {
		return fmt.Errorf("%w: execution %s already exists", ErrRunConflict, record.ID)
	}
	store.runs[key] = cloneRun(record)
	return nil
}

func (store *MemoryStore) FinalizeRun(ctx context.Context, record Run) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if err := validateRunRecord(record); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	key := recordKey(record.ProjectID, record.ID)
	current, exists := store.runs[key]
	if !exists {
		return fmt.Errorf("%w: %s", ErrRunNotFound, record.ID)
	}
	if err := validateRunFinalization(current, record); err != nil {
		return err
	}
	store.runs[key] = cloneRun(record)
	return nil
}

func (store *MemoryStore) GetRun(
	ctx context.Context,
	projectID string,
	runID string,
) (Run, error) {
	if err := contextError(ctx); err != nil {
		return Run{}, err
	}
	if err := validateIdentifier("project_id", projectID); err != nil {
		return Run{}, err
	}
	if err := validateIdentifier("execution_id", runID); err != nil {
		return Run{}, err
	}
	store.mu.RLock()
	record, exists := store.runs[recordKey(projectID, runID)]
	store.mu.RUnlock()
	if !exists {
		return Run{}, fmt.Errorf("%w: %s", ErrRunNotFound, runID)
	}
	return cloneRun(record), nil
}

func (store *MemoryStore) ListRuns(
	ctx context.Context,
	projectID string,
	deploymentID string,
	limit int,
) ([]Run, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	if err := validateIdentifier("project_id", projectID); err != nil {
		return nil, err
	}
	if deploymentID != "" {
		if err := validateIdentifier("deployment_id", deploymentID); err != nil {
			return nil, err
		}
	}
	store.mu.RLock()
	records := make([]Run, 0)
	for _, record := range store.runs {
		if record.ProjectID == projectID &&
			(deploymentID == "" || record.DeploymentID == deploymentID) {
			records = append(records, cloneRun(record))
		}
	}
	store.mu.RUnlock()
	sortRuns(records)
	if limit > 0 && len(records) > limit {
		records = records[:limit]
	}
	return records, nil
}

func (store *MemoryStore) ClaimQueuedRun(
	ctx context.Context,
	now time.Time,
) (Run, error) {
	if err := contextError(ctx); err != nil {
		return Run{}, err
	}
	if now.IsZero() {
		return Run{}, fmt.Errorf("%w: claim time is required", ErrInvalid)
	}
	now = now.UTC().Round(0)

	store.mu.Lock()
	defer store.mu.Unlock()
	var selectedKey string
	var selected Run
	for key, record := range store.runs {
		if record.Status != RunQueued {
			continue
		}
		if selectedKey == "" ||
			record.CreatedAt.Before(selected.CreatedAt) ||
			(record.CreatedAt.Equal(selected.CreatedAt) && record.ID < selected.ID) {
			selectedKey = key
			selected = record
		}
	}
	if selectedKey == "" {
		return Run{}, ErrNoQueuedRun
	}
	selected.Status = RunRunning
	selected.StartedAt = &now
	selected.FinishedAt = nil
	selected.Output = nil
	selected.Failure = nil
	store.runs[selectedKey] = cloneRun(selected)
	return cloneRun(selected), nil
}

func (store *MemoryStore) RecoverRunningRuns(
	ctx context.Context,
	now time.Time,
	failure Failure,
) (int, error) {
	if err := contextError(ctx); err != nil {
		return 0, err
	}
	if err := validateRecovery(now, failure); err != nil {
		return 0, err
	}
	now = now.UTC().Round(0)
	store.mu.Lock()
	defer store.mu.Unlock()
	recovered := 0
	for key, record := range store.runs {
		if record.Status != RunRunning {
			continue
		}
		record.Status = RunFailed
		record.Failure = cloneFailure(&failure)
		record.FinishedAt = &now
		store.runs[key] = cloneRun(record)
		recovered++
	}
	return recovered, nil
}

func validateDeploymentRecord(record Deployment) error {
	if err := validateIdentifier("project_id", record.ProjectID); err != nil {
		return err
	}
	if err := validateIdentifier("deployment_id", record.ID); err != nil {
		return err
	}
	if err := validateIdentifier("app_id", record.AppID); err != nil {
		return err
	}
	if !record.Runtime.Valid() {
		return fmt.Errorf("%w: deployment runtime is invalid", ErrInvalid)
	}
	normalizedEntryPoint, err := normalizeEntryPoint(record.Runtime, record.EntryPoint)
	if err != nil || normalizedEntryPoint != record.EntryPoint {
		return fmt.Errorf("%w: deployment entrypoint is not normalized", ErrInvalid)
	}
	if !record.Status.Valid() {
		return fmt.Errorf("%w: deployment status is invalid", ErrInvalid)
	}
	if record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() ||
		record.UpdatedAt.Before(record.CreatedAt) {
		return fmt.Errorf("%w: deployment timestamps are invalid", ErrInvalid)
	}
	if err := validateArtifact(record.Source, ArtifactSource); err != nil {
		return err
	}
	for index, build := range record.Builds {
		if err := validateBuild(build, index+1, record); err != nil {
			return err
		}
	}
	if len(record.Builds) == 0 {
		if record.Status != StatusUploaded {
			return fmt.Errorf("%w: deployment without a build must be uploaded", ErrInvalid)
		}
	} else if record.Status != record.Builds[len(record.Builds)-1].Status {
		return fmt.Errorf("%w: deployment status differs from latest build", ErrInvalid)
	}
	return nil
}

func validateBuild(build Build, expectedNumber int, record Deployment) error {
	if err := validateIdentifier("build_id", build.ID); err != nil {
		return err
	}
	if build.ProjectID != record.ProjectID || build.DeploymentID != record.ID {
		return fmt.Errorf("%w: build ownership is inconsistent", ErrInvalid)
	}
	if build.Number != expectedNumber {
		return fmt.Errorf("%w: build numbers must be contiguous", ErrInvalid)
	}
	if !build.Status.Valid() || build.Status == StatusUploaded ||
		build.Runtime != record.Runtime ||
		build.EntryPoint != record.EntryPoint ||
		build.SourceSHA256 != record.Source.SHA256 {
		return fmt.Errorf("%w: build metadata is inconsistent", ErrInvalid)
	}
	if build.StartedAt.IsZero() || build.StartedAt.Before(record.CreatedAt) {
		return fmt.Errorf("%w: build start time is invalid", ErrInvalid)
	}
	if build.FinishedAt != nil && build.FinishedAt.Before(build.StartedAt) {
		return fmt.Errorf("%w: build finish time is invalid", ErrInvalid)
	}
	switch build.Status {
	case StatusBuilding:
		if build.FinishedAt != nil || build.Failure != nil {
			return fmt.Errorf("%w: building build cannot be finished", ErrInvalid)
		}
	case StatusReady:
		if build.FinishedAt == nil || build.Failure != nil || len(build.Artifacts) == 0 {
			return fmt.Errorf("%w: ready build is incomplete", ErrInvalid)
		}
	case StatusFailed:
		if build.FinishedAt == nil || build.Failure == nil {
			return fmt.Errorf("%w: failed build requires failure metadata", ErrInvalid)
		}
	}
	kinds := make(map[string]struct{}, len(build.Artifacts))
	for _, stored := range build.Artifacts {
		if err := validateArtifact(stored, ""); err != nil {
			return err
		}
		if stored.Kind != ArtifactCodeLayer && stored.Kind != ArtifactInstallLayer {
			return fmt.Errorf("%w: build artifact kind is invalid", ErrInvalid)
		}
		if _, duplicate := kinds[stored.Kind]; duplicate {
			return fmt.Errorf("%w: build artifact kinds must be unique", ErrInvalid)
		}
		kinds[stored.Kind] = struct{}{}
	}
	if build.Status == StatusReady {
		if _, exists := kinds[ArtifactCodeLayer]; !exists {
			return fmt.Errorf("%w: ready build requires a code layer", ErrInvalid)
		}
	}
	return validateFailure(build.Failure)
}

func validateArtifact(record Artifact, expectedKind string) error {
	if err := validateIdentifier("artifact_id", record.ID); err != nil {
		return err
	}
	if expectedKind != "" && record.Kind != expectedKind {
		return fmt.Errorf("%w: artifact kind is invalid", ErrInvalid)
	}
	if record.Kind == "" || record.Name == "" || record.MediaType == "" ||
		record.SizeBytes < 0 || len(record.SHA256) != 64 ||
		record.CreatedAt.IsZero() {
		return fmt.Errorf("%w: artifact metadata is incomplete", ErrInvalid)
	}
	for _, character := range record.SHA256 {
		if !((character >= '0' && character <= '9') ||
			(character >= 'a' && character <= 'f')) {
			return fmt.Errorf("%w: artifact digest is invalid", ErrInvalid)
		}
	}
	if err := artifact.ValidateStorageKey(record.storageKey); err != nil {
		return fmt.Errorf("%w: artifact storage handle: %v", ErrInvalid, err)
	}
	return nil
}

func validateRunRecord(record Run) error {
	if err := validateIdentifier("project_id", record.ProjectID); err != nil {
		return err
	}
	if err := validateIdentifier("execution_id", record.ID); err != nil {
		return err
	}
	if err := validateIdentifier("deployment_id", record.DeploymentID); err != nil {
		return err
	}
	if err := validateIdentifier("build_id", record.BuildID); err != nil {
		return err
	}
	if record.RerunOfRunID != "" {
		if err := validateIdentifier("rerun_of_execution_id", record.RerunOfRunID); err != nil {
			return err
		}
	}
	if !record.Status.Valid() || record.CreatedAt.IsZero() || !json.Valid(record.Input) {
		return fmt.Errorf("%w: execution metadata is invalid", ErrInvalid)
	}
	switch record.Status {
	case RunQueued:
		if record.StartedAt != nil || record.FinishedAt != nil ||
			record.Output != nil || record.Failure != nil || record.Logs != "" {
			return fmt.Errorf("%w: queued execution contains terminal state", ErrInvalid)
		}
	case RunRunning:
		if record.StartedAt == nil || record.FinishedAt != nil ||
			record.Output != nil || record.Failure != nil || record.Logs != "" {
			return fmt.Errorf("%w: running execution state is invalid", ErrInvalid)
		}
	case RunSucceeded:
		if record.StartedAt == nil || record.FinishedAt == nil ||
			!json.Valid(record.Output) || record.Failure != nil {
			return fmt.Errorf("%w: succeeded execution state is invalid", ErrInvalid)
		}
	case RunFailed:
		if record.StartedAt == nil || record.FinishedAt == nil ||
			record.Output != nil || record.Failure == nil {
			return fmt.Errorf("%w: failed execution state is invalid", ErrInvalid)
		}
	}
	if len(record.Logs) > 256<<10 {
		return fmt.Errorf("%w: execution logs exceed the persistence bound", ErrInvalid)
	}
	if record.StartedAt != nil && record.StartedAt.Before(record.CreatedAt) {
		return fmt.Errorf("%w: execution start time is invalid", ErrInvalid)
	}
	if record.FinishedAt != nil &&
		(record.StartedAt == nil || record.FinishedAt.Before(*record.StartedAt)) {
		return fmt.Errorf("%w: execution finish time is invalid", ErrInvalid)
	}
	return validateFailure(record.Failure)
}

func validateRunFinalization(current Run, next Run) error {
	if current.Status != RunRunning || !next.Status.Terminal() {
		return fmt.Errorf(
			"%w: execution must transition from running to a terminal status",
			ErrRunConflict,
		)
	}
	if current.ID != next.ID ||
		current.ProjectID != next.ProjectID ||
		current.DeploymentID != next.DeploymentID ||
		current.BuildID != next.BuildID ||
		current.RerunOfRunID != next.RerunOfRunID ||
		!current.CreatedAt.Equal(next.CreatedAt) ||
		!bytes.Equal(current.Input, next.Input) ||
		current.StartedAt == nil ||
		next.StartedAt == nil ||
		!current.StartedAt.Equal(*next.StartedAt) {
		return fmt.Errorf("%w: immutable execution metadata changed", ErrRunConflict)
	}
	return nil
}

func validateFailure(failure *Failure) error {
	if failure == nil {
		return nil
	}
	if failure.Code == "" || len(failure.Code) > 128 ||
		strings.TrimSpace(failure.Message) == "" || len(failure.Message) > 4_096 {
		return fmt.Errorf("%w: failure metadata is invalid", ErrInvalid)
	}
	return nil
}

func validateRecovery(now time.Time, failure Failure) error {
	if now.IsZero() {
		return fmt.Errorf("%w: recovery time is required", ErrInvalid)
	}
	return validateFailure(&failure)
}

func failInterruptedBuild(record *Deployment, now time.Time, failure Failure) bool {
	if record == nil || record.Status != StatusBuilding || len(record.Builds) == 0 {
		return false
	}
	index := len(record.Builds) - 1
	if record.Builds[index].Status != StatusBuilding {
		return false
	}
	record.Builds[index].Status = StatusFailed
	record.Builds[index].Failure = cloneFailure(&failure)
	record.Builds[index].FinishedAt = &now
	record.Status = StatusFailed
	record.UpdatedAt = now
	return true
}

func validateIdentifier(field, value string) error {
	if value == "" || len(value) > 255 || !utf8.ValidString(value) ||
		value != strings.TrimSpace(value) || value == "." || value == ".." {
		return fmt.Errorf("%w: %s is invalid", ErrInvalid, field)
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			character == '_' || character == '-' || character == '.' {
			continue
		}
		return fmt.Errorf("%w: %s contains an unsafe character", ErrInvalid, field)
	}
	if windowsDeviceName(value) {
		return fmt.Errorf("%w: %s uses a reserved device name", ErrInvalid, field)
	}
	return nil
}

func windowsDeviceName(value string) bool {
	base := value
	if index := strings.IndexByte(base, '.'); index >= 0 {
		base = base[:index]
	}
	switch strings.ToUpper(base) {
	case "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return true
	default:
		return false
	}
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

func recordKey(projectID, identifier string) string {
	return projectID + "\x00" + identifier
}

func sortDeployments(records []Deployment) {
	sort.Slice(records, func(left, right int) bool {
		if records[left].CreatedAt.Equal(records[right].CreatedAt) {
			return records[left].ID > records[right].ID
		}
		return records[left].CreatedAt.After(records[right].CreatedAt)
	})
}

func sortRuns(records []Run) {
	sort.Slice(records, func(left, right int) bool {
		if records[left].CreatedAt.Equal(records[right].CreatedAt) {
			return records[left].ID > records[right].ID
		}
		return records[left].CreatedAt.After(records[right].CreatedAt)
	})
}

func sortBuilds(records []Build) {
	sort.Slice(records, func(left, right int) bool {
		if records[left].StartedAt.Equal(records[right].StartedAt) {
			return records[left].ID > records[right].ID
		}
		return records[left].StartedAt.After(records[right].StartedAt)
	})
}

func cloneFailure(failure *Failure) *Failure {
	if failure == nil {
		return nil
	}
	cloned := *failure
	return &cloned
}

var _ Store = (*MemoryStore)(nil)
