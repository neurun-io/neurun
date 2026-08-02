// Package deployment owns immutable uploaded source, build metadata, and
// durable executions pinned to an exact ready build.
package deployment

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrInvalid           = errors.New("invalid deployment")
	ErrNotFound          = errors.New("deployment not found")
	ErrExecutionNotFound = errors.New("deployment execution not found")
	ErrExecutionConflict = errors.New("deployment execution state conflict")
	// Compatibility aliases keep worker internals concise while public errors
	// and JSON consistently use execution vocabulary.
	ErrRunNotFound       = ErrExecutionNotFound
	ErrRunConflict       = ErrExecutionConflict
	ErrNoReadyBuild      = errors.New("deployment has no ready build")
	ErrNoQueuedExecution = errors.New("no queued deployment execution")
	ErrNoQueuedRun       = ErrNoQueuedExecution
	ErrSourceTooLarge    = errors.New("deployment source is too large")
)

// Runtime identifies a pluggable builder/runner pair. The first release only
// enables Python, while keeping the boundary explicit for future runtimes.
type Runtime string

const RuntimePython Runtime = "python"

func (runtime Runtime) Valid() bool {
	return runtime == RuntimePython
}

func ParseRuntime(raw string) (Runtime, error) {
	runtime := Runtime(strings.ToLower(strings.TrimSpace(raw)))
	if !runtime.Valid() {
		return "", fmt.Errorf("%w: runtime must be python", ErrInvalid)
	}
	return runtime, nil
}

type Status string

const (
	StatusUploaded Status = "uploaded"
	StatusBuilding Status = "building"
	StatusReady    Status = "ready"
	StatusFailed   Status = "failed"
)

func (status Status) Valid() bool {
	switch status {
	case StatusUploaded, StatusBuilding, StatusReady, StatusFailed:
		return true
	default:
		return false
	}
}

const (
	ArtifactSource       = "deployment_source"
	ArtifactInstallLayer = "install_layer"
	ArtifactCodeLayer    = "code_layer"
)

// Artifact is immutable payload metadata. StorageKey is the blob handle
// builders and workers open; it names internal storage topology, so the API
// projects artifacts into a response type rather than serving this one.
type Artifact struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"`
	Name       string    `json:"name"`
	MediaType  string    `json:"media_type"`
	SizeBytes  int64     `json:"size_bytes"`
	SHA256     string    `json:"sha256"`
	StorageKey string    `json:"storage_key"`
	CreatedAt  time.Time `json:"created_at"`
}

