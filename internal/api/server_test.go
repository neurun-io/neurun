package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dagflows/neurun-io/internal/auth"
	"github.com/dagflows/neurun-io/internal/function"
	"github.com/dagflows/neurun-io/internal/job"
)

const (
	testKeyA = "neu_test_a.secret-a"
	testKeyB = "neu_test_b.secret-b"
)

type serverFixture struct {
	authenticator *auth.Authenticator
	handler       *Server
	registry      *function.Registry
	invocations   *function.Service
	jobs          *job.MemoryRepository
}

func newServerFixture(t *testing.T, ready ReadyCheck) serverFixture {
	t.Helper()
	registry := function.NewRegistry()
	if err := function.RegisterBuiltins(registry); err != nil {
		t.Fatal(err)
	}
	fetchFunction, err := testFetchFunction()
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(fetchFunction); err != nil {
		t.Fatal(err)
	}
	invocations := function.NewService(registry, function.NewMemoryStore())
	jobs := job.NewMemoryRepository()
	authenticator, err := auth.New(
		auth.Credential{
			ID:        "key_a",
			ProjectID: "prj_a",
			RawKey:    testKeyA,
			Scopes:    []string{"*"},
		},
		auth.Credential{
			ID:        "key_b",
			ProjectID: "prj_b",
			RawKey:    testKeyB,
			Scopes:    []string{"*"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewServer(ServerOptions{
		Authenticator:  authenticator,
		Registry:       registry,
		Invocations:    invocations,
		Jobs:           jobs,
		Ready:          ready,
		JobDurability:  JobDurabilityProcessLocal,
		AllowAsyncJobs: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return serverFixture{
		authenticator: authenticator,
		handler:       handler,
		registry:      registry,
		invocations:   invocations,
		jobs:          jobs,
	}
}

func testFetchFunction() (function.AtomicFunction, error) {
	return function.NewAtomicFunction(function.Manifest{
		Name:             "http.fetch",
		Version:          "1",
		Category:         "http",
		Description:      "Test HTTP function.",
		ExecutionContext: function.ExecutionContextHTTPAttempt,
		SideEffects:      function.SideEffectIdempotent,
		Timeout: function.TimeoutPolicy{
			DefaultMS: 1000,
			MaximumMS: 5000,
		},
		Capabilities: []string{"http"},
		InputSchema: function.Schema{
			Type:                 function.TypeObject,
			Required:             []string{"url"},
			AdditionalProperties: function.Bool(false),
			Properties: map[string]function.Schema{
				"url": {Type: function.TypeString},
			},
		},
		OutputSchema: function.Schema{},
	}, func(
		ctx context.Context,
		execution *function.ExecutionContext,
		input json.RawMessage,
	) (function.FunctionResult, error) {
		if err := ctx.Err(); err != nil {
			return function.FunctionResult{}, err
		}
		output, err := json.Marshal(map[string]any{
			"project_id": execution.ProjectID,
			"input":      json.RawMessage(input),
		})
		if err != nil {
			return function.FunctionResult{}, err
		}
		return function.FunctionResult{Output: output}, nil
	})
}

func TestServerPublicHealthAndAuthenticatedCatalog(t *testing.T) {
	t.Parallel()
	fixture := newServerFixture(t, nil)

	response := performRequest(t, fixture.handler, http.MethodGet, "/healthz", "", "", "")
	if response.Code != http.StatusOK {
		t.Fatalf("health status = %d, body = %s", response.Code, response.Body)
	}
	if response.Header().Get("Request-ID") == "" {
		t.Fatal("health response is missing Request-ID")
	}
	if response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("health response is missing security headers")
	}

	response = performRequest(t, fixture.handler, http.MethodGet, "/v1/functions", "", "", "")
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated catalog status = %d, body = %s", response.Code, response.Body)
	}
	if response.Header().Get("WWW-Authenticate") == "" {
		t.Fatal("unauthenticated response is missing challenge")
	}

	response = performRequest(t, fixture.handler, http.MethodGet, "/v1/functions", "", testKeyA, "")
	if response.Code != http.StatusOK {
		t.Fatalf("catalog status = %d, body = %s", response.Code, response.Body)
	}
	var catalog struct {
		Functions []function.Manifest `json:"functions"`
	}
	decodeResponse(t, response, &catalog)
	if len(catalog.Functions) != 3 {
		t.Fatalf("catalog function count = %d, want 3", len(catalog.Functions))
	}
}

func TestServerReadinessFailureUsesProblemEnvelope(t *testing.T) {
	t.Parallel()
	fixture := newServerFixture(t, func(context.Context) error {
		return context.DeadlineExceeded
	})
	response := performRequest(t, fixture.handler, http.MethodGet, "/readyz", "", "", "")
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("ready status = %d, body = %s", response.Code, response.Body)
	}
	var envelope ErrorEnvelope
	decodeResponse(t, response, &envelope)
	if envelope.Error.Code != "not_ready" || envelope.RequestID == "" {
		t.Fatalf("ready problem = %#v", envelope)
	}
}

func TestServerSyncInvocationIsProjectScoped(t *testing.T) {
	t.Parallel()
	fixture := newServerFixture(t, nil)
	body := `{"project_id":"prj_a","version":"1","execution":"sync","input":{"hello":"world"}}`
	response := performRequest(
		t,
		fixture.handler,
		http.MethodPost,
		"/v1/functions/system.echo/invoke",
		body,
		testKeyA,
		"",
	)
	if response.Code != http.StatusOK {
		t.Fatalf("invoke status = %d, body = %s", response.Code, response.Body)
	}
	var invoked invocationResponse
	decodeResponse(t, response, &invoked)
	if invoked.ProjectID != "prj_a" || invoked.Status != function.InvocationSucceeded {
		t.Fatalf("invocation = %#v", invoked.Invocation)
	}
	if invoked.Context == nil || invoked.Context.ProjectID != "prj_a" {
		t.Fatalf("execution context = %#v", invoked.Context)
	}
	if invoked.RequestID == "" || invoked.TraceID == invoked.RequestID {
		t.Fatalf("request_id = %q, trace_id = %q", invoked.RequestID, invoked.TraceID)
	}

	response = performRequest(
		t,
		fixture.handler,
		http.MethodGet,
		"/v1/function-invocations/"+invoked.ID,
		"",
		testKeyB,
		"",
	)
	if response.Code != http.StatusNotFound {
		t.Fatalf("cross-project invocation status = %d, body = %s", response.Code, response.Body)
	}

	body = `{"version":"1","execution":"sync","context":{"project_id":"prj_b"},"input":{}}`
	response = performRequest(
		t,
		fixture.handler,
		http.MethodPost,
		"/v1/functions/system.echo/invoke",
		body,
		testKeyA,
		"",
	)
	if response.Code != http.StatusForbidden {
		t.Fatalf("forged context status = %d, body = %s", response.Code, response.Body)
	}

	for _, contextJSON := range []string{
		`{"attempt_id":"att_forged"}`,
		`{"session_id":"ses_forged"}`,
		`{"ephemeral_http":true}`,
		`{"capabilities":["http"]}`,
	} {
		body = `{"version":"1","execution":"sync","context":` +
			contextJSON + `,"input":{}}`
		response = performRequest(
			t,
			fixture.handler,
			http.MethodPost,
			"/v1/functions/system.echo/invoke",
			body,
			testKeyA,
			"",
		)
		if response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("server-owned context %s status = %d, body = %s",
				contextJSON, response.Code, response.Body)
		}
	}
}

func TestServerAsyncJobAcceptanceRequiresIdempotencyAndReplays(t *testing.T) {
	t.Parallel()
	fixture := newServerFixture(t, nil)
	body := `{"version":"1","execution":"async","input":{"work":1},"max_attempts":2}`
	response := performRequest(
		t,
		fixture.handler,
		http.MethodPost,
		"/v1/functions/system.echo/invoke",
		body,
		testKeyA,
		"",
	)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("missing idempotency status = %d, body = %s", response.Code, response.Body)
	}

	response = performRequest(
		t,
		fixture.handler,
		http.MethodPost,
		"/v1/functions/system.echo/invoke",
		body,
		testKeyA,
		"idem-one",
	)
	if response.Code != http.StatusAccepted {
		t.Fatalf("async invoke status = %d, body = %s", response.Code, response.Body)
	}
	var accepted struct {
		Job struct {
			ID      string `json:"id"`
			Request struct {
				Function job.FunctionRef `json:"function"`
			} `json:"request"`
		} `json:"job"`
		JobID      string `json:"job_id"`
		Duplicate  bool   `json:"duplicate"`
		Durability string `json:"durability"`
		RequestID  string `json:"request_id"`
	}
	decodeResponse(t, response, &accepted)
	if accepted.JobID == "" ||
		accepted.Job.ID != accepted.JobID ||
		accepted.RequestID == "" ||
		accepted.Durability != string(JobDurabilityProcessLocal) ||
		response.Header().Get("Neurun-Job-Durability") != string(JobDurabilityProcessLocal) {
		t.Fatalf("accepted response = %#v", accepted)
	}
	if accepted.Job.Request.Function.Version != "1" ||
		accepted.Job.Request.Function.Digest == "" {
		t.Fatalf("job function was not pinned: %#v", accepted.Job.Request.Function)
	}

	response = performRequest(
		t,
		fixture.handler,
		http.MethodPost,
		"/v1/functions/system.echo/invoke",
		body,
		testKeyA,
		"idem-one",
	)
	if response.Code != http.StatusAccepted || response.Header().Get("Idempotent-Replayed") != "true" {
		t.Fatalf("replay status = %d, headers = %#v, body = %s", response.Code, response.Header(), response.Body)
	}
	var replayed struct {
		JobID     string `json:"job_id"`
		Duplicate bool   `json:"duplicate"`
	}
	decodeResponse(t, response, &replayed)
	if !replayed.Duplicate || replayed.JobID != accepted.JobID {
		t.Fatalf("replayed response = %#v", replayed)
	}

	changed := `{"version":"1","execution":"async","input":{"work":2},"max_attempts":2}`
	response = performRequest(
		t,
		fixture.handler,
		http.MethodPost,
		"/v1/functions/system.echo/invoke",
		changed,
		testKeyA,
		"idem-one",
	)
	if response.Code != http.StatusConflict {
		t.Fatalf("idempotency conflict status = %d, body = %s", response.Code, response.Body)
	}

	response = performRequest(
		t,
		fixture.handler,
		http.MethodGet,
		"/v1/jobs/"+accepted.JobID+"/events",
		"",
		testKeyA,
		"",
	)
	if response.Code != http.StatusOK {
		t.Fatalf("job events status = %d, body = %s", response.Code, response.Body)
	}

	response = performRequest(
		t,
		fixture.handler,
		http.MethodGet,
		"/v1/jobs/"+accepted.JobID,
		"",
		testKeyB,
		"",
	)
	if response.Code != http.StatusNotFound {
		t.Fatalf("cross-project job status = %d, body = %s", response.Code, response.Body)
	}
}

