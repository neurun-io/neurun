package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/neurun-io/neurun/internal/domain/organization"
	sessiondomain "github.com/neurun-io/neurun/internal/domain/session"
	"github.com/neurun-io/neurun/internal/repository/database"
	"github.com/neurun-io/neurun/internal/repository/memory"
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

// SessionService exchanges a username and password for a session, and resolves
// session tokens back to the person holding them.
type SessionService struct {
	users         *database.UserRepository
	organizations *database.OrganizationRepository
	sessions      *memory.SessionRepository
	sessionTTL    time.Duration
	now           func() time.Time
}

func NewSessionService(
	users *database.UserRepository,
	organizations *database.OrganizationRepository,
	sessions *memory.SessionRepository,
	sessionTTL time.Duration,
	now func() time.Time,
) (*SessionService, error) {
	if users == nil || organizations == nil || sessions == nil {
		return nil, errors.New("session service requires its repositories")
	}
	if sessionTTL <= 0 {
		return nil, errors.New("session TTL must be positive")
	}
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &SessionService{
		users: users, organizations: organizations, sessions: sessions,
		sessionTTL: sessionTTL, now: now,
	}, nil
}

// Login verifies credentials and issues a session in one organization. The
// returned token is the only time the caller sees it: storage keeps a hash.
func (service *SessionService) Login(
	ctx context.Context,
	email string,
	password string,
) (sessiondomain.Session, string, error) {
	found, err := service.authenticate(ctx, email, password)
	if err != nil {
		return sessiondomain.Session{}, "", err
	}
	membership, err := service.organizations.FirstForUser(ctx, found.ID)
	if err != nil && !errors.Is(err, organization.ErrNotMember) {
		return sessiondomain.Session{}, "", err
	}
	// Belonging nowhere is a real state, not a failure: the account signs in and
	// is asked to create an organization or accept an invitation.
	return service.issue(ctx, found, membership)
}

// StartSession issues a session for an account whose membership the caller
// already holds — the path registration and invitation acceptance take, where
// the password has just been set rather than presented.
func (service *SessionService) StartSession(
	ctx context.Context,
	userID string,
	membership organization.Member,
) (sessiondomain.Session, string, error) {
	found, err := service.users.CredentialByID(ctx, userID)
	if err != nil {
		return sessiondomain.Session{}, "", err
	}
	return service.issue(ctx, found, membership)
}

func (service *SessionService) authenticate(
	ctx context.Context,
	email string,
	password string,
) (sessiondomain.Account, error) {
	found, err := service.users.CredentialByEmail(ctx, email)
	switch {
	case errors.Is(err, sessiondomain.ErrAccountNotFound):
		// Spend comparable time before failing, so timing does not reveal
		// whether the address exists.
		_, _ = sessiondomain.VerifyPassword(timingDecoyHash(), password)
		return sessiondomain.Account{}, ErrInvalidCredentials
	case err != nil:
		return sessiondomain.Account{}, err
	}

	matched, err := found.Authenticate(password)
	if err != nil {
		// A malformed stored hash is a configuration fault, not a bad password.
		return sessiondomain.Account{}, fmt.Errorf("verify password: %w", err)
	}
	if !matched {
		if found.Disabled {
			_, _ = sessiondomain.VerifyPassword(timingDecoyHash(), password)
		}
		return sessiondomain.Account{}, ErrInvalidCredentials
	}
	return found, nil
}

func (service *SessionService) issue(
	ctx context.Context,
	found sessiondomain.Account,
	membership organization.Member,
) (sessiondomain.Session, string, error) {
	token, err := sessiondomain.NewToken()
	if err != nil {
		return sessiondomain.Session{}, "", err
	}
	session, err := service.sessions.Create(
		ctx, found, membership, token, service.now().Add(service.sessionTTL),
	)
	if err != nil {
		return sessiondomain.Session{}, "", err
	}
	return session, token, nil
}

// Session resolves a token to its live session, or ErrSessionNotFound /
// ErrSessionExpired.
func (service *SessionService) Session(
	ctx context.Context,
	token string,
) (sessiondomain.Session, error) {
	if token == "" {
		return sessiondomain.Session{}, sessiondomain.ErrSessionNotFound
	}
	return service.sessions.ByToken(ctx, token, service.now())
}

// Logout revokes a session. Idempotent.
func (service *SessionService) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return service.sessions.Delete(ctx, token)
}

// PruneSessions drops expired sessions, for periodic maintenance.
func (service *SessionService) PruneSessions(ctx context.Context) (int, error) {
	return service.sessions.DeleteExpired(ctx, service.now())
}
