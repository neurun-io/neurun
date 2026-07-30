package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dagflows/neurun-io/internal/operator"
)

const operatorPassword = "correct horse battery staple"

// Hashing at the production iteration count is deliberately slow, so the test
// accounts are hashed once for the whole package.
var testHash = sync.OnceValues(func() (string, error) {
	return operator.HashPassword(operatorPassword)
})

func operatorAccount(t *testing.T, username string, role operator.Role) operator.Account {
	t.Helper()

	hash, err := testHash()
	if err != nil {
		t.Fatal(err)
	}
	return operator.Account{
		Username:     username,
		Role:         role,
		ProjectID:    "prj_a",
		PasswordHash: hash,
	}
}

type operatorFixture struct {
	serverFixture
	handler   *Server
	operators *operator.Authenticator
}

// newOperatorFixture builds a server with both credential kinds enabled. The
// API-key authenticator is the same one the rest of the suite uses, so these
// tests also prove bearer access is unaffected.
func newOperatorFixture(t *testing.T, accounts ...operator.Account) operatorFixture {
	t.Helper()

	base := newServerFixture(t, nil)
	if len(accounts) == 0 {
		accounts = []operator.Account{operatorAccount(t, "alice", operator.RoleAdmin)}
	}

	store, err := operator.NewMemoryStore(accounts...)
	if err != nil {
		t.Fatal(err)
	}
	operators, err := operator.NewAuthenticator(store, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	handler, err := NewServer(ServerOptions{
		Authenticator:  base.authenticator,
		Registry:       base.registry,
		Invocations:    base.invocations,
		Jobs:           base.jobs,
		JobDurability:  JobDurabilityProcessLocal,
		AllowAsyncJobs: true,
		Operators:      operators,
		// Exercised as true so the cookie attributes under test are the ones a
		// real deployment sets.
		OperatorCookieSecure: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return operatorFixture{serverFixture: base, handler: handler, operators: operators}
}

func login(t *testing.T, handler http.Handler, username, password string) *httptest.ResponseRecorder {
	t.Helper()

	body := `{"username":` + strconv.Quote(username) + `,"password":` + strconv.Quote(password) + `}`
	request := httptest.NewRequest(http.MethodPost, "/v1/auth/login", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func sessionCookieFrom(t *testing.T, response *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()

	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == SessionCookieName {
			return cookie
		}
	}
	t.Fatalf("response did not set a %s cookie", SessionCookieName)
	return nil
}

func requestWithCookie(
	t *testing.T,
	handler http.Handler,
	method, path string,
	cookie *http.Cookie,
) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(method, path, nil)
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestOperatorLoginSetsAHardenedCookie(t *testing.T) {
	t.Parallel()

	fixture := newOperatorFixture(t)
	response := login(t, fixture.handler, "alice", operatorPassword)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body.String())
	}

	cookie := sessionCookieFrom(t, response)
	if !cookie.HttpOnly {
		t.Error("session cookie is not HttpOnly, so script can read the token")
	}
	if !cookie.Secure {
		t.Error("session cookie is not Secure")
	}
	if cookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("session cookie SameSite = %v, want Strict", cookie.SameSite)
	}
	if cookie.Value == "" {
		t.Error("session cookie has no value")
	}

	// The token belongs in the cookie and nowhere else — a token in the JSON body
	// would be readable by script and land in logs.
	if strings.Contains(response.Body.String(), cookie.Value) {
		t.Error("login response body leaked the session token")
	}
	if strings.Contains(response.Body.String(), operatorPassword) {
		t.Error("login response body leaked the password")
	}
}

func TestOperatorLoginReportsRoleAndScopes(t *testing.T) {
	t.Parallel()

	fixture := newOperatorFixture(t, operatorAccount(t, "vera", operator.RoleViewer))
	response := login(t, fixture.handler, "vera", operatorPassword)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}

	var body struct {
		Operator struct {
			Username string   `json:"username"`
			Role     string   `json:"role"`
			Scopes   []string `json:"scopes"`
		} `json:"operator"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Operator.Username != "vera" || body.Operator.Role != "viewer" {
		t.Fatalf("operator = %+v, want vera/viewer", body.Operator)
	}
	for _, scope := range body.Operator.Scopes {
		if scope == "*" || scope == ScopeJobsWrite || scope == ScopeFunctionsInvoke {
			t.Errorf("viewer unexpectedly granted %q", scope)
		}
	}
}

func TestSessionCookieAuthenticatesAProtectedRoute(t *testing.T) {
	t.Parallel()

	fixture := newOperatorFixture(t)
	cookie := sessionCookieFrom(t, login(t, fixture.handler, "alice", operatorPassword))

	response := requestWithCookie(t, fixture.handler, http.MethodGet, "/v1/functions", cookie)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body.String())
	}
}

func TestBearerKeyStillWorksAlongsideOperatorSignIn(t *testing.T) {
	t.Parallel()

	fixture := newOperatorFixture(t)

	response := performRequest(t, fixture.handler, http.MethodGet, "/v1/functions", "", testKeyA, "")
	if response.Code != http.StatusOK {
		t.Fatalf("bearer status = %d, want 200; body = %s", response.Code, response.Body.String())
	}

	// An unknown key must still be rejected rather than falling through to the
	// cookie path.
	rejected := performRequest(t, fixture.handler, http.MethodGet, "/v1/functions", "", "neu_test_x.nope", "")
	if rejected.Code != http.StatusUnauthorized {
		t.Fatalf("bad bearer status = %d, want 401", rejected.Code)
	}
}

func TestBearerHeaderTakesPrecedenceOverCookie(t *testing.T) {
	t.Parallel()

	fixture := newOperatorFixture(t)
	cookie := sessionCookieFrom(t, login(t, fixture.handler, "alice", operatorPassword))

	// A valid cookie must not rescue a rejected Authorization header: an explicit
	// header is a deliberate act and its failure has to surface.
	request := httptest.NewRequest(http.MethodGet, "/v1/functions", nil)
	request.Header.Set("Authorization", "Bearer neu_test_x.nope")
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	fixture.handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
}

func TestProtectedRouteWithoutAnyCredentialIsUnauthorized(t *testing.T) {
	t.Parallel()

	fixture := newOperatorFixture(t)
	response := requestWithCookie(t, fixture.handler, http.MethodGet, "/v1/functions", nil)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
	if response.Header().Get("WWW-Authenticate") == "" {
		t.Error("401 did not advertise WWW-Authenticate")
	}
}

func TestUnknownSessionCookieIsRejectedAndCleared(t *testing.T) {
	t.Parallel()

	fixture := newOperatorFixture(t)
	response := requestWithCookie(t, fixture.handler, http.MethodGet, "/v1/functions",
		&http.Cookie{Name: SessionCookieName, Value: "not-a-real-token"})

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
	// The browser should stop resending a token that will never work.
	cookie := sessionCookieFrom(t, response)
	if cookie.MaxAge >= 0 {
		t.Errorf("rejected session cookie MaxAge = %d, want negative", cookie.MaxAge)
	}
}

func TestExpiredSessionIsRejected(t *testing.T) {
	t.Parallel()

	fixture := newOperatorFixture(t)
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	fixture.operators.Now = func() time.Time { return now }

	cookie := sessionCookieFrom(t, login(t, fixture.handler, "alice", operatorPassword))
	now = now.Add(fixture.operators.SessionTTL + time.Minute)

	response := requestWithCookie(t, fixture.handler, http.MethodGet, "/v1/functions", cookie)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
}

func TestOperatorLoginRejectsBadCredentials(t *testing.T) {
	t.Parallel()

	fixture := newOperatorFixture(t)

	for name, credentials := range map[string][2]string{
		"wrong password": {"alice", "not the password"},
		"unknown user":   {"nobody", operatorPassword},
	} {
		response := login(t, fixture.handler, credentials[0], credentials[1])
		if response.Code != http.StatusUnauthorized {
			t.Errorf("%s: status = %d, want 401", name, response.Code)
			continue
		}
		var envelope ErrorEnvelope
		if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		// The same code and message for both, so the endpoint cannot be used to
		// discover which usernames exist.
		if envelope.Error.Code != "invalid_credentials" {
			t.Errorf("%s: code = %q, want invalid_credentials", name, envelope.Error.Code)
		}
		if envelope.RequestID == "" {
			t.Errorf("%s: response did not carry a request ID", name)
		}
	}
}

func TestOperatorLoginRequiresBothFields(t *testing.T) {
	t.Parallel()

	fixture := newOperatorFixture(t)
	for _, body := range []string{`{"username":"alice"}`, `{"password":"x"}`, `{}`} {
		request := httptest.NewRequest(http.MethodPost, "/v1/auth/login", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		fixture.handler.ServeHTTP(response, request)

		if response.Code != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want 400", body, response.Code)
		}
	}
}

func TestRepeatedFailuresAreThrottled(t *testing.T) {
	t.Parallel()

	fixture := newOperatorFixture(t)

	var lastCode int
	for range fixture.operators.Throttle.MaxAttempts + 2 {
		lastCode = login(t, fixture.handler, "alice", "wrong").Code
	}
	if lastCode != http.StatusUnauthorized && lastCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 401 or 429", lastCode)
	}

	response := login(t, fixture.handler, "alice", operatorPassword)
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429 after repeated failures", response.Code)
	}
	retryAfter := response.Header().Get("Retry-After")
	if retryAfter == "" {
		t.Fatal("429 did not carry Retry-After")
	}
	seconds, err := strconv.Atoi(retryAfter)
	if err != nil || seconds < 1 {
		t.Fatalf("Retry-After = %q, want a positive integer", retryAfter)
	}
}

func TestOperatorLogoutRevokesTheSession(t *testing.T) {
	t.Parallel()

	fixture := newOperatorFixture(t)
	cookie := sessionCookieFrom(t, login(t, fixture.handler, "alice", operatorPassword))

	logout := requestWithCookie(t, fixture.handler, http.MethodPost, "/v1/auth/logout", cookie)
	if logout.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d, want 204", logout.Code)
	}
	cleared := sessionCookieFrom(t, logout)
	if cleared.MaxAge >= 0 {
		t.Errorf("cleared cookie MaxAge = %d, want negative", cleared.MaxAge)
	}

	// The revoked token must no longer authenticate.
	after := requestWithCookie(t, fixture.handler, http.MethodGet, "/v1/functions", cookie)
	if after.Code != http.StatusUnauthorized {
		t.Fatalf("status after logout = %d, want 401", after.Code)
	}
}

func TestOperatorLogoutIsIdempotent(t *testing.T) {
	t.Parallel()

	fixture := newOperatorFixture(t)

	// Signing out without a session must not reveal that there was none.
	response := requestWithCookie(t, fixture.handler, http.MethodPost, "/v1/auth/logout", nil)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", response.Code)
	}
}

func TestOperatorSessionEndpoint(t *testing.T) {
	t.Parallel()

	fixture := newOperatorFixture(t)
	cookie := sessionCookieFrom(t, login(t, fixture.handler, "alice", operatorPassword))

	response := requestWithCookie(t, fixture.handler, http.MethodGet, "/v1/auth/session", cookie)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), cookie.Value) {
		t.Error("session response leaked the token")
	}

	anonymous := requestWithCookie(t, fixture.handler, http.MethodGet, "/v1/auth/session", nil)
	if anonymous.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d, want 401", anonymous.Code)
	}
}

func TestViewerRoleCannotSubmitWork(t *testing.T) {
	t.Parallel()

	fixture := newOperatorFixture(t, operatorAccount(t, "vera", operator.RoleViewer))
	cookie := sessionCookieFrom(t, login(t, fixture.handler, "vera", operatorPassword))

	// Reading is allowed…
	read := requestWithCookie(t, fixture.handler, http.MethodGet, "/v1/functions", cookie)
	if read.Code != http.StatusOK {
		t.Fatalf("viewer read status = %d, want 200", read.Code)
	}

	// …but submitting a job is not. Role scopes flow through the same
	// RequireScope path as API-key scopes.
	request := httptest.NewRequest(http.MethodPost, "/v1/jobs",
		strings.NewReader(`{"function":{"name":"system.echo","version":"1"},"input":{}}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "viewer-attempt-1")
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	fixture.handler.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("viewer write status = %d, want 403; body = %s", response.Code, response.Body.String())
	}
}