func TestServerGatesProcessLocalAsyncJobsUnlessExplicitlyEnabled(t *testing.T) {
	t.Parallel()
	fixture := newServerFixture(t, nil)
	handler, err := NewServer(ServerOptions{
		Authenticator: fixture.authenticator,
		Registry:      fixture.registry,
		Invocations:   fixture.invocations,
		Jobs:          fixture.jobs,
	})
	if err != nil {
		t.Fatal(err)
	}
	if handler.jobDurability != JobDurabilityProcessLocal {
		t.Fatalf("zero-value durability = %q, want %q",
			handler.jobDurability, JobDurabilityProcessLocal)
	}
	if handler.allowAsyncJobs {
		t.Fatal("zero-value server options unexpectedly enabled asynchronous jobs")
	}

	response := performRequest(
		t,
		handler,
		http.MethodPost,
		"/v1/jobs",
		`{"function":{"name":"system.echo","version":"1"},"input":{}}`,
		testKeyA,
		"idem-disabled",
	)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("disabled jobs status = %d, body = %s", response.Code, response.Body)
	}
	var envelope ErrorEnvelope
	decodeResponse(t, response, &envelope)
	if envelope.Error.Code != "durable_backend_unavailable" {
		t.Fatalf("disabled jobs problem = %#v", envelope)
	}
}