type Failure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Build struct {
	ID           string     `json:"id"`
	ProjectID    string     `json:"project_id"`
	DeploymentID string     `json:"deployment_id"`
	Number       int        `json:"number"`
	Status       Status     `json:"status"`
	Runtime      Runtime    `json:"runtime"`
	EntryPoint   string     `json:"entrypoint"`
	SourceSHA256 string     `json:"source_sha256"`
	Artifacts    []Artifact `json:"artifacts"`
	Failure      *Failure   `json:"failure,omitempty"`
	StartedAt    time.Time  `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
}

type Deployment struct {
	ID         string    `json:"id"`
	ProjectID  string    `json:"project_id"`
	AppID      string    `json:"app_id"`
	Runtime    Runtime   `json:"runtime"`
	EntryPoint string    `json:"entrypoint"`
	Status     Status    `json:"status"`
	Source     Artifact  `json:"source"`
	Builds     []Build   `json:"builds"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// New assembles an uploaded deployment around its stored source artifact.
func New(
	deploymentID string,
	projectID string,
	appID string,
	runtime Runtime,
	entryPoint string,
	source Artifact,
	now time.Time,
) (Deployment, error) {
	normalized, err := NormalizeEntryPoint(runtime, entryPoint)
	if err != nil {
		return Deployment{}, err
	}
	record := Deployment{
		ID: deploymentID, ProjectID: projectID, AppID: appID,
		Runtime: runtime, EntryPoint: normalized, Status: StatusUploaded,
		Source: source, Builds: []Build{}, CreatedAt: now, UpdatedAt: now,
	}
	if err := record.Validate(); err != nil {
		return Deployment{}, err
	}
	return record, nil
}

func (record Deployment) LatestBuild() (Build, bool) {
	if len(record.Builds) == 0 {
		return Build{}, false
	}
	return CloneBuild(record.Builds[len(record.Builds)-1]), true
}

func (record Deployment) ReadyBuild() (Build, bool) {
	for index := len(record.Builds) - 1; index >= 0; index-- {
		if record.Builds[index].Status == StatusReady {
			return CloneBuild(record.Builds[index]), true
		}
	}
	return Build{}, false
}

func (record Deployment) BuildByID(buildID string) (Build, bool) {
	for _, build := range record.Builds {
		if build.ID == buildID {
			return CloneBuild(build), true
		}
	}
	return Build{}, false
}

// StartBuild appends a building build numbered after the last one and moves the
// deployment's own status with it.
func (record *Deployment) StartBuild(buildID string, now time.Time) (Build, error) {
	if err := ValidateIdentifier("build_id", buildID); err != nil {
		return Build{}, err
	}
	if now.IsZero() || now.Before(record.CreatedAt) {
		return Build{}, fmt.Errorf("%w: build start time is invalid", ErrInvalid)
	}
	build := Build{
		ID: buildID, ProjectID: record.ProjectID, DeploymentID: record.ID,
		Number: len(record.Builds) + 1, Status: StatusBuilding,
		Runtime: record.Runtime, EntryPoint: record.EntryPoint,
		SourceSHA256: record.Source.SHA256, Artifacts: []Artifact{},
		StartedAt: now,
	}
	record.Builds = append(record.Builds, build)
	record.Status = StatusBuilding
	record.UpdatedAt = now
	return CloneBuild(build), nil
}

// MarkBuildReady completes a building build with the artifacts it produced.
func (record *Deployment) MarkBuildReady(
	buildID string,
	artifacts []Artifact,
	now time.Time,
) error {
	build, err := record.buildInProgress(buildID)
	if err != nil {
		return err
	}
	if len(artifacts) == 0 {
		return fmt.Errorf("%w: ready build requires artifacts", ErrInvalid)
	}
	finished := now
	build.Status = StatusReady
	build.Artifacts = artifacts
	build.Failure = nil
	build.FinishedAt = &finished
	record.Status = StatusReady
	record.UpdatedAt = finished
	return nil
}

// FailBuild terminates a building build with the reason it stopped.
func (record *Deployment) FailBuild(
	buildID string,
	failure Failure,
	now time.Time,
) error {
	build, err := record.buildInProgress(buildID)
	if err != nil {
		return err
	}
	if err := validateFailure(&failure); err != nil {
		return err
	}
	finished := now
	build.Status = StatusFailed
	build.Failure = CloneFailure(&failure)
	build.FinishedAt = &finished
	record.Status = StatusFailed
	record.UpdatedAt = finished
	return nil
}

// FailInterruptedBuild fails a build a crashed process left running, reporting
// whether it changed anything. It never retries the build's side effects.
func (record *Deployment) FailInterruptedBuild(now time.Time, failure Failure) bool {
	if record.Status != StatusBuilding || len(record.Builds) == 0 {
		return false
	}
	index := len(record.Builds) - 1
	if record.Builds[index].Status != StatusBuilding {
		return false
	}
	finished := now
	record.Builds[index].Status = StatusFailed
	record.Builds[index].Failure = CloneFailure(&failure)
	record.Builds[index].FinishedAt = &finished
	record.Status = StatusFailed
	record.UpdatedAt = now
	return true
}

func (record *Deployment) buildInProgress(buildID string) (*Build, error) {
	for index := range record.Builds {
		if record.Builds[index].ID != buildID {
			continue
		}
		if record.Builds[index].Status != StatusBuilding {
			return nil, fmt.Errorf(
				"%w: build %s is no longer building", ErrInvalid, buildID,
			)
		}
		return &record.Builds[index], nil
	}
	return nil, fmt.Errorf("%w: build %s", ErrNotFound, buildID)
}

func CloneDeployment(record Deployment) Deployment {
	cloned := record
	cloned.Builds = make([]Build, len(record.Builds))
	for index := range record.Builds {
		cloned.Builds[index] = CloneBuild(record.Builds[index])
	}
	return cloned
}

func CloneBuild(build Build) Build {
	cloned := build
	cloned.Artifacts = append([]Artifact(nil), build.Artifacts...)
	cloned.Failure = CloneFailure(build.Failure)
	cloned.FinishedAt = cloneTime(build.FinishedAt)
	return cloned
}

func CloneFailure(failure *Failure) *Failure {
	if failure == nil {
		return nil
	}
	cloned := *failure
	return &cloned
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
