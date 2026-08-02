package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/neurun-io/neurun/internal/domain/operator"
	"github.com/neurun-io/neurun/internal/repository"
)

// ErrInvalidCredentials is returned for an unknown username, a wrong password,
// and a disabled account alike. The three are deliberately indistinguishable so
// the response cannot be used to enumerate accounts.
var ErrInvalidCredentials = errors.New("invalid username or password")

// timingDecoyHash spends the same CPU on a missing or disabled account as on a
// real one. Without it, "user not found" returns noticeably faster than "wrong
// password" and the login endpoint becomes a username oracle.
//
// Computed on first use rather than at package init, so importing this package
// does not pay bcrypt's cost up front.
var timingDecoyHash = sync.OnceValue(func() string {
	hash, err := bcrypt.GenerateFromPassword(
		[]byte("timing-decoy-never-matches"), bcrypt.DefaultCost,
	)
	if err != nil {
		// Only reachable if the platform RNG is unavailable, in which case the
		// process has larger problems than login timing.
		return ""
	}
	return string(hash)
})

// OperatorService exchanges a username and password for a session, and resolves
// session tokens back to the person holding them.
type OperatorService struct {
	users      *repository.UserRepository
	sessions   *repository.SessionRepository
	sessionTTL time.Duration
	now        func() time.Time
}

func NewOperatorService(
	users *repository.UserRepository,
	sessions *repository.SessionRepository,
	sessionTTL time.Duration,
	now func() time.Time,
) (*OperatorService, error) {
	if users == nil || sessions == nil {
		return nil, errors.New("operator service requires its repositories")
	}
	if sessionTTL <= 0 {
		return nil, errors.New("operator session TTL must be positive")
	}
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &OperatorService{
		users: users, sessions: sessions, sessionTTL: sessionTTL, now: now,
	}, nil
}

// Login verifies credentials and issues a session. The returned token is the
// only time the caller sees it: storage keeps a hash.
func (service *OperatorService) Login(
	ctx context.Context,
	username string,
	password string,
) (operator.Session, string, error) {
	now := service.now()

	found, err := service.users.CredentialByUsername(ctx, username)
	switch {
	case errors.Is(err, operator.ErrAccountNotFound):
		// Spend comparable time before failing, so timing does not reveal
		// whether the username exists.
		_, _ = operator.VerifyPassword(timingDecoyHash(), password)
		return operator.Session{}, "", ErrInvalidCredentials
	case err != nil:
		return operator.Session{}, "", err
	}

	matched, err := found.Authenticate(password)
	if err != nil {
		// A malformed stored hash is a configuration fault, not a bad password.
		return operator.Session{}, "", fmt.Errorf("verify operator password: %w", err)
	}
	if !matched {
		if found.Disabled {
			_, _ = operator.VerifyPassword(timingDecoyHash(), password)
		}
		return operator.Session{}, "", ErrInvalidCredentials
	}

	token, err := operator.NewToken()
	if err != nil {
		return operator.Session{}, "", err
	}
	session, err := service.sessions.Create(
		ctx, found, token, now.Add(service.sessionTTL),
	)
	if err != nil {
		return operator.Session{}, "", err
	}
	return session, token, nil
}

// Session resolves a token to its live session, or ErrSessionNotFound /
// ErrSessionExpired.
func (service *OperatorService) Session(
	ctx context.Context,
	token string,
) (operator.Session, error) {
	if token == "" {
		return operator.Session{}, operator.ErrSessionNotFound
	}
	return service.sessions.ByToken(ctx, token, service.now())
}

// Logout revokes a session. Idempotent.
func (service *OperatorService) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return service.sessions.Delete(ctx, token)
}

// PruneSessions drops expired sessions, for periodic maintenance.
func (service *OperatorService) PruneSessions(ctx context.Context) (int, error) {
	return service.sessions.DeleteExpired(ctx, service.now())
}
