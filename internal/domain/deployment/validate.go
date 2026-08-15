package deployment

import (
	"fmt"
	"strings"
	"time"

	"github.com/neurun-io/neurun/internal/domain/build"
	"github.com/neurun-io/neurun/internal/ids"
)

func ValidateIdentifier(field, value string) error {
	if err := ids.Validate(field, value); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return nil
}

func (record Deployment) Validate() error {
	if err := ValidateIdentifier("project_id", record.ProjectID); err != nil {
		return err
	}
	if err := ValidateIdentifier("deployment_id", record.ID); err != nil {
		return err
	}
	if err := ValidateIdentifier("app_id", record.AppID); err != nil {
		return err
	}
	if !record.Runtime.Valid() {
		return fmt.Errorf("%w: deployment runtime is invalid", ErrInvalid)
	}
	normalized, err := build.NormalizeEntryPoint(record.Runtime, record.EntryPoint)
	if err != nil || normalized != record.EntryPoint {
		return fmt.Errorf("%w: deployment entrypoint is not normalized", ErrInvalid)
	}
	if !record.Status.Valid() {
		return fmt.Errorf("%w: deployment status is invalid", ErrInvalid)
	}
	if record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() ||
		record.UpdatedAt.Before(record.CreatedAt) {
		return fmt.Errorf("%w: deployment timestamps are invalid", ErrInvalid)
	}
	if err := build.ValidateArtifact(record.Source); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if len(record.Logs) > MaxLogBytes {
		return fmt.Errorf("%w: deployment logs exceed the cap", ErrInvalid)
	}
	if err := validateTiming(record); err != nil {
		return err
	}
	if err := validateOutcome(record); err != nil {
		return err
	}
	return validateFailure(record.Failure)
}

func validateTiming(record Deployment) error {
	if record.StartedAt != nil && record.StartedAt.Before(record.CreatedAt) {
		return fmt.Errorf("%w: deployment start time is invalid", ErrInvalid)
	}
	if record.FinishedAt == nil {
		return nil
	}
	if record.FinishedAt.Before(record.CreatedAt) ||
		(record.StartedAt != nil && record.FinishedAt.Before(*record.StartedAt)) {
		return fmt.Errorf("%w: deployment finish time is invalid", ErrInvalid)
	}
	return nil
}

// validateOutcome holds the statuses to what they claim: a running deployment
// has not finished, a ready one has the build it made, and only a failed one
// carries the reason it made none.
func validateOutcome(record Deployment) error {
	if record.Status.Running() &&
		(record.Build != nil || record.Failure != nil || record.FinishedAt != nil) {
		return fmt.Errorf("%w: a running deployment has not finished", ErrInvalid)
	}
	switch record.Status {
	case StatusQueued:
		if record.StartedAt != nil {
			return fmt.Errorf("%w: a queued deployment has not started", ErrInvalid)
		}
	case StatusBuilding, StatusPublishing:
		if record.StartedAt == nil {
			return fmt.Errorf("%w: a running deployment has a start time", ErrInvalid)
		}
	case StatusReady:
		if record.Build == nil || record.FinishedAt == nil {
			return fmt.Errorf("%w: a ready deployment has a build", ErrInvalid)
		}
	case StatusFailed:
		if record.Failure == nil || record.FinishedAt == nil {
			return fmt.Errorf("%w: a failed deployment requires failure metadata", ErrInvalid)
		}
		if record.Build != nil {
			return fmt.Errorf("%w: a failed deployment produced no build", ErrInvalid)
		}
	}
	if record.Build == nil {
		return nil
	}
	if err := record.Build.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if record.Build.Runtime != record.Runtime ||
		record.Build.EntryPoint != record.EntryPoint ||
		record.Build.SourceSHA256 != record.Source.SHA256 {
		return fmt.Errorf("%w: build metadata is inconsistent", ErrInvalid)
	}
	if record.Build.CreatedAt.Before(record.CreatedAt) {
		return fmt.Errorf("%w: build creation time is invalid", ErrInvalid)
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

func ValidateRecovery(now time.Time, failure Failure) error {
	if now.IsZero() {
		return fmt.Errorf("%w: recovery time is required", ErrInvalid)
	}
	return validateFailure(&failure)
}
