package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neurun-io/neurun/internal/domain/build"
)

// buildColumns is the whole of a build. A deployment reads it joined to its own
// row, so the shape is declared here and used from both.
const buildColumns = `b.id, b.app_id, b.deployment_id, b.runtime,
	b.source_sha256, b.artifacts, b.created_at`

// BuildRepository stores what deployments produced. The ZIPs stay in the file
// repository; only their handles reach the artifacts column.
type BuildRepository struct {
	pool *pgxpool.Pool
}

func NewBuildRepository(pool *pgxpool.Pool) (*BuildRepository, error) {
	if pool == nil {
		return nil, errors.New("build repository requires a database pool")
	}
	return &BuildRepository{pool: pool}, nil
}

func scanBuild(row pgx.CollectableRow) (build.Build, error) {
	return scanBuildWith(row)
}

// scanBuildWith reads a build, and whatever the caller selected after it.
func scanBuildWith(row pgx.CollectableRow, extra ...any) (build.Build, error) {
	var record build.Build
	var artifactsJSON []byte
	targets := append([]any{
		&record.ID, &record.AppID, &record.DeploymentID, &record.Runtime,
		&record.SourceSHA256, &artifactsJSON, &record.CreatedAt,
	}, extra...)
	if err := row.Scan(targets...); err != nil {
		return build.Build{}, err
	}
	if err := json.Unmarshal(artifactsJSON, &record.Artifacts); err != nil {
		return build.Build{}, fmt.Errorf("decode build artifacts: %w", err)
	}
	return record, nil
}

// Save writes the output once. A build is sealed when it is written, so a
// repeat write is the same bytes and nothing needs updating.
func (repository *BuildRepository) Save(
	ctx context.Context,
	record build.Build,
) error {
	if err := record.Validate(); err != nil {
		return err
	}
	artifactsJSON, err := json.Marshal(record.Artifacts)
	if err != nil {
		return fmt.Errorf("encode build artifacts: %w", err)
	}
	if _, err := repository.pool.Exec(
		ctx,
		`INSERT INTO builds
		 (id, app_id, deployment_id, runtime, source_sha256,
		  artifacts, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (id) DO NOTHING`,
		record.ID, record.AppID, record.DeploymentID, record.Runtime,
		record.SourceSHA256, artifactsJSON, record.CreatedAt,
	); err != nil {
		return fmt.Errorf("save build: %w", err)
	}
	return nil
}

func (repository *BuildRepository) GetByID(
	ctx context.Context,
	buildID string,
) (build.Build, error) {
	if err := build.ValidateIdentifier("build_id", buildID); err != nil {
		return build.Build{}, err
	}
	rows, err := repository.pool.Query(
		ctx, `SELECT `+buildColumns+` FROM builds b WHERE b.id = $1`, buildID,
	)
	if err != nil {
		return build.Build{}, fmt.Errorf("read build: %w", err)
	}
	record, err := pgx.CollectExactlyOneRow(rows, scanBuild)
	if errors.Is(err, pgx.ErrNoRows) {
		return build.Build{}, fmt.Errorf("%w: %s", build.ErrNotFound, buildID)
	}
	if err != nil {
		return build.Build{}, fmt.Errorf("read build: %w", err)
	}
	return record, nil
}

// ownedSelect scopes builds to an organization. It reaches the app for the
// project only: a build says which app it belongs to and which deployment made
// it, so neither costs a join.
const ownedSelect = `SELECT ` + buildColumns + `
FROM builds b
JOIN apps a ON a.id = b.app_id
JOIN projects p ON p.id = a.project_id`

