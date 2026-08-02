package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neurun-io/neurun/internal/domain/execution"
)

const executionSelect = `SELECT id, project_id, deployment_id, build_id,
	status, input, output, failure, logs, created_at, started_at,
	finished_at, rerun_of_execution_id, version
FROM executions`

type ExecutionRepository struct {
	pool *pgxpool.Pool
}

func NewExecutionRepository(pool *pgxpool.Pool) (*ExecutionRepository, error) {
	if pool == nil {
		return nil, errors.New("execution repository requires a database pool")
	}
	return &ExecutionRepository{pool: pool}, nil
}

// executionRow pairs a record with the optimistic-locking version it was read
// at, so a write can require that nothing changed in between.
type executionRow struct {
	record  execution.Execution
	version int64
}

func scanExecution(row pgx.CollectableRow) (executionRow, error) {
	var result executionRow
	var inputJSON, outputJSON, failureJSON []byte
	var rerunOf *string
	err := row.Scan(
		&result.record.ID, &result.record.ProjectID,
		&result.record.DeploymentID, &result.record.BuildID,
		&result.record.Status, &inputJSON, &outputJSON, &failureJSON,
		&result.record.Logs, &result.record.CreatedAt,
		&result.record.StartedAt, &result.record.FinishedAt,
		&rerunOf, &result.version,
	)
	if err != nil {
		return executionRow{}, err
	}
	result.record.Input = append(json.RawMessage(nil), inputJSON...)
	if len(outputJSON) > 0 {
		result.record.Output = append(json.RawMessage(nil), outputJSON...)
	}
	// Decoding through the pointer maps a JSON null back to an absent failure
	// rather than an empty one.
	if len(failureJSON) > 0 {
		if err := json.Unmarshal(failureJSON, &result.record.Failure); err != nil {
			return executionRow{}, fmt.Errorf("decode execution failure: %w", err)
		}
	}
	if rerunOf != nil {
		result.record.RerunOfExecutionID = *rerunOf
	}
	if err := result.record.Validate(); err != nil {
		return executionRow{}, fmt.Errorf("invalid persisted execution: %w", err)
	}
	return result, nil
}

func (repository *ExecutionRepository) Create(
	ctx context.Context,
	record execution.Execution,
) error {
	if err := record.Validate(); err != nil {
		return err
	}
	if record.Status != execution.StatusQueued {
		return fmt.Errorf("%w: a new execution must be queued", execution.ErrConflict)
	}
	_, err := repository.pool.Exec(
		ctx,
		`INSERT INTO executions
		 (id, project_id, deployment_id, build_id, status, input, output,
		  failure, logs, created_at, started_at, finished_at,
		  rerun_of_execution_id, version)
		 VALUES ($1, $2, $3, $4, $5, $6, NULL, NULL, '', $7, NULL, NULL, $8, 1)`,
		record.ID, record.ProjectID, record.DeploymentID, record.BuildID,
		record.Status, []byte(record.Input), record.CreatedAt,
		nullableString(record.RerunOfExecutionID),
	)
	if err != nil {
		return fmt.Errorf("%w: create execution: %v", execution.ErrConflict, err)
	}
	return nil
}

func (repository *ExecutionRepository) GetByID(
	ctx context.Context,
	executionID string,
) (execution.Execution, error) {
	rows, err := repository.pool.Query(
		ctx, executionSelect+` WHERE id = $1`, executionID,
	)
	if err != nil {
		return execution.Execution{}, fmt.Errorf("read execution: %w", err)
	}
	row, err := pgx.CollectExactlyOneRow(rows, scanExecution)
	if errors.Is(err, pgx.ErrNoRows) {
		return execution.Execution{}, fmt.Errorf(
			"%w: %s", execution.ErrNotFound, executionID,
		)
	}
	if err != nil {
		return execution.Execution{}, fmt.Errorf("read execution: %w", err)
	}
	return row.record, nil
}

func (repository *ExecutionRepository) List(
	ctx context.Context,
	projectID string,
	deploymentID string,
	limit int,
) ([]execution.Execution, error) {
	rows, err := repository.pool.Query(
		ctx,
		executionSelect+` WHERE ($1 = '' OR project_id = $1)
		 AND ($2 = '' OR deployment_id = $2)
		 ORDER BY created_at DESC, id DESC LIMIT $3`,
		projectID, deploymentID, postgresLimit(limit),
	)
	if err != nil {
		return nil, fmt.Errorf("list executions: %w", err)
	}
	collected, err := pgx.CollectRows(rows, scanExecution)
	if err != nil {
		return nil, fmt.Errorf("list executions: %w", err)
	}
	records := make([]execution.Execution, 0, len(collected))
	for _, row := range collected {
		records = append(records, row.record)
	}
	return records, nil
}

// Finalize writes a terminal execution, refusing the write if the stored record
// is no longer the running one the caller started from.
func (repository *ExecutionRepository) Finalize(
	ctx context.Context,
	record execution.Execution,
) error {
	if err := record.Validate(); err != nil {
		return err
	}
	return transaction(ctx, repository.pool, func(tx pgx.Tx) error {
		current, err := getExecution(ctx, tx, record.ID, true)
		if err != nil {
			return err
		}
		if err := current.record.ValidateTransitionTo(record); err != nil {
			return err
		}
		return updateExecution(
			ctx, tx, record, current.version, execution.StatusRunning,
		)
	})
}

