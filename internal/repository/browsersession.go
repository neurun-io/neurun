package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/neurun-io/neurun/internal/domain/browser"
)

// sessionTTL is how long a session survives without being touched. It is a dead
// man's switch: a worker that crashes stops refreshing, and its sessions leave
// the list on their own rather than needing a recovery sweep.
const sessionTTL = 5 * time.Minute

const browserSessionPrefix = "browser-session"

// BrowserSessionRepository keeps live sessions in the cache.
//
// There is no table on purpose. A session is worthless once it ends, high churn
// while it runs, and must be visible to whichever replica answers the dashboard
// — which is the same shape as an issued session token, and the same store.
type BrowserSessionRepository struct {
	cache Cache
	ttl   time.Duration
}

func NewBrowserSessionRepository(cache Cache) (*BrowserSessionRepository, error) {
	if cache == nil {
		return nil, errors.New("browser session repository requires a cache")
	}
	return &BrowserSessionRepository{cache: cache, ttl: sessionTTL}, nil
}

// key namespaces by organization so a list is one scan and a read cannot cross
// a tenant boundary even when an identifier is guessed.
func browserSessionKey(organizationID, sessionID string) string {
	return browserSessionPrefix + ":" + organizationID + ":" + sessionID
}

// Save writes the session and refreshes its lease.
func (repository *BrowserSessionRepository) Save(
	ctx context.Context,
	record browser.Session,
) error {
	if err := record.Validate(); err != nil {
		return err
	}
	encoded, err := json.Marshal(storedSession{
		Session:        record,
		DisplayAddress: record.DisplayAddress,
	})
	if err != nil {
		return fmt.Errorf("encode browser session: %w", err)
	}
	return repository.cache.Set(
		ctx,
		browserSessionKey(record.OrganizationID, record.ID),
		encoded,
		repository.ttl,
	)
}

func (repository *BrowserSessionRepository) Get(
	ctx context.Context,
	organizationID string,
	sessionID string,
) (browser.Session, error) {
	encoded, found, err := repository.cache.Get(
		ctx, browserSessionKey(organizationID, sessionID),
	)
	if err != nil {
		return browser.Session{}, fmt.Errorf("read browser session: %w", err)
	}
	if !found {
		return browser.Session{}, browser.ErrSessionNotFound
	}
	return decodeSession(encoded)
}

// List returns the organization's live sessions, newest first.
func (repository *BrowserSessionRepository) List(
	ctx context.Context,
	organizationID string,
) ([]browser.Session, error) {
	keys, err := repository.cache.Keys(
		ctx, browserSessionPrefix+":"+organizationID+":",
	)
	if err != nil {
		return nil, fmt.Errorf("list browser sessions: %w", err)
	}
	records := make([]browser.Session, 0, len(keys))
	for _, key := range keys {
		// Keys come back namespaced by the cache itself, and Get namespaces
		// again, so the identifier is taken from the tail rather than the whole.
		index := strings.LastIndex(key, ":")
		if index < 0 {
			continue
		}
		record, err := repository.Get(ctx, organizationID, key[index+1:])
		if errors.Is(err, browser.ErrSessionNotFound) {
			// Expired between the scan and the read, which SCAN makes no promise
			// against. It is simply no longer live.
			continue
		}
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	sortSessionsNewestFirst(records)
	return records, nil
}

func (repository *BrowserSessionRepository) Delete(
	ctx context.Context,
	organizationID string,
	sessionID string,
) error {
	return repository.cache.Delete(ctx, browserSessionKey(organizationID, sessionID))
}

// storedSession carries the fields the JSON view deliberately drops, so the
// address survives a round trip through the cache without ever being encoded
// into a response.
type storedSession struct {
	browser.Session
	DisplayAddress string `json:"display_address,omitempty"`
}

func decodeSession(encoded []byte) (browser.Session, error) {
	var stored storedSession
	if err := json.Unmarshal(encoded, &stored); err != nil {
		return browser.Session{}, fmt.Errorf("decode browser session: %w", err)
	}
	record := stored.Session
	record.DisplayAddress = stored.DisplayAddress
	return record, nil
}

func sortSessionsNewestFirst(records []browser.Session) {
	for outer := 1; outer < len(records); outer++ {
		current := records[outer]
		inner := outer - 1
		for inner >= 0 && records[inner].StartedAt.Before(current.StartedAt) {
			records[inner+1] = records[inner]
			inner--
		}
		records[inner+1] = current
	}
}
