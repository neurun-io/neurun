// Package app owns the durable thing a deployment deploys: one project-scoped
// unit, its GitHub repository, and the ref it follows.
package app

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/neurun-io/neurun/internal/ids"
)

var (
	ErrInvalid  = errors.New("invalid app")
	ErrNotFound = errors.New("app not found")
	ErrConflict = errors.New("app conflict")
)

// App is the durable project-scoped owner of deployments.
type App struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	// Repository is owner/name on GitHub, empty when the app is deployed by
	// upload instead.
	Repository    string `json:"repository,omitempty"`
	ProductionRef string `json:"production_ref,omitempty"`
	// ActiveBuildID is the build the app runs. Empty runs its newest ready one.
	ActiveBuildID string    `json:"active_build_id,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func New(id, projectID, name string, now time.Time) (App, error) {
	normalized, err := NormalizeName(name)
	if err != nil {
		return App{}, err
	}
	record := App{
		ID: id, ProjectID: projectID, Name: normalized,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := record.Validate(); err != nil {
		return App{}, err
	}
	return record, nil
}

// Connect points the app at a GitHub repository. An empty repository
// disconnects it, which also clears the ref there is no longer anything to
// resolve against.
func (record *App) Connect(repository, productionRef string, now time.Time) error {
	repository = strings.TrimSpace(repository)
	productionRef = strings.TrimSpace(productionRef)
	if repository == "" {
		record.Repository, record.ProductionRef = "", ""
		record.touch(now)
		return record.Validate()
	}
	record.Repository = repository
	record.ProductionRef = productionRef
	record.touch(now)
	return record.Validate()
}

// TracksRef reports whether a push to ref should deploy this app. An app that
// names no production ref follows the repository's default branch, which is
// what HEAD resolves to when the ref is deployed by hand.
func (record App) TracksRef(ref, defaultBranch string) bool {
	if record.Repository == "" || ref == "" {
		return false
	}
	tracked := strings.TrimSpace(record.ProductionRef)
	if tracked == "" || tracked == "HEAD" {
		tracked = strings.TrimSpace(defaultBranch)
	}
	if tracked == "" {
		return false
	}
	return ref == tracked ||
		ref == "refs/heads/"+tracked ||
		ref == "refs/tags/"+tracked
}

// ActivateBuild pins the build the app runs. An empty build releases the pin,
// and the app follows its newest ready build again.
func (record *App) ActivateBuild(buildID string, now time.Time) error {
	record.ActiveBuildID = strings.TrimSpace(buildID)
	record.touch(now)
	return record.Validate()
}

func (record *App) Rename(name string, now time.Time) error {
	normalized, err := NormalizeName(name)
	if err != nil {
		return err
	}
	record.Name = normalized
	record.touch(now)
	return record.Validate()
}

func (record *App) touch(now time.Time) {
	if now.Before(record.CreatedAt) {
		now = record.CreatedAt
	}
	record.UpdatedAt = now
}

func (record App) Validate() error {
	if err := ValidateIdentifier("project_id", record.ProjectID); err != nil {
		return err
	}
	if err := ValidateIdentifier("app_id", record.ID); err != nil {
		return err
	}
	name, err := NormalizeName(record.Name)
	if err != nil || name != record.Name {
		return fmt.Errorf("%w: app name is not normalized", ErrInvalid)
	}
	if record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() ||
		record.UpdatedAt.Before(record.CreatedAt) {
		return fmt.Errorf("%w: app timestamps are invalid", ErrInvalid)
	}
	if record.Repository != "" && !repositoryPattern.MatchString(record.Repository) {
		return fmt.Errorf("%w: repository must be owner/name", ErrInvalid)
	}
	if record.ProductionRef != "" && record.Repository == "" {
		return fmt.Errorf("%w: a production ref needs a repository", ErrInvalid)
	}
	if record.ActiveBuildID != "" {
		if err := ValidateIdentifier("active_build_id", record.ActiveBuildID); err != nil {
			return err
		}
	}
	return nil
}

var repositoryPattern = regexp.MustCompile(`^[^/[:space:]]+/[^/[:space:]]+$`)

func ValidateIdentifier(field, value string) error {
	if err := ids.Validate(field, value); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return nil
}

// NormalizeName trims a human-facing name and rejects anything that would not
// survive a round trip through JSON or a terminal.
func NormalizeName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" || len(name) > 120 || !utf8.ValidString(name) {
		return "", fmt.Errorf("%w: app name must contain 1 to 120 bytes", ErrInvalid)
	}
	for _, character := range name {
		if unicode.IsControl(character) {
			return "", fmt.Errorf("%w: app name contains a control character", ErrInvalid)
		}
	}
	return name, nil
}

// ValidateNameFilter accepts an empty filter, and otherwise requires the caller
// to have sent the exact stored form — a list filter that silently trimmed its
// argument would not match what a rename wrote.
func ValidateNameFilter(name string) error {
	if name == "" {
		return nil
	}
	normalized, err := NormalizeName(name)
	if err != nil {
		return err
	}
	if normalized != name {
		return fmt.Errorf("%w: app name filter is not normalized", ErrInvalid)
	}
	return nil
}
