package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neurun-io/neurun/internal/domain/deployment"
)

const deploymentSelect = `SELECT d.id, a.project_id, d.app_id, d.runtime,
	d.entrypoint, d.status, d.source, COALESCE(d.commit_sha, ''),
	COALESCE(d.git_ref, ''), d.created_at, d.updated_at
FROM deployments d JOIN apps a ON a.id = d.app_id`

const buildSelect = `SELECT b.id, a.project_id, b.deployment_id, b.number,
	b.status, b.runtime, b.entrypoint, b.source_sha256, b.artifacts,
	b.failure, b.started_at, b.finished_at
FROM builds b
JOIN deployments d ON d.id = b.deployment_id
JOIN apps a ON a.id = d.app_id`

// DeploymentRepository stores deployments and the builds under them. Blob bytes
// stay in the artifact store; only the handles reach the source and artifacts
// JSON columns.
type DeploymentRepository struct {
	pool *pgxpool.Pool
}

func NewDeploymentRepository(pool *pgxpool.Pool) (*DeploymentRepository, error) {
	if pool == nil {
		return nil, errors.New("deployment repository requires a database pool")
	}
	return &DeploymentRepository{pool: pool}, nil
}

func (repository *DeploymentRepository) Check(ctx context.Context) error {
	if repository == nil || repository.pool == nil {
		return errors.New("deployment repository is not configured")
	}
	if err := repository.pool.Ping(contextOrBackground(ctx)); err != nil {
		return fmt.Errorf("check deployment repository: %w", err)
	}
	return nil
}

func scanBuild(row pgx.CollectableRow) (deployment.Build, error) {
	var record deployment.Build
	var artifactsJSON, failureJSON []byte
	var finishedAt *time.Time
	err := row.Scan(
		&record.ID, &record.ProjectID, &record.DeploymentID, &record.Number,
		&record.Status, &record.Runtime, &record.EntryPoint,
		&record.SourceSHA256, &artifactsJSON, &failureJSON,
		&record.StartedAt, &finishedAt,
	)
	if err != nil {
		return deployment.Build{}, err
	}
	if err := json.Unmarshal(artifactsJSON, &record.Artifacts); err != nil {
		return deployment.Build{}, fmt.Errorf("decode build artifacts: %w", err)
	}
	// Decoding through the pointer leaves Failure nil for a JSON null, which is
	// how a row with no failure is written.
	if len(failureJSON) > 0 {
		if err := json.Unmarshal(failureJSON, &record.Failure); err != nil {
			return deployment.Build{}, fmt.Errorf("decode build failure: %w", err)
		}
	}
	record.FinishedAt = finishedAt
	return record, nil
}

func (repository *DeploymentRepository) Save(
	ctx context.Context,
	record deployment.Deployment,
) error {
	if err := record.Validate(); err != nil {
		return err
	}
	return transaction(ctx, repository.pool, func(tx pgx.Tx) error {
		return saveDeployment(ctx, tx, record)
	})
}

func saveDeployment(
	ctx context.Context,
	tx pgx.Tx,
	record deployment.Deployment,
) error {
	if err := advisoryLock(
		ctx, tx, advisoryKey("deployment", record.ProjectID, record.ID),
	); err != nil {
		return err
	}
	source, err := json.Marshal(record.Source)
	if err != nil {
		return fmt.Errorf("encode deployment source metadata: %w", err)
	}
	tag, err := tx.Exec(
		ctx,
		`INSERT INTO deployments
		 (id, app_id, runtime, entrypoint, status, source,
		  commit_sha, git_ref, created_at, updated_at, version)
		 VALUES ($1, $2, $3, $4, $5, $6, $9, $10, $7, $8, 1)
		 ON CONFLICT (id) DO UPDATE SET
		     runtime = EXCLUDED.runtime,
		     entrypoint = EXCLUDED.entrypoint,
		     status = EXCLUDED.status,
		     source = EXCLUDED.source,
		     commit_sha = EXCLUDED.commit_sha,
		     git_ref = EXCLUDED.git_ref,
		     created_at = EXCLUDED.created_at,
		     updated_at = EXCLUDED.updated_at,
		     version = deployments.version + 1
		 WHERE deployments.app_id = EXCLUDED.app_id
		   AND (deployments.status IN ('uploaded', 'building')
		        OR deployments.status = EXCLUDED.status)`,
		record.ID, record.AppID, record.Runtime,
		record.EntryPoint, record.Status, source,
		record.CreatedAt, record.UpdatedAt,
		nullableString(record.CommitSHA), nullableString(record.GitRef),
	)
	if err != nil {
		return fmt.Errorf("save deployment: %w", err)
	}
	if err := requireOneRow(tag, "deployment changed concurrently"); err != nil {
		return err
	}
	for _, build := range record.Builds {
		if err := saveBuild(ctx, tx, build); err != nil {
			return err
		}
	}
	return nil
}

