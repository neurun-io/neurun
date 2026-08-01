package operator

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// ErrInvalidCredentials is returned for an unknown username, a wrong password,
// and a disabled account alike. The three are deliberately indistinguishable to
// the caller so the response cannot be used to enumerate accounts.
var ErrInvalidCredentials = errors.New("invalid username or password")

// timingDecoyHash spends the same CPU on a missing or disabled account as on a
// real one. Without it, "user not found" returns noticeably faster than "wrong
// password" and the login endpoint becomes a username oracle.
//
// Computed on first use rather than at package init, so importing this package —
// which every test binary does — does not pay bcrypt's cost up front.
var timingDecoyHash = sync.OnceValue(func() string {
	hash, err := hashPasswordWithCost("timing-decoy-never-matches", bcrypt.DefaultCost)
	if err != nil {
		// Only reachable if the platform's RNG is unavailable, in which case
		// the process has larger problems than login timing.
		return ""
	}
	return hash
})

// Authenticator performs the username/password exchange and validates sessions.
type Authenticator struct {
	Store Store
	// SessionTTL is the absolute lifetime of an issued session.
	SessionTTL time.Duration
	// Now is injectable for tests; defaults to time.Now().UTC().
	Now func() time.Time
}

func NewAuthenticator(store Store, sessionTTL time.Duration) (*Authenticator, error) {
	if store == nil {
		return nil, errors.New("operator store is required")
	}
	if sessionTTL <= 0 {
		return nil, errors.New("operator session TTL must be positive")
	}
	return &Authenticator{Store: store, SessionTTL: sessionTTL}, nil
}

func (a *Authenticator) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now().UTC()
}

// Login verifies credentials and issues a session.
//
// Returns the session and its token. The token is the only time the caller ever
// sees it: the store keeps a hash.
func (a *Authenticator) Login(
	ctx context.Context,
	username, password string,
) (Session, string, error) {
	now := a.now()

	account, err := a.Store.AccountByUsername(ctx, username)
	switch {
	case errors.Is(err, ErrAccountNotFound):
		// Spend comparable time before failing, so timing does not reveal
		// whether the username exists.
		_, _ = VerifyPassword(timingDecoyHash(), password)
		return Session{}, "", ErrInvalidCredentials
	case err != nil:
		return Session{}, "", err
	}

	if account.Disabled {
		_, _ = VerifyPassword(timingDecoyHash(), password)
		return Session{}, "", ErrInvalidCredentials
	}

	matched, err := VerifyPassword(account.PasswordHash, password)
	if err != nil {
		// A malformed stored hash is a configuration fault, not a bad password.
		return Session{}, "", fmt.Errorf("verify operator password: %w", err)
	}
	if !matched {
		return Session{}, "", ErrInvalidCredentials
	}

	token, err := NewToken()
	if err != nil {
		return Session{}, "", err
	}
	return a.issue(ctx, account, token, now)
}

func (a *Authenticator) issue(
	ctx context.Context,
	account Account,
	token string,
	now time.Time,
) (Session, string, error) {
	session, err := a.Store.CreateSession(ctx, account, token, now.Add(a.SessionTTL))
	if err != nil {
		return Session{}, "", err
	}
	return session, token, nil
}

// Session resolves a token to its live session, or ErrSessionNotFound /
// ErrSessionExpired.
func (a *Authenticator) Session(ctx context.Context, token string) (Session, error) {
	if token == "" {
		return Session{}, ErrSessionNotFound
	}
	return a.Store.SessionByToken(ctx, token, a.now())
}

// Logout revokes a session. Idempotent.
func (a *Authenticator) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return a.Store.DeleteSession(ctx, token)
}

// PruneSessions removes expired sessions, for periodic maintenance.
func (a *Authenticator) PruneSessions(ctx context.Context) (int, error) {
	return a.Store.DeleteExpiredSessions(ctx, a.now())
}
