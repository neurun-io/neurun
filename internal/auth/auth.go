package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// Kind distinguishes what sort of caller a principal represents.
//
// Both kinds carry scopes and a project, so authorization stays in one place —
// but audit and error messages need to tell a program apart from a person.
type Kind string

const (
	// KindAPIKey is a program authenticated by a bearer API key.
	KindAPIKey Kind = "api_key"
	// KindOperator is a human authenticated by a username/password session.
	KindOperator Kind = "operator"
)

type Principal struct {
	Kind Kind `json:"kind,omitempty"`
	// KeyID is set for KindAPIKey.
	KeyID string `json:"key_id,omitempty"`
	// OperatorID and Username are set for KindOperator.
	OperatorID string   `json:"operator_id,omitempty"`
	Username   string   `json:"username,omitempty"`
	ProjectID  string   `json:"project_id"`
	Scopes     []string `json:"scopes"`
}

func (p Principal) HasScope(required string) bool {
	for _, scope := range p.Scopes {
		if scope == "*" || scope == required {
			return true
		}
	}
	return false
}

type Credential struct {
	ID        string
	ProjectID string
	RawKey    string
	Scopes    []string
}

type hashedCredential struct {
	principal Principal
	hash      [sha256.Size]byte
}

type Authenticator struct {
	credentials []hashedCredential
}

func New(credentials ...Credential) (*Authenticator, error) {
	if len(credentials) == 0 {
		return nil, errors.New("at least one API credential is required")
	}

	hashed := make([]hashedCredential, 0, len(credentials))
	for _, credential := range credentials {
		if strings.TrimSpace(credential.ID) == "" || strings.TrimSpace(credential.ProjectID) == "" {
			return nil, errors.New("API credential ID and project ID are required")
		}
		if err := validateRawKey(credential.RawKey); err != nil {
			return nil, err
		}
		scopes := append([]string(nil), credential.Scopes...)
		hashed = append(hashed, hashedCredential{
			principal: Principal{
				Kind:      KindAPIKey,
				KeyID:     credential.ID,
				ProjectID: credential.ProjectID,
				Scopes:    scopes,
			},
			hash: sha256.Sum256([]byte(credential.RawKey)),
		})
	}
	return &Authenticator{credentials: hashed}, nil
}

func (a *Authenticator) Authenticate(raw string) (Principal, bool) {
	candidate := sha256.Sum256([]byte(raw))
	var principal Principal
	matched := 0
	for _, credential := range a.credentials {
		equal := subtle.ConstantTimeCompare(candidate[:], credential.hash[:])
		if equal == 1 {
			principal = credential.principal
		}
		matched |= equal
	}
	if matched != 1 {
		return Principal{}, false
	}
	principal.Scopes = append([]string(nil), principal.Scopes...)
	return principal, true
}

func (a *Authenticator) AuthenticateContext(_ context.Context, raw string) (Principal, bool) {
	return a.Authenticate(raw)
}

func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		raw, ok := BearerToken(request.Header.Get("Authorization"))
		if !ok {
			writeUnauthorized(w)
			return
		}
		principal, ok := a.Authenticate(raw)
		if !ok {
			writeUnauthorized(w)
			return
		}
		next.ServeHTTP(w, request.WithContext(WithPrincipal(request.Context(), principal)))
	})
}

func RequireScope(scope string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		principal, ok := FromContext(request.Context())
		if !ok || !principal.HasScope(scope) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{
					"code":    "forbidden",
					"message": "the API key does not grant the required scope",
				},
			})
			return
		}
		next.ServeHTTP(w, request)
	})
}

type principalKey struct{}

func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, principal)
}

func FromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalKey{}).(Principal)
	return principal, ok
}

// BearerToken extracts the token from an Authorization header value, reporting
// false when the header is absent or not a well-formed bearer credential.
//
// Exported so callers that must choose between credential kinds before
// delegating — the control API accepts either an API key or a session cookie —
// share this parsing rather than reimplementing it.
func BearerToken(header string) (string, bool) {
	scheme, token, found := strings.Cut(strings.TrimSpace(header), " ")
	if !found || !strings.EqualFold(scheme, "Bearer") || token == "" || strings.Contains(token, " ") {
		return "", false
	}
	return token, true
}

func validateRawKey(raw string) error {
	if !strings.HasPrefix(raw, "neu_") {
		return errors.New("API key must begin with neu_")
	}
	prefix, secret, found := strings.Cut(raw, ".")
	if !found || len(prefix) < len("neu_x") || secret == "" {
		return errors.New("API key must contain a non-empty prefix and secret separated by a dot")
	}
	return nil
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("WWW-Authenticate", `Bearer realm="neurun"`)
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":    "unauthorized",
			"message": "a valid bearer API key is required",
		},
	})
}
