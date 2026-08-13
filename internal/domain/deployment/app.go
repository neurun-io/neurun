package deployment

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	ErrAppNotFound = errors.New("app not found")
	ErrAppConflict = errors.New("app conflict")
)

// App is the durable project-scoped owner of deployments.
type App struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	// Repository is owner/name on GitHub, empty when the app is deployed by
	// upload instead.
	Repository    string    `json:"repository,omitempty"`
	ProductionRef string    `json:"production_ref,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Connect points the app at a GitHub repository. An empty repository
// disconnects it, which also clears the ref there is no longer anything to
// resolve against.
func (record *App) Connect(repository, productionRef string, now time.Time) error {
	repository = strings.TrimSpace(repository)
	productionRef = strings.TrimSpace(productionRef)
	if repository == "" {
		record.Repository, record.ProductionRef = "", ""
		record.UpdatedAt = notBefore(now, record.CreatedAt)
		return record.Validate()
	}
	record.Repository = repository
	record.ProductionRef = productionRef
	record.UpdatedAt = notBefore(now, record.CreatedAt)
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

func NewApp(id, projectID, name string, now time.Time) (App, error) {
	normalized, err := normalizeAppName(name)
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

func (record *App) Rename(name string, now time.Time) error {
	normalized, err := normalizeAppName(name)
	if err != nil {
		return err
	}
	record.Name = normalized
	record.UpdatedAt = notBefore(now, record.CreatedAt)
	return record.Validate()
}

func (record App) Validate() error {
	if err := ValidateIdentifier("project_id", record.ProjectID); err != nil {
		return err
	}
	if err := ValidateIdentifier("app_id", record.ID); err != nil {
		return err
	}
	name, err := normalizeAppName(record.Name)
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
	return nil
}

var repositoryPattern = regexp.MustCompile(`^[^/[:space:]]+/[^/[:space:]]+$`)

func normalizeAppName(raw string) (string, error) {
	return normalizeDisplayName("app", raw)
}

// ValidateAppNameFilter accepts an empty filter, and otherwise requires the
// caller to have sent the exact stored form — a list filter that silently
// trimmed its argument would not match what a rename wrote.
func ValidateAppNameFilter(name string) error {
	if name == "" {
		return nil
	}
	normalized, err := normalizeAppName(name)
	if err != nil {
		return err
	}
	if normalized != name {
		return fmt.Errorf("%w: app name filter is not normalized", ErrInvalid)
	}
	return nil
}