func saveBuild(ctx context.Context, tx pgx.Tx, build deployment.Build) error {
	artifactsJSON, err := json.Marshal(build.Artifacts)
	if err != nil {
		return fmt.Errorf("encode build artifacts: %w", err)
	}
	var failureJSON []byte
	if build.Failure != nil {
		if failureJSON, err = json.Marshal(build.Failure); err != nil {
			return fmt.Errorf("encode build failure: %w", err)
		}
	}
	tag, err := tx.Exec(
		ctx,
		`INSERT INTO builds
		 (id, deployment_id, number, status, runtime, entrypoint,
		  source_sha256, artifacts, failure, started_at, finished_at, version)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, 1)
		 ON CONFLICT (id) DO UPDATE SET
		     number = EXCLUDED.number,
		     status = EXCLUDED.status,
		     runtime = EXCLUDED.runtime,
		     entrypoint = EXCLUDED.entrypoint,
		     source_sha256 = EXCLUDED.source_sha256,
		     artifacts = EXCLUDED.artifacts,
		     failure = EXCLUDED.failure,
		     started_at = EXCLUDED.started_at,
		     finished_at = EXCLUDED.finished_at,
		     version = builds.version + 1
		 WHERE builds.deployment_id = EXCLUDED.deployment_id
		   AND (builds.status = 'building' OR builds.status = EXCLUDED.status)`,
		build.ID, build.DeploymentID, build.Number, build.Status,
		build.Runtime, build.EntryPoint, build.SourceSHA256,
		artifactsJSON, failureJSON, build.StartedAt, build.FinishedAt,
	)
	if err != nil {
		return fmt.Errorf("save build: %w", err)
	}
	return requireOneRow(tag, "build changed concurrently")
}

// GetByIDUnscoped crosses organizations and exists for the worker, which acts
// on an execution it has already claimed and holds no principal to scope to.
func (repository *DeploymentRepository) GetByIDUnscoped(
	ctx context.Context,
	deploymentID string,
) (deployment.Deployment, error) {
	var record deployment.Deployment
	err := transaction(ctx, repository.pool, func(tx pgx.Tx) error {
		loaded, err := getDeploymentUnscoped(ctx, tx, deploymentID)
		record = loaded
		return err
	})
	if err != nil {
		return deployment.Deployment{}, err
	}
	return record, nil
}

func (repository *DeploymentRepository) GetByID(
	ctx context.Context,
	organizationID string,
	deploymentID string,
) (deployment.Deployment, error) {
	var record deployment.Deployment
	err := transaction(ctx, repository.pool, func(tx pgx.Tx) error {
		loaded, err := getDeployment(ctx, tx, organizationID, deploymentID)
		record = loaded
		return err
	})
	if err != nil {
		return deployment.Deployment{}, err
	}
	return record, nil
}

// getDeployment reads a deployment and its builds. Both queries run on the same
// transaction so the two cannot disagree about the build list.
// getDeploymentUnscoped crosses every organization and exists for exactly one
// caller: the boot-time recovery sweep, which has no principal to scope to.
// Nothing reachable from a request may call it.
func getDeploymentUnscoped(
	ctx context.Context,
	tx pgx.Tx,
	deploymentID string,
) (deployment.Deployment, error) {
	return loadDeployment(ctx, tx, "", deploymentID)
}

func getDeployment(
	ctx context.Context,
	tx pgx.Tx,
	organizationID string,
	deploymentID string,
) (deployment.Deployment, error) {
	if organizationID == "" {
		return deployment.Deployment{}, errors.New("organization is required")
	}
	return loadDeployment(ctx, tx, organizationID, deploymentID)
}

func loadDeployment(
	ctx context.Context,
	tx pgx.Tx,
	organizationID string,
	deploymentID string,
) (deployment.Deployment, error) {
	if err := deployment.ValidateIdentifier("deployment_id", deploymentID); err != nil {
		return deployment.Deployment{}, err
	}
	var record deployment.Deployment
	var sourceJSON []byte
	err := tx.QueryRow(
		ctx,
		deploymentSelect+`
		 WHERE d.id = $1 AND ($2 = '' OR`+fmt.Sprintf(appsInOrganization, "$2")+`)`,
		deploymentID, organizationID,
	).Scan(
		&record.ID, &record.ProjectID, &record.AppID, &record.Runtime,
		&record.EntryPoint, &record.Status, &sourceJSON,
		&record.CommitSHA, &record.GitRef,
		&record.CreatedAt, &record.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return deployment.Deployment{}, fmt.Errorf(
			"%w: %s", deployment.ErrNotFound, deploymentID,
		)
	}
	if err != nil {
		return deployment.Deployment{}, fmt.Errorf("read deployment: %w", err)
	}
	if err := json.Unmarshal(sourceJSON, &record.Source); err != nil {
		return deployment.Deployment{}, fmt.Errorf("decode deployment source: %w", err)
	}
	rows, err := tx.Query(
		ctx,
		buildSelect+` WHERE b.deployment_id = $1 ORDER BY b.number ASC`,
		deploymentID,
	)
	if err != nil {
		return deployment.Deployment{}, fmt.Errorf("list deployment builds: %w", err)
	}
	record.Builds, err = pgx.CollectRows(rows, scanBuild)
	if err != nil {
		return deployment.Deployment{}, fmt.Errorf("list deployment builds: %w", err)
	}
	if err := record.Validate(); err != nil {
		return deployment.Deployment{}, fmt.Errorf("invalid persisted deployment: %w", err)
	}
	return record, nil
}

