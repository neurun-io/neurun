// Package operator provides human identity for the control plane.
//
// The API key in internal/domain/auth authenticates a *program*. This package
// describes a *person*: an account whose email and password are exchanged for
// an opaque session token delivered as an HttpOnly cookie. Both paths converge
// on auth.Principal, so scope enforcement stays in one place.
package operator

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/neurun-io/neurun/internal/domain/organization"
)

var (
	ErrAccountNotFound = errors.New("operator account not found")
	ErrAccountDisabled = errors.New("operator account is disabled")
	ErrSessionNotFound = errors.New("operator session not found")
	ErrSessionExpired  = errors.New("operator session has expired")
	// ErrNoOrganization is a real account with nowhere to act: every
	// membership it held was removed. It signs in and can do nothing.
	ErrNoOrganization = errors.New("account belongs to no organization")
)

// Account is a human login. The plaintext password is never stored or held.
type Account struct {
	ID           string
	Email        string
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
//
// A session is scoped to one organization. Role is the member role held there,
// re-read on every request, so removing somebody takes effect on their next
// call rather than when their cookie happens to expire.
type Session struct {
	ID             string            `json:"id"`
	AccountID      string            `json:"account_id"`
	Email          string            `json:"email"`
	OrganizationID string            `json:"organization_id"`
	Role           organization.Role `json:"role"`
	CreatedAt      time.Time         `json:"created_at"`
	ExpiresAt      time.Time         `json:"expires_at"`
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
