// Package browser owns browser profiles: who a browser appears to be, and what
// it remembers between sessions.
//
// Profiles are organization-scoped rather than project-scoped, because the
// account that owns a logged-in state owns it everywhere.
package browser

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/neurun-io/neurun/internal/ids"
)

var (
	ErrInvalid  = errors.New("invalid browser profile request")
	ErrNotFound = errors.New("browser profile not found")
	ErrConflict = errors.New("browser profile conflict")
)

// Kind is the browser a profile launches. Identity.Brand is what it claims to
// be, which is a different question.
type Kind string

const (
	KindChrome  Kind = "chrome"
	KindFirefox Kind = "firefox"
)

func (kind Kind) Valid() bool {
	switch kind {
	case KindChrome, KindFirefox:
		return true
	default:
		return false
	}
}

func ParseKind(raw string) (Kind, error) {
	kind := Kind(strings.ToLower(strings.TrimSpace(raw)))
	if !kind.Valid() {
		return "", fmt.Errorf(
			"%w: unknown browser %q (expected chrome or firefox)", ErrInvalid, raw,
		)
	}
	return kind, nil
}

type Cookie struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Domain string `json:"domain"`
	Path   string `json:"path"`
	// Expires is Unix seconds. Nil for a session cookie.
	Expires  *float64 `json:"expires,omitempty"`
	Secure   bool     `json:"secure"`
	HTTPOnly bool     `json:"http_only"`
	SameSite string   `json:"same_site,omitempty"`
}

// Storage is origin to key to value.
type Storage map[string]map[string]string

// Profile is one browser persona: an optional identity, and the state it
// accumulated. A nil Identity launches the browser as itself.
type Profile struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	Name           string    `json:"name"`
	Browser        Kind      `json:"browser"`
	Identity       *Identity `json:"identity,omitempty"`
	Cookies        []Cookie  `json:"cookies"`
	LocalStorage   Storage   `json:"local_storage"`
	SessionStorage Storage   `json:"session_storage"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func New(
	id, organizationID, name string,
	kind Kind,
	identity *Identity,
	now time.Time,
) (Profile, error) {
	normalized, err := normalizeName(name)
	if err != nil {
		return Profile{}, err
	}
	record := Profile{
		ID:             id,
		OrganizationID: organizationID,
		Name:           normalized,
		Browser:        kind,
		Identity:       identity,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	record.normalize()
	if err := record.Validate(); err != nil {
		return Profile{}, err
	}
	return record, nil
}

func (record *Profile) Rename(name string, now time.Time) error {
	normalized, err := normalizeName(name)
	if err != nil {
		return err
	}
	record.Name = normalized
	record.UpdatedAt = notBefore(now, record.CreatedAt)
	return record.Validate()
}

// SetIdentity replaces the presentation half. A nil identity strips it, which
// leaves the profile's state intact.
func (record *Profile) SetIdentity(identity *Identity, now time.Time) error {
	record.Identity = identity
	record.UpdatedAt = notBefore(now, record.CreatedAt)
	return record.Validate()
}

// Capture replaces the state half with what a closing session handed back.
func (record *Profile) Capture(
	cookies []Cookie,
	local, session Storage,
	now time.Time,
) error {
	record.Cookies = cookies
	record.LocalStorage = local
	record.SessionStorage = session
	record.UpdatedAt = notBefore(now, record.CreatedAt)
	record.normalize()
	return record.Validate()
}

// normalize keeps the empty collections non-nil, because the columns are jsonb
// with array and object CHECKs and a nil slice marshals to null.
func (record *Profile) normalize() {
	if record.Cookies == nil {
		record.Cookies = []Cookie{}
	}
	if record.LocalStorage == nil {
		record.LocalStorage = Storage{}
	}
	if record.SessionStorage == nil {
		record.SessionStorage = Storage{}
	}
}

func (record Profile) Validate() error {
	if err := ValidateIdentifier("organization_id", record.OrganizationID); err != nil {
		return err
	}
	if err := ValidateIdentifier("browser_profile_id", record.ID); err != nil {
		return err
	}
	name, err := normalizeName(record.Name)
	if err != nil || name != record.Name {
		return fmt.Errorf("%w: profile name is not normalized", ErrInvalid)
	}
	if !record.Browser.Valid() {
		return fmt.Errorf("%w: profile browser is invalid", ErrInvalid)
	}
	if record.Identity != nil {
		if err := record.Identity.Validate(); err != nil {
			return err
		}
	}
	for _, cookie := range record.Cookies {
		if cookie.Name == "" || cookie.Domain == "" {
			return fmt.Errorf("%w: a cookie needs a name and a domain", ErrInvalid)
		}
	}
	for origin := range record.LocalStorage {
		if origin == "" {
			return fmt.Errorf("%w: local storage origin is empty", ErrInvalid)
		}
	}
	for origin := range record.SessionStorage {
		if origin == "" {
			return fmt.Errorf("%w: session storage origin is empty", ErrInvalid)
		}
	}
	if record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() ||
		record.UpdatedAt.Before(record.CreatedAt) {
		return fmt.Errorf("%w: profile timestamps are invalid", ErrInvalid)
	}
	return nil
}

func ValidateIdentifier(field, value string) error {
	if err := ids.Validate(field, value); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return nil
}

func normalizeName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" || len(name) > 120 || !utf8.ValidString(name) {
		return "", fmt.Errorf("%w: profile name must contain 1 to 120 bytes", ErrInvalid)
	}
	for _, character := range name {
		if unicode.IsControl(character) {
			return "", fmt.Errorf("%w: profile name contains a control character", ErrInvalid)
		}
	}
	return name, nil
}

func notBefore(value, floor time.Time) time.Time {
	if value.Before(floor) {
		return floor
	}
	return value
}