// List returns the organization's builds, newest first. An app or a deployment
// narrows it to what that one produced.
func (repository *BuildRepository) List(
	ctx context.Context,
	organizationID string,
	appID string,
	deploymentID string,
	limit int,
) ([]build.Build, error) {
	for field, value := range map[string]string{
		"app_id": appID, "deployment_id": deploymentID,
	} {
		if value == "" {
			continue
		}
		if err := build.ValidateIdentifier(field, value); err != nil {
			return nil, err
		}
	}
	rows, err := repository.pool.Query(
		ctx,
		ownedSelect+`
		 WHERE p.organization_id = $1 AND ($2 = '' OR b.app_id = $2)
		   AND ($3 = '' OR b.deployment_id = $3)
		 ORDER BY b.created_at DESC, b.id DESC LIMIT $4`,
		organizationID, appID, deploymentID, postgresLimit(limit),
	)
	if err != nil {
		return nil, fmt.Errorf("list builds: %w", err)
	}
	records, err := pgx.CollectRows(rows, scanBuild)
	if err != nil {
		return nil, fmt.Errorf("list builds: %w", err)
	}
	return records, nil
}

// Get returns one of the organization's builds.
func (repository *BuildRepository) Get(
	ctx context.Context,
	organizationID string,
	buildID string,
) (build.Build, error) {
	if err := build.ValidateIdentifier("build_id", buildID); err != nil {
		return build.Build{}, err
	}
	rows, err := repository.pool.Query(
		ctx,
		ownedSelect+` WHERE p.organization_id = $1 AND b.id = $2`,
		organizationID, buildID,
	)
	if err != nil {
		return build.Build{}, fmt.Errorf("read build: %w", err)
	}
	record, err := pgx.CollectExactlyOneRow(rows, scanBuild)
	if errors.Is(err, pgx.ErrNoRows) {
		return build.Build{}, fmt.Errorf("%w: %s", build.ErrNotFound, buildID)
	}
	if err != nil {
		return build.Build{}, fmt.Errorf("read build: %w", err)
	}
	return record, nil
}

// ByDeployment returns what one deployment produced, unscoped: it is read as
// part of the deployment the caller already reached.
func (repository *BuildRepository) ByDeployment(
	ctx context.Context,
	deploymentID string,
) (build.Build, error) {
	if err := build.ValidateIdentifier("deployment_id", deploymentID); err != nil {
		return build.Build{}, err
	}
	rows, err := repository.pool.Query(
		ctx,
		`SELECT `+buildColumns+` FROM builds b WHERE b.deployment_id = $1`,
		deploymentID,
	)
	if err != nil {
		return build.Build{}, fmt.Errorf("read build: %w", err)
	}
	record, err := pgx.CollectExactlyOneRow(rows, scanBuild)
	if errors.Is(err, pgx.ErrNoRows) {
		return build.Build{}, fmt.Errorf("%w: for %s", build.ErrNotFound, deploymentID)
	}
	if err != nil {
		return build.Build{}, fmt.Errorf("read build: %w", err)
	}
	return record, nil
}

// ActiveForApp returns the build an app runs: the one it is pinned to, or the
// newest it has. It also returns the project, which is what scopes anything
// started from it.
func (repository *BuildRepository) ActiveForApp(
	ctx context.Context,
	organizationID string,
	appID string,
) (build.Build, string, error) {
	if err := build.ValidateIdentifier("app_id", appID); err != nil {
		return build.Build{}, "", err
	}
	var projectID string
	rows, err := repository.pool.Query(
		ctx,
		`SELECT `+buildColumns+`, a.project_id
		 FROM builds b
		 JOIN apps a ON a.id = b.app_id
		 JOIN projects p ON p.id = a.project_id
		 WHERE p.organization_id = $1 AND b.app_id = $2
		   AND (a.active_build_id IS NULL OR b.id = a.active_build_id)
		 ORDER BY b.created_at DESC, b.id DESC LIMIT 1`,
		organizationID, appID,
	)
	if err != nil {
		return build.Build{}, "", fmt.Errorf("read active build: %w", err)
	}
	record, err := pgx.CollectExactlyOneRow(
		rows,
		func(row pgx.CollectableRow) (build.Build, error) {
			return scanBuildWith(row, &projectID)
		},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return build.Build{}, "", fmt.Errorf(
			"%w: app %s has no build to run", build.ErrNotReady, appID,
		)
	}
	if err != nil {
		return build.Build{}, "", fmt.Errorf("read active build: %w", err)
	}
	return record, projectID, nil
}
