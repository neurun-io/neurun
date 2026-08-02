package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neurun-io/neurun/internal/domain/account"
	"github.com/neurun-io/neurun/internal/domain/operator"
)

const userColumns = `id, username, display_name, role, disabled,
	created_at, updated_at`

// UserRepository stores global identities. A user is not scoped to a project;
// what they may do comes from their role, and what they may reach comes from
// the scopes their session or key carries.
type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) (*UserRepository, error) {
	if pool == nil {
		return nil, errors.New("user repository requires a database pool")
	}
	return &UserRepository{pool: pool}, nil
}

func scanUser(row pgx.CollectableRow) (account.User, error) {
	var record account.User
	err := row.Scan(
		&record.ID, &record.Username, &record.DisplayName, &record.Role,
		&record.Disabled, &record.CreatedAt, &record.UpdatedAt,
	)
	return record, err
}

func (repository *UserRepository) Create(
	ctx context.Context,
	record account.User,
	passwordHash string,
) error {
	_, err := repository.pool.Exec(
		ctx,
		`INSERT INTO users
		 (id, username, display_name, role, password_hash,
		  disabled, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, false, $6, $6)`,
		record.ID, record.Username, record.DisplayName,
		record.Role, passwordHash, record.CreatedAt,
	)
	if err != nil {
		return classifyWriteError("create user", err)
	}
	return nil
}

func (repository *UserRepository) GetByID(
	ctx context.Context,
	userID string,
) (account.User, error) {
	rows, err := repository.pool.Query(
		ctx, `SELECT `+userColumns+` FROM users WHERE id = $1`, userID,
	)
	if err != nil {
		return account.User{}, fmt.Errorf("read user: %w", err)
	}
	record, err := pgx.CollectExactlyOneRow(rows, scanUser)
	if errors.Is(err, pgx.ErrNoRows) {
		return account.User{}, account.ErrNotFound
	}
	if err != nil {
		return account.User{}, fmt.Errorf("read user: %w", err)
	}
	return record, nil
}

func (repository *UserRepository) List(
	ctx context.Context,
	limit int,
) ([]account.User, error) {
	rows, err := repository.pool.Query(
		ctx,
		`SELECT `+userColumns+` FROM users
		 ORDER BY created_at DESC, id DESC LIMIT $1`,
		postgresLimit(limit),
	)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	records, err := pgx.CollectRows(rows, scanUser)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	return records, nil
}

func (repository *UserRepository) Update(
	ctx context.Context,
	record account.User,
) error {
	tag, err := repository.pool.Exec(
		ctx,
		`UPDATE users SET display_name = $2, role = $3, disabled = $4,
		     updated_at = $5
		 WHERE id = $1`,
		record.ID, record.DisplayName, record.Role,
		record.Disabled, record.UpdatedAt,
	)
	if err != nil {
		return classifyWriteError("update user", err)
	}
	if tag.RowsAffected() != 1 {
		return account.ErrNotFound
	}
	return nil
}

// Delete removes a person. Nothing they own goes with them: keys they minted
// survive with their attribution cleared, and every project resource is
// untouched.
func (repository *UserRepository) Delete(ctx context.Context, userID string) error {
	tag, err := repository.pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return account.ErrNotFound
	}
	return nil
}

// Exists reports whether an enabled user holds this identity, which is what
// stops a key being issued to a disabled or unknown owner.
func (repository *UserRepository) Exists(
	ctx context.Context,
	userID string,
) (bool, error) {
	var exists bool
	err := repository.pool.QueryRow(
		ctx,
		`SELECT EXISTS(SELECT 1 FROM users WHERE id = $1 AND disabled = false)`,
		userID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check user: %w", err)
	}
	return exists, nil
}

// CredentialByUsername returns the sign-in projection, password hash included.
func (repository *UserRepository) CredentialByUsername(
	ctx context.Context,
	username string,
) (operator.Account, error) {
	var record operator.Account
	var role string
	err := repository.pool.QueryRow(
		ctx,
		`SELECT id, username, role, password_hash, disabled, created_at
		 FROM users WHERE username = $1`,
		strings.ToLower(strings.TrimSpace(username)),
	).Scan(
		&record.ID, &record.Username, &role,
		&record.PasswordHash, &record.Disabled, &record.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return operator.Account{}, operator.ErrAccountNotFound
	}
	if err != nil {
		return operator.Account{}, fmt.Errorf("read operator account: %w", err)
	}
	record.Role = operator.Role(role)
	return record, nil
}

// LiveRole returns the current role of an enabled user, so a session issued
// before a demotion or a disable stops carrying the old scopes.
func (repository *UserRepository) LiveRole(
	ctx context.Context,
	userID string,
) (operator.Role, error) {
	var role string
	var disabled bool
	err := repository.pool.QueryRow(
		ctx, `SELECT role, disabled FROM users WHERE id = $1`, userID,
	).Scan(&role, &disabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", operator.ErrAccountNotFound
	}
	if err != nil {
		return "", fmt.Errorf("read operator role: %w", err)
	}
	if disabled {
		return "", operator.ErrAccountDisabled
	}
	return operator.Role(role), nil
}

func classifyWriteError(operation string, err error) error {
	var postgres *pgconn.PgError
	if errors.As(err, &postgres) {
		switch postgres.Code {
		case "23505":
			return fmt.Errorf(
				"%w: %s conflicts with an existing resource", account.ErrConflict, operation,
			)
		case "23503", "23514":
			return fmt.Errorf(
				"%w: %s violates a resource constraint", account.ErrInvalid, operation,
			)
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}
