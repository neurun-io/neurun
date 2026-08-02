// Package auth describes who is making a request and what they may do.
//
// It holds no credentials and verifies nothing: an API key is checked by the
// account service against stored digests, and a session by the operator
// service. Both converge here, so scope enforcement has one implementation.
package auth

import (
	"context"
	"strings"
)

// Kind distinguishes what sort of caller a principal represents.
//
// Both kinds carry scopes, so authorization stays in one place — but audit and
// error messages need to tell a program apart from a person.
type Kind string

const (
	// KindAPIKey is a program authenticated by a bearer API key.
	KindAPIKey Kind = "api_key"
	// KindOperator is a human authenticated by a username/password session.
	KindOperator Kind = "operator"
)

// ScopeAll grants every scope, including ones added later.
const ScopeAll = "*"

// Principal is what a request may do, and nothing about where.
//
// Scopes are the whole authorization model. A caller is not bound to a project:
// projects scope *resources*, not callers, so a key reaches as far as its
// scopes allow and a signed-in user is unrestricted.
type Principal struct {
	Kind Kind `json:"kind,omitempty"`
	// KeyID is set for KindAPIKey.
	KeyID string `json:"key_id,omitempty"`
	// OperatorID and Username are set for KindOperator.
	OperatorID string   `json:"operator_id,omitempty"`
	Username   string   `json:"username,omitempty"`
	Scopes     []string `json:"scopes"`
}

func (principal Principal) HasScope(required string) bool {
	for _, scope := range principal.Scopes {
		if scope == ScopeAll || scope == required {
			return true
		}
	}
	return false
}

type principalKey struct{}

func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, principal)
}

func FromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalKey{}).(Principal)
	return principal, ok
}

// BearerToken extracts the token from an Authorization header, reporting false
// when the header is absent or not a well-formed bearer credential.
func BearerToken(header string) (string, bool) {
	scheme, token, found := strings.Cut(strings.TrimSpace(header), " ")
	if !found || !strings.EqualFold(scheme, "Bearer") ||
		token == "" || strings.Contains(token, " ") {
		return "", false
	}
	return token, true
}
