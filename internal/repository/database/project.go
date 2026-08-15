package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neurun-io/neurun/internal/domain/project"
)

const projectColumns = `id, organization_id, name, created_at, updated_at`

type ProjectRepository struct {
	pool *pgxpool.Pool
}

func NewProjectRepository(pool *pgxpool.Pool) (*ProjectRepository, error) {
	if pool == nil {
		return nil, errors.New("project repository requires a database pool")
	}
	return &ProjectRepository{pool: pool}, nil
}

func (repository *ProjectRepository) GetByID(
	ctx context.Context,
	organizationID string,
	projectID string,
) (project.Project, error) {
	if err := project.ValidateIdentifier("project_id", projectID); err != nil {
		return project.Project{}, err
	}
	var record project.Project
	err := repository.pool.QueryRow(
		ctx,
		`SELECT `+projectColumns+` FROM projects
		 WHERE id = $1 AND organization_id = $2`,
		projectID, organizationID,
	).Scan(
		&record.ID, &record.OrganizationID, &record.Name,
		&record.CreatedAt, &record.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return project.Project{}, fmt.Errorf(
			"%w: %s", project.ErrNotFound, projectID,
		)
	}
	if err != nil {
		return project.Project{}, fmt.Errorf("read project: %w", err)
	}
	if err := record.Validate(); err != nil {
		return project.Project{}, fmt.Errorf("invalid persisted project: %w", err)
	}
	return record, nil
}

// Create inserts a project and fails if the identifier or name is taken.
func (repository *ProjectRepository) Create(
	ctx context.Context,
	record project.Project,
) (project.Project, error) {
	if err := record.Validate(); err != nil {
		return project.Project{}, err
	}
	_, err := repository.pool.Exec(
		ctx,
		`INSERT INTO projects (id, organization_id, name, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		record.ID, record.OrganizationID, record.Name,
		record.CreatedAt, record.UpdatedAt,
	)
	if err != nil {
		return project.Project{}, fmt.Errorf(
			"%w: create project: %v", project.ErrConflict, err,
		)
	}
	return record, nil
}

// Delete removes a project and, by way of the schema's cascades, every app,
// deployment, build, execution, user and API key beneath it. There is no
// recovering the rows afterwards.
func (repository *ProjectRepository) Delete(
	ctx context.Context,
	organizationID string,
	projectID string,
) error {
	if err := project.ValidateIdentifier("project_id", projectID); err != nil {
		return err
	}
	tag, err := repository.pool.Exec(
		ctx,
		`DELETE FROM projects WHERE id = $1 AND organization_id = $2`,
		projectID, organizationID,
	)
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", project.ErrNotFound, projectID)
	}
	return nil
}

func (repository *ProjectRepository) List(
	ctx context.Context,
	organizationID string,
	limit int,
) ([]project.Project, error) {
	rows, err := repository.pool.Query(
		ctx,
		`SELECT `+projectColumns+` FROM projects
		 WHERE organization_id = $1
		 ORDER BY created_at DESC, id DESC LIMIT $2`,
		organizationID, postgresLimit(limit),
	)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	records, err := pgx.CollectRows(rows, scanProject)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	return records, nil
}

func scanProject(row pgx.CollectableRow) (project.Project, error) {
	var record project.Project
	err := row.Scan(
		&record.ID, &record.OrganizationID, &record.Name,
		&record.CreatedAt, &record.UpdatedAt,
	)
	return record, err
}

// Update writes a renamed project, using created_at as the compare-and-set
// guard so a concurrent replacement cannot be silently overwritten.
func (repository *ProjectRepository) Update(
	ctx context.Context,
	record project.Project,
) (project.Project, error) {
	if err := record.Validate(); err != nil {
		return project.Project{}, err
	}
	var updated project.Project
	err := repository.pool.QueryRow(
		ctx,
		`UPDATE projects SET name = $2, updated_at = $3
		 WHERE id = $1 AND created_at = $4 AND organization_id = $5
		 RETURNING `+projectColumns,
		record.ID, record.Name, record.UpdatedAt, record.CreatedAt,
		record.OrganizationID,
	).Scan(
		&updated.ID, &updated.OrganizationID, &updated.Name,
		&updated.CreatedAt, &updated.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return project.Project{}, fmt.Errorf(
			"%w: %s", project.ErrNotFound, record.ID,
		)
	}
	if err != nil {
		return project.Project{}, fmt.Errorf(
			"%w: update project: %v", project.ErrConflict, err,
		)
	}
	return updated, nil
}
