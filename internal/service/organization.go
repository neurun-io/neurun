package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/neurun-io/neurun/internal/domain/account"
	"github.com/neurun-io/neurun/internal/domain/organization"
	"github.com/neurun-io/neurun/internal/dto"
	"github.com/neurun-io/neurun/internal/ids"
	"github.com/neurun-io/neurun/internal/repository"
)

// OrganizationService owns tenancy: who belongs where, and with what standing.
type OrganizationService struct {
	organizations *repository.OrganizationRepository
	users         *repository.UserRepository
	now           func() time.Time
	newID         func(string) (string, error)
}

func NewOrganizationService(
	organizations *repository.OrganizationRepository,
	users *repository.UserRepository,
	now func() time.Time,
	newID func(string) (string, error),
) (*OrganizationService, error) {
	if organizations == nil || users == nil {
		return nil, errors.New("organization service requires its repositories")
	}
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	if newID == nil {
		newID = ids.New
	}
	return &OrganizationService{
		organizations: organizations, users: users, now: now, newID: newID,
	}, nil
}

// Create starts an organization owned by one user, who is seeded as its admin.
func (service *OrganizationService) Create(
	ctx context.Context,
	ownerUserID string,
	name string,
) (organization.Organization, error) {
	id, err := service.newID("org")
	if err != nil {
		return organization.Organization{}, err
	}
	record, err := organization.New(id, ownerUserID, name, service.now())
	if err != nil {
		return organization.Organization{}, err
	}
	return service.organizations.Create(ctx, record)
}

func (service *OrganizationService) Get(
	ctx context.Context,
	organizationID string,
) (organization.Organization, error) {
	return service.organizations.GetByID(ctx, organizationID)
}

func (service *OrganizationService) ListForUser(
	ctx context.Context,
	userID string,
) ([]organization.Organization, error) {
	return service.organizations.ListForUser(ctx, userID)
}

func (service *OrganizationService) Rename(
	ctx context.Context,
	organizationID string,
	name string,
) (organization.Organization, error) {
	record, err := service.organizations.GetByID(ctx, organizationID)
	if err != nil {
		return organization.Organization{}, err
	}
	if err := record.Rename(name, service.now()); err != nil {
		return organization.Organization{}, err
	}
	return service.organizations.Rename(ctx, record)
}

func (service *OrganizationService) Membership(
	ctx context.Context,
	organizationID string,
	userID string,
) (organization.Member, error) {
	return service.organizations.Member(ctx, organizationID, userID)
}

func (service *OrganizationService) ListMembers(
	ctx context.Context,
	organizationID string,
	limit int,
) ([]organization.Member, error) {
	return service.organizations.ListMembers(ctx, organizationID, limit)
}

func (service *OrganizationService) SetMemberRole(
	ctx context.Context,
	organizationID string,
	userID string,
	rawRole string,
) (organization.Member, error) {
	role, err := organization.ParseRole(rawRole)
	if err != nil {
		return organization.Member{}, err
	}
	return service.organizations.SetMemberRole(
		ctx, organizationID, userID, role, service.now(),
	)
}

func (service *OrganizationService) RemoveMember(
	ctx context.Context,
	organizationID string,
	userID string,
) error {
	return service.organizations.RemoveMember(ctx, organizationID, userID)
}

// Invite offers membership to an address. The token is returned exactly once;
// only its digest is stored.
func (service *OrganizationService) Invite(
	ctx context.Context,
	organizationID string,
	invitedBy string,
	request dto.InviteRequest,
) (organization.CreatedInvite, error) {
	role, err := organization.ParseRole(request.Role)
	if err != nil {
		return organization.CreatedInvite{}, err
	}
	email := account.NormalizeEmail(request.Email)
	if !account.ValidEmail(email) {
		return organization.CreatedInvite{}, fmt.Errorf(
			"%w: email is invalid", organization.ErrInvalid,
		)
	}
	id, err := service.newID("inv")
	if err != nil {
		return organization.CreatedInvite{}, err
	}
	record, err := organization.NewInvite(
		id, organizationID, email, role, invitedBy, service.now(),
	)
	if err != nil {
		return organization.CreatedInvite{}, err
	}
	token, err := organization.NewInviteToken()
	if err != nil {
		return organization.CreatedInvite{}, err
	}
	digest := organization.InviteDigest(token)
	if err := service.organizations.CreateInvite(ctx, record, digest[:]); err != nil {
		return organization.CreatedInvite{}, err
	}
	return organization.CreatedInvite{Invite: record, Token: token}, nil
}

func (service *OrganizationService) ListInvites(
	ctx context.Context,
	organizationID string,
	limit int,
) ([]organization.Invite, error) {
	return service.organizations.ListInvites(ctx, organizationID, limit)
}

func (service *OrganizationService) RevokeInvite(
	ctx context.Context,
	organizationID string,
	inviteID string,
) error {
	return service.organizations.RevokeInvite(
		ctx, organizationID, inviteID, service.now(),
	)
}

// InviteByToken resolves a presented token without spending it, so a caller can
// be shown which organization they are about to join before they commit.
func (service *OrganizationService) InviteByToken(
	ctx context.Context,
	token string,
) (organization.Invite, error) {
	digest := organization.InviteDigest(token)
	return service.organizations.InviteByToken(ctx, digest[:])
}

// Accept spends an invitation for a user whose address must match it.
func (service *OrganizationService) Accept(
	ctx context.Context,
	token string,
	userID string,
	email string,
) (organization.Member, error) {
	invite, err := service.InviteByToken(ctx, token)
	if err != nil {
		return organization.Member{}, err
	}
	if err := invite.Redeemable(email, service.now()); err != nil {
		return organization.Member{}, err
	}
	return service.organizations.Redeem(ctx, invite, userID, service.now())
}
