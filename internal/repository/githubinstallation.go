package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neurun-io/neurun/internal/domain/github"
)

type GitHubInstallationRepository struct {
	pool *pgxpool.Pool
}

func NewGitHubInstallationRepository(
	pool *pgxpool.Pool,
) (*GitHubInstallationRepository, error) {
	if pool == nil {
		return nil, errors.New("github installation repository requires a database pool")
	}
	return &GitHubInstallationRepository{pool: pool}, nil
}

const installationColumns = `id, organization_id, installation_id, account_login,
	created_at, updated_at`

func scanInstallation(row pgx.CollectableRow) (github.Installation, error) {
	var record github.Installation
	err := row.Scan(
		&record.ID, &record.OrganizationID, &record.InstallationID,
		&record.AccountLogin, &record.CreatedAt, &record.UpdatedAt,
	)
	return record, err
}

// Save records the installation for an organization. Re-installing replaces the
// previous record rather than accumulating one per attempt.
func (repository *GitHubInstallationRepository) Save(
	ctx context.Context,
	record github.Installation,
) (github.Installation, error) {
	if err := record.Validate(); err != nil {
		return github.Installation{}, err
	}
	rows, err := repository.pool.Query(
		ctx,
		`INSERT INTO github_installations
		 (id, organization_id, installation_id, account_login, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $5)
		 ON CONFLICT (organization_id) DO UPDATE SET
		     installation_id = EXCLUDED.installation_id,
		     account_login = EXCLUDED.account_login,
		     updated_at = EXCLUDED.updated_at
		 RETURNING `+installationColumns,
		record.ID, record.OrganizationID, record.InstallationID,
		record.AccountLogin, record.CreatedAt,
	)
	if err != nil {
		return github.Installation{}, classifyWriteError("save installation", err)
	}
	saved, err := pgx.CollectExactlyOneRow(rows, scanInstallation)
	if err != nil {
		return github.Installation{}, fmt.Errorf("save installation: %w", err)
	}
	return saved, nil
}

func (repository *GitHubInstallationRepository) ByOrganization(
	ctx context.Context,
	organizationID string,
) (github.Installation, error) {
	rows, err := repository.pool.Query(
		ctx,
		`SELECT `+installationColumns+` FROM github_installations
		 WHERE organization_id = $1`,
		organizationID,
	)
	if err != nil {
		return github.Installation{}, fmt.Errorf("read installation: %w", err)
	}
	record, err := pgx.CollectExactlyOneRow(rows, scanInstallation)
	if errors.Is(err, pgx.ErrNoRows) {
		return github.Installation{}, github.ErrNoInstallation
	}
	if err != nil {
		return github.Installation{}, fmt.Errorf("read installation: %w", err)
	}
	return record, nil
}

// ByInstallationID resolves the organization behind a webhook delivery. The
// installation id is the only identity a delivery carries, and the column is
// unique, so it names at most one organization.
func (repository *GitHubInstallationRepository) ByInstallationID(
	ctx context.Context,
	installationID int64,
) (github.Installation, error) {
	rows, err := repository.pool.Query(
		ctx,
		`SELECT `+installationColumns+` FROM github_installations
		 WHERE installation_id = $1`,
		installationID,
	)
	if err != nil {
		return github.Installation{}, fmt.Errorf("read installation: %w", err)
	}
	record, err := pgx.CollectExactlyOneRow(rows, scanInstallation)
	if errors.Is(err, pgx.ErrNoRows) {
		return github.Installation{}, github.ErrNoInstallation
	}
	if err != nil {
		return github.Installation{}, fmt.Errorf("read installation: %w", err)
	}
	return record, nil
}

// ByApp resolves the installation that owns an app, walking app to project to
// organization so a caller never supplies it.
func (repository *GitHubInstallationRepository) ByApp(
	ctx context.Context,
	appID string,
) (github.Installation, error) {
	rows, err := repository.pool.Query(
		ctx,
		`SELECT g.id, g.organization_id, g.installation_id, g.account_login,
		        g.created_at, g.updated_at
		 FROM github_installations g
		 JOIN projects p ON p.organization_id = g.organization_id
		 JOIN apps a ON a.project_id = p.id
		 WHERE a.id = $1`,
		appID,
	)
	if err != nil {
		return github.Installation{}, fmt.Errorf("read installation: %w", err)
	}
	record, err := pgx.CollectExactlyOneRow(rows, scanInstallation)
	if errors.Is(err, pgx.ErrNoRows) {
		return github.Installation{}, github.ErrNoInstallation
	}
	if err != nil {
		return github.Installation{}, fmt.Errorf("read installation: %w", err)
	}
	return record, nil
}

func (repository *GitHubInstallationRepository) Delete(
	ctx context.Context,
	organizationID string,
	now time.Time,
) error {
	_ = now
	tag, err := repository.pool.Exec(
		ctx, `DELETE FROM github_installations WHERE organization_id = $1`,
		organizationID,
	)
	if err != nil {
		return fmt.Errorf("delete installation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return github.ErrNoInstallation
	}
	return nil
}
