package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neurun-io/neurun/internal/domain/deployment"
)

const projectColumns = `id, name, created_at, updated_at`

type ProjectRepository struct {
	pool *pgxpool.Pool
}

func NewProjectRepository(pool *pgxpool.Pool) (*ProjectRepository, error) {
	if pool == nil {
		return nil, errors.New("project repository requires a database pool")
	}
	return &ProjectRepository{pool: pool}, nil
}

func (repository *ProjectRepository) Ensure(
	ctx context.Context,
	record deployment.Project,
) (deployment.Project, error) {
	if err := record.Validate(); err != nil {
		return deployment.Project{}, err
	}
	_, err := repository.pool.Exec(
		ctx,
		`INSERT INTO projects (id, name, created_at, updated_at)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (id) DO NOTHING`,
		record.ID, record.Name, record.CreatedAt, record.UpdatedAt,
	)
	if err != nil {
		return deployment.Project{}, fmt.Errorf(
			"%w: ensure project: %v", deployment.ErrProjectConflict, err,
		)
	}
	return repository.GetByID(ctx, record.ID)
}

func (repository *ProjectRepository) GetByID(
	ctx context.Context,
	projectID string,
) (deployment.Project, error) {
	if err := deployment.ValidateIdentifier("project_id", projectID); err != nil {
		return deployment.Project{}, err
	}
	var record deployment.Project
	err := repository.pool.QueryRow(
		ctx,
		`SELECT `+projectColumns+` FROM projects WHERE id = $1`,
		projectID,
	).Scan(&record.ID, &record.Name, &record.CreatedAt, &record.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return deployment.Project{}, fmt.Errorf(
			"%w: %s", deployment.ErrProjectNotFound, projectID,
		)
	}
	if err != nil {
		return deployment.Project{}, fmt.Errorf("read project: %w", err)
	}
	if err := record.Validate(); err != nil {
		return deployment.Project{}, fmt.Errorf("invalid persisted project: %w", err)
	}
	return record, nil
}

// Create inserts a project and fails if the identifier or name is taken. Ensure
// is the idempotent variant, used where a caller may legitimately run twice.
func (repository *ProjectRepository) Create(
	ctx context.Context,
	record deployment.Project,
) (deployment.Project, error) {
	if err := record.Validate(); err != nil {
		return deployment.Project{}, err
	}
	_, err := repository.pool.Exec(
		ctx,
		`INSERT INTO projects (id, name, created_at, updated_at)
		 VALUES ($1, $2, $3, $4)`,
		record.ID, record.Name, record.CreatedAt, record.UpdatedAt,
	)
	if err != nil {
		return deployment.Project{}, fmt.Errorf(
			"%w: create project: %v", deployment.ErrProjectConflict, err,
		)
	}
	return record, nil
}

// Delete removes a project and, by way of the schema's cascades, every app,
// deployment, build, execution, user and API key beneath it. There is no
// recovering the rows afterwards.
func (repository *ProjectRepository) Delete(
	ctx context.Context,
	projectID string,
) error {
	if err := deployment.ValidateIdentifier("project_id", projectID); err != nil {
		return err
	}
	tag, err := repository.pool.Exec(
		ctx, `DELETE FROM projects WHERE id = $1`, projectID,
	)
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", deployment.ErrProjectNotFound, projectID)
	}
	return nil
}

func (repository *ProjectRepository) List(
	ctx context.Context,
	limit int,
) ([]deployment.Project, error) {
	rows, err := repository.pool.Query(
		ctx,
		`SELECT `+projectColumns+` FROM projects
		 ORDER BY created_at DESC, id DESC LIMIT $1`,
		postgresLimit(limit),
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

func scanProject(row pgx.CollectableRow) (deployment.Project, error) {
	var record deployment.Project
	err := row.Scan(&record.ID, &record.Name, &record.CreatedAt, &record.UpdatedAt)
	return record, err
}

// Update writes a renamed project, using created_at as the compare-and-set
// guard so a concurrent replacement cannot be silently overwritten.
func (repository *ProjectRepository) Update(
	ctx context.Context,
	record deployment.Project,
) (deployment.Project, error) {
	if err := record.Validate(); err != nil {
		return deployment.Project{}, err
	}
	var updated deployment.Project
	err := repository.pool.QueryRow(
		ctx,
		`UPDATE projects SET name = $2, updated_at = $3
		 WHERE id = $1 AND created_at = $4
		 RETURNING `+projectColumns,
		record.ID, record.Name, record.UpdatedAt, record.CreatedAt,
	).Scan(&updated.ID, &updated.Name, &updated.CreatedAt, &updated.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return deployment.Project{}, fmt.Errorf(
			"%w: %s", deployment.ErrProjectNotFound, record.ID,
		)
	}
	if err != nil {
		return deployment.Project{}, fmt.Errorf(
			"%w: update project: %v", deployment.ErrProjectConflict, err,
		)
	}
	return updated, nil
}
