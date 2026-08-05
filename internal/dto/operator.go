package dto

import (
	"time"

	"github.com/neurun-io/neurun/internal/domain/operator"
)

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// RegisterRequest is how an account comes into being. Sign-up is the only path:
// there is no CLI bootstrap and the server never invents an administrator.
//
// Exactly one of OrganizationName and InviteToken is supplied: you either start
// an organization and own it, or you join one you were invited to.
type RegisterRequest struct {
	Email            string `json:"email"`
	Password         string `json:"password"`
	OrganizationName string `json:"organization_name"`
	InviteToken      string `json:"invite_token"`
}

type InviteRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

type AcceptInviteRequest struct {
	Token string `json:"token"`
}

type UpdateMemberRequest struct {
	Role string `json:"role"`
}

// OperatorResponse is the safe projection of a session: no token, no password
// material, no API key.
type OperatorResponse struct {
	OperatorID     string    `json:"operator_id"`
	Email          string    `json:"email"`
	OrganizationID string    `json:"organization_id"`
	Role           string    `json:"role"`
	Scopes         []string  `json:"scopes"`
	SessionID      string    `json:"session_id"`
	ExpiresAt      time.Time `json:"expires_at"`
}

func NewOperatorResponse(session operator.Session) OperatorResponse {
	return OperatorResponse{
		OperatorID:     session.AccountID,
		Email:          session.Email,
		OrganizationID: session.OrganizationID,
		Role:           string(session.Role),
		Scopes:         session.Role.Scopes(),
		SessionID:      session.ID,
		ExpiresAt:      session.ExpiresAt.UTC(),
	}
}
