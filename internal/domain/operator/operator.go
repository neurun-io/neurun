// Package operator provides human identity for the control plane.
//
// The API key in internal/domain/auth authenticates a *program* against a
// project. This package describes a *person*: an account whose username and
// password are exchanged for an opaque session token delivered as an HttpOnly
// cookie. Both paths converge on auth.Principal, so scope enforcement stays in
// one place.
package operator

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
	ErrAccountNotFound = errors.New("operator account not found")
	ErrAccountDisabled = errors.New("operator account is disabled")
	ErrSessionNotFound = errors.New("operator session not found")
	ErrSessionExpired  = errors.New("operator session has expired")
)

// Role determines which API scopes a session carries.
type Role string

const (
	// RoleAdmin carries every scope, including future ones.
	RoleAdmin Role = "admin"
	// RoleOperator can read evidence and submit or cancel work.
	RoleOperator Role = "operator"
	// RoleViewer can read evidence and nothing else.
	RoleViewer Role = "viewer"
)

// Scopes returns the API scopes granted by the role.
func (role Role) Scopes() []string {
	switch role {
	case RoleAdmin:
		return []string{"*"}
	case RoleOperator:
		return []string{
			"projects:read", "apps:read", "apps:write",
			"deployments:read", "deployments:write",
			"builds:read", "executions:read", "executions:write",
		}
	case RoleViewer:
		return []string{
			"projects:read", "apps:read", "deployments:read", "builds:read",
			"executions:read", "users:read", "api_keys:read",
		}
	default:
		return nil
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
			"unknown operator role %q (expected admin, operator, or viewer)", raw,
		)
	}
	return role, nil
}

// Account is a human login. The plaintext password is never stored or held.
type Account struct {
	ID           string
	Username     string
	Role         Role
	PasswordHash string
	Disabled     bool
	CreatedAt    time.Time
}

// Authenticate reports whether password signs this account in.
//
// A disabled account and a wrong password are both a plain false: the caller
// must not be able to tell them apart. A malformed stored hash is an error
// instead, because that is a configuration fault rather than a failed login.
func (account Account) Authenticate(password string) (bool, error) {
	if account.Disabled {
		return false, nil
	}
	return VerifyPassword(account.PasswordHash, password)
}

// Session is an issued login. The token itself is not stored: only its SHA-256
// hash, so a dump of the session store cannot be replayed as a live cookie.
type Session struct {
	ID        string    `json:"id"`
	AccountID string    `json:"account_id"`
	Username  string    `json:"username"`
	Role      Role      `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (session Session) Expired(now time.Time) bool {
	return !now.Before(session.ExpiresAt)
}

// NewToken returns a fresh opaque session token: 32 bytes of entropy, hex
// encoded. It is a bearer secret in cookie form, so it is generated here and
// never derived from anything guessable.
func NewToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

// TokenDigest is the stored form of a session token.
func TokenDigest(token string) [sha256.Size]byte {
	return sha256.Sum256([]byte(token))
}
