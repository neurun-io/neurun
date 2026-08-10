package repository

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/neurun-io/neurun/internal/domain/organization"
	sessiondomain "github.com/neurun-io/neurun/internal/domain/session"
	"github.com/neurun-io/neurun/internal/ids"
)

const sessionKeyPrefix = "session:"

// SessionRepository keeps issued sessions in the cache, keyed by the
// token's digest — the token itself is never stored, so a dump of the cache
// cannot be replayed as a live cookie.
//
// The account behind a session is re-read from the users table on every lookup,
// so disabling or demoting a user takes effect on their next request rather
// than when their cookie happens to expire.
type SessionRepository struct {
	cache *CacheRepository
	users *UserRepository
}

func NewSessionRepository(
	cache *CacheRepository,
	users *UserRepository,
) (*SessionRepository, error) {
	if cache == nil {
		return nil, errors.New("session repository requires a cache")
	}
	if users == nil {
		return nil, errors.New("session repository requires a user repository")
	}
	return &SessionRepository{cache: cache, users: users}, nil
}

func sessionKey(token string) string {
	digest := sessiondomain.TokenDigest(token)
	return sessionKeyPrefix + hex.EncodeToString(digest[:])
}

func (repository *SessionRepository) Create(
	ctx context.Context,
	account sessiondomain.Account,
	membership organization.Member,
	token string,
	expiresAt time.Time,
) (sessiondomain.Session, error) {
	if strings.TrimSpace(token) == "" {
		return sessiondomain.Session{}, errors.New("session token is required")
	}
	sessionID, err := ids.New("oses")
	if err != nil {
		return sessiondomain.Session{}, fmt.Errorf("allocate session ID: %w", err)
	}
	session := sessiondomain.Session{
		ID:             sessionID,
		AccountID:      account.ID,
		Email:          account.Email,
		OrganizationID: membership.OrganizationID,
		Role:           membership.Role,
		CreatedAt:      time.Now().UTC(),
		ExpiresAt:      expiresAt.UTC(),
	}
	encoded, err := json.Marshal(session)
	if err != nil {
		return sessiondomain.Session{}, fmt.Errorf("encode session: %w", err)
	}
	// The entry expires with the session, so an abandoned cookie cannot outlive
	// its own deadline in the cache.
	ttl := time.Until(session.ExpiresAt)
	if ttl <= 0 {
		return sessiondomain.Session{}, errors.New("session expiry is already past")
	}
	if err := repository.cache.Set(ctx, sessionKey(token), encoded, ttl); err != nil {
		return sessiondomain.Session{}, fmt.Errorf("store session: %w", err)
	}
	return session, nil
}

func (repository *SessionRepository) ByToken(
	ctx context.Context,
	token string,
	now time.Time,
) (sessiondomain.Session, error) {
	key := sessionKey(token)
	encoded, found, err := repository.cache.Get(ctx, key)
	if err != nil {
		return sessiondomain.Session{}, fmt.Errorf("read session: %w", err)
	}
	if !found {
		return sessiondomain.Session{}, sessiondomain.ErrSessionNotFound
	}
	var session sessiondomain.Session
	if err := json.Unmarshal(encoded, &session); err != nil {
		return sessiondomain.Session{}, fmt.Errorf("decode session: %w", err)
	}
	if session.Expired(now) {
		// Drop it here so an expired token cannot be probed repeatedly.
		_ = repository.cache.Delete(ctx, key)
		return sessiondomain.Session{}, sessiondomain.ErrSessionExpired
	}
	if session.OrganizationID == "" {
		// No membership to re-read; the account itself is still the gate.
		if live, err := repository.users.Exists(ctx, session.AccountID); err != nil || !live {
			_ = repository.cache.Delete(ctx, key)
			return sessiondomain.Session{}, sessiondomain.ErrSessionNotFound
		}
		return session, nil
	}
	role, err := repository.users.LiveRole(ctx, session.AccountID, session.OrganizationID)
	if err != nil {
		// A deleted or disabled account, or one removed from the organization,
		// invalidates the session outright; the caller must not be told which.
		_ = repository.cache.Delete(ctx, key)
		return sessiondomain.Session{}, sessiondomain.ErrSessionNotFound
	}
	session.Role = role
	return session, nil
}

func (repository *SessionRepository) Delete(ctx context.Context, token string) error {
	return repository.cache.Delete(ctx, sessionKey(token))
}

// DeleteExpired reclaims the memory held by sessions the cache has already
// stopped serving, and reports how many it dropped.
func (repository *SessionRepository) DeleteExpired(
	_ context.Context,
	_ time.Time,
) (int, error) {
	return repository.cache.DeleteExpired(), nil
}

// Count reports live sessions, for tests and operational logging.
func (repository *SessionRepository) Count(ctx context.Context) (int, error) {
	keys, err := repository.cache.Keys(ctx, sessionKeyPrefix)
	if err != nil {
		return 0, err
	}
	return len(keys), nil
}
