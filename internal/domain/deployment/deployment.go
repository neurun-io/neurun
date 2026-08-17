// Package deployment owns the act of turning one source archive into a build:
// how far it got, what the toolchain printed on the way, and what it produced.
package deployment

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/neurun-io/neurun/internal/domain/build"
)

var (
	ErrInvalid  = errors.New("invalid deployment")
	ErrNotFound = errors.New("deployment not found")
	ErrNotReady = errors.New("deployment is not ready")
	// ErrNoQueued is an idle queue, which is the ordinary state of one.
	ErrNoQueued = errors.New("no queued deployment")
)

type Status string

// The stages a deployment moves through. Each is a different thing to be
// waiting on, which is why queueing and publishing are not folded into
// building: one waits for a slot, the other for an upload.
const (
	StatusQueued     Status = "queued"
	StatusBuilding   Status = "building"
	StatusPublishing Status = "publishing"
	StatusReady      Status = "ready"
	StatusFailed     Status = "failed"
)

func (status Status) Valid() bool {
	switch status {
	case StatusQueued, StatusBuilding, StatusPublishing,
		StatusReady, StatusFailed:
		return true
	default:
		return false
	}
}

// Running reports whether the deployment still has somewhere to go, which is
// also what tells a reader to keep following its output.
func (status Status) Running() bool {
	switch status {
	case StatusQueued, StatusBuilding, StatusPublishing:
		return true
	default:
		return false
	}
}

// stages is the only path through the statuses. Anything else is a bug in the
// caller rather than a state a deployment can hold.
var stages = map[Status]Status{
	StatusQueued:   StatusBuilding,
	StatusBuilding: StatusPublishing,
}

type Failure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Deployment struct {
	ID        string        `json:"id"`
	ProjectID string        `json:"project_id"`
	AppID     string        `json:"app_id"`
	Runtime   build.Runtime `json:"runtime"`
	Status    Status        `json:"status"`
	// CommitSHA and GitRef say which bytes were built. The SHA is what actually
	// built; the ref is what the caller asked for.
	CommitSHA string `json:"commit_sha,omitempty"`
	GitRef    string `json:"git_ref,omitempty"`
	// Build is the output, nil until there is one. Failure is why there is not.
	Build      *build.Build `json:"build,omitempty"`
	Failure    *Failure     `json:"failure,omitempty"`
	Logs       string       `json:"logs"`
	StartedAt  *time.Time   `json:"started_at,omitempty"`
	FinishedAt *time.Time   `json:"finished_at,omitempty"`
	CreatedAt  time.Time    `json:"created_at"`
	UpdatedAt  time.Time    `json:"updated_at"`
}

// FromGit records which commit the archive was fetched from.
func (record *Deployment) FromGit(commitSHA, ref string) {
	record.CommitSHA = strings.TrimSpace(commitSHA)
	record.GitRef = strings.TrimSpace(ref)
}

// New assembles a queued deployment, ready for the source it is about to build.
func New(
	deploymentID string,
	projectID string,
	appID string,
	runtime build.Runtime,
	now time.Time,
) (Deployment, error) {
	record := Deployment{
		ID: deploymentID, ProjectID: projectID, AppID: appID,
		Runtime: runtime, Status: StatusQueued,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := record.Validate(); err != nil {
		return Deployment{}, err
	}
	return record, nil
}

// Advance moves the deployment on to the stage that follows the one it is in.
// Entering the toolchain is what starts the clock.
func (record *Deployment) Advance(now time.Time) error {
	following, ok := stages[record.Status]
	if !ok {
		return fmt.Errorf(
			"%w: deployment %s cannot advance from %s", ErrInvalid, record.ID, record.Status,
		)
	}
	if following == StatusBuilding {
		started := now
		record.StartedAt = &started
	}
	record.Status = following
	record.UpdatedAt = now
	return nil
}

// MarkReady hands the deployment the build it produced. The build arrives
// already sealed: a deployment finishes one, it does not make one.
func (record *Deployment) MarkReady(produced build.Build, now time.Time) error {
	if !record.Status.Running() {
		return fmt.Errorf(
			"%w: deployment %s is already finished", ErrInvalid, record.ID,
		)
	}
	if err := produced.Validate(); err != nil {
		return err
	}
	finished := now
	record.Build = &produced
	record.Failure = nil
	record.Status = StatusReady
	record.FinishedAt = &finished
	record.UpdatedAt = finished
	return nil
}

// Fail stops the deployment with the reason it stopped. It is reachable from
// every running stage, including before the toolchain ever ran — which is why
// a failure leaves no build behind: there was nothing to point at.
func (record *Deployment) Fail(failure Failure, now time.Time) error {
	if !record.Status.Running() {
		return fmt.Errorf(
			"%w: deployment %s is already finished", ErrInvalid, record.ID,
		)
	}
	if err := validateFailure(&failure); err != nil {
		return err
	}
	finished := now
	record.Failure = CloneFailure(&failure)
	record.Status = StatusFailed
	record.FinishedAt = &finished
	record.UpdatedAt = finished
	return nil
}

// FailInterrupted fails a deployment a crashed process left running, reporting
// whether it changed anything. It never retries the build's side effects.
func (record *Deployment) FailInterrupted(now time.Time, failure Failure) bool {
	if !record.Status.Running() {
		return false
	}
	finished := now
	record.Failure = CloneFailure(&failure)
	record.Status = StatusFailed
	record.FinishedAt = &finished
	record.UpdatedAt = now
	return true
}

// MaxLogBytes is what a deployment keeps of what the toolchain printed. The
// tail is what survives: the end of a cargo build is where the error is.
const MaxLogBytes = 262_144

// Log appends toolchain output, trimming from the front once it outgrows the
// cap so the newest output is always the part that is kept.
func (record *Deployment) Log(output string) {
	if output == "" {
		return
	}
	combined := record.Logs + output
	if len(combined) > MaxLogBytes {
		combined = combined[len(combined)-MaxLogBytes:]
	}
	record.Logs = combined
}

func CloneDeployment(record Deployment) Deployment {
	cloned := record
	cloned.Build = build.Clone(record.Build)
	cloned.Failure = CloneFailure(record.Failure)
	cloned.StartedAt = cloneTime(record.StartedAt)
	cloned.FinishedAt = cloneTime(record.FinishedAt)
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
