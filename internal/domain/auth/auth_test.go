package auth

import (
	"context"
	"testing"
)

// "*" has to grant scopes that did not exist when the key was issued, or every
// new endpoint would silently lock out existing admin keys.
func TestScopeAllGrantsEverythingIncludingUnknownScopes(t *testing.T) {
	t.Parallel()
	admin := Principal{Kind: KindAPIKey, Scopes: []string{ScopeAll}}
	for _, scope := range []string{
		"deployments:read", "executions:write", "a:scope:added:next:year",
	} {
		if !admin.HasScope(scope) {
			t.Fatalf("ScopeAll did not grant %q", scope)
		}
	}

	limited := Principal{Kind: KindAPIKey, Scopes: []string{"deployments:read"}}
	if !limited.HasScope("deployments:read") {
		t.Fatal("granted scope was refused")
	}
	if limited.HasScope("deployments:write") {
		t.Fatal("ungranted scope was allowed")
	}
	if (Principal{}).HasScope("deployments:read") {
		t.Fatal("the zero principal granted a scope")
	}
}

func TestPrincipalRoundTripsThroughContext(t *testing.T) {
	t.Parallel()
	if _, ok := FromContext(context.Background()); ok {
		t.Fatal("bare context reported a principal")
	}
	want := Principal{
		Kind: KindSession, UserID: "usr_1", Email: "ada@example.com",
		OrganizationID: "org_1",
		Scopes:         []string{ScopeAll},
	}
	got, ok := FromContext(WithPrincipal(context.Background(), want))
	if !ok || got.UserID != want.UserID || got.Kind != KindSession {
		t.Fatalf("round trip = %#v, %v", got, ok)
	}
}

func TestBearerTokenAcceptsOnlyWellFormedHeaders(t *testing.T) {
	t.Parallel()
	for header, want := range map[string]string{
		"Bearer neu_live_abc.secret": "neu_live_abc.secret",
		"bearer neu_live_abc.secret": "neu_live_abc.secret",
		" Bearer  ":                  "",
		"Basic neu_live_abc.secret":  "",
		"neu_live_abc.secret":        "",
		"":                           "",
		"Bearer with space":          "",
	} {
		token, ok := BearerToken(header)
		if want == "" {
			if ok {
				t.Fatalf("BearerToken(%q) accepted, got %q", header, token)
			}
			continue
		}
		if !ok || token != want {
			t.Fatalf("BearerToken(%q) = %q, %v", header, token, ok)
		}
	}
}
