package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/neurun-io/neurun/internal/service"
)

// gin builds its routing tree at registration time and panics on a conflict,
// so constructing the server at all is the assertion: every route in routes()
// coexists. The services are zero values because no request here reaches one.
func newTestServer(t *testing.T) *Server {
	t.Helper()
	server, err := NewServer(ServerOptions{
		Deployments:   &service.DeploymentService{},
		Executions:    &service.ExecutionService{},
		Accounts:      &service.AccountService{},
		Sessions:      &service.SessionService{},
		Organizations: &service.OrganizationService{},
	})
	if err != nil {
		t.Fatalf("construct server: %v", err)
	}
	return server
}

func do(t *testing.T, server *Server, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, httptest.NewRequest(method, target, nil))
	return recorder
}

func TestRoutesRegisterAndUnauthenticatedPathsRespond(t *testing.T) {
	t.Parallel()
	server := newTestServer(t)

	response := do(t, server, http.MethodGet, "/healthz")
	if response.Code != http.StatusOK {
		t.Fatalf("healthz = %d", response.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" {
		t.Fatalf("healthz body = %#v", body)
	}
	// Readiness with no check configured is ready.
	if code := do(t, server, http.MethodGet, "/readyz").Code; code != http.StatusOK {
		t.Fatalf("readyz = %d", code)
	}
	if code := do(t, server, http.MethodGet, "/version").Code; code != http.StatusOK {
		t.Fatalf("version = %d", code)
	}
}

func TestSecurityHeadersAreSetOnEveryResponse(t *testing.T) {
	t.Parallel()
	server := newTestServer(t)
	response := do(t, server, http.MethodGet, "/healthz")
	for header, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
		"Cache-Control":          "no-store",
	} {
		if got := response.Header().Get(header); got != want {
			t.Fatalf("%s = %q, want %q", header, got, want)
		}
	}
}

// The whole /v1 tree is behind authentication. Without a bearer key or a
// session cookie every route has to answer 401 rather than reaching a handler.
func TestProtectedRoutesRejectAnonymousCallers(t *testing.T) {
	t.Parallel()
	server := newTestServer(t)
	for _, target := range []string{
		"/v1/deployments",
		"/v1/deployments/dep_one",
		"/v1/deployments/dep_one/executions",
		"/v1/executions",
		"/v1/executions/exe_one",
		"/v1/projects",
		"/v1/projects/prj_one",
		"/v1/apps",
		"/v1/apps/app_one",
		"/v1/builds",
		"/v1/builds/bld_one",
		"/v1/users",
		"/v1/users/usr_one",
		"/v1/api-keys",
		"/v1/browser-profiles",
		"/v1/browser-profiles/bpr_one",
		"/v1/identity-catalog",
	} {
		response := do(t, server, http.MethodGet, target)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("GET %s = %d, want 401", target, response.Code)
		}
		if response.Header().Get("WWW-Authenticate") == "" {
			t.Fatalf("GET %s: no WWW-Authenticate challenge", target)
		}
	}
}

// Sign-in has to sit outside the authenticated group, or there would be no way
// to obtain the credential the group requires.
func TestSignInRoutesAreReachableWithoutCredentials(t *testing.T) {
	t.Parallel()
	server := newTestServer(t)

	// Reaching the handler rather than the authentication gate: only the handler
	// can decide that an empty body is a bad request.
	if code := do(t, server, http.MethodPost, "/v1/auth/login").Code; code != http.StatusBadRequest {
		t.Fatalf("login = %d, want 400 from the handler", code)
	}
	// 401 is the signed-out answer, and the only one the dashboard needs:
	// authenticated-but-unscoped is a 403, so a 401 always means sign out.
	if code := do(t, server, http.MethodGet, "/v1/auth/session").Code; code != http.StatusUnauthorized {
		t.Fatalf("session lookup = %d, want 401 when signed out", code)
	}
	// Logout is unconditional: it always clears the cookie and answers 204.
	response := do(t, server, http.MethodPost, "/v1/auth/logout")
	if response.Code != http.StatusNoContent {
		t.Fatalf("logout = %d", response.Code)
	}
	if response.Header().Get("Set-Cookie") == "" {
		t.Fatal("logout did not clear the session cookie")
	}
}

func TestUnknownRouteIsAProblemDocument(t *testing.T) {
	t.Parallel()
	server := newTestServer(t)
	response := do(t, server, http.MethodGet, "/v1/nope")
	if response.Code != http.StatusNotFound {
		t.Fatalf("unknown route = %d", response.Code)
	}
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error.Code != "resource_not_found" {
		t.Fatalf("problem document = %s", response.Body)
	}
}
