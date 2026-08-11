package dto

import (
	"time"

	"github.com/neurun-io/neurun/internal/domain/session"
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

// SessionResponse is the safe projection of a session: no token, no password
// material, no API key.
type SessionResponse struct {
	UserID         string    `json:"user_id"`
	Email          string    `json:"email"`
	OrganizationID string    `json:"organization_id"`
	Role           string    `json:"role"`
	Scopes         []string  `json:"scopes"`
	ExpiresAt      time.Time `json:"expires_at"`
}

func NewSessionResponse(session session.Session) SessionResponse {
	return SessionResponse{
		UserID:         session.AccountID,
		Email:          session.Email,
		OrganizationID: session.OrganizationID,
		Role:           string(session.Role),
		Scopes:         session.Role.Scopes(),
		ExpiresAt:      session.ExpiresAt.UTC(),
	}
}
