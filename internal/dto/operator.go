package dto

import (
	"time"

	"github.com/neurun-io/neurun/internal/domain/operator"
)

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// OperatorResponse is the safe projection of a session: no token, no password
// material, no API key.
type OperatorResponse struct {
	OperatorID string    `json:"operator_id"`
	Username   string    `json:"username"`
	Role       string    `json:"role"`
	Scopes     []string  `json:"scopes"`
	SessionID  string    `json:"session_id"`
	ExpiresAt  time.Time `json:"expires_at"`
}

func NewOperatorResponse(session operator.Session) OperatorResponse {
	return OperatorResponse{
		OperatorID: session.AccountID,
		Username:   session.Username,
		Role:       string(session.Role),
		Scopes:     session.Role.Scopes(),
		SessionID:  session.ID,
		ExpiresAt:  session.ExpiresAt.UTC(),
	}
}
