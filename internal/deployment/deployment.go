// Package deployment owns immutable uploaded source, build metadata, and
// durable executions pinned to an exact ready build.
package deployment

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
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

// Artifact is public immutable payload metadata. The backing object key is
// deliberately private so JSON responses cannot reveal filesystem topology.
type Artifact struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Name      string    `json:"name"`
	MediaType string    `json:"media_type"`
	SizeBytes int64     `json:"size_bytes"`
	SHA256    string    `json:"sha256"`
	CreatedAt time.Time `json:"created_at"`

	storageKey string
}

// StorageKey returns the internal immutable blob handle used by builders and
// workers. It is intentionally absent from Artifact's JSON representation.
func (artifact Artifact) StorageKey() string {
	return artifact.storageKey
}

func newArtifact(
	id string,
	kind string,
	name string,
	mediaType string,
	sizeBytes int64,
	sha256 string,
	storageKey string,
	createdAt time.Time,
) Artifact {
	return Artifact{
		ID: id, Kind: kind, Name: name, MediaType: mediaType,
		SizeBytes: sizeBytes, SHA256: sha256, CreatedAt: createdAt,
		storageKey: storageKey,
	}
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

type RunStatus string

const (
	RunQueued    RunStatus = "queued"
	RunRunning   RunStatus = "running"
	RunSucceeded RunStatus = "succeeded"
	RunFailed    RunStatus = "failed"
)

func (status RunStatus) Valid() bool {
	switch status {
	case RunQueued, RunRunning, RunSucceeded, RunFailed:
		return true
	default:
		return false
	}
}

func (status RunStatus) Terminal() bool {
	return status == RunSucceeded || status == RunFailed
}

// Run is persisted independently from its deployment and is pinned to one
// immutable build. Input and output are canonical JSON values.
type Run struct {
	ID           string          `json:"id"`
	ProjectID    string          `json:"project_id"`
	DeploymentID string          `json:"deployment_id"`
	BuildID      string          `json:"build_id"`
	Status       RunStatus       `json:"status"`
	Input        json.RawMessage `json:"input"`
	Output       json.RawMessage `json:"output,omitempty"`
	Failure      *Failure        `json:"failure,omitempty"`
	Logs         string          `json:"logs"`
	CreatedAt    time.Time       `json:"created_at"`
	StartedAt    *time.Time      `json:"started_at,omitempty"`
	FinishedAt   *time.Time      `json:"finished_at,omitempty"`
	RerunOfRunID string          `json:"rerun_of_execution_id,omitempty"`
}

// Execution aliases retain compatibility for the worker implementation while
// exposing first-class execution terminology to API code.
type Execution = Run
type ExecutionStatus = RunStatus

const (
	ExecutionQueued    = RunQueued
	ExecutionRunning   = RunRunning
	ExecutionSucceeded = RunSucceeded
	ExecutionFailed    = RunFailed
)

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

func (deployment Deployment) LatestBuild() (Build, bool) {
	if len(deployment.Builds) == 0 {
		return Build{}, false
	}
	return cloneBuild(deployment.Builds[len(deployment.Builds)-1]), true
}

func (deployment Deployment) ReadyBuild() (Build, bool) {
	for index := len(deployment.Builds) - 1; index >= 0; index-- {
		if deployment.Builds[index].Status == StatusReady {
			return cloneBuild(deployment.Builds[index]), true
		}
	}
	return Build{}, false
}

func (deployment Deployment) BuildByID(buildID string) (Build, bool) {
	for _, build := range deployment.Builds {
		if build.ID == buildID {
			return cloneBuild(build), true
		}
	}
	return Build{}, false
}

func defaultEntryPoint(runtime Runtime) string {
	if runtime == RuntimePython {
		return "main.py:handler"
	}
	return ""
}

func normalizeEntryPoint(runtime Runtime, raw string) (string, error) {
	if runtime != RuntimePython {
		return "", fmt.Errorf("%w: runtime must be python", ErrInvalid)
	}
	entryPoint := strings.TrimSpace(raw)
	if entryPoint == "" {
		entryPoint = defaultEntryPoint(runtime)
	}
	if len(entryPoint) > 512 ||
		!utf8.ValidString(entryPoint) ||
		strings.ContainsRune(entryPoint, '\x00') ||
		strings.Contains(entryPoint, "\\") {
		return "", fmt.Errorf("%w: entrypoint is malformed", ErrInvalid)
	}
	subject, handler, ok := strings.Cut(entryPoint, ":")
	if !ok || subject == "" || handler == "" || strings.Contains(handler, ":") {
		return "", fmt.Errorf(
			"%w: entrypoint must use module_or_file:handler",
			ErrInvalid,
		)
	}
	if err := validateHandler(handler); err != nil {
		return "", err
	}
	if strings.HasSuffix(subject, ".py") || strings.Contains(subject, "/") {
		if err := validateRelativeSlashPath("entrypoint", subject); err != nil {
			return "", err
		}
		if !strings.HasSuffix(subject, ".py") {
			return "", fmt.Errorf("%w: file entrypoint must end in .py", ErrInvalid)
		}
		return subject + ":" + handler, nil
	}
	for _, component := range strings.Split(subject, ".") {
		if !pythonIdentifier(component) {
			return "", fmt.Errorf("%w: entrypoint module is invalid", ErrInvalid)
		}
	}
	return subject + ":" + handler, nil
}

func validateHandler(handler string) error {
	for _, component := range strings.Split(handler, ".") {
		if !pythonIdentifier(component) {
			return fmt.Errorf("%w: entrypoint handler is invalid", ErrInvalid)
		}
	}
	return nil
}

func pythonIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for index, character := range value {
		if character == '_' || unicode.IsLetter(character) ||
			(index > 0 && unicode.IsDigit(character)) {
			continue
		}
		return false
	}
	return true
}

