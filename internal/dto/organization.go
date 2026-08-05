package dto

import (
	"time"

	"github.com/neurun-io/neurun/internal/domain/organization"
)

type OrganizationResponse struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	OwnerUserID string    `json:"owner_user_id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type MemberResponse struct {
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	Owner     bool      `json:"owner"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type InviteResponse struct {
	ID         string     `json:"id"`
	Email      string     `json:"email"`
	Role       string     `json:"role"`
	InvitedBy  string     `json:"invited_by,omitempty"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	AcceptedAt *time.Time `json:"accepted_at,omitempty"`
}

// CreatedInviteResponse carries the one and only sight of the token.
type CreatedInviteResponse struct {
	InviteResponse
	Token string `json:"token"`
}

func NewOrganizationResponse(record organization.Organization) OrganizationResponse {
	return OrganizationResponse{
		ID: record.ID, Name: record.Name, OwnerUserID: record.OwnerUserID,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

func NewOrganizationResponses(
	records []organization.Organization,
) []OrganizationResponse {
	responses := make([]OrganizationResponse, len(records))
	for index, record := range records {
		responses[index] = NewOrganizationResponse(record)
	}
	return responses
}

func NewMemberResponse(record organization.Member) MemberResponse {
	return MemberResponse{
		UserID: record.UserID, Email: record.Email,
		Role: string(record.Role), Owner: record.Owner,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

func NewMemberResponses(records []organization.Member) []MemberResponse {
	responses := make([]MemberResponse, len(records))
	for index, record := range records {
		responses[index] = NewMemberResponse(record)
	}
	return responses
}

func NewInviteResponse(record organization.Invite) InviteResponse {
	return InviteResponse{
		ID: record.ID, Email: record.Email, Role: string(record.Role),
		InvitedBy: record.InvitedBy, Status: inviteStatus(record),
		CreatedAt: record.CreatedAt, ExpiresAt: record.ExpiresAt,
		AcceptedAt: record.AcceptedAt,
	}
}

func NewInviteResponses(records []organization.Invite) []InviteResponse {
	responses := make([]InviteResponse, len(records))
	for index, record := range records {
		responses[index] = NewInviteResponse(record)
	}
	return responses
}

func inviteStatus(record organization.Invite) string {
	switch {
	case record.AcceptedAt != nil:
		return "accepted"
	case record.RevokedAt != nil:
		return "revoked"
	case !time.Now().UTC().Before(record.ExpiresAt):
		return "expired"
	default:
		return "pending"
	}
}
