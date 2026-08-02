// Package execution owns the durable record of one invocation: what was sent
// in, which exact build answered it, and how it ended.
//
// The record outlives the invocation on purpose. Accepting work is asynchronous,
// so something has to hold the request between acceptance and the worker picking
// it up; the result is read back long afterwards; and the pinned build is what
// makes a rerun reproducible.
package execution

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/neurun-io/neurun/internal/ids"
)

const MaxLogBytes = 256 << 10

var (
	ErrInvalid   = errors.New("invalid execution")
	ErrNotFound  = errors.New("execution not found")
	ErrConflict  = errors.New("execution state conflict")
	ErrNoQueued  = errors.New("no queued execution")
	ErrNotQueued = errors.New("execution is not queued")
)

type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
)

func (status Status) Valid() bool {
	switch status {
	case StatusQueued, StatusRunning, StatusSucceeded, StatusFailed:
		return true
	default:
		return false
	}
}

func (status Status) Terminal() bool {
	return status == StatusSucceeded || status == StatusFailed
}

type Failure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (failure Failure) Validate() error {
	if failure.Code == "" || len(failure.Code) > 128 ||
		strings.TrimSpace(failure.Message) == "" || len(failure.Message) > 4_096 {
		return fmt.Errorf("%w: failure metadata is invalid", ErrInvalid)
	}
	return nil
}

// Execution references its deployment and build by identifier only. It carries
// no deployment structure, so a rebuild cannot alter a finished record.
type Execution struct {
	ID                 string          `json:"id"`
	ProjectID          string          `json:"project_id"`
	DeploymentID       string          `json:"deployment_id"`
	BuildID            string          `json:"build_id"`
	Status             Status          `json:"status"`
	Input              json.RawMessage `json:"input"`
	Output             json.RawMessage `json:"output,omitempty"`
	Failure            *Failure        `json:"failure,omitempty"`
	Logs               string          `json:"logs"`
	CreatedAt          time.Time       `json:"created_at"`
	StartedAt          *time.Time      `json:"started_at,omitempty"`
	FinishedAt         *time.Time      `json:"finished_at,omitempty"`
	RerunOfExecutionID string          `json:"rerun_of_execution_id,omitempty"`
}

// New queues an invocation of buildID.
func New(
	id string,
	projectID string,
	deploymentID string,
	buildID string,
	input json.RawMessage,
	now time.Time,
) (Execution, error) {
	record := Execution{
		ID: id, ProjectID: projectID, DeploymentID: deploymentID,
		BuildID: buildID, Status: StatusQueued,
		Input: append(json.RawMessage(nil), input...), CreatedAt: now,
	}
	if err := record.Validate(); err != nil {
		return Execution{}, err
	}
	return record, nil
}

// Rerun queues a fresh invocation of the exact build this one was pinned to,
// carrying its input across unchanged.
func (record Execution) Rerun(id string, now time.Time) (Execution, error) {
	if !record.Status.Terminal() {
		return Execution{}, fmt.Errorf(
			"%w: only a finished execution can be rerun", ErrInvalid,
		)
	}
	rerun := Execution{
		ID: id, ProjectID: record.ProjectID,
		DeploymentID: record.DeploymentID, BuildID: record.BuildID,
		Status: StatusQueued,
		Input:  append(json.RawMessage(nil), record.Input...),
		// The original may itself be a rerun. Pointing at it rather than the
		// root names the execution actually repeated.
		RerunOfExecutionID: record.ID,
		CreatedAt:          now,
	}
	if err := rerun.Validate(); err != nil {
		return Execution{}, err
	}
	return rerun, nil
}

// Claim moves a queued execution to running. State left by an earlier attempt
// is cleared, so a claim always starts from a clean running record.
func (record *Execution) Claim(now time.Time) error {
	if record.Status != StatusQueued {
		return fmt.Errorf("%w: %s", ErrNotQueued, record.ID)
	}
	if now.IsZero() {
		return fmt.Errorf("%w: claim time is required", ErrInvalid)
	}
	started := now
	record.Status = StatusRunning
	record.StartedAt = &started
	record.FinishedAt = nil
	record.Output = nil
	record.Failure = nil
	record.Logs = ""
	return record.Validate()
}

func (record *Execution) Succeed(output json.RawMessage, logs string, now time.Time) error {
	if record.Status != StatusRunning {
		return fmt.Errorf("%w: only a running execution can succeed", ErrConflict)
	}
	finished := now
	record.Status = StatusSucceeded
	record.Output = append(json.RawMessage(nil), output...)
	record.Failure = nil
	record.Logs = logs
	record.FinishedAt = &finished
	return record.Validate()
}

