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
const buildColumns = `b.id, b.runtime, b.entrypoint, b.source_sha256,
	b.artifacts, b.created_at`

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
	var record build.Build
	var artifactsJSON []byte
	err := row.Scan(
		&record.ID, &record.Runtime, &record.EntryPoint,
		&record.SourceSHA256, &artifactsJSON, &record.CreatedAt,
	)
	if err != nil {
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
		 (id, runtime, entrypoint, source_sha256, artifacts, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (id) DO NOTHING`,
		record.ID, record.Runtime, record.EntryPoint, record.SourceSHA256,
		artifactsJSON, record.CreatedAt,
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

// Produced is a build with the deployment that made it. The link is a join
// rather than a column on the build: a build is an output, and asking where an
// output came from is a question about deployments.
type Produced struct {
	Build        build.Build
	DeploymentID string
	AppID        string
}

func scanProduced(row pgx.CollectableRow) (Produced, error) {
	var record Produced
	var artifactsJSON []byte
	err := row.Scan(
		&record.Build.ID, &record.Build.Runtime, &record.Build.EntryPoint,
		&record.Build.SourceSHA256, &artifactsJSON,
		&record.Build.CreatedAt, &record.DeploymentID, &record.AppID,
	)
	if err != nil {
		return Produced{}, err
	}
	if err := json.Unmarshal(artifactsJSON, &record.Build.Artifacts); err != nil {
		return Produced{}, fmt.Errorf("decode build artifacts: %w", err)
	}
	return record, nil
}

const producedSelect = `SELECT ` + buildColumns + `, d.id, d.app_id
FROM builds b
JOIN deployments d ON d.build_id = b.id
JOIN apps a ON a.id = d.app_id
JOIN projects p ON p.id = a.project_id`

// List returns the organization's builds, newest first.
func (repository *BuildRepository) List(
	ctx context.Context,
	organizationID string,
	deploymentID string,
	limit int,
) ([]Produced, error) {
	if deploymentID != "" {
		if err := build.ValidateIdentifier("deployment_id", deploymentID); err != nil {
			return nil, err
		}
	}
	rows, err := repository.pool.Query(
		ctx,
		producedSelect+`
		 WHERE p.organization_id = $1 AND ($2 = '' OR d.id = $2)
		 ORDER BY b.created_at DESC, b.id DESC LIMIT $3`,
		organizationID, deploymentID, postgresLimit(limit),
	)
	if err != nil {
		return nil, fmt.Errorf("list builds: %w", err)
	}
	records, err := pgx.CollectRows(rows, scanProduced)
	if err != nil {
		return nil, fmt.Errorf("list builds: %w", err)
	}
	return records, nil
}

// Get returns one of the organization's builds. A build no deployment points at
// is not visible: nothing owns it.
func (repository *BuildRepository) Get(
	ctx context.Context,
	organizationID string,
	buildID string,
) (Produced, error) {
	if err := build.ValidateIdentifier("build_id", buildID); err != nil {
		return Produced{}, err
	}
	rows, err := repository.pool.Query(
		ctx,
		producedSelect+` WHERE p.organization_id = $1 AND b.id = $2`,
		organizationID, buildID,
	)
	if err != nil {
		return Produced{}, fmt.Errorf("read build: %w", err)
	}
	record, err := pgx.CollectExactlyOneRow(rows, scanProduced)
	if errors.Is(err, pgx.ErrNoRows) {
		return Produced{}, fmt.Errorf("%w: %s", build.ErrNotFound, buildID)
	}
	if err != nil {
		return Produced{}, fmt.Errorf("read build: %w", err)
	}
	return record, nil
}
