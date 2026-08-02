package deployment

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
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

type CreateAppRequest struct {
	ProjectID string
	Name      string
}

type UpdateAppRequest struct {
	Name *string
}

func (service *Service) CreateApp(
	ctx context.Context,
	request CreateAppRequest,
) (App, error) {
	if err := ValidateIdentifier("project_id", request.ProjectID); err != nil {
		return App{}, err
	}
	name, err := normalizeAppName(request.Name)
	if err != nil {
		return App{}, err
	}
	if _, err := service.store.GetProject(ctx, request.ProjectID); err != nil {
		return App{}, err
	}
	id, err := service.allocateID("app")
	if err != nil {
		return App{}, err
	}
	now := service.now().UTC().Round(0)
	return service.store.CreateApp(ctx, App{
		ID: id, ProjectID: request.ProjectID, Name: name,
		CreatedAt: now, UpdatedAt: now,
	})
}

func (service *Service) GetApp(
	ctx context.Context,
	projectID string,
	appID string,
) (App, error) {
	return service.store.GetApp(ctx, projectID, appID)
}

func (service *Service) ListApps(
	ctx context.Context,
	projectID string,
	name string,
	limit int,
) ([]App, error) {
	if limit < 1 || limit > 200 {
		return nil, fmt.Errorf("%w: limit must be between 1 and 200", ErrInvalid)
	}
	if err := ValidateAppNameFilter(name); err != nil {
		return nil, err
	}
	if _, err := service.store.GetProject(ctx, projectID); err != nil {
		return nil, err
	}
	return service.store.ListApps(ctx, projectID, name, limit)
}

func (service *Service) UpdateApp(
	ctx context.Context,
	projectID string,
	appID string,
	request UpdateAppRequest,
) (App, error) {
	if request.Name == nil {
		return App{}, fmt.Errorf("%w: app update is empty", ErrInvalid)
	}
	record, err := service.store.GetApp(ctx, projectID, appID)
	if err != nil {
		return App{}, err
	}
	record.Name, err = normalizeAppName(*request.Name)
	if err != nil {
		return App{}, err
	}
	record.UpdatedAt = service.now().UTC().Round(0)
	if record.UpdatedAt.Before(record.CreatedAt) {
		record.UpdatedAt = record.CreatedAt
	}
	return service.store.UpdateApp(ctx, record)
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

func cloneApp(record App) App { return record }
