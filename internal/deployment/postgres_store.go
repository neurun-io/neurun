package deployment

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// PostgresStore persists the four execution-engine concepts in the relational
// projects, deployments, builds, and executions tables. Blob bytes remain in
// artifact.BlobStore; private storage handles are encoded only in source and
// artifacts JSON columns.
type PostgresStore struct {
	database *sql.DB
}

func NewPostgresStore(database *sql.DB) (*PostgresStore, error) {
	if database == nil {
		return nil, errors.New("deployment PostgreSQL database is required")
	}
	return &PostgresStore{database: database}, nil
}

func (store *PostgresStore) Check(ctx context.Context) error {
	if store == nil || store.database == nil {
		return errors.New("deployment PostgreSQL store is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := store.database.PingContext(ctx); err != nil {
		return fmt.Errorf("check deployment PostgreSQL store: %w", err)
	}
	return nil
}

func (store *PostgresStore) EnsureProject(
	ctx context.Context,
	record Project,
) (Project, error) {
	if err := contextError(ctx); err != nil {
		return Project{}, err
	}
	if err := validateProject(record); err != nil {
		return Project{}, err
	}
	_, err := store.database.ExecContext(
		ctx,
		`INSERT INTO projects (id, name, created_at, updated_at)
         VALUES ($1, $2, $3, $4)
         ON CONFLICT (id) DO NOTHING`,
		record.ID,
		record.Name,
		record.CreatedAt,
		record.UpdatedAt,
	)
	if err != nil {
		return Project{}, fmt.Errorf("%w: ensure project: %v", ErrProjectConflict, err)
	}
	return store.GetProject(ctx, record.ID)
}

func (store *PostgresStore) GetProject(
	ctx context.Context,
	projectID string,
) (Project, error) {
	if err := contextError(ctx); err != nil {
		return Project{}, err
	}
	if err := validateIdentifier("project_id", projectID); err != nil {
		return Project{}, err
	}
	var record Project
	err := store.database.QueryRowContext(
		ctx,
		`SELECT id, name, created_at, updated_at FROM projects WHERE id = $1`,
		projectID,
	).Scan(&record.ID, &record.Name, &record.CreatedAt, &record.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Project{}, fmt.Errorf("%w: %s", ErrProjectNotFound, projectID)
	}
	if err != nil {
		return Project{}, fmt.Errorf("read project: %w", err)
	}
	if err := validateProject(record); err != nil {
		return Project{}, fmt.Errorf("invalid persisted project: %w", err)
	}
	return record, nil
}

func (store *PostgresStore) ListProjects(
	ctx context.Context,
	principalProjectID string,
	limit int,
) ([]Project, error) {
	if limit < 1 {
		return []Project{}, nil
	}
	record, err := store.GetProject(ctx, principalProjectID)
	if errors.Is(err, ErrProjectNotFound) {
		return []Project{}, nil
	}
	if err != nil {
		return nil, err
	}
	return []Project{record}, nil
}

func (store *PostgresStore) UpdateProject(
	ctx context.Context,
	record Project,
) (Project, error) {
	if err := contextError(ctx); err != nil {
		return Project{}, err
	}
	if err := validateProject(record); err != nil {
		return Project{}, err
	}
	var updated Project
	err := store.database.QueryRowContext(
		ctx,
		`UPDATE projects SET name = $2, updated_at = $3
         WHERE id = $1 AND created_at = $4
         RETURNING id, name, created_at, updated_at`,
		record.ID,
		record.Name,
		record.UpdatedAt,
		record.CreatedAt,
	).Scan(&updated.ID, &updated.Name, &updated.CreatedAt, &updated.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Project{}, fmt.Errorf("%w: %s", ErrProjectNotFound, record.ID)
	}
	if err != nil {
		return Project{}, fmt.Errorf("%w: update project: %v", ErrProjectConflict, err)
	}
	return updated, nil
}

func (store *PostgresStore) CreateApp(ctx context.Context, record App) (App, error) {
	if err := contextError(ctx); err != nil {
		return App{}, err
	}
	if err := validateApp(record); err != nil {
		return App{}, err
	}
	var created App
	err := store.database.QueryRowContext(
		ctx,
		`INSERT INTO apps (id, project_id, name, created_at, updated_at)
         VALUES ($1, $2, $3, $4, $5)
         RETURNING id, project_id, name, created_at, updated_at`,
		record.ID,
		record.ProjectID,
		record.Name,
		record.CreatedAt,
		record.UpdatedAt,
	).Scan(&created.ID, &created.ProjectID, &created.Name, &created.CreatedAt, &created.UpdatedAt)
	if err != nil {
		return App{}, fmt.Errorf("%w: create app: %v", ErrAppConflict, err)
	}
	return created, nil
}

func (store *PostgresStore) GetApp(
	ctx context.Context,
	projectID string,
	appID string,
) (App, error) {
	if err := contextError(ctx); err != nil {
		return App{}, err
	}
	if err := validateIdentifier("project_id", projectID); err != nil {
		return App{}, err
	}
	if err := validateIdentifier("app_id", appID); err != nil {
		return App{}, err
	}
	var record App
	err := store.database.QueryRowContext(
		ctx,
		`SELECT id, project_id, name, created_at, updated_at
         FROM apps WHERE project_id = $1 AND id = $2`,
		projectID,
		appID,
	).Scan(&record.ID, &record.ProjectID, &record.Name, &record.CreatedAt, &record.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return App{}, fmt.Errorf("%w: %s", ErrAppNotFound, appID)
	}
	if err != nil {
		return App{}, fmt.Errorf("read app: %w", err)
	}
	if err := validateApp(record); err != nil {
		return App{}, fmt.Errorf("invalid persisted app: %w", err)
	}
	return record, nil
}

func (store *PostgresStore) ListApps(
	ctx context.Context,
	projectID string,
	name string,
	limit int,
) ([]App, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	if err := validateIdentifier("project_id", projectID); err != nil {
		return nil, err
	}
	if err := validateOptionalAppNameFilter(name); err != nil {
		return nil, err
	}
	rows, err := store.database.QueryContext(
		ctx,
		`SELECT id, project_id, name, created_at, updated_at
		 FROM apps WHERE project_id = $1 AND ($2 = '' OR name = $2)
		 ORDER BY created_at DESC, id DESC LIMIT $3`,
		projectID,
		name,
		postgresLimit(limit),
	)
	if err != nil {
		return nil, fmt.Errorf("list apps: %w", err)
	}
	defer rows.Close()
	records := make([]App, 0)
	for rows.Next() {
		var record App
		if err := rows.Scan(
			&record.ID, &record.ProjectID, &record.Name,
			&record.CreatedAt, &record.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if err := validateApp(record); err != nil {
			return nil, fmt.Errorf("invalid persisted app: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func (store *PostgresStore) UpdateApp(ctx context.Context, record App) (App, error) {
	if err := contextError(ctx); err != nil {
		return App{}, err
	}
	if err := validateApp(record); err != nil {
		return App{}, err
	}
	var updated App
	err := store.database.QueryRowContext(
		ctx,
		`UPDATE apps SET name = $3, updated_at = $4
         WHERE project_id = $1 AND id = $2 AND created_at = $5
         RETURNING id, project_id, name, created_at, updated_at`,
		record.ProjectID,
		record.ID,
		record.Name,
		record.UpdatedAt,
		record.CreatedAt,
	).Scan(&updated.ID, &updated.ProjectID, &updated.Name, &updated.CreatedAt, &updated.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return App{}, fmt.Errorf("%w: %s", ErrAppNotFound, record.ID)
	}
	if err != nil {
		return App{}, fmt.Errorf("%w: update app: %v", ErrAppConflict, err)
	}
	return updated, nil
}

func (store *PostgresStore) SaveDeployment(
	ctx context.Context,
	record Deployment,
) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if err := validateDeploymentRecord(record); err != nil {
		return err
	}
	if _, err := store.GetApp(ctx, record.ProjectID, record.AppID); err != nil {
		return err
	}
	return store.transaction(ctx, func(transaction *sql.Tx) error {
		return saveDeploymentTx(ctx, transaction, record)
	})
}

func saveDeploymentTx(
	ctx context.Context,
	transaction *sql.Tx,
	record Deployment,
) error {
	if err := advisoryLock(
		ctx,
		transaction,
		"deployment\x00"+record.ProjectID+"\x00"+record.ID,
	); err != nil {
		return err
	}
	source, err := json.Marshal(artifactToSnapshot(record.Source))
	if err != nil {
		return fmt.Errorf("encode deployment source metadata: %w", err)
	}
	result, err := transaction.ExecContext(
		ctx,
		`INSERT INTO deployments
         (id, project_id, app_id, runtime, entrypoint, status, source,
          created_at, updated_at, version)
         VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 1)
         ON CONFLICT (id) DO UPDATE SET
             runtime = EXCLUDED.runtime,
             entrypoint = EXCLUDED.entrypoint,
             status = EXCLUDED.status,
             source = EXCLUDED.source,
             created_at = EXCLUDED.created_at,
             updated_at = EXCLUDED.updated_at,
             version = deployments.version + 1
         WHERE deployments.project_id = EXCLUDED.project_id
		   AND deployments.app_id = EXCLUDED.app_id
           AND (deployments.status IN ('uploaded', 'building')
                OR deployments.status = EXCLUDED.status)`,
		record.ID,
		record.ProjectID,
		record.AppID,
		record.Runtime,
		record.EntryPoint,
		record.Status,
		source,
		record.CreatedAt,
		record.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("save deployment: %w", err)
	}
	if err := requireOneRow(result, "deployment changed concurrently"); err != nil {
		return err
	}
	for _, build := range record.Builds {
		if err := saveBuildTx(ctx, transaction, build); err != nil {
			return err
		}
	}
	return nil
}

func saveBuildTx(ctx context.Context, transaction *sql.Tx, build Build) error {
	artifacts := make([]artifactSnapshot, len(build.Artifacts))
	for index, stored := range build.Artifacts {
		artifacts[index] = artifactToSnapshot(stored)
	}
	artifactsJSON, err := json.Marshal(artifacts)
	if err != nil {
		return fmt.Errorf("encode build artifacts: %w", err)
	}
	failureJSON, err := nullableJSON(build.Failure)
	if err != nil {
		return fmt.Errorf("encode build failure: %w", err)
	}
	result, err := transaction.ExecContext(
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
		build.ID,
		build.DeploymentID,
		build.Number,
		build.Status,
		build.Runtime,
		build.EntryPoint,
		build.SourceSHA256,
		artifactsJSON,
		failureJSON,
		build.StartedAt,
		build.FinishedAt,
	)
	if err != nil {
		return fmt.Errorf("save build: %w", err)
	}
	return requireOneRow(result, "build changed concurrently")
}

func (store *PostgresStore) GetDeployment(
	ctx context.Context,
	projectID string,
	deploymentID string,
) (Deployment, error) {
	if err := contextError(ctx); err != nil {
		return Deployment{}, err
	}
	return getDeployment(ctx, store.database, projectID, deploymentID)
}

func (store *PostgresStore) ListDeployments(
	ctx context.Context,
	projectID string,
	appID string,
	limit int,
) ([]Deployment, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	if err := validateIdentifier("project_id", projectID); err != nil {
		return nil, err
	}
	if appID != "" {
		if err := validateIdentifier("app_id", appID); err != nil {
			return nil, err
		}
	}
	rows, err := store.database.QueryContext(
		ctx,
		`SELECT id FROM deployments
		 WHERE project_id = $1 AND ($2 = '' OR app_id = $2)
		 ORDER BY created_at DESC, id DESC LIMIT $3`,
		projectID,
		appID,
		postgresLimit(limit),
	)
	if err != nil {
		return nil, fmt.Errorf("list deployments: %w", err)
	}
	var identifiers []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		identifiers = append(identifiers, id)
	}
	rowsErr := rows.Err()
	closeErr := rows.Close()
	if rowsErr != nil || closeErr != nil {
		return nil, errors.Join(rowsErr, closeErr)
	}
	records := make([]Deployment, 0, len(identifiers))
	for _, id := range identifiers {
		record, err := getDeployment(ctx, store.database, projectID, id)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func (store *PostgresStore) GetBuild(
	ctx context.Context,
	projectID string,
	buildID string,
) (Build, error) {
	if err := contextError(ctx); err != nil {
		return Build{}, err
	}
	if err := validateIdentifier("project_id", projectID); err != nil {
		return Build{}, err
	}
	if err := validateIdentifier("build_id", buildID); err != nil {
		return Build{}, err
	}
	row := store.database.QueryRowContext(
		ctx,
		buildSelect+` WHERE d.project_id = $1 AND b.id = $2`,
		projectID,
		buildID,
	)
	build, err := scanBuild(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Build{}, fmt.Errorf("%w: build %s", ErrNotFound, buildID)
	}
	if err != nil {
		return Build{}, fmt.Errorf("read build: %w", err)
	}
	return build, nil
}

func (store *PostgresStore) ListBuilds(
	ctx context.Context,
	projectID string,
	deploymentID string,
	limit int,
) ([]Build, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	if err := validateIdentifier("project_id", projectID); err != nil {
		return nil, err
	}
	query := buildSelect + ` WHERE d.project_id = $1`
	arguments := []any{projectID}
	if deploymentID != "" {
		if err := validateIdentifier("deployment_id", deploymentID); err != nil {
			return nil, err
		}
		query += ` AND b.deployment_id = $2 ORDER BY b.started_at DESC, b.id DESC LIMIT $3`
		arguments = append(arguments, deploymentID, postgresLimit(limit))
	} else {
		query += ` ORDER BY b.started_at DESC, b.id DESC LIMIT $2`
		arguments = append(arguments, postgresLimit(limit))
	}
	rows, err := store.database.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, fmt.Errorf("list builds: %w", err)
	}
	defer rows.Close()
	records := make([]Build, 0)
	for rows.Next() {
		record, err := scanBuild(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func (store *PostgresStore) RecoverBuildingDeployments(
	ctx context.Context,
	now time.Time,
	failure Failure,
) (int, error) {
	if err := contextError(ctx); err != nil {
		return 0, err
	}
	if err := validateRecovery(now, failure); err != nil {
		return 0, err
	}
	now = now.UTC().Round(0)
	recovered := 0
	err := store.transaction(ctx, func(transaction *sql.Tx) error {
		rows, err := transaction.QueryContext(
			ctx,
			`SELECT project_id, id FROM deployments
             WHERE status = 'building' FOR UPDATE`,
		)
		if err != nil {
			return err
		}
		type identity struct{ projectID, id string }
		var identities []identity
		for rows.Next() {
			var value identity
			if err := rows.Scan(&value.projectID, &value.id); err != nil {
				rows.Close()
				return err
			}
			identities = append(identities, value)
		}
		rowsErr := rows.Err()
		closeErr := rows.Close()
		if rowsErr != nil || closeErr != nil {
			return errors.Join(rowsErr, closeErr)
		}
		for _, value := range identities {
			record, err := getDeployment(ctx, transaction, value.projectID, value.id)
			if err != nil {
				return err
			}
			if !failInterruptedBuild(&record, now, failure) {
				continue
			}
			if err := saveDeploymentTx(ctx, transaction, record); err != nil {
				return err
			}
			recovered++
		}
		return nil
	})
	return recovered, err
}

type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func getDeployment(
	ctx context.Context,
	database queryer,
	projectID string,
	deploymentID string,
) (Deployment, error) {
	if err := validateIdentifier("project_id", projectID); err != nil {
		return Deployment{}, err
	}
	if err := validateIdentifier("deployment_id", deploymentID); err != nil {
		return Deployment{}, err
	}
	var record Deployment
	var runtimeText, statusText string
	var sourceJSON []byte
	err := database.QueryRowContext(
		ctx,
		`SELECT id, project_id, app_id, runtime, entrypoint, status, source,
                created_at, updated_at
         FROM deployments WHERE project_id = $1 AND id = $2`,
		projectID,
		deploymentID,
	).Scan(
		&record.ID,
		&record.ProjectID,
		&record.AppID,
		&runtimeText,
		&record.EntryPoint,
		&statusText,
		&sourceJSON,
		&record.CreatedAt,
		&record.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Deployment{}, fmt.Errorf("%w: %s", ErrNotFound, deploymentID)
	}
	if err != nil {
		return Deployment{}, fmt.Errorf("read deployment: %w", err)
	}
	record.Runtime = Runtime(runtimeText)
	record.Status = Status(statusText)
	var source artifactSnapshot
	if err := json.Unmarshal(sourceJSON, &source); err != nil {
		return Deployment{}, fmt.Errorf("decode deployment source: %w", err)
	}
	record.Source = artifactFromSnapshot(source)
	rows, err := database.QueryContext(
		ctx,
		buildSelect+` WHERE d.project_id = $1 AND b.deployment_id = $2
                      ORDER BY b.number ASC`,
		projectID,
		deploymentID,
	)
	if err != nil {
		return Deployment{}, fmt.Errorf("list deployment builds: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		build, err := scanBuild(rows)
		if err != nil {
			return Deployment{}, err
		}
		record.Builds = append(record.Builds, build)
	}
	if err := rows.Err(); err != nil {
		return Deployment{}, err
	}
	if record.Builds == nil {
		record.Builds = []Build{}
	}
	if err := validateDeploymentRecord(record); err != nil {
		return Deployment{}, fmt.Errorf("invalid persisted deployment: %w", err)
	}
	return record, nil
}

const buildSelect = `SELECT b.id, d.project_id, b.deployment_id, b.number,
       b.status, b.runtime, b.entrypoint, b.source_sha256, b.artifacts,
       b.failure, b.started_at, b.finished_at
FROM builds b JOIN deployments d ON d.id = b.deployment_id`

type rowScanner interface {
	Scan(...any) error
}

func scanBuild(scanner rowScanner) (Build, error) {
	var record Build
	var statusText, runtimeText string
	var artifactsJSON, failureJSON []byte
	var finishedAt sql.NullTime
	err := scanner.Scan(
		&record.ID,
		&record.ProjectID,
		&record.DeploymentID,
		&record.Number,
		&statusText,
		&runtimeText,
		&record.EntryPoint,
		&record.SourceSHA256,
		&artifactsJSON,
		&failureJSON,
		&record.StartedAt,
		&finishedAt,
	)
	if err != nil {
		return Build{}, err
	}
	record.Status = Status(statusText)
	record.Runtime = Runtime(runtimeText)
	var snapshots []artifactSnapshot
	if err := json.Unmarshal(artifactsJSON, &snapshots); err != nil {
		return Build{}, fmt.Errorf("decode build artifacts: %w", err)
	}
	record.Artifacts = make([]Artifact, len(snapshots))
	for index, snapshot := range snapshots {
		record.Artifacts[index] = artifactFromSnapshot(snapshot)
	}
	if len(failureJSON) > 0 {
		var failure Failure
		if err := json.Unmarshal(failureJSON, &failure); err != nil {
			return Build{}, fmt.Errorf("decode build failure: %w", err)
		}
		record.Failure = &failure
	}
	if finishedAt.Valid {
		finished := finishedAt.Time
		record.FinishedAt = &finished
	}
	return record, nil
}

func (store *PostgresStore) transaction(
	ctx context.Context,
	operation func(*sql.Tx) error,
) error {
	if store == nil || store.database == nil {
		return errors.New("deployment PostgreSQL store is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin deployment transaction: %w", err)
	}
	if err := operation(transaction); err != nil {
		return errors.Join(err, transaction.Rollback())
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit deployment transaction: %w", err)
	}
	return nil
}

func advisoryLock(ctx context.Context, transaction *sql.Tx, key string) error {
	if _, err := transaction.ExecContext(
		ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		key,
	); err != nil {
		return fmt.Errorf("lock deployment record: %w", err)
	}
	return nil
}

func nullableJSON(value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	body, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func requireOneRow(result sql.Result, message string) error {
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New(message)
	}
	return nil
}

func postgresLimit(limit int) int {
	if limit <= 0 {
		return 2_147_483_647
	}
	return limit
}
