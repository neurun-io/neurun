package api

import (
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/neurun-io/neurun/internal/domain/account"
	"github.com/neurun-io/neurun/internal/domain/auth"
	"github.com/neurun-io/neurun/internal/domain/deployment"
)

const testAPIKey = "neu_test.local-secret"

type fakeDeployments struct {
	createRun           deployment.CreateExecutionRequest
	createApp           deployment.CreateAppRequest
	listAppName         string
	listDeploymentAppID string
	rerunID             string
}

func (*fakeDeployments) Create(
	context.Context, deployment.CreateRequest,
) (deployment.Deployment, error) {
	return deployment.Deployment{ID: "dep_test", ProjectID: "prj_test"}, nil
}

func (*fakeDeployments) Get(
	_ context.Context, projectID, deploymentID string,
) (deployment.Deployment, error) {
	if projectID != "prj_test" || deploymentID != "dep_test" {
		return deployment.Deployment{}, deployment.ErrNotFound
	}
	return deployment.Deployment{ID: deploymentID, ProjectID: projectID}, nil
}

func (fake *fakeDeployments) List(
	_ context.Context, _ string, appID string, _ int,
) ([]deployment.Deployment, error) {
	fake.listDeploymentAppID = appID
	return []deployment.Deployment{}, nil
}

func (fake *fakeDeployments) CreateExecution(
	_ context.Context, request deployment.CreateExecutionRequest,
) (deployment.Execution, error) {
	fake.createRun = request
	return deployment.Run{
		ID: "run_test", ProjectID: request.ProjectID,
		DeploymentID: request.DeploymentID, BuildID: "bld_test",
		Status: deployment.RunQueued, Input: request.Input,
	}, nil
}

func (*fakeDeployments) GetExecution(
	_ context.Context, projectID, runID string,
) (deployment.Execution, error) {
	if projectID != "prj_test" || runID != "run_test" {
		return deployment.Run{}, deployment.ErrRunNotFound
	}
	return deployment.Run{ID: runID, ProjectID: projectID}, nil
}

func (*fakeDeployments) ListDeploymentExecutions(
	context.Context, string, string, int,
) ([]deployment.Execution, error) {
	return []deployment.Execution{}, nil
}

func (fake *fakeDeployments) RerunExecution(
	_ context.Context, projectID, runID string,
) (deployment.Execution, error) {
	fake.rerunID = runID
	return deployment.Run{
		ID: "run_again", ProjectID: projectID, RerunOfRunID: runID,
		Status: deployment.RunQueued,
	}, nil
}

func (*fakeDeployments) GetProject(context.Context, string) (deployment.Project, error) {
	return deployment.Project{}, nil
}
func (*fakeDeployments) ListProjects(context.Context, string, int) ([]deployment.Project, error) {
	return []deployment.Project{}, nil
}
func (*fakeDeployments) UpdateProject(context.Context, string, deployment.UpdateProjectRequest) (deployment.Project, error) {
	return deployment.Project{}, nil
}
func (*fakeDeployments) GetBuild(context.Context, string, string) (deployment.Build, error) {
	return deployment.Build{}, nil
}
func (*fakeDeployments) ListBuilds(context.Context, string, string, int) ([]deployment.Build, error) {
	return []deployment.Build{}, nil
}
func (*fakeDeployments) ListExecutions(context.Context, string, string, int) ([]deployment.Execution, error) {
	return []deployment.Execution{}, nil
}
func (fake *fakeDeployments) CreateApp(
	_ context.Context, request deployment.CreateAppRequest,
) (deployment.App, error) {
	fake.createApp = request
	return deployment.App{
		ID: "app_test", ProjectID: request.ProjectID, Name: request.Name,
	}, nil
}
func (*fakeDeployments) GetApp(context.Context, string, string) (deployment.App, error) {
	return deployment.App{}, nil
}

func (fake *fakeDeployments) ListApps(
	_ context.Context, _ string, name string, _ int,
) ([]deployment.App, error) {
	fake.listAppName = name
	return []deployment.App{}, nil
}
func (*fakeDeployments) UpdateApp(context.Context, string, string, deployment.UpdateAppRequest) (deployment.App, error) {
	return deployment.App{}, nil
}

