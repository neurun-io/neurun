package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neurun-io/neurun/internal/domain/deployment"
)

const appColumns = `id, project_id, name, COALESCE(repository, ''),
	COALESCE(production_ref, ''), created_at, updated_at`

type AppRepository struct {
	pool *pgxpool.Pool
}

func NewAppRepository(pool *pgxpool.Pool) (*AppRepository, error) {
	if pool == nil {
		return nil, errors.New("app repository requires a database pool")
	}
	return &AppRepository{pool: pool}, nil
}

func scanApp(row pgx.CollectableRow) (deployment.App, error) {
	var record deployment.App
	if err := row.Scan(
		&record.ID, &record.ProjectID, &record.Name,
		&record.Repository, &record.ProductionRef,
		&record.CreatedAt, &record.UpdatedAt,
	); err != nil {
		return deployment.App{}, err
	}
	if err := record.Validate(); err != nil {
		return deployment.App{}, fmt.Errorf("invalid persisted app: %w", err)
	}
	return record, nil
}

func (repository *AppRepository) Create(
	ctx context.Context,
	record deployment.App,
) (deployment.App, error) {
	if err := record.Validate(); err != nil {
		return deployment.App{}, err
	}
	rows, err := repository.pool.Query(
		ctx,
		`INSERT INTO apps
		 (id, project_id, name, repository, production_ref, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING `+appColumns,
		record.ID, record.ProjectID, record.Name,
		nullableString(record.Repository), nullableString(record.ProductionRef),
		record.CreatedAt, record.UpdatedAt,
	)
	if err != nil {
		return deployment.App{}, fmt.Errorf(
			"%w: create app: %v", deployment.ErrAppConflict, err,
		)
	}
	created, err := pgx.CollectExactlyOneRow(rows, scanApp)
	if err != nil {
		return deployment.App{}, fmt.Errorf(
			"%w: create app: %v", deployment.ErrAppConflict, err,
		)
	}
	return created, nil
}

// GetByID addresses an app by identifier within one organization. An app that
// belongs to another organization reads as absent, never as forbidden: the
// caller must not learn that the identifier exists.
func (repository *AppRepository) GetByID(
	ctx context.Context,
	organizationID string,
	appID string,
) (deployment.App, error) {
	if err := deployment.ValidateIdentifier("app_id", appID); err != nil {
		return deployment.App{}, err
	}
	rows, err := repository.pool.Query(
		ctx,
		`SELECT `+appColumns+` FROM apps WHERE id = $1 AND`+
			fmt.Sprintf(inOrganization, "$2"),
		appID, organizationID,
	)
	if err != nil {
		return deployment.App{}, fmt.Errorf("read app: %w", err)
	}
	record, err := pgx.CollectExactlyOneRow(rows, scanApp)
	if errors.Is(err, pgx.ErrNoRows) {
		return deployment.App{}, fmt.Errorf("%w: %s", deployment.ErrAppNotFound, appID)
	}
	if err != nil {
		return deployment.App{}, fmt.Errorf("read app: %w", err)
	}
	return record, nil
}

// List returns apps newest first. Empty projectID or name drops that filter.
func (repository *AppRepository) List(
	ctx context.Context,
	organizationID string,
	projectID string,
	name string,
	limit int,
) ([]deployment.App, error) {
	if projectID != "" {
		if err := deployment.ValidateIdentifier("project_id", projectID); err != nil {
			return nil, err
		}
	}
	if err := deployment.ValidateAppNameFilter(name); err != nil {
		return nil, err
	}
	rows, err := repository.pool.Query(
		ctx,
		`SELECT `+appColumns+` FROM apps
		 WHERE ($1 = '' OR project_id = $1) AND ($2 = '' OR name = $2) AND`+
			fmt.Sprintf(inOrganization, "$4")+
			` ORDER BY created_at DESC, id DESC LIMIT $3`,
		projectID, name, postgresLimit(limit), organizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("list apps: %w", err)
	}
	records, err := pgx.CollectRows(rows, scanApp)
	if err != nil {
		return nil, fmt.Errorf("list apps: %w", err)
	}
	return records, nil
}

