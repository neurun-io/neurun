package database

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
	COALESCE(d.git_ref, ''), COALESCE(d.build_id, ''), d.failure, d.logs,
	d.started_at, d.finished_at, d.created_at, d.updated_at
FROM deployments d JOIN apps a ON a.id = d.app_id`

// DeploymentRepository stores deployments. The build a deployment points at is
// read back through the build repository, which owns that table; blob bytes
// stay in the artifact store, and only the handle reaches the source column.
type DeploymentRepository struct {
	pool   *pgxpool.Pool
	builds *BuildRepository
}

func NewDeploymentRepository(
	pool *pgxpool.Pool,
	builds *BuildRepository,
) (*DeploymentRepository, error) {
	if pool == nil {
		return nil, errors.New("deployment repository requires a database pool")
	}
	if builds == nil {
		return nil, errors.New("deployment repository requires the build repository")
	}
	return &DeploymentRepository{pool: pool, builds: builds}, nil
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
	var failureJSON []byte
	if record.Failure != nil {
		if failureJSON, err = json.Marshal(record.Failure); err != nil {
			return fmt.Errorf("encode deployment failure: %w", err)
		}
	}
	tag, err := tx.Exec(
		ctx,
		`INSERT INTO deployments
		 (id, app_id, runtime, entrypoint, status, source,
		  commit_sha, git_ref, build_id, failure, logs,
		  started_at, finished_at, created_at, updated_at, version)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11,
		         $12, $13, $14, $15, 1)
		 ON CONFLICT (id) DO UPDATE SET
		     runtime = EXCLUDED.runtime,
		     entrypoint = EXCLUDED.entrypoint,
		     status = EXCLUDED.status,
		     source = EXCLUDED.source,
		     commit_sha = EXCLUDED.commit_sha,
		     git_ref = EXCLUDED.git_ref,
		     build_id = EXCLUDED.build_id,
		     failure = EXCLUDED.failure,
		     logs = EXCLUDED.logs,
		     started_at = EXCLUDED.started_at,
		     finished_at = EXCLUDED.finished_at,
		     created_at = EXCLUDED.created_at,
		     updated_at = EXCLUDED.updated_at,
		     version = deployments.version + 1
		 WHERE deployments.app_id = EXCLUDED.app_id
		   AND (deployments.status IN ('queued', 'building', 'publishing')
		        OR deployments.status = EXCLUDED.status)`,
		record.ID, record.AppID, record.Runtime,
		record.EntryPoint, record.Status, source,
		nullableString(record.CommitSHA), nullableString(record.GitRef),
		nullableString(buildID(record)), failureJSON, record.Logs,
		record.StartedAt, record.FinishedAt,
		record.CreatedAt, record.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("save deployment: %w", err)
	}
	return requireOneRow(tag, "deployment changed concurrently")
}

func buildID(record deployment.Deployment) string {
	if record.Build == nil {
		return ""
	}
	return record.Build.ID
}

// GetByIDUnscoped crosses organizations and exists for the worker, which acts
// on an execution it has already claimed and holds no principal to scope to.
func (repository *DeploymentRepository) GetByIDUnscoped(
	ctx context.Context,
	deploymentID string,
) (deployment.Deployment, error) {
	var record deployment.Deployment
	err := transaction(ctx, repository.pool, func(tx pgx.Tx) error {
		loaded, err := getDeploymentUnscoped(ctx, tx, repository.builds, deploymentID)
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
		loaded, err := getDeployment(ctx, tx, repository.builds, organizationID, deploymentID)
		record = loaded
		return err
	})
	if err != nil {
		return deployment.Deployment{}, err
	}
	return record, nil
}

// getDeploymentUnscoped crosses every organization and exists for exactly one
// caller: the boot-time recovery sweep, which has no principal to scope to.
// Nothing reachable from a request may call it.
func getDeploymentUnscoped(
	ctx context.Context,
	tx pgx.Tx,
	builds *BuildRepository,
	deploymentID string,
) (deployment.Deployment, error) {
	return loadDeployment(ctx, tx, builds, "", deploymentID)
}

func getDeployment(
	ctx context.Context,
	tx pgx.Tx,
	builds *BuildRepository,
	organizationID string,
	deploymentID string,
) (deployment.Deployment, error) {
	if organizationID == "" {
		return deployment.Deployment{}, errors.New("organization is required")
	}
	return loadDeployment(ctx, tx, builds, organizationID, deploymentID)
}

func loadDeployment(
	ctx context.Context,
	tx pgx.Tx,
	builds *BuildRepository,
	organizationID string,
	deploymentID string,
) (deployment.Deployment, error) {
	if err := deployment.ValidateIdentifier("deployment_id", deploymentID); err != nil {
		return deployment.Deployment{}, err
	}
	var record deployment.Deployment
	var sourceJSON, failureJSON []byte
	var produced string
	err := tx.QueryRow(
		ctx,
		deploymentSelect+`
		 WHERE d.id = $1 AND ($2 = '' OR`+fmt.Sprintf(appsInOrganization, "$2")+`)`,
		deploymentID, organizationID,
	).Scan(
		&record.ID, &record.ProjectID, &record.AppID, &record.Runtime,
		&record.EntryPoint, &record.Status, &sourceJSON,
		&record.CommitSHA, &record.GitRef, &produced, &failureJSON, &record.Logs,
		&record.StartedAt, &record.FinishedAt,
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
	// Decoding through the pointer leaves Failure nil for a JSON null, which is
	// how a row with no failure is written.
	if len(failureJSON) > 0 {
		if err := json.Unmarshal(failureJSON, &record.Failure); err != nil {
			return deployment.Deployment{}, fmt.Errorf("decode deployment failure: %w", err)
		}
	}
	// Read outside the transaction on purpose: a build is written once and never
	// changes, so there is nothing for a concurrent write to disagree about.
	if produced != "" {
		output, err := builds.GetByID(ctx, produced)
		if err != nil {
			return deployment.Deployment{}, err
		}
		record.Build = &output
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
			record, err := getDeployment(ctx, tx, repository.builds, organizationID, id)
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

// RecoverBuilding fails deployments a crashed process left running and reports
// how many it changed. It never retries their side effects.
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
			`SELECT id FROM deployments
			 WHERE status IN ('queued', 'building', 'publishing') FOR UPDATE`,
		)
		if err != nil {
			return err
		}
		identifiers, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		for _, id := range identifiers {
			record, err := getDeploymentUnscoped(ctx, tx, repository.builds, id)
			if err != nil {
				return err
			}
			if !record.FailInterrupted(now, failure) {
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