type fakeAccounts struct{}

func (*fakeAccounts) CreateUser(context.Context, account.CreateUserRequest) (account.User, error) {
	return account.User{}, nil
}
func (*fakeAccounts) GetUser(context.Context, string, string) (account.User, error) {
	return account.User{}, nil
}
func (*fakeAccounts) ListUsers(context.Context, string, int) ([]account.User, error) {
	return []account.User{}, nil
}
func (*fakeAccounts) UpdateUser(context.Context, string, string, account.UpdateUserRequest) (account.User, error) {
	return account.User{}, nil
}
func (*fakeAccounts) CreateKey(context.Context, account.CreateKeyRequest) (account.CreatedKey, error) {
	return account.CreatedKey{}, nil
}
func (*fakeAccounts) ListKeys(context.Context, string, int) ([]account.Key, error) {
	return []account.Key{}, nil
}
func (*fakeAccounts) RevokeKey(context.Context, string, string) (account.Key, error) {
	return account.Key{}, nil
}

func newTestServer(t *testing.T) (*Server, *fakeDeployments) {
	t.Helper()
	authenticator, err := auth.New(auth.Credential{
		ID: "key_test", ProjectID: "prj_test", RawKey: testAPIKey,
		Scopes: []string{"*"},
	})
	if err != nil {
		t.Fatal(err)
	}
	service := &fakeDeployments{}
	server, err := NewServer(ServerOptions{
		Authenticator: authenticator, Deployments: service, Accounts: &fakeAccounts{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return server, service
}

func TestPublicHealthAndRemovedRoutes(t *testing.T) {
	t.Parallel()
	server, _ := newTestServer(t)
	response := request(t, server, http.MethodGet, "/healthz", "", "")
	if response.Code != http.StatusOK || response.Header().Get("Request-ID") == "" {
		t.Fatalf("health = %d %s", response.Code, response.Body)
	}
	response = request(t, server, http.MethodGet, "/v1/functions", "", testAPIKey)
	if response.Code != http.StatusNotFound {
		t.Fatalf("removed route = %d %s", response.Code, response.Body)
	}
}

func TestDeploymentRoutesRequireAPIKey(t *testing.T) {
	t.Parallel()
	server, _ := newTestServer(t)
	response := request(t, server, http.MethodGet, "/v1/deployments", "", "")
	if response.Code != http.StatusUnauthorized ||
		response.Header().Get("WWW-Authenticate") == "" {
		t.Fatalf("response = %d %s", response.Code, response.Body)
	}
}

func TestCreateRunPreservesExplicitNullAndLocation(t *testing.T) {
	t.Parallel()
	server, service := newTestServer(t)
	response := request(
		t, server, http.MethodPost, "/v1/deployments/dep_test/executions",
		`{"input":null}`, testAPIKey,
	)
	if response.Code != http.StatusAccepted {
		t.Fatalf("response = %d %s", response.Code, response.Body)
	}
	if response.Header().Get("Location") != "/v1/executions/run_test" {
		t.Fatalf("Location = %q", response.Header().Get("Location"))
	}
	if string(service.createRun.Input) != "null" {
		t.Fatalf("input = %s", service.createRun.Input)
	}
}

func TestCreateRunRejectsMissingAndUnknownFields(t *testing.T) {
	t.Parallel()
	server, _ := newTestServer(t)
	for _, body := range []string{`{}`, `{"input":{},"extra":true}`} {
		response := request(
			t, server, http.MethodPost, "/v1/deployments/dep_test/executions",
			body, testAPIKey,
		)
		if response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("body %s = %d %s", body, response.Code, response.Body)
		}
	}
}

func TestRerunTargetsRunAndReturnsAcceptedLocation(t *testing.T) {
	t.Parallel()
	server, service := newTestServer(t)
	response := request(
		t, server, http.MethodPost, "/v1/executions/run_test/rerun",
		"", testAPIKey,
	)
	if response.Code != http.StatusAccepted ||
		response.Header().Get("Location") != "/v1/executions/run_again" ||
		service.rerunID != "run_test" {
		t.Fatalf("response = %d %s, Location = %q, rerun = %q",
			response.Code, response.Body, response.Header().Get("Location"), service.rerunID)
	}
}

func TestArtifactStorageKeyIsNotSerialized(t *testing.T) {
	t.Parallel()
	body, err := json.Marshal(deployment.Artifact{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "storage") {
		t.Fatalf("artifact leaked internal storage: %s", body)
	}
}

func TestExecutionAlwaysSerializesLogs(t *testing.T) {
	t.Parallel()
	body, err := json.Marshal(deployment.Execution{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"logs":""`) {
		t.Fatalf("execution omitted bounded logs: %s", body)
	}
}

func TestEmptyUserPatchIsRejected(t *testing.T) {
	t.Parallel()
	server, _ := newTestServer(t)
	response := request(t, server, http.MethodPatch, "/v1/users/usr_test", `{}`, testAPIKey)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("response = %d %s", response.Code, response.Body)
	}
}

func TestCreateAppUsesAuthenticatedProjectAndHasNoDelete(t *testing.T) {
	t.Parallel()
	server, service := newTestServer(t)
	response := request(t, server, http.MethodPost, "/v1/apps", `{"name":"scraper"}`, testAPIKey)
	if response.Code != http.StatusCreated ||
		response.Header().Get("Location") != "/v1/apps/app_test" ||
		service.createApp.ProjectID != "prj_test" ||
		service.createApp.Name != "scraper" {
		t.Fatalf("response = %d %s, Location = %q, request = %#v",
			response.Code, response.Body, response.Header().Get("Location"), service.createApp)
	}
	response = request(t, server, http.MethodDelete, "/v1/apps/app_test", "", testAPIKey)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("delete response = %d %s", response.Code, response.Body)
	}
}

func TestListFiltersAreForwarded(t *testing.T) {
	t.Parallel()
	server, service := newTestServer(t)
	response := request(t, server, http.MethodGet, "/v1/apps?name=scrape&limit=1", "", testAPIKey)
	if response.Code != http.StatusOK || service.listAppName != "scrape" {
		t.Fatalf("apps response = %d %s, name = %q", response.Code, response.Body, service.listAppName)
	}
	response = request(t, server, http.MethodGet, "/v1/deployments?app_id=app_test&limit=1", "", testAPIKey)
	if response.Code != http.StatusOK || service.listDeploymentAppID != "app_test" {
		t.Fatalf("deployments response = %d %s, app_id = %q", response.Code, response.Body, service.listDeploymentAppID)
	}
}

func TestDeploymentFormRequiresAppID(t *testing.T) {
	t.Parallel()
	form := &multipart.Form{
		Value: map[string][]string{"runtime": {"python"}},
		File:  map[string][]*multipart.FileHeader{"source": {&multipart.FileHeader{Filename: "source.zip"}}},
	}
	if message := validateDeploymentForm(form); message != "app_id is required exactly once" {
		t.Fatalf("validation message = %q", message)
	}
	form.Value["app_id"] = []string{"app_test"}
	if message := validateDeploymentForm(form); message != "" {
		t.Fatalf("valid form rejected: %q", message)
	}
}

func TestOnlyAPIKeysRouteIsExposed(t *testing.T) {
	t.Parallel()
	server, _ := newTestServer(t)
	if response := request(t, server, http.MethodGet, "/v1/api-keys", "", testAPIKey); response.Code != http.StatusOK {
		t.Fatalf("api-keys response = %d %s", response.Code, response.Body)
	}
	if response := request(t, server, http.MethodGet, "/v1/keys", "", testAPIKey); response.Code != http.StatusNotFound {
		t.Fatalf("legacy keys response = %d %s", response.Code, response.Body)
	}
}

func request(
	t *testing.T,
	handler http.Handler,
	method string,
	target string,
	body string,
	apiKey string,
) *httptest.ResponseRecorder {
	t.Helper()
	httpRequest := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		httpRequest.Header.Set("Content-Type", "application/json")
	}
	if apiKey != "" {
		httpRequest.Header.Set("Authorization", "Bearer "+apiKey)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httpRequest)
	return response
}
