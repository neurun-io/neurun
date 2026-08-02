package deployment

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrAppNotFound = errors.New("app not found")
	ErrAppConflict = errors.New("app conflict")
)

// App is the durable project-scoped owner of deployments.
type App struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
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
	return nil
}

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
