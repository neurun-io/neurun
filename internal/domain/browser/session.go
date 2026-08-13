package browser

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrSessionNotFound is a session that is not live. There is no other kind: a
// session that ended has left the cache, and one that never existed reads the
// same way — which is deliberate, so an identifier cannot be probed.
var ErrSessionNotFound = errors.New("browser session not found")

// SessionStatus is where a session is in its life. There is no terminal state
// kept anywhere: a session that ends stops existing, because nothing reads a
// closed one.
type SessionStatus string

const (
	SessionStarting SessionStatus = "starting"
	SessionLive     SessionStatus = "live"
	SessionFailed   SessionStatus = "failed"
)

func (status SessionStatus) Valid() bool {
	switch status {
	case SessionStarting, SessionLive, SessionFailed:
		return true
	}
	return false
}

// Session is one browser a handler has open.
//
// It is live state, not a record: it lives in the cache under a TTL, so a worker
// that dies takes its sessions with it rather than leaving rows that claim to be
// running. Anything worth keeping afterwards belongs on the execution.
type Session struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	AppID          string `json:"app_id"`
	ExecutionID    string `json:"execution_id,omitempty"`
	// ProfileID names the browser profile this session wears, when it wears one.
	// A plain browser has none, and that is the ordinary case.
	ProfileID string        `json:"browser_profile_id,omitempty"`
	Browser   Kind          `json:"browser"`
	Status    SessionStatus `json:"status"`
	// DisplayAddress is the loopback address of the VNC server in front of this
	// session's Xvfb. It never leaves the server: a caller is given a session id
	// and asks the control plane to stream, which is what keeps an unauthenticated
	// port unreachable.
	DisplayAddress string    `json:"-"`
	StartedAt      time.Time `json:"started_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func NewSession(
	id, organizationID, appID string,
	browser Kind,
	now time.Time,
) (Session, error) {
	record := Session{
		ID:             strings.TrimSpace(id),
		OrganizationID: strings.TrimSpace(organizationID),
		AppID:          strings.TrimSpace(appID),
		Browser:        browser,
		Status:         SessionStarting,
		StartedAt:      now,
		UpdatedAt:      now,
	}
	if err := record.Validate(); err != nil {
		return Session{}, err
	}
	return record, nil
}

func (record Session) Validate() error {
	if record.ID == "" || record.OrganizationID == "" || record.AppID == "" {
		return fmt.Errorf(
			"%w: session requires an id, an organization and an app", ErrInvalid,
		)
	}
	if !record.Browser.Valid() {
		return fmt.Errorf("%w: unknown browser %q", ErrInvalid, record.Browser)
	}
	if !record.Status.Valid() {
		return fmt.Errorf("%w: unknown session status %q", ErrInvalid, record.Status)
	}
	if record.StartedAt.IsZero() || record.UpdatedAt.IsZero() {
		return fmt.Errorf("%w: session timestamps are invalid", ErrInvalid)
	}
	return nil
}
