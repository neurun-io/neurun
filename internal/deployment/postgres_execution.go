package deployment

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

func (store *PostgresStore) CreateRun(ctx context.Context, record Run) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if err := validateRunRecord(record); err != nil {
		return err
	}
	if record.Status != RunQueued {
		return fmt.Errorf("%w: a new execution must be queued", ErrRunConflict)
	}
	_, err := store.database.ExecContext(
		ctx,
		`INSERT INTO executions
         (id, project_id, deployment_id, build_id, status, input, output,
          failure, logs, created_at, started_at, finished_at,
          rerun_of_execution_id, version)
         VALUES ($1, $2, $3, $4, $5, $6, NULL, NULL, '', $7,
                 NULL, NULL, $8, 1)`,
		record.ID,
		record.ProjectID,
		record.DeploymentID,
		record.BuildID,
		record.Status,
		[]byte(record.Input),
		record.CreatedAt,
		nullableString(record.RerunOfRunID),
	)
	if err != nil {
		return fmt.Errorf("%w: create execution: %v", ErrRunConflict, err)
	}
	return nil
}

func (store *PostgresStore) FinalizeRun(ctx context.Context, record Run) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if err := validateRunRecord(record); err != nil {
		return err
	}
	return store.transaction(ctx, func(transaction *sql.Tx) error {
		current, version, err := getExecutionTx(
			ctx, transaction, record.ProjectID, record.ID, true,
		)
		if err != nil {
			return err
		}
		if err := validateRunFinalization(current, record); err != nil {
			return err
		}
		return updateExecutionTx(
			ctx, transaction, record, version, RunRunning,
		)
	})
}

