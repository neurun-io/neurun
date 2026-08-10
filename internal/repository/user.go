package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neurun-io/neurun/internal/domain/account"
	"github.com/neurun-io/neurun/internal/domain/organization"
	"github.com/neurun-io/neurun/internal/domain/session"
)

const userColumns = `id, email, disabled, created_at, updated_at`

// UserRepository stores global identities. What a user may do comes from their
// membership of an organization, not from anything held here.
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
		&record.ID, &record.Email, &record.Disabled,
		&record.CreatedAt, &record.UpdatedAt,
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
		`INSERT INTO users (id, email, password_hash, disabled, created_at, updated_at)
		 VALUES ($1, $2, $3, false, $4, $4)`,
		record.ID, record.Email, passwordHash, record.CreatedAt,
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

func (repository *UserRepository) GetByEmail(
	ctx context.Context,
	email string,
) (account.User, error) {
	rows, err := repository.pool.Query(
		ctx, `SELECT `+userColumns+` FROM users WHERE email = $1`,
		account.NormalizeEmail(email),
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

// List returns the members of one organization, not every user on the install.
func (repository *UserRepository) List(
	ctx context.Context,
	organizationID string,
	limit int,
) ([]account.User, error) {
	rows, err := repository.pool.Query(
		ctx,
		`SELECT u.id, u.email, u.disabled, u.created_at, u.updated_at
		 FROM users u
		 JOIN organization_members m ON m.user_id = u.id
		 JOIN organizations o ON o.id = m.organization_id
		 WHERE m.organization_id = $1 AND u.id <> o.owner_user_id
		 ORDER BY u.created_at DESC, u.id DESC LIMIT $2`,
		organizationID, postgresLimit(limit),
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
		`UPDATE users SET email = $2, disabled = $3, updated_at = $4 WHERE id = $1`,
		record.ID, record.Email, record.Disabled, record.UpdatedAt,
	)
	if err != nil {
		return classifyWriteError("update user", err)
	}
	if tag.RowsAffected() != 1 {
		return account.ErrNotFound
	}
	return nil
}

// Delete removes a person. Keys they minted survive with their attribution
// cleared; an organization they own refuses the delete.
func (repository *UserRepository) Delete(ctx context.Context, userID string) error {
	tag, err := repository.pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	if err != nil {
		return classifyWriteError("delete user", err)
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

// CredentialByEmail returns the sign-in projection, password hash included.
func (repository *UserRepository) CredentialByEmail(
	ctx context.Context,
	email string,
) (session.Account, error) {
	var record session.Account
	err := repository.pool.QueryRow(
		ctx,
		`SELECT id, email, password_hash, disabled, created_at
		 FROM users WHERE email = $1`,
		account.NormalizeEmail(email),
	).Scan(
		&record.ID, &record.Email,
		&record.PasswordHash, &record.Disabled, &record.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return session.Account{}, session.ErrAccountNotFound
	}
	if err != nil {
		return session.Account{}, fmt.Errorf("read account: %w", err)
	}
	return record, nil
}

// CredentialByID is the same projection as CredentialByEmail, for the paths
// that have already established who the caller is.
func (repository *UserRepository) CredentialByID(
	ctx context.Context,
	userID string,
) (session.Account, error) {
	var record session.Account
	err := repository.pool.QueryRow(
		ctx,
		`SELECT id, email, password_hash, disabled, created_at
		 FROM users WHERE id = $1`,
		userID,
	).Scan(
		&record.ID, &record.Email,
		&record.PasswordHash, &record.Disabled, &record.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return session.Account{}, session.ErrAccountNotFound
	}
	if err != nil {
		return session.Account{}, fmt.Errorf("read account: %w", err)
	}
	return record, nil
}

// LiveRole returns the role an enabled user currently holds in one
// organization, so a session issued before a demotion, a removal or a disable
// stops carrying the old scopes on its next request.
func (repository *UserRepository) LiveRole(
	ctx context.Context,
	userID string,
	organizationID string,
) (organization.Role, error) {
	var role string
	var disabled bool
	err := repository.pool.QueryRow(
		ctx,
		`SELECT m.role, u.disabled
		 FROM users u
		 JOIN organization_members m ON m.user_id = u.id
		 WHERE u.id = $1 AND m.organization_id = $2`,
		userID, organizationID,
	).Scan(&role, &disabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", organization.ErrNotMember
	}
	if err != nil {
		return "", fmt.Errorf("read member role: %w", err)
	}
	if disabled {
		return "", session.ErrAccountDisabled
	}
	return organization.Role(role), nil
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
		// Unmapped codes become a 500, so carry enough to diagnose one: 23502
		// (not-null) and 42703 (undefined column) both mean the schema on disk
		// is older than this binary.
		return fmt.Errorf(
			"%s: postgres %s on %s: %w",
			operation, postgres.Code, postgres.TableName, err,
		)
	}
	return fmt.Errorf("%s: %w", operation, err)
}