func validateRelativeSlashPath(field, value string) error {
	if value == "" || strings.HasPrefix(value, "/") ||
		strings.HasSuffix(value, "/") || strings.Contains(value, "\\") {
		return fmt.Errorf("%w: %s must be a relative slash path", ErrInvalid, field)
	}
	for _, component := range strings.Split(value, "/") {
		if component == "" || component == "." || component == ".." {
			return fmt.Errorf("%w: %s contains an unsafe path component", ErrInvalid, field)
		}
	}
	return nil
}

// NormalizeJSON validates one complete JSON value and returns a stable compact
// representation. Decoding with UseNumber avoids lossy float conversion.
func NormalizeJSON(raw json.RawMessage, maximumBytes int64) (json.RawMessage, error) {
	if maximumBytes < 1 {
		return nil, fmt.Errorf("%w: JSON byte limit must be positive", ErrInvalid)
	}
	if int64(len(raw)) > maximumBytes {
		return nil, fmt.Errorf("%w: JSON value exceeds %d bytes", ErrInvalid, maximumBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("%w: malformed JSON: %v", ErrInvalid, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("%w: JSON must contain exactly one value", ErrInvalid)
		}
		return nil, fmt.Errorf("%w: malformed JSON: %v", ErrInvalid, err)
	}
	normalized, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("%w: normalize JSON: %v", ErrInvalid, err)
	}
	if int64(len(normalized)) > maximumBytes {
		return nil, fmt.Errorf("%w: JSON value exceeds %d bytes", ErrInvalid, maximumBytes)
	}
	return json.RawMessage(normalized), nil
}

func cloneDeployment(record Deployment) Deployment {
	cloned := record
	cloned.Source = cloneArtifact(record.Source)
	cloned.Builds = make([]Build, len(record.Builds))
	for index := range record.Builds {
		cloned.Builds[index] = cloneBuild(record.Builds[index])
	}
	return cloned
}

func cloneBuild(build Build) Build {
	cloned := build
	cloned.Artifacts = make([]Artifact, len(build.Artifacts))
	for index := range build.Artifacts {
		cloned.Artifacts[index] = cloneArtifact(build.Artifacts[index])
	}
	if build.Failure != nil {
		failure := *build.Failure
		cloned.Failure = &failure
	}
	if build.FinishedAt != nil {
		finished := *build.FinishedAt
		cloned.FinishedAt = &finished
	}
	return cloned
}

func cloneArtifact(record Artifact) Artifact {
	return record
}

func cloneRun(record Run) Run {
	cloned := record
	cloned.Input = append(json.RawMessage(nil), record.Input...)
	cloned.Output = append(json.RawMessage(nil), record.Output...)
	if record.Failure != nil {
		failure := *record.Failure
		cloned.Failure = &failure
	}
	if record.StartedAt != nil {
		started := *record.StartedAt
		cloned.StartedAt = &started
	}
	if record.FinishedAt != nil {
		finished := *record.FinishedAt
		cloned.FinishedAt = &finished
	}
	return cloned
}
