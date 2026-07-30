// Package operator provides human authentication for the control plane.
//
// The API key in internal/auth authenticates a *program* against a project. This
// package authenticates a *person* against the dashboard: a username and
// password are exchanged for an opaque session token delivered as an HttpOnly
// cookie. Both paths converge on auth.Principal, so scope enforcement stays in
// one place.
//
// Storage follows the same pattern as jobs and invocations in this foundation: a
// port with a process-local adapter. Operator sessions therefore do not survive
// a restart, and accounts come from configuration rather than a database. The
// operators and operator_sessions tables in migrations/ describe the durable
// shape for when the PostgreSQL adapter lands.
package operator

import (
	"context"
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
//
// `viewer` deliberately omits functions:invoke and jobs:write, so a read-only
// operator cannot start or cancel work even though the UI is the same build.
func (r Role) Scopes() []string {
	switch r {
	case RoleAdmin:
		return []string{"*"}
	case RoleOperator:
		return []string{"functions:read", "functions:invoke", "jobs:read", "jobs:write"}
	case RoleViewer:
		return []string{"functions:read", "jobs:read"}
	default:
		return nil
	}
}

func (r Role) Valid() bool {
	switch r {
	case RoleAdmin, RoleOperator, RoleViewer:
		return true
	default:
		return false
	}
}

func ParseRole(raw string) (Role, error) {
	role := Role(strings.ToLower(strings.TrimSpace(raw)))
	if !role.Valid() {
		return "", fmt.Errorf("unknown operator role %q (expected admin, operator, or viewer)", raw)
	}
	return role, nil
}

// Account is a human login. The plaintext password is never stored or held.
type Account struct {
	ID       string
	Username string
	Role     Role
	// ProjectID the session acts within. The current foundation is
	// single-project; per-operator project scoping arrives with the project API.
	ProjectID string
	// Encoded PBKDF2 hash — see password.go.
	PasswordHash string
	Disabled     bool
	CreatedAt    time.Time
}

// Session is an issued login. The token itself is not stored: only its SHA-256
// hash, so a dump of the session store cannot be replayed as a live cookie.
type Session struct {
	ID        string
	AccountID string
	Username  string
	Role      Role
	ProjectID string
	CreatedAt time.Time
	ExpiresAt time.Time
}

func (s Session) Expired(now time.Time) bool {
	return !now.Before(s.ExpiresAt)
}

// Store persists operator accounts and their sessions.
type Store interface {
	// AccountByUsername returns ErrAccountNotFound when no such account exists.
	AccountByUsername(ctx context.Context, username string) (Account, error)
	// CreateSession issues a session for account, keyed by token.
	CreateSession(ctx context.Context, account Account, token string, expiresAt time.Time) (Session, error)
	// SessionByToken returns ErrSessionNotFound or ErrSessionExpired as
	// appropriate. An expired session is removed as a side effect.
	SessionByToken(ctx context.Context, token string, now time.Time) (Session, error)
	// DeleteSession is idempotent: deleting an unknown token is not an error.
	DeleteSession(ctx context.Context, token string) error
	// DeleteExpiredSessions prunes the store and reports how many it removed.
	DeleteExpiredSessions(ctx context.Context, now time.Time) (int, error)
}

// NewToken returns a fresh opaque session token.
//
// 32 bytes of entropy, hex-encoded. It is a bearer secret in cookie form, so it
// is generated here and never derived from anything guessable.
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