// OrganizationOf resolves the organization behind an app, walking app to
// project. A handler names its app; everything else about it is looked up here
// rather than taken from the caller.
func (repository *AppRepository) OrganizationOf(
	ctx context.Context,
	appID string,
) (string, error) {
	if err := deployment.ValidateIdentifier("app_id", appID); err != nil {
		return "", err
	}
	rows, err := repository.pool.Query(
		ctx,
		`SELECT p.organization_id FROM apps a
		 JOIN projects p ON p.id = a.project_id
		 WHERE a.id = $1`,
		appID,
	)
	if err != nil {
		return "", fmt.Errorf("read app organization: %w", err)
	}
	organizationID, err := pgx.CollectExactlyOneRow(
		rows, pgx.RowTo[string],
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("%w: %s", deployment.ErrAppNotFound, appID)
	}
	if err != nil {
		return "", fmt.Errorf("read app organization: %w", err)
	}
	return organizationID, nil
}

// ConnectedTo returns the apps in one organization pointed at a repository.
// GitHub preserves the case of a repository name but does not distinguish it,
// so neither does the match.
func (repository *AppRepository) ConnectedTo(
	ctx context.Context,
	organizationID string,
	name string,
) ([]deployment.App, error) {
	rows, err := repository.pool.Query(
		ctx,
		`SELECT `+appColumns+` FROM apps
		 WHERE lower(repository) = lower($1) AND`+
			fmt.Sprintf(inOrganization, "$2")+
			` ORDER BY created_at DESC, id DESC`,
		name, organizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("list connected apps: %w", err)
	}
	records, err := pgx.CollectRows(rows, scanApp)
	if err != nil {
		return nil, fmt.Errorf("list connected apps: %w", err)
	}
	return records, nil
}

func (repository *AppRepository) Update(
	ctx context.Context,
	organizationID string,
	record deployment.App,
) (deployment.App, error) {
	if err := record.Validate(); err != nil {
		return deployment.App{}, err
	}
	rows, err := repository.pool.Query(
		ctx,
		`UPDATE apps SET name = $2, updated_at = $3,
		     repository = $6, production_ref = $7
		 WHERE id = $1 AND created_at = $4 AND`+
			fmt.Sprintf(inOrganization, "$5")+
			` RETURNING `+appColumns,
		record.ID, record.Name, record.UpdatedAt, record.CreatedAt, organizationID,
		nullableString(record.Repository), nullableString(record.ProductionRef),
	)
	if err != nil {
		return deployment.App{}, fmt.Errorf(
			"%w: update app: %v", deployment.ErrAppConflict, err,
		)
	}
	updated, err := pgx.CollectExactlyOneRow(rows, scanApp)
	if errors.Is(err, pgx.ErrNoRows) {
		return deployment.App{}, fmt.Errorf("%w: %s", deployment.ErrAppNotFound, record.ID)
	}
	if err != nil {
		return deployment.App{}, fmt.Errorf(
			"%w: update app: %v", deployment.ErrAppConflict, err,
		)
	}
	return updated, nil
}

// Delete removes an app and, by way of the schema's cascades, every deployment,
// build and execution beneath it.
func (repository *AppRepository) Delete(
	ctx context.Context,
	organizationID string,
	appID string,
) error {
	if err := deployment.ValidateIdentifier("app_id", appID); err != nil {
		return err
	}
	tag, err := repository.pool.Exec(
		ctx,
		`DELETE FROM apps WHERE id = $1 AND`+fmt.Sprintf(inOrganization, "$2"),
		appID, organizationID,
	)
	if err != nil {
		return fmt.Errorf("delete app: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", deployment.ErrAppNotFound, appID)
	}
	return nil
}