func TestServerCreateListAndCancelJob(t *testing.T) {
	t.Parallel()
	fixture := newServerFixture(t, nil)
	body := `{"function":{"name":"system.echo","version":"stable"},"input":{"job":true}}`
	response := performRequest(
		t,
		fixture.handler,
		http.MethodPost,
		"/v1/jobs",
		body,
		testKeyA,
		"idem-job",
	)
	if response.Code != http.StatusAccepted {
		t.Fatalf("create job status = %d, body = %s", response.Code, response.Body)
	}
	var accepted struct {
		JobID string `json:"job_id"`
	}
	decodeResponse(t, response, &accepted)

	response = performRequest(t, fixture.handler, http.MethodGet, "/v1/jobs?status=queued", "", testKeyA, "")
	if response.Code != http.StatusOK {
		t.Fatalf("list jobs status = %d, body = %s", response.Code, response.Body)
	}
	var listed struct {
		Jobs []job.Job `json:"jobs"`
	}
	decodeResponse(t, response, &listed)
	if len(listed.Jobs) != 1 || listed.Jobs[0].ID != accepted.JobID {
		t.Fatalf("listed jobs = %#v", listed.Jobs)
	}

	response = performRequest(
		t,
		fixture.handler,
		http.MethodPost,
		"/v1/jobs/"+accepted.JobID+"/cancel",
		`{"reason":"operator request"}`,
		testKeyA,
		"",
	)
	if response.Code != http.StatusOK {
		t.Fatalf("cancel job status = %d, body = %s", response.Code, response.Body)
	}
	var canceled struct {
		Job       job.Job `json:"job"`
		RequestID string  `json:"request_id"`
	}
	decodeResponse(t, response, &canceled)
	if canceled.Job.State != job.StateCanceled || canceled.RequestID == "" {
		t.Fatalf("canceled response = %#v", canceled)
	}
}