func (record *Execution) Fail(failure Failure, logs string, now time.Time) error {
	if record.Status != StatusRunning {
		return fmt.Errorf("%w: only a running execution can fail", ErrConflict)
	}
	finished := now
	record.Status = StatusFailed
	record.Output = nil
	record.Failure = &failure
	record.Logs = logs
	record.FinishedAt = &finished
	return record.Validate()
}

func (record Execution) Validate() error {
	fields := [][2]string{
		{"project_id", record.ProjectID},
		{"execution_id", record.ID},
		{"deployment_id", record.DeploymentID},
		{"build_id", record.BuildID},
	}
	if record.RerunOfExecutionID != "" {
		fields = append(fields, [2]string{"rerun_of_execution_id", record.RerunOfExecutionID})
	}
	for _, field := range fields {
		if err := ids.Validate(field[0], field[1]); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalid, err)
		}
	}
	if !record.Status.Valid() || record.CreatedAt.IsZero() || !json.Valid(record.Input) {
		return fmt.Errorf("%w: execution metadata is invalid", ErrInvalid)
	}
	switch record.Status {
	case StatusQueued:
		if record.StartedAt != nil || record.FinishedAt != nil ||
			record.Output != nil || record.Failure != nil || record.Logs != "" {
			return fmt.Errorf("%w: queued execution contains terminal state", ErrInvalid)
		}
	case StatusRunning:
		if record.StartedAt == nil || record.FinishedAt != nil ||
			record.Output != nil || record.Failure != nil || record.Logs != "" {
			return fmt.Errorf("%w: running execution state is invalid", ErrInvalid)
		}
	case StatusSucceeded:
		if record.StartedAt == nil || record.FinishedAt == nil ||
			!json.Valid(record.Output) || record.Failure != nil {
			return fmt.Errorf("%w: succeeded execution state is invalid", ErrInvalid)
		}
	case StatusFailed:
		if record.StartedAt == nil || record.FinishedAt == nil ||
			record.Output != nil || record.Failure == nil {
			return fmt.Errorf("%w: failed execution state is invalid", ErrInvalid)
		}
	}
	if len(record.Logs) > MaxLogBytes {
		return fmt.Errorf("%w: execution logs exceed the persistence bound", ErrInvalid)
	}
	if record.StartedAt != nil && record.StartedAt.Before(record.CreatedAt) {
		return fmt.Errorf("%w: execution start time is invalid", ErrInvalid)
	}
	if record.FinishedAt != nil &&
		(record.StartedAt == nil || record.FinishedAt.Before(*record.StartedAt)) {
		return fmt.Errorf("%w: execution finish time is invalid", ErrInvalid)
	}
	if record.Failure != nil {
		return record.Failure.Validate()
	}
	return nil
}

// ValidateTransitionTo guards the compare-and-set a repository performs when
// finalizing: only a running execution may go terminal, and nothing immutable
// about it may have changed in the meantime.
func (record Execution) ValidateTransitionTo(next Execution) error {
	if record.Status != StatusRunning || !next.Status.Terminal() {
		return fmt.Errorf(
			"%w: execution must transition from running to a terminal status",
			ErrConflict,
		)
	}
	if record.ID != next.ID ||
		record.ProjectID != next.ProjectID ||
		record.DeploymentID != next.DeploymentID ||
		record.BuildID != next.BuildID ||
		record.RerunOfExecutionID != next.RerunOfExecutionID ||
		!record.CreatedAt.Equal(next.CreatedAt) ||
		!bytes.Equal(record.Input, next.Input) ||
		record.StartedAt == nil ||
		next.StartedAt == nil ||
		!record.StartedAt.Equal(*next.StartedAt) {
		return fmt.Errorf("%w: immutable execution metadata changed", ErrConflict)
	}
	return nil
}

// NormalizeInput validates one complete JSON value and returns a stable compact
// form of it. Decoding with UseNumber avoids turning large integers into lossy
// floats on the way through.
func NormalizeInput(raw json.RawMessage, maximumBytes int64) (json.RawMessage, error) {
	if maximumBytes < 1 {
		return nil, fmt.Errorf("%w: JSON byte limit must be positive", ErrInvalid)
	}
	if int64(len(raw)) > maximumBytes {
		return nil, fmt.Errorf("%w: input exceeds %d bytes", ErrInvalid, maximumBytes)
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
			return nil, fmt.Errorf("%w: input must contain exactly one JSON value", ErrInvalid)
		}
		return nil, fmt.Errorf("%w: malformed JSON: %v", ErrInvalid, err)
	}
	normalized, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("%w: normalize input: %v", ErrInvalid, err)
	}
	if int64(len(normalized)) > maximumBytes {
		return nil, fmt.Errorf("%w: input exceeds %d bytes", ErrInvalid, maximumBytes)
	}
	return normalized, nil
}

func Clone(record Execution) Execution {
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
