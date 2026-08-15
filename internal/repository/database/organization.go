package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neurun-io/neurun/internal/domain/account"
	"github.com/neurun-io/neurun/internal/domain/organization"
)

const organizationColumns = `id, owner_user_id, name, created_at, updated_at`

const inviteColumns = `id, organization_id, email, role, COALESCE(invited_by, ''),
	created_at, expires_at, accepted_at, revoked_at`

type OrganizationRepository struct {
	pool *pgxpool.Pool
}

func NewOrganizationRepository(pool *pgxpool.Pool) (*OrganizationRepository, error) {
	if pool == nil {
		return nil, errors.New("organization repository requires a database pool")
	}
	return &OrganizationRepository{pool: pool}, nil
}

func scanOrganization(row pgx.CollectableRow) (organization.Organization, error) {
	var record organization.Organization
	err := row.Scan(
		&record.ID, &record.OwnerUserID, &record.Name,
		&record.CreatedAt, &record.UpdatedAt,
	)
	return record, err
}

// Create writes the organization and seeds its owner as an admin member in one
// transaction: an organization whose owner is not a member of it would be a
// tenant nobody can reach.
func (repository *OrganizationRepository) Create(
	ctx context.Context,
	record organization.Organization,
) (organization.Organization, error) {
	if err := record.Validate(); err != nil {
		return organization.Organization{}, err
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return organization.Organization{}, fmt.Errorf("create organization: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var owned bool
	if err := tx.QueryRow(
		ctx, `SELECT EXISTS(SELECT 1 FROM organizations WHERE owner_user_id = $1)`,
		record.OwnerUserID,
	).Scan(&owned); err != nil {
		return organization.Organization{}, fmt.Errorf("create organization: %w", err)
	}
	if owned {
		return organization.Organization{}, organization.ErrAlreadyOwner
	}
	if _, err := tx.Exec(
		ctx,
		`INSERT INTO organizations (id, owner_user_id, name, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $4)`,
		record.ID, record.OwnerUserID, record.Name, record.CreatedAt,
	); err != nil {
		return organization.Organization{}, classifyOrganizationError("create organization", err)
	}
	if _, err := tx.Exec(
		ctx,
		`INSERT INTO organization_members
		 (organization_id, user_id, role, created_at, updated_at)
		 VALUES ($1, $2, 'admin', $3, $3)`,
		record.ID, record.OwnerUserID, record.CreatedAt,
	); err != nil {
		return organization.Organization{}, classifyOrganizationError("seed owner", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return organization.Organization{}, fmt.Errorf("create organization: %w", err)
	}
	return record, nil
}

func (repository *OrganizationRepository) GetByID(
	ctx context.Context,
	organizationID string,
) (organization.Organization, error) {
	rows, err := repository.pool.Query(
		ctx, `SELECT `+organizationColumns+` FROM organizations WHERE id = $1`,
		organizationID,
	)
	if err != nil {
		return organization.Organization{}, fmt.Errorf("read organization: %w", err)
	}
	record, err := pgx.CollectExactlyOneRow(rows, scanOrganization)
	if errors.Is(err, pgx.ErrNoRows) {
		return organization.Organization{}, organization.ErrNotFound
	}
	if err != nil {
		return organization.Organization{}, fmt.Errorf("read organization: %w", err)
	}
	return record, nil
}

// ListForUser returns every organization the user belongs to, newest first.
func (repository *OrganizationRepository) ListForUser(
	ctx context.Context,
	userID string,
) ([]organization.Organization, error) {
	rows, err := repository.pool.Query(
		ctx,
		`SELECT o.id, o.owner_user_id, o.name, o.created_at, o.updated_at
		 FROM organizations o
		 JOIN organization_members m ON m.organization_id = o.id
		 WHERE m.user_id = $1
		 ORDER BY o.created_at DESC, o.id DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list organizations: %w", err)
	}
	records, err := pgx.CollectRows(rows, scanOrganization)
	if err != nil {
		return nil, fmt.Errorf("list organizations: %w", err)
	}
	return records, nil
}

func (repository *OrganizationRepository) Rename(
	ctx context.Context,
	record organization.Organization,
) (organization.Organization, error) {
	if err := record.Validate(); err != nil {
		return organization.Organization{}, err
	}
	rows, err := repository.pool.Query(
		ctx,
		`UPDATE organizations SET name = $2, updated_at = $3 WHERE id = $1
		 RETURNING `+organizationColumns,
		record.ID, record.Name, record.UpdatedAt,
	)
	if err != nil {
		return organization.Organization{}, classifyOrganizationError("rename organization", err)
	}
	updated, err := pgx.CollectExactlyOneRow(rows, scanOrganization)
	if errors.Is(err, pgx.ErrNoRows) {
		return organization.Organization{}, organization.ErrNotFound
	}
	if err != nil {
		return organization.Organization{}, fmt.Errorf("rename organization: %w", err)
	}
	return updated, nil
}

/* ------------------------------- members -------------------------------- */

func scanMember(row pgx.CollectableRow) (organization.Member, error) {
	var record organization.Member
	var role string
	err := row.Scan(
		&record.OrganizationID, &record.UserID, &record.Email, &role,
		&record.Owner, &record.CreatedAt, &record.UpdatedAt,
	)
	record.Role = organization.Role(role)
	return record, err
}

const memberQuery = `SELECT m.organization_id, m.user_id, u.email, m.role,
	(o.owner_user_id = m.user_id) AS owner, m.created_at, m.updated_at
	FROM organization_members m
	JOIN users u ON u.id = m.user_id
	JOIN organizations o ON o.id = m.organization_id`

func (repository *OrganizationRepository) Member(
	ctx context.Context,
	organizationID string,
	userID string,
) (organization.Member, error) {
	rows, err := repository.pool.Query(
		ctx, memberQuery+` WHERE m.organization_id = $1 AND m.user_id = $2`,
		organizationID, userID,
	)
	if err != nil {
		return organization.Member{}, fmt.Errorf("read membership: %w", err)
	}
	record, err := pgx.CollectExactlyOneRow(rows, scanMember)
	if errors.Is(err, pgx.ErrNoRows) {
		return organization.Member{}, organization.ErrNotMember
	}
	if err != nil {
		return organization.Member{}, fmt.Errorf("read membership: %w", err)
	}
	return record, nil
}

// FirstForUser is where a session lands when it does not name an organization:
// the oldest membership, so sign-in is stable rather than depending on ordering.
func (repository *OrganizationRepository) FirstForUser(
	ctx context.Context,
	userID string,
) (organization.Member, error) {
	rows, err := repository.pool.Query(
		ctx, memberQuery+` WHERE m.user_id = $1 ORDER BY m.created_at ASC LIMIT 1`,
		userID,
	)
	if err != nil {
		return organization.Member{}, fmt.Errorf("read membership: %w", err)
	}
	record, err := pgx.CollectExactlyOneRow(rows, scanMember)
	if errors.Is(err, pgx.ErrNoRows) {
		return organization.Member{}, organization.ErrNotMember
	}
	if err != nil {
		return organization.Member{}, fmt.Errorf("read membership: %w", err)
	}
	return record, nil
}

func (repository *OrganizationRepository) ListMembers(
	ctx context.Context,
	organizationID string,
	limit int,
) ([]organization.Member, error) {
	rows, err := repository.pool.Query(
		ctx,
		memberQuery+` WHERE m.organization_id = $1
		 ORDER BY m.created_at ASC, m.user_id ASC LIMIT $2`,
		organizationID, postgresLimit(limit),
	)
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	records, err := pgx.CollectRows(rows, scanMember)
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	return records, nil
}

// SetMemberRole refuses to move the owner: the owner is the one membership that
// cannot be demoted, so an organization always has somebody who can administer
// it.
func (repository *OrganizationRepository) SetMemberRole(
	ctx context.Context,
	organizationID string,
	userID string,
	role organization.Role,
	now time.Time,
) (organization.Member, error) {
	owner, err := repository.isOwner(ctx, organizationID, userID)
	if err != nil {
		return organization.Member{}, err
	}
	if owner {
		return organization.Member{}, organization.ErrOwnerLocked
	}
	tag, err := repository.pool.Exec(
		ctx,
		`UPDATE organization_members SET role = $3, updated_at = $4
		 WHERE organization_id = $1 AND user_id = $2`,
		organizationID, userID, string(role), now,
	)
	if err != nil {
		return organization.Member{}, classifyOrganizationError("update member", err)
	}
	if tag.RowsAffected() == 0 {
		return organization.Member{}, organization.ErrNotMember
	}
	return repository.Member(ctx, organizationID, userID)
}

func (repository *OrganizationRepository) RemoveMember(
	ctx context.Context,
	organizationID string,
	userID string,
) error {
	owner, err := repository.isOwner(ctx, organizationID, userID)
	if err != nil {
		return err
	}
	if owner {
		return organization.ErrOwnerLocked
	}
	tag, err := repository.pool.Exec(
		ctx,
		`DELETE FROM organization_members WHERE organization_id = $1 AND user_id = $2`,
		organizationID, userID,
	)
	if err != nil {
		return fmt.Errorf("remove member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return organization.ErrNotMember
	}
	return nil
}

func (repository *OrganizationRepository) isOwner(
	ctx context.Context,
	organizationID string,
	userID string,
) (bool, error) {
	var owner bool
	err := repository.pool.QueryRow(
		ctx,
		`SELECT owner_user_id = $2 FROM organizations WHERE id = $1`,
		organizationID, userID,
	).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, organization.ErrNotFound
	}
	if err != nil {
		return false, fmt.Errorf("read organization owner: %w", err)
	}
	return owner, nil
}

/* ------------------------------- invites -------------------------------- */

func scanInvite(row pgx.CollectableRow) (organization.Invite, error) {
	var record organization.Invite
	var role string
	err := row.Scan(
		&record.ID, &record.OrganizationID, &record.Email, &role,
		&record.InvitedBy, &record.CreatedAt, &record.ExpiresAt,
		&record.AcceptedAt, &record.RevokedAt,
	)
	record.Role = organization.Role(role)
	return record, err
}

func (repository *OrganizationRepository) CreateInvite(
	ctx context.Context,
	record organization.Invite,
	digest []byte,
) error {
	if err := record.Validate(); err != nil {
		return err
	}
	_, err := repository.pool.Exec(
		ctx,
		`INSERT INTO organization_invites
		 (id, organization_id, email, role, token_hash, invited_by,
		  created_at, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		record.ID, record.OrganizationID, record.Email, string(record.Role),
		digest, nullableString(record.InvitedBy), record.CreatedAt, record.ExpiresAt,
	)
	if err != nil {
		return classifyOrganizationError("create invitation", err)
	}
	return nil
}

func (repository *OrganizationRepository) ListInvites(
	ctx context.Context,
	organizationID string,
	limit int,
) ([]organization.Invite, error) {
	rows, err := repository.pool.Query(
		ctx,
		`SELECT `+inviteColumns+` FROM organization_invites
		 WHERE organization_id = $1
		 ORDER BY created_at DESC, id DESC LIMIT $2`,
		organizationID, postgresLimit(limit),
	)
	if err != nil {
		return nil, fmt.Errorf("list invitations: %w", err)
	}
	records, err := pgx.CollectRows(rows, scanInvite)
	if err != nil {
		return nil, fmt.Errorf("list invitations: %w", err)
	}
	return records, nil
}

// InviteByToken resolves a presented token to its invitation. The token is
// never stored, so the lookup is by digest.
func (repository *OrganizationRepository) InviteByToken(
	ctx context.Context,
	digest []byte,
) (organization.Invite, error) {
	rows, err := repository.pool.Query(
		ctx, `SELECT `+inviteColumns+` FROM organization_invites WHERE token_hash = $1`,
		digest,
	)
	if err != nil {
		return organization.Invite{}, fmt.Errorf("read invitation: %w", err)
	}
	record, err := pgx.CollectExactlyOneRow(rows, scanInvite)
	if errors.Is(err, pgx.ErrNoRows) {
		return organization.Invite{}, organization.ErrNotFound
	}
	if err != nil {
		return organization.Invite{}, fmt.Errorf("read invitation: %w", err)
	}
	return record, nil
}

// Redeem marks an invitation accepted and adds the membership in one
// transaction, so a token cannot be spent twice by two concurrent calls.
func (repository *OrganizationRepository) Redeem(
	ctx context.Context,
	invite organization.Invite,
	userID string,
	now time.Time,
) (organization.Member, error) {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return organization.Member{}, fmt.Errorf("accept invitation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(
		ctx,
		`UPDATE organization_invites SET accepted_at = $2
		 WHERE id = $1 AND accepted_at IS NULL AND revoked_at IS NULL`,
		invite.ID, now,
	)
	if err != nil {
		return organization.Member{}, fmt.Errorf("accept invitation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return organization.Member{}, organization.ErrInviteSpent
	}
	if _, err := tx.Exec(
		ctx,
		`INSERT INTO organization_members
		 (organization_id, user_id, role, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $4)
		 ON CONFLICT (organization_id, user_id) DO UPDATE
		 SET role = EXCLUDED.role, updated_at = EXCLUDED.updated_at`,
		invite.OrganizationID, userID, string(invite.Role), now,
	); err != nil {
		return organization.Member{}, classifyOrganizationError("accept invitation", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return organization.Member{}, fmt.Errorf("accept invitation: %w", err)
	}
	return repository.Member(ctx, invite.OrganizationID, userID)
}

func (repository *OrganizationRepository) RevokeInvite(
	ctx context.Context,
	organizationID string,
	inviteID string,
	now time.Time,
) error {
	tag, err := repository.pool.Exec(
		ctx,
		`UPDATE organization_invites SET revoked_at = COALESCE(revoked_at, $3)
		 WHERE id = $1 AND organization_id = $2 AND accepted_at IS NULL`,
		inviteID, organizationID, now,
	)
	if err != nil {
		return fmt.Errorf("revoke invitation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return organization.ErrNotFound
	}
	return nil
}

func classifyOrganizationError(operation string, err error) error {
	converted := classifyWriteError(operation, err)
	switch {
	case errors.Is(converted, account.ErrConflict):
		return fmt.Errorf("%w: %s", organization.ErrConflict, operation)
	case errors.Is(converted, account.ErrInvalid):
		return fmt.Errorf("%w: %s", organization.ErrInvalid, operation)
	}
	return converted
}
