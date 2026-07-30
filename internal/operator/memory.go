package operator

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/dagflows/neurun-io/internal/ids"
)

// MemoryStore is the process-local Store adapter.
//
// Accounts are fixed at construction from configuration. Sessions live only in
// this process, so a restart logs every operator out — the honest consequence of
// having no durable adapter yet, and a mild security benefit in development.
type MemoryStore struct {
	mu       sync.RWMutex
	accounts map[string]Account            // lowercased username → account
	sessions map[[sha256.Size]byte]Session // token digest → session
}

var _ Store = (*MemoryStore)(nil)

// NewMemoryStore builds a store from the supplied accounts.
//
// Usernames are compared case-insensitively, so two accounts differing only in
// case are rejected rather than silently shadowing one another.
func NewMemoryStore(accounts ...Account) (*MemoryStore, error) {
	store := &MemoryStore{
		accounts: make(map[string]Account, len(accounts)),
		sessions: make(map[[sha256.Size]byte]Session),
	}

	for _, account := range accounts {
		username := normalizeUsername(account.Username)
		if username == "" {
			return nil, errors.New("operator username is required")
		}
		if !account.Role.Valid() {
			return nil, fmt.Errorf("operator %q has an invalid role %q", account.Username, account.Role)
		}
		if err := ValidateHash(account.PasswordHash); err != nil {
			return nil, fmt.Errorf("operator %q: %w", account.Username, err)
		}
		if _, exists := store.accounts[username]; exists {
			return nil, fmt.Errorf("operator %q is defined more than once", account.Username)
		}
		if strings.TrimSpace(account.ID) == "" {
			generated, err := ids.New("opr")
			if err != nil {
				return nil, fmt.Errorf("allocate operator ID: %w", err)
			}
			account.ID = generated
		}
		if account.CreatedAt.IsZero() {
			account.CreatedAt = time.Now().UTC()
		}
		account.Username = username
		store.accounts[username] = account
	}

	return store, nil
}

// Accounts returns every configured account, for startup reporting. Password
// hashes are cleared: nothing outside this package needs them.
func (s *MemoryStore) Accounts() []Account {
	s.mu.RLock()
	defer s.mu.RUnlock()

	accounts := make([]Account, 0, len(s.accounts))
	for _, account := range s.accounts {
		account.PasswordHash = ""
		accounts = append(accounts, account)
	}
	return accounts
}

func (s *MemoryStore) AccountByUsername(_ context.Context, username string) (Account, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	account, ok := s.accounts[normalizeUsername(username)]
	if !ok {
		return Account{}, ErrAccountNotFound
	}
	return account, nil
}

func (s *MemoryStore) CreateSession(
	_ context.Context,
	account Account,
	token string,
	expiresAt time.Time,
) (Session, error) {
	if strings.TrimSpace(token) == "" {
		return Session{}, errors.New("session token is required")
	}
	sessionID, err := ids.New("oses")
	if err != nil {
		return Session{}, fmt.Errorf("allocate operator session ID: %w", err)
	}

	session := Session{
		ID:        sessionID,
		AccountID: account.ID,
		Username:  account.Username,
		Role:      account.Role,
		ProjectID: account.ProjectID,
		CreatedAt: time.Now().UTC(),
		ExpiresAt: expiresAt.UTC(),
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[TokenDigest(token)] = session
	return session, nil
}

func (s *MemoryStore) SessionByToken(
	_ context.Context,
	token string,
	now time.Time,
) (Session, error) {
	digest := TokenDigest(token)

	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[digest]
	if !ok {
		return Session{}, ErrSessionNotFound
	}
	if session.Expired(now) {
		// Drop it here so an expired token cannot be probed repeatedly.
		delete(s.sessions, digest)
		return Session{}, ErrSessionExpired
	}
	return session, nil
}

func (s *MemoryStore) DeleteSession(_ context.Context, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, TokenDigest(token))
	return nil
}

func (s *MemoryStore) DeleteExpiredSessions(_ context.Context, now time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	removed := 0
	for digest, session := range s.sessions {
		if session.Expired(now) {
			delete(s.sessions, digest)
			removed++
		}
	}
	return removed, nil
}

// SessionCount reports live sessions, for tests and operational logging.
func (s *MemoryStore) SessionCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.sessions)
}

func normalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}