func (store *PostgresStore) GetRun(
	ctx context.Context,
	projectID string,
	executionID string,
) (Run, error) {
	if err := contextError(ctx); err != nil {
		return Run{}, err
	}
	if err := validateIdentifier("project_id", projectID); err != nil {
		return Run{}, err
	}
	if err := validateIdentifier("execution_id", executionID); err != nil {
		return Run{}, err
	}
	record, _, err := scanExecution(store.database.QueryRowContext(
		ctx,
		executionSelect+` WHERE project_id = $1 AND id = $2`,
		projectID,
		executionID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return Run{}, fmt.Errorf("%w: %s", ErrRunNotFound, executionID)
	}
	if err != nil {
		return Run{}, fmt.Errorf("read execution: %w", err)
	}
	return record, nil
}

func (store *PostgresStore) ListRuns(
	ctx context.Context,
	projectID string,
	deploymentID string,
	limit int,
) ([]Run, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	if err := validateIdentifier("project_id", projectID); err != nil {
		return nil, err
	}
	query := executionSelect + ` WHERE project_id = $1`
	arguments := []any{projectID}
	if deploymentID != "" {
		if err := validateIdentifier("deployment_id", deploymentID); err != nil {
			return nil, err
		}
		query += ` AND deployment_id = $2 ORDER BY created_at DESC, id DESC LIMIT $3`
		arguments = append(arguments, deploymentID, postgresLimit(limit))
	} else {
		query += ` ORDER BY created_at DESC, id DESC LIMIT $2`
		arguments = append(arguments, postgresLimit(limit))
	}
	rows, err := store.database.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, fmt.Errorf("list executions: %w", err)
	}
	defer rows.Close()
	records := make([]Run, 0)
	for rows.Next() {
		record, _, err := scanExecution(rows)
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

func (store *PostgresStore) ClaimQueuedRun(
	ctx context.Context,
	now time.Time,
) (Run, error) {
	if err := contextError(ctx); err != nil {
		return Run{}, err
	}
	if now.IsZero() {
		return Run{}, fmt.Errorf("%w: claim time is required", ErrInvalid)
	}
	now = postgresTime(now)
	var claimed Run
	err := store.transaction(ctx, func(transaction *sql.Tx) error {
		record, version, err := scanExecution(transaction.QueryRowContext(
			ctx,
			executionSelect+` WHERE status = 'queued'
                ORDER BY created_at ASC, id ASC
                FOR UPDATE SKIP LOCKED LIMIT 1`,
		))
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNoQueuedRun
		}
		if err != nil {
			return fmt.Errorf("claim queued execution: %w", err)
		}
		record.Status = RunRunning
		record.StartedAt = &now
		record.FinishedAt = nil
		record.Output = nil
		record.Failure = nil
		record.Logs = ""
		if err := updateExecutionTx(
			ctx, transaction, record, version, RunQueued,
		); err != nil {
			return err
		}
		claimed = record
		return nil
	})
	if err != nil {
		return Run{}, err
	}
	return cloneRun(claimed), nil
}

func (store *PostgresStore) RecoverRunningRuns(
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
	now = postgresTime(now)
	recovered := 0
	err := store.transaction(ctx, func(transaction *sql.Tx) error {
		rows, err := transaction.QueryContext(
			ctx,
			`SELECT project_id, id FROM executions
             WHERE status = 'running' FOR UPDATE`,
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
			record, version, err := getExecutionTx(
				ctx, transaction, value.projectID, value.id, false,
			)
			if err != nil {
				return err
			}
			record.Status = RunFailed
			record.Failure = cloneFailure(&failure)
			record.FinishedAt = &now
			if err := updateExecutionTx(
				ctx, transaction, record, version, RunRunning,
			); err != nil {
				return err
			}
			recovered++
		}
		return nil
	})
	return recovered, err
}

func getExecutionTx(
	ctx context.Context,
	transaction *sql.Tx,
	projectID string,
	executionID string,
	lock bool,
) (Run, int64, error) {
	query := executionSelect + ` WHERE project_id = $1 AND id = $2`
	if lock {
		query += ` FOR UPDATE`
	}
	record, version, err := scanExecution(
		transaction.QueryRowContext(ctx, query, projectID, executionID),
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Run{}, 0, fmt.Errorf("%w: %s", ErrRunNotFound, executionID)
	}
	if err != nil {
		return Run{}, 0, fmt.Errorf("read execution: %w", err)
	}
	return record, version, nil
}

func updateExecutionTx(
	ctx context.Context,
	transaction *sql.Tx,
	record Run,
	currentVersion int64,
	expectedStatus RunStatus,
) error {
	if err := validateRunRecord(record); err != nil {
		return err
	}
	failureJSON, err := nullableJSON(record.Failure)
	if err != nil {
		return err
	}
	var output any
	if record.Output != nil {
		output = []byte(record.Output)
	}
	result, err := transaction.ExecContext(
		ctx,
		`UPDATE executions SET
             deployment_id = $3, build_id = $4, status = $5,
             input = $6, output = $7, failure = $8, logs = $9,
             created_at = $10, started_at = $11, finished_at = $12,
             rerun_of_execution_id = $13, version = version + 1
         WHERE project_id = $1 AND id = $2
           AND version = $14 AND status = $15`,
		record.ProjectID,
		record.ID,
		record.DeploymentID,
		record.BuildID,
		record.Status,
		[]byte(record.Input),
		output,
		failureJSON,
		record.Logs,
		record.CreatedAt,
		record.StartedAt,
		record.FinishedAt,
		nullableString(record.RerunOfRunID),
		currentVersion,
		expectedStatus,
	)
	if err != nil {
		return fmt.Errorf("update execution: %w", err)
	}
	if err := requireOneRow(result, "execution changed concurrently"); err != nil {
		return fmt.Errorf("%w: %v", ErrRunConflict, err)
	}
	return nil
}

const executionSelect = `SELECT id, project_id, deployment_id, build_id,
       status, input, output, failure, logs, created_at, started_at,
       finished_at, rerun_of_execution_id, version
FROM executions`

func scanExecution(scanner rowScanner) (Run, int64, error) {
	var record Run
	var statusText string
	var inputJSON, outputJSON, failureJSON []byte
	var startedAt, finishedAt sql.NullTime
	var rerunOf sql.NullString
	var version int64
	err := scanner.Scan(
		&record.ID,
		&record.ProjectID,
		&record.DeploymentID,
		&record.BuildID,
		&statusText,
		&inputJSON,
		&outputJSON,
		&failureJSON,
		&record.Logs,
		&record.CreatedAt,
		&startedAt,
		&finishedAt,
		&rerunOf,
		&version,
	)
	if err != nil {
		return Run{}, 0, err
	}
	record.Status = RunStatus(statusText)
	record.Input = append(json.RawMessage(nil), inputJSON...)
	if len(outputJSON) > 0 {
		record.Output = append(json.RawMessage(nil), outputJSON...)
	}
	// As in scanBuild, decoding through the pointer maps a JSON null back to an
	// absent failure rather than an empty one.
	if len(failureJSON) > 0 {
		if err := json.Unmarshal(failureJSON, &record.Failure); err != nil {
			return Run{}, 0, fmt.Errorf("decode execution failure: %w", err)
		}
	}
	if startedAt.Valid {
		started := startedAt.Time
		record.StartedAt = &started
	}
	if finishedAt.Valid {
		finished := finishedAt.Time
		record.FinishedAt = &finished
	}
	if rerunOf.Valid {
		record.RerunOfRunID = rerunOf.String
	}
	if err := validateRunRecord(record); err != nil {
		return Run{}, 0, fmt.Errorf("invalid persisted execution: %w", err)
	}
	return record, version, nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

var _ Store = (*PostgresStore)(nil)