func (repository *DeploymentRepository) List(
	ctx context.Context,
	organizationID string,
	projectID string,
	appID string,
	limit int,
) ([]deployment.Deployment, error) {
	if projectID != "" {
		if err := deployment.ValidateIdentifier("project_id", projectID); err != nil {
			return nil, err
		}
	}
	if appID != "" {
		if err := deployment.ValidateIdentifier("app_id", appID); err != nil {
			return nil, err
		}
	}
	records := make([]deployment.Deployment, 0)
	err := transaction(ctx, repository.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(
			ctx,
			`SELECT d.id FROM deployments d JOIN apps a ON a.id = d.app_id
			 JOIN projects p ON p.id = a.project_id
			 WHERE ($1 = '' OR a.project_id = $1) AND ($2 = '' OR d.app_id = $2)
			   AND p.organization_id = $4
			 ORDER BY d.created_at DESC, d.id DESC LIMIT $3`,
			projectID, appID, postgresLimit(limit), organizationID,
		)
		if err != nil {
			return fmt.Errorf("list deployments: %w", err)
		}
		identifiers, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return fmt.Errorf("list deployments: %w", err)
		}
		for _, id := range identifiers {
			record, err := getDeployment(ctx, tx, organizationID, id)
			if err != nil {
				return err
			}
			records = append(records, record)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return records, nil
}

func (repository *DeploymentRepository) GetBuild(
	ctx context.Context,
	organizationID string,
	buildID string,
) (deployment.Build, error) {
	if err := deployment.ValidateIdentifier("build_id", buildID); err != nil {
		return deployment.Build{}, err
	}
	rows, err := repository.pool.Query(
		ctx,
		buildSelect+` WHERE b.id = $1 AND`+fmt.Sprintf(buildsInOrganization, "$2"),
		buildID, organizationID,
	)
	if err != nil {
		return deployment.Build{}, fmt.Errorf("read build: %w", err)
	}
	record, err := pgx.CollectExactlyOneRow(rows, scanBuild)
	if errors.Is(err, pgx.ErrNoRows) {
		return deployment.Build{}, fmt.Errorf(
			"%w: build %s", deployment.ErrNotFound, buildID,
		)
	}
	if err != nil {
		return deployment.Build{}, fmt.Errorf("read build: %w", err)
	}
	return record, nil
}

func (repository *DeploymentRepository) ListBuilds(
	ctx context.Context,
	organizationID string,
	deploymentID string,
	limit int,
) ([]deployment.Build, error) {
	if deploymentID != "" {
		if err := deployment.ValidateIdentifier("deployment_id", deploymentID); err != nil {
			return nil, err
		}
	}
	rows, err := repository.pool.Query(
		ctx,
		buildSelect+` WHERE ($1 = '' OR b.deployment_id = $1) AND`+
			fmt.Sprintf(buildsInOrganization, "$3")+
			` ORDER BY b.started_at DESC, b.id DESC LIMIT $2`,
		deploymentID, postgresLimit(limit), organizationID,
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

// RecoverBuilding fails builds a crashed process left running and reports how
// many it changed. It never retries the build's side effects.
func (repository *DeploymentRepository) RecoverBuilding(
	ctx context.Context,
	now time.Time,
	failure deployment.Failure,
) (int, error) {
	if err := deployment.ValidateRecovery(now, failure); err != nil {
		return 0, err
	}
	now = postgresTime(now)
	recovered := 0
	err := transaction(ctx, repository.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(
			ctx,
			`SELECT id FROM deployments WHERE status = 'building' FOR UPDATE`,
		)
		if err != nil {
			return err
		}
		identifiers, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		for _, id := range identifiers {
			record, err := getDeploymentUnscoped(ctx, tx, id)
			if err != nil {
				return err
			}
			if !record.FailInterruptedBuild(now, failure) {
				continue
			}
			if err := saveDeployment(ctx, tx, record); err != nil {
				return err
			}
			recovered++
		}
		return nil
	})
	return recovered, err
}
