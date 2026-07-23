package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthenticator(t *testing.T) {
	t.Parallel()

	authenticator, err := New(Credential{
		ID:        "key_1",
		ProjectID: "prj_1",
		RawKey:    "neu_live_prefix.secret",
		Scopes:    []string{"functions:invoke"},
	})
	if err != nil {
		t.Fatal(err)
	}

	principal, ok := authenticator.Authenticate("neu_live_prefix.secret")
	if !ok {
		t.Fatal("expected authentication to succeed")
	}
	if principal.ProjectID != "prj_1" || !principal.HasScope("functions:invoke") {
		t.Fatalf("unexpected principal: %#v", principal)
	}
	if _, ok := authenticator.Authenticate("neu_live_prefix.wrong"); ok {
		t.Fatal("invalid secret authenticated")
	}
}

func TestMiddleware(t *testing.T) {
	t.Parallel()

	authenticator, err := New(Credential{
		ID:        "key_1",
		ProjectID: "prj_1",
		RawKey:    "neu_live_prefix.secret",
		Scopes:    []string{"*"},
	})
	if err != nil {
		t.Fatal(err)
	}

	handler := authenticator.Middleware(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		principal, ok := FromContext(request.Context())
		if !ok || principal.ProjectID != "prj_1" {
			t.Fatalf("missing principal: %#v", principal)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}

	authorizedRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	authorizedRequest.Header.Set("Authorization", "Bearer neu_live_prefix.secret")
	authorized := httptest.NewRecorder()
	handler.ServeHTTP(authorized, authorizedRequest)
	if authorized.Code != http.StatusNoContent {
		t.Fatalf("authorized status = %d", authorized.Code)
	}
}
