// Package account owns the humans and programs that may call the control
// plane: named users and the API keys issued against a project.
package account

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	ErrInvalid  = errors.New("invalid account request")
	ErrNotFound = errors.New("account resource not found")
	ErrConflict = errors.New("account resource conflict")
)

// Scopes an API key may carry. "*" grants everything, including scopes added
// after the key was issued.
var knownScopes = []string{
	"*", "users:read", "users:write", "api_keys:read", "api_keys:write",
	"projects:read", "projects:write", "apps:read", "apps:write",
	"deployments:read", "deployments:write",
	"executions:read", "executions:write",
}

// User is a global identity. What it may do lives in an organization
// membership, not here.
type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Disabled  bool      `json:"disabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func NewUser(id, email string, now time.Time) (User, error) {
	record := User{
		ID:        id,
		Email:     NormalizeEmail(email),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := record.Validate(); err != nil {
		return User{}, err
	}
	return record, nil
}

// Apply folds a partial update into the user, leaving absent fields alone.
func (record *User) Apply(email *string, disabled *bool, now time.Time) error {
	if email == nil && disabled == nil {
		return fmt.Errorf("%w: user update is empty", ErrInvalid)
	}
	if email != nil {
		record.Email = NormalizeEmail(*email)
	}
	if disabled != nil {
		record.Disabled = *disabled
	}
	record.UpdatedAt = now
	return record.Validate()
}

func (record User) Validate() error {
	if !ValidEmail(record.Email) {
		return fmt.Errorf("%w: email is invalid", ErrInvalid)
	}
	return nil
}

func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// ValidEmail is deliberately shallow: one @, something either side, no spaces
// or control characters. Deliverability is proved by sending mail, not by a
// regular expression.
func ValidEmail(email string) bool {
	if len(email) < 3 || len(email) > 320 || !utf8.ValidString(email) {
		return false
	}
	local, domain, found := strings.Cut(email, "@")
	if !found || local == "" || domain == "" || strings.Contains(domain, "@") {
		return false
	}
	if !strings.Contains(domain, ".") || strings.HasPrefix(domain, ".") ||
		strings.HasSuffix(domain, ".") {
		return false
	}
	for _, character := range email {
		if unicode.IsSpace(character) || unicode.IsControl(character) {
			return false
		}
	}
	return true
}

type Key struct {
	ID             string     `json:"id"`
	OrganizationID string     `json:"organization_id"`
	UserID         string     `json:"user_id,omitempty"`
	Name           string     `json:"name"`
	Prefix         string     `json:"prefix"`
	Scopes         []string   `json:"scopes"`
	CreatedAt      time.Time  `json:"created_at"`
	RevokedAt      *time.Time `json:"revoked_at,omitempty"`
}

// CreatedKey carries the one and only sight of a key's secret. Nothing stores
// it; only its digest is kept.
type CreatedKey struct {
	Key
	Secret string `json:"secret"`
}

func NewKey(
	id, organizationID, userID, name string,
	scopes []string,
	now time.Time,
) (Key, error) {
	record := Key{
		ID:             id,
		OrganizationID: strings.TrimSpace(organizationID),
		UserID:         strings.TrimSpace(userID),
		Name:           strings.TrimSpace(name),
		Scopes:         NormalizeScopes(scopes),
		CreatedAt:      now,
	}
	if err := record.Validate(); err != nil {
		return Key{}, err
	}
	return record, nil
}

func (record Key) Validate() error {
	if record.OrganizationID == "" {
		return fmt.Errorf("%w: key requires an organization", ErrInvalid)
	}
	if record.Name == "" || len(record.Name) > 128 {
		return fmt.Errorf("%w: key name must contain 1 to 128 bytes", ErrInvalid)
	}
	if len(record.Scopes) > 32 {
		return fmt.Errorf("%w: a key may carry at most 32 scopes", ErrInvalid)
	}
	for _, scope := range record.Scopes {
		if !slices.Contains(knownScopes, scope) {
			return fmt.Errorf("%w: unknown scope %q", ErrInvalid, scope)
		}
	}
	return nil
}

// NormalizeScopes trims, deduplicates and sorts, so two keys granted the same
// permissions in a different order compare and store identically.
func NormalizeScopes(scopes []string) []string {
	normalized := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if !slices.Contains(normalized, scope) {
			normalized = append(normalized, scope)
		}
	}
	slices.Sort(normalized)
	return normalized
}
