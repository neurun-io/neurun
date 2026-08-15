// Package organization owns the tenant: the container every project, app,
// deployment and execution hangs from, and the memberships that decide who may
// do what inside it.
package organization

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrInvalid     = errors.New("invalid organization request")
	ErrNotFound    = errors.New("organization resource not found")
	ErrConflict    = errors.New("organization resource conflict")
	ErrNotMember   = errors.New("caller is not a member of this organization")
	ErrOwnerLocked = errors.New("the owner's membership cannot be changed or removed")
	// ErrAlreadyOwner is one user trying to own a second organization. Owning
	// is capped at one; joining is not capped at all.
	ErrAlreadyOwner  = errors.New("this account already owns an organization")
	ErrInviteSpent   = errors.New("invitation has already been accepted or revoked")
	ErrInviteStale   = errors.New("invitation has expired")
	ErrInviteAddress = errors.New("invitation was issued to a different address")
)

// Role decides what a member may do inside one organization. A user can hold a
// different role in each organization they belong to.
type Role string

const (
	RoleAdmin    Role = "admin"
	RoleOperator Role = "operator"
	RoleViewer   Role = "viewer"
)

func (role Role) Scopes() []string {
	switch role {
	case RoleAdmin:
		return []string{"*"}
	case RoleOperator:
		return []string{
			"projects:read", "apps:read", "apps:write",
			"deployments:read", "deployments:write",
			"executions:read", "executions:write",
			"browser_profiles:read", "browser_profiles:write",
		}
	case RoleViewer:
		// No browser_profiles:write, which is also what gates reading a
		// profile's cookies in the clear.
		return []string{
			"projects:read", "apps:read", "deployments:read",
			"executions:read", "users:read", "api_keys:read",
			"browser_profiles:read",
		}
	default:
		// Empty, not nil: an account between organizations grants nothing, and
		// the contract publishes scopes as an array. nil marshals to null.
		return []string{}
	}
}

func (role Role) Valid() bool {
	switch role {
	case RoleAdmin, RoleOperator, RoleViewer:
		return true
	default:
		return false
	}
}

func ParseRole(raw string) (Role, error) {
	role := Role(strings.ToLower(strings.TrimSpace(raw)))
	if !role.Valid() {
		return "", fmt.Errorf(
			"%w: unknown role %q (expected admin, operator, or viewer)", ErrInvalid, raw,
		)
	}
	return role, nil
}

type Organization struct {
	ID          string    `json:"id"`
	OwnerUserID string    `json:"owner_user_id"`
	Name        string    `json:"name"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func New(id, ownerUserID, name string, now time.Time) (Organization, error) {
	record := Organization{
		ID:          id,
		OwnerUserID: strings.TrimSpace(ownerUserID),
		Name:        strings.TrimSpace(name),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := record.Validate(); err != nil {
		return Organization{}, err
	}
	return record, nil
}

func (record *Organization) Rename(name string, now time.Time) error {
	record.Name = strings.TrimSpace(name)
	record.UpdatedAt = now
	return record.Validate()
}

func (record Organization) Validate() error {
	if record.OwnerUserID == "" {
		return fmt.Errorf("%w: organization requires an owner", ErrInvalid)
	}
	if length := len(record.Name); length < 1 || length > 120 {
		return fmt.Errorf("%w: name must contain 1 to 120 characters", ErrInvalid)
	}
	return nil
}

// Member is one user's standing in one organization.
type Member struct {
	OrganizationID string    `json:"organization_id"`
	UserID         string    `json:"user_id"`
	Email          string    `json:"email"`
	Role           Role      `json:"role"`
	Owner          bool      `json:"owner"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (record Member) Validate() error {
	if record.OrganizationID == "" || record.UserID == "" {
		return fmt.Errorf("%w: membership requires an organization and a user", ErrInvalid)
	}
	if !record.Role.Valid() {
		return fmt.Errorf("%w: role must be admin, operator, or viewer", ErrInvalid)
	}
	return nil
}

// Invite is an offer of membership, addressed to an email rather than a user,
// so somebody without an account can be invited before they have one.
type Invite struct {
	ID             string     `json:"id"`
	OrganizationID string     `json:"organization_id"`
	Email          string     `json:"email"`
	Role           Role       `json:"role"`
	InvitedBy      string     `json:"invited_by,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	ExpiresAt      time.Time  `json:"expires_at"`
	AcceptedAt     *time.Time `json:"accepted_at,omitempty"`
	RevokedAt      *time.Time `json:"revoked_at,omitempty"`
}

// CreatedInvite carries the one and only sight of the token. Storage keeps a
// digest, so a dump of the table cannot be redeemed.
type CreatedInvite struct {
	Invite
	Token string `json:"token"`
}

const InviteTTL = 7 * 24 * time.Hour

func NewInvite(
	id, organizationID, email string,
	role Role,
	invitedBy string,
	now time.Time,
) (Invite, error) {
	record := Invite{
		ID:             id,
		OrganizationID: strings.TrimSpace(organizationID),
		Email:          strings.ToLower(strings.TrimSpace(email)),
		Role:           role,
		InvitedBy:      strings.TrimSpace(invitedBy),
		CreatedAt:      now,
		ExpiresAt:      now.Add(InviteTTL),
	}
	if err := record.Validate(); err != nil {
		return Invite{}, err
	}
	return record, nil
}

func (record Invite) Validate() error {
	if record.OrganizationID == "" {
		return fmt.Errorf("%w: invitation requires an organization", ErrInvalid)
	}
	if record.Email == "" {
		return fmt.Errorf("%w: invitation requires an email", ErrInvalid)
	}
	if !record.Role.Valid() {
		return fmt.Errorf("%w: role must be admin, operator, or viewer", ErrInvalid)
	}
	return nil
}

func (record Invite) Pending() bool {
	return record.AcceptedAt == nil && record.RevokedAt == nil
}

// Redeemable reports why an invitation cannot be accepted, if it cannot.
func (record Invite) Redeemable(email string, now time.Time) error {
	if !record.Pending() {
		return ErrInviteSpent
	}
	if !now.Before(record.ExpiresAt) {
		return ErrInviteStale
	}
	if !strings.EqualFold(strings.TrimSpace(email), record.Email) {
		return ErrInviteAddress
	}
	return nil
}

func NewInviteToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate invitation token: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

func InviteDigest(token string) [sha256.Size]byte {
	return sha256.Sum256([]byte(strings.TrimSpace(token)))
}
