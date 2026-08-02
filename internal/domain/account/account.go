// Package account owns the humans and programs that may call the control
// plane: named users and the API keys issued against a project.
package account

import (
	"errors"
	"fmt"
	"regexp"
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

var usernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

// Scopes an API key may carry. "*" grants everything, including scopes added
// after the key was issued.
var knownScopes = []string{
	"*", "users:read", "users:write", "api_keys:read", "api_keys:write",
	"projects:read", "projects:write", "apps:read", "apps:write",
	"deployments:read", "deployments:write",
	"builds:read", "executions:read", "executions:write",
}

type User struct {
	ID          string    `json:"id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	Role        string    `json:"role"`
	Disabled    bool      `json:"disabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func NewUser(
	id, username, displayName, role string,
	now time.Time,
) (User, error) {
	record := User{
		ID:          id,
		Username:    strings.ToLower(strings.TrimSpace(username)),
		DisplayName: strings.TrimSpace(displayName),
		Role:        strings.TrimSpace(role),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := record.Validate(); err != nil {
		return User{}, err
	}
	return record, nil
}

// Apply folds a partial update into the user, leaving absent fields alone.
func (record *User) Apply(
	displayName *string,
	role *string,
	disabled *bool,
	now time.Time,
) error {
	if displayName == nil && role == nil && disabled == nil {
		return fmt.Errorf("%w: user update is empty", ErrInvalid)
	}
	if displayName != nil {
		record.DisplayName = strings.TrimSpace(*displayName)
	}
	if role != nil {
		record.Role = strings.TrimSpace(*role)
	}
	if disabled != nil {
		record.Disabled = *disabled
	}
	record.UpdatedAt = now
	return record.Validate()
}

func (record User) Validate() error {
	if !usernamePattern.MatchString(record.Username) {
		return fmt.Errorf("%w: username is invalid", ErrInvalid)
	}
	if !validDisplayName(record.DisplayName) {
		return fmt.Errorf("%w: display name is invalid", ErrInvalid)
	}
	if !ValidRole(record.Role) {
		return fmt.Errorf("%w: role must be admin, operator, or viewer", ErrInvalid)
	}
	return nil
}

type Key struct {
	ID        string     `json:"id"`
	UserID    string     `json:"user_id,omitempty"`
	Name      string     `json:"name"`
	Prefix    string     `json:"prefix"`
	Scopes    []string   `json:"scopes"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

// CreatedKey carries the one and only sight of a key's secret. Nothing stores
// it; only its digest is kept.
type CreatedKey struct {
	Key
	Secret string `json:"secret"`
}

func NewKey(
	id, userID, name string,
	scopes []string,
	now time.Time,
) (Key, error) {
	record := Key{
		ID:        id,
		UserID:    strings.TrimSpace(userID),
		Name:      strings.TrimSpace(name),
		Scopes:    NormalizeScopes(scopes),
		CreatedAt: now,
	}
	if err := record.Validate(); err != nil {
		return Key{}, err
	}
	return record, nil
}

func (record Key) Revoked() bool {
	return record.RevokedAt != nil
}

func (record Key) Validate() error {
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

func ValidRole(role string) bool {
	return role == "admin" || role == "operator" || role == "viewer"
}

func validDisplayName(name string) bool {
	if name == "" || len(name) > 128 || !utf8.ValidString(name) {
		return false
	}
	for _, character := range name {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
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
