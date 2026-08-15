// Package repository is the persistence layer. Every query in the system lives
// here; domains describe records, repositories store and load them.
package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// transaction runs operation against a transaction, rolling back on any error.
//
// Multi-statement reads use this too: a deployment and its builds come from two
// queries, and a snapshot keeps them from disagreeing.
func transaction(
	ctx context.Context,
	pool *pgxpool.Pool,
	operation func(pgx.Tx) error,
) error {
	if pool == nil {
		return errors.New("repository pool is not configured")
	}
	tx, err := pool.Begin(contextOrBackground(ctx))
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	if err := operation(tx); err != nil {
		return errors.Join(err, tx.Rollback(context.WithoutCancel(ctx)))
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// advisoryKey joins the parts of a lock name into one text value.
//
// The separator has to be a character ids.Validate rejects, so distinct part
// sequences cannot collide into one lock. It cannot be NUL: PostgreSQL refuses
// a NUL byte in any text value, so hashing such a key fails the whole
// transaction with SQLSTATE 22021 rather than locking anything.
func advisoryKey(parts ...string) string {
	return strings.Join(parts, "/")
}

func advisoryLock(ctx context.Context, tx pgx.Tx, key string) error {
	if _, err := tx.Exec(
		ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		key,
	); err != nil {
		return fmt.Errorf("lock record: %w", err)
	}
	return nil
}

// postgresTime normalizes a minted timestamp to the precision a timestamptz
// column actually keeps.
//
// PostgreSQL stores microseconds and truncates on the way in, while Go carries
// nanoseconds. Handing a caller the untruncated value while the database keeps
// the truncated one makes the two disagree by up to a microsecond, and
// ValidateTransitionTo compares StartedAt exactly — so an untruncated claim time
// rejected every finalization as changed metadata. Round(0) alone is not enough:
// it strips the monotonic reading, not precision.
func postgresTime(value time.Time) time.Time {
	return value.UTC().Round(0).Truncate(time.Microsecond)
}

func nullableString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func requireOneRow(tag pgconn.CommandTag, message string) error {
	if tag.RowsAffected() != 1 {
		return errors.New(message)
	}
	return nil
}

func postgresLimit(limit int) int32 {
	if limit <= 0 {
		return 2_147_483_647
	}
	return int32(limit)
}

func contextOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

const inOrganization = ` project_id IN (
	SELECT id FROM projects WHERE organization_id = %s
)`

const appsInOrganization = ` app_id IN (
	SELECT a.id FROM apps a
	JOIN projects p ON p.id = a.project_id
	WHERE p.organization_id = %s
)`
