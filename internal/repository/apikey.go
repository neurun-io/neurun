package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neurun-io/neurun/internal/domain/account"
)

const apiKeyColumns = `id, COALESCE(user_id, ''), name, key_prefix, scopes,
	created_at, revoked_at`

// APIKeyRepository stores issued keys. A key carries scopes and nothing else:
// it is not bound to a project, because scopes already say what it may reach.
type APIKeyRepository struct {
	pool *pgxpool.Pool
}

func NewAPIKeyRepository(pool *pgxpool.Pool) (*APIKeyRepository, error) {
	if pool == nil {
		return nil, errors.New("API key repository requires a database pool")
	}
	return &APIKeyRepository{pool: pool}, nil
}

func scanKey(row pgx.CollectableRow) (account.Key, error) {
	var record account.Key
	err := row.Scan(
		&record.ID, &record.UserID, &record.Name, &record.Prefix,
		&record.Scopes, &record.CreatedAt, &record.RevokedAt,
	)
	return record, err
}

func (repository *APIKeyRepository) Create(
	ctx context.Context,
	record account.Key,
	digest []byte,
) error {
	_, err := repository.pool.Exec(
		ctx,
		`INSERT INTO api_keys
		 (id, user_id, name, key_prefix, key_hash, scopes, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		record.ID, nullableString(record.UserID), record.Name,
		record.Prefix, digest, record.Scopes, record.CreatedAt,
	)
	if err != nil {
		return classifyWriteError("create key", err)
	}
	return nil
}

func (repository *APIKeyRepository) List(
	ctx context.Context,
	limit int,
) ([]account.Key, error) {
	rows, err := repository.pool.Query(
		ctx,
		`SELECT `+apiKeyColumns+` FROM api_keys
		 ORDER BY created_at DESC, id DESC LIMIT $1`,
		postgresLimit(limit),
	)
	if err != nil {
		return nil, fmt.Errorf("list keys: %w", err)
	}
	records, err := pgx.CollectRows(rows, scanKey)
	if err != nil {
		return nil, fmt.Errorf("list keys: %w", err)
	}
	return records, nil
}

// Revoke is idempotent: revoking an already-revoked key keeps the first
// revocation time rather than moving it.
func (repository *APIKeyRepository) Revoke(
	ctx context.Context,
	keyID string,
	now time.Time,
) (account.Key, error) {
	rows, err := repository.pool.Query(
		ctx,
		`UPDATE api_keys SET revoked_at = COALESCE(revoked_at, $2)
		 WHERE id = $1
		 RETURNING `+apiKeyColumns,
		keyID, now,
	)
	if err != nil {
		return account.Key{}, fmt.Errorf("revoke key: %w", err)
	}
	record, err := pgx.CollectExactlyOneRow(rows, scanKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return account.Key{}, account.ErrNotFound
	}
	if err != nil {
		return account.Key{}, fmt.Errorf("revoke key: %w", err)
	}
	return record, nil
}

// CredentialByPrefix returns the stored half of a live key. Verifying the
// presented secret against it is the caller's job — this layer never compares
// secrets.
func (repository *APIKeyRepository) CredentialByPrefix(
	ctx context.Context,
	prefix string,
) (account.KeyCredential, error) {
	var credential account.KeyCredential
	err := repository.pool.QueryRow(
		ctx,
		`SELECT id, scopes, key_hash FROM api_keys
		 WHERE key_prefix = $1 AND revoked_at IS NULL`,
		prefix,
	).Scan(&credential.ID, &credential.Scopes, &credential.Digest)
	if errors.Is(err, pgx.ErrNoRows) {
		return account.KeyCredential{}, account.ErrNotFound
	}
	if err != nil {
		return account.KeyCredential{}, fmt.Errorf("read key credential: %w", err)
	}
	return credential, nil
}