func TestAuthEndpointsReportWhenSignInIsNotConfigured(t *testing.T) {
	t.Parallel()

	base := newServerFixture(t, nil)
	handler, err := NewServer(ServerOptions{
		Authenticator: base.authenticator,
		Registry:      base.registry,
		Invocations:   base.invocations,
		Jobs:          base.jobs,
		// Operators deliberately omitted.
	})
	if err != nil {
		t.Fatal(err)
	}

	response := login(t, handler, "alice", operatorPassword)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("login status = %d, want 503", response.Code)
	}
	var envelope ErrorEnvelope
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error.Code != "operator_signin_unavailable" {
		t.Fatalf("code = %q, want operator_signin_unavailable", envelope.Error.Code)
	}

	// API-key access must be entirely unaffected.
	keyed := performRequest(t, handler, http.MethodGet, "/v1/functions", "", testKeyA, "")
	if keyed.Code != http.StatusOK {
		t.Fatalf("bearer status = %d, want 200", keyed.Code)
	}
}

func TestAuthRoutesRejectTheWrongMethod(t *testing.T) {
	t.Parallel()

	fixture := newOperatorFixture(t)
	for _, probe := range []struct {
		method, path string
	}{
		{http.MethodGet, "/v1/auth/login"},
		{http.MethodGet, "/v1/auth/logout"},
		{http.MethodPost, "/v1/auth/session"},
	} {
		response := requestWithCookie(t, fixture.handler, probe.method, probe.path, nil)
		if response.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s: status = %d, want 405", probe.method, probe.path, response.Code)
		}
	}
}