func TestServerFetchSuppliesTrustedHTTPContext(t *testing.T) {
	t.Parallel()
	fixture := newServerFixture(t, nil)
	body := `{"project_id":"prj_a","mode":"auto","request":{"url":"https://example.test"}}`
	response := performRequest(
		t,
		fixture.handler,
		http.MethodPost,
		"/v1/fetch",
		body,
		testKeyA,
		"",
	)
	if response.Code != http.StatusOK {
		t.Fatalf("fetch status = %d, body = %s", response.Code, response.Body)
	}
	var invoked invocationResponse
	decodeResponse(t, response, &invoked)
	if invoked.Context == nil ||
		invoked.Context.ProjectID != "prj_a" ||
		!invoked.Context.EphemeralHTTP ||
		len(invoked.Context.Capabilities) != 1 ||
		invoked.Context.Capabilities[0] != "http" {
		t.Fatalf("fetch execution context = %#v", invoked.Context)
	}
}

func TestServerEnforcesScopesAndJSONMethods(t *testing.T) {
	t.Parallel()
	registry := function.NewRegistry()
	if err := function.RegisterBuiltins(registry); err != nil {
		t.Fatal(err)
	}
	authenticator, err := auth.New(auth.Credential{
		ID:        "read_only",
		ProjectID: "prj_a",
		RawKey:    testKeyA,
		Scopes:    []string{ScopeFunctionsRead},
	})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewServer(ServerOptions{
		Authenticator: authenticator,
		Registry:      registry,
		Invocations:   function.NewService(registry, function.NewMemoryStore()),
		Jobs:          job.NewMemoryRepository(),
	})
	if err != nil {
		t.Fatal(err)
	}

	response := performRequest(
		t,
		handler,
		http.MethodPost,
		"/v1/functions/system.echo/invoke",
		`{"input":{}}`,
		testKeyA,
		"",
	)
	if response.Code != http.StatusForbidden {
		t.Fatalf("scope status = %d, body = %s", response.Code, response.Body)
	}
	var envelope ErrorEnvelope
	decodeResponse(t, response, &envelope)
	if envelope.Error.Code != "permission_denied" || envelope.RequestID == "" {
		t.Fatalf("scope problem = %#v", envelope)
	}

	response = performRequest(t, handler, http.MethodPost, "/healthz", "", "", "")
	if response.Code != http.StatusMethodNotAllowed ||
		!strings.Contains(response.Header().Get("Allow"), http.MethodGet) {
		t.Fatalf("method response = %d, headers = %#v, body = %s", response.Code, response.Header(), response.Body)
	}
	decodeResponse(t, response, &envelope)
	if envelope.Error.Code != "method_not_allowed" {
		t.Fatalf("method problem = %#v", envelope)
	}
}

func performRequest(
	t *testing.T,
	handler http.Handler,
	method string,
	path string,
	body string,
	key string,
	idempotencyKey string,
) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		request.Header.Set("Authorization", "Bearer "+key)
	}
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func decodeResponse(t *testing.T, response *httptest.ResponseRecorder, destination any) {
	t.Helper()
	if err := json.NewDecoder(response.Body).Decode(destination); err != nil {
		t.Fatalf("decode response: %v; body = %s", err, response.Body)
	}
}