// ClaimQueued takes the oldest queued execution and marks it running. SKIP
// LOCKED lets several workers claim different rows concurrently.
func (repository *ExecutionRepository) ClaimQueued(
	ctx context.Context,
	now time.Time,
) (execution.Execution, error) {
	if now.IsZero() {
		return execution.Execution{}, fmt.Errorf(
			"%w: claim time is required", execution.ErrInvalid,
		)
	}
	now = postgresTime(now)
	var claimed execution.Execution
	err := transaction(ctx, repository.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(
			ctx,
			executionSelect+` WHERE status = 'queued'
			 ORDER BY created_at ASC, id ASC
			 FOR UPDATE SKIP LOCKED LIMIT 1`,
		)
		if err != nil {
			return fmt.Errorf("claim queued execution: %w", err)
		}
		row, err := pgx.CollectExactlyOneRow(rows, scanExecution)
		if errors.Is(err, pgx.ErrNoRows) {
			return execution.ErrNoQueued
		}
		if err != nil {
			return fmt.Errorf("claim queued execution: %w", err)
		}
		if err := row.record.Claim(now); err != nil {
			return err
		}
		if err := updateExecution(
			ctx, tx, row.record, row.version, execution.StatusQueued,
		); err != nil {
			return err
		}
		claimed = row.record
		return nil
	})
	if err != nil {
		return execution.Execution{}, err
	}
	return execution.Clone(claimed), nil
}

// RecoverRunning fails executions a crashed worker left running and reports how
// many it changed.
func (repository *ExecutionRepository) RecoverRunning(
	ctx context.Context,
	now time.Time,
	failure execution.Failure,
) (int, error) {
	if now.IsZero() {
		return 0, fmt.Errorf("%w: recovery time is required", execution.ErrInvalid)
	}
	if err := failure.Validate(); err != nil {
		return 0, err
	}
	now = postgresTime(now)
	recovered := 0
	err := transaction(ctx, repository.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(
			ctx,
			`SELECT id FROM executions WHERE status = 'running' FOR UPDATE`,
		)
		if err != nil {
			return err
		}
		identifiers, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		for _, id := range identifiers {
			row, err := getExecution(ctx, tx, id, false)
			if err != nil {
				return err
			}
			if err := row.record.Fail(failure, row.record.Logs, now); err != nil {
				return err
			}
			if err := updateExecution(
				ctx, tx, row.record, row.version, execution.StatusRunning,
			); err != nil {
				return err
			}
			recovered++
		}
		return nil
	})
	return recovered, err
}

func getExecution(
	ctx context.Context,
	tx pgx.Tx,
	executionID string,
	lock bool,
) (executionRow, error) {
	query := executionSelect + ` WHERE id = $1`
	if lock {
		query += ` FOR UPDATE`
	}
	rows, err := tx.Query(ctx, query, executionID)
	if err != nil {
		return executionRow{}, fmt.Errorf("read execution: %w", err)
	}
	row, err := pgx.CollectExactlyOneRow(rows, scanExecution)
	if errors.Is(err, pgx.ErrNoRows) {
		return executionRow{}, fmt.Errorf("%w: %s", execution.ErrNotFound, executionID)
	}
	if err != nil {
		return executionRow{}, fmt.Errorf("read execution: %w", err)
	}
	return row, nil
}

func updateExecution(
	ctx context.Context,
	tx pgx.Tx,
	record execution.Execution,
	currentVersion int64,
	expectedStatus execution.Status,
) error {
	if err := record.Validate(); err != nil {
		return err
	}
	var failureJSON []byte
	if record.Failure != nil {
		encoded, err := json.Marshal(record.Failure)
		if err != nil {
			return fmt.Errorf("encode execution failure: %w", err)
		}
		failureJSON = encoded
	}
	var output []byte
	if record.Output != nil {
		output = []byte(record.Output)
	}
	tag, err := tx.Exec(
		ctx,
		`UPDATE executions SET
		     deployment_id = $2, build_id = $3, status = $4,
		     input = $5, output = $6, failure = $7, logs = $8,
		     created_at = $9, started_at = $10, finished_at = $11,
		     rerun_of_execution_id = $12, version = version + 1
		 WHERE id = $1 AND version = $13 AND status = $14`,
		record.ID, record.DeploymentID, record.BuildID,
		record.Status, []byte(record.Input), output, failureJSON, record.Logs,
		record.CreatedAt, record.StartedAt, record.FinishedAt,
		nullableString(record.RerunOfExecutionID), currentVersion, expectedStatus,
	)
	if err != nil {
		return fmt.Errorf("update execution: %w", err)
	}
	if err := requireOneRow(tag, "execution changed concurrently"); err != nil {
		return fmt.Errorf("%w: %v", execution.ErrConflict, err)
	}
	return nil
}
