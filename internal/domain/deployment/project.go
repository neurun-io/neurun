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
	ErrProjectNotFound = errors.New("project not found")
	ErrProjectConflict = errors.New("project conflict")
)

type Project struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type UpdateProjectRequest struct {
	Name *string
}

func (service *Service) EnsureProject(
	ctx context.Context,
	projectID string,
	name string,
) (Project, error) {
	if err := ValidateIdentifier("project_id", projectID); err != nil {
		return Project{}, err
	}
	name, err := normalizeProjectName(name)
	if err != nil {
		return Project{}, err
	}
	now := service.now().UTC().Round(0)
	return service.store.EnsureProject(ctx, Project{
		ID: projectID, Name: name,
		CreatedAt: now, UpdatedAt: now,
	})
}

func (service *Service) GetProject(
	ctx context.Context,
	projectID string,
) (Project, error) {
	return service.store.GetProject(ctx, projectID)
}

func (service *Service) ListProjects(
	ctx context.Context,
	principalProjectID string,
	limit int,
) ([]Project, error) {
	if limit < 1 || limit > 200 {
		return nil, fmt.Errorf("%w: limit must be between 1 and 200", ErrInvalid)
	}
	return service.store.ListProjects(ctx, principalProjectID, limit)
}

func (service *Service) UpdateProject(
	ctx context.Context,
	projectID string,
	request UpdateProjectRequest,
) (Project, error) {
	if request.Name == nil {
		return Project{}, fmt.Errorf("%w: project update is empty", ErrInvalid)
	}
	record, err := service.store.GetProject(ctx, projectID)
	if err != nil {
		return Project{}, err
	}
	if request.Name != nil {
		record.Name, err = normalizeProjectName(*request.Name)
		if err != nil {
			return Project{}, err
		}
	}
	record.UpdatedAt = service.now().UTC().Round(0)
	if record.UpdatedAt.Before(record.CreatedAt) {
		record.UpdatedAt = record.CreatedAt
	}
	return service.store.UpdateProject(ctx, record)
}

func (record Project) Validate() error {
	if err := ValidateIdentifier("project_id", record.ID); err != nil {
		return err
	}
	name, err := normalizeProjectName(record.Name)
	if err != nil || name != record.Name {
		return fmt.Errorf("%w: project name is not normalized", ErrInvalid)
	}
	if record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() ||
		record.UpdatedAt.Before(record.CreatedAt) {
		return fmt.Errorf("%w: project timestamps are invalid", ErrInvalid)
	}
	return nil
}

func normalizeProjectName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" || len(name) > 120 || !utf8.ValidString(name) {
		return "", fmt.Errorf("%w: project name must contain 1 to 120 bytes", ErrInvalid)
	}
	for _, character := range name {
		if unicode.IsControl(character) {
			return "", fmt.Errorf("%w: project name contains a control character", ErrInvalid)
		}
	}
	return name, nil
}

func cloneProject(record Project) Project {
	return record
}
