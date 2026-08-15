// Package build owns what a deployment produces: the artifacts, and enough
// about them to know what they run. Nothing here records how the build went —
// that belongs to the deployment that ran it.
package build

import (
	"errors"
	"fmt"
	"time"

	"github.com/neurun-io/neurun/internal/ids"
)

var (
	ErrInvalid  = errors.New("invalid build")
	ErrNotFound = errors.New("build not found")
)

// Build is what one deployment produced: the layers it made, each named for
// what it is to the runtime. Every build has a code layer; a runtime that
// resolves dependencies separately adds an install layer, and a toolchain that
// grows a third only has to name it.
type Build struct {
	ID           string     `json:"id"`
	Runtime      Runtime    `json:"runtime"`
	EntryPoint   string     `json:"entrypoint"`
	SourceSHA256 string     `json:"source_sha256"`
	Artifacts    []Artifact `json:"artifacts"`
	CreatedAt    time.Time  `json:"created_at"`
}

// New seals what a deployment built into the build it points at.
func New(
	buildID string,
	runtime Runtime,
	entryPoint string,
	sourceSHA256 string,
	artifacts []Artifact,
	now time.Time,
) (Build, error) {
	record := Build{
		ID: buildID, Runtime: runtime, EntryPoint: entryPoint,
		SourceSHA256: sourceSHA256,
		Artifacts:    append([]Artifact(nil), artifacts...),
		CreatedAt:    now,
	}
	if err := record.Validate(); err != nil {
		return Build{}, err
	}
	return record, nil
}

// Layer returns the artifact a runner unpacks under that name.
func (record Build) Layer(name string) (Artifact, bool) {
	for _, artifact := range record.Artifacts {
		if artifact.Name == name {
			return artifact, true
		}
	}
	return Artifact{}, false
}

// Validate checks the output on its own terms. Whether it matches the
// deployment that made it is the deployment's business.
func (record Build) Validate() error {
	if err := ValidateIdentifier("build_id", record.ID); err != nil {
		return err
	}
	if !record.Runtime.Valid() {
		return fmt.Errorf("%w: build runtime is invalid", ErrInvalid)
	}
	if len(record.SourceSHA256) != 64 {
		return fmt.Errorf("%w: build source digest is invalid", ErrInvalid)
	}
	if record.CreatedAt.IsZero() {
		return fmt.Errorf("%w: build creation time is required", ErrInvalid)
	}
	names := make(map[string]struct{}, len(record.Artifacts))
	for _, artifact := range record.Artifacts {
		if err := ValidateArtifact(artifact); err != nil {
			return err
		}
		if _, duplicate := names[artifact.Name]; duplicate {
			return fmt.Errorf(
				"%w: build has two %s layers", ErrInvalid, artifact.Name,
			)
		}
		names[artifact.Name] = struct{}{}
	}
	if _, exists := names[LayerCode]; !exists {
		return fmt.Errorf("%w: a build requires a code layer", ErrInvalid)
	}
	if _, exists := names[LayerInstall]; exists && record.Runtime.Compiled() {
		return fmt.Errorf("%w: a compiled build has no install layer", ErrInvalid)
	}
	return nil
}

func Clone(record *Build) *Build {
	if record == nil {
		return nil
	}
	cloned := *record
	cloned.Artifacts = append([]Artifact(nil), record.Artifacts...)
	return &cloned
}

func ValidateIdentifier(field, value string) error {
	if err := ids.Validate(field, value); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return nil
}
