package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/neurun-io/neurun/internal/account"
	"github.com/neurun-io/neurun/internal/auth"
	"github.com/neurun-io/neurun/internal/buildinfo"
	"github.com/neurun-io/neurun/internal/deployment"
	"github.com/neurun-io/neurun/internal/operator"
)

const (
	ScopeDeploymentsRead  = "deployments:read"
	ScopeDeploymentsWrite = "deployments:write"
	ScopeUsersRead        = "users:read"
	ScopeUsersWrite       = "users:write"
	ScopeAPIKeysRead      = "api_keys:read"
	ScopeAPIKeysWrite     = "api_keys:write"
	ScopeProjectsRead     = "projects:read"
	ScopeProjectsWrite    = "projects:write"
	ScopeBuildsRead       = "builds:read"
	ScopeExecutionsRead   = "executions:read"
	ScopeExecutionsWrite  = "executions:write"
	ScopeAppsRead         = "apps:read"
	ScopeAppsWrite        = "apps:write"

	defaultMaximumBodyBytes = int64(1 << 20)
	defaultPageSize         = 50
	maximumPageSize         = 200
)

// DeploymentService is the deployment and run application boundary consumed by
// the HTTP API. Keeping this port local lets storage and execution evolve
// without leaking those details into the wire contract.
type DeploymentService interface {
	Create(context.Context, deployment.CreateRequest) (deployment.Deployment, error)
	Get(context.Context, string, string) (deployment.Deployment, error)
	List(context.Context, string, string, int) ([]deployment.Deployment, error)
	CreateExecution(context.Context, deployment.CreateExecutionRequest) (deployment.Execution, error)
	GetExecution(context.Context, string, string) (deployment.Execution, error)
	ListDeploymentExecutions(context.Context, string, string, int) ([]deployment.Execution, error)
	RerunExecution(context.Context, string, string) (deployment.Execution, error)
	GetProject(context.Context, string) (deployment.Project, error)
	ListProjects(context.Context, string, int) ([]deployment.Project, error)
	UpdateProject(context.Context, string, deployment.UpdateProjectRequest) (deployment.Project, error)
	GetBuild(context.Context, string, string) (deployment.Build, error)
	ListBuilds(context.Context, string, string, int) ([]deployment.Build, error)
	ListExecutions(context.Context, string, string, int) ([]deployment.Execution, error)
	CreateApp(context.Context, deployment.CreateAppRequest) (deployment.App, error)
	GetApp(context.Context, string, string) (deployment.App, error)
	ListApps(context.Context, string, string, int) ([]deployment.App, error)
	UpdateApp(context.Context, string, string, deployment.UpdateAppRequest) (deployment.App, error)
}

type ReadyCheck func(context.Context) error

type APIKeyAuthenticator interface {
	AuthenticateContext(context.Context, string) (auth.Principal, bool)
}

type AccountService interface {
	CreateUser(context.Context, account.CreateUserRequest) (account.User, error)
	GetUser(context.Context, string, string) (account.User, error)
	ListUsers(context.Context, string, int) ([]account.User, error)
	UpdateUser(context.Context, string, string, account.UpdateUserRequest) (account.User, error)
	CreateKey(context.Context, account.CreateKeyRequest) (account.CreatedKey, error)
	ListKeys(context.Context, string, int) ([]account.Key, error)
	RevokeKey(context.Context, string, string) (account.Key, error)
}

type ServerOptions struct {
	Authenticator          APIKeyAuthenticator
	Deployments            DeploymentService
	Accounts               AccountService
	Ready                  ReadyCheck
	MaximumBodyBytes       int64
	MaximumDeploymentBytes int64
	Operators              *operator.Authenticator
	OperatorCookieSecure   bool
}

// Server exposes the health endpoints and authenticated deployment API.
type Server struct {
	deployments            DeploymentService
	accounts               AccountService
	ready                  ReadyCheck
	maximumBodyBytes       int64
	maximumDeploymentBytes int64
	apiKeys                APIKeyAuthenticator
	operators              *operator.Authenticator
	operatorCookieSecure   bool
	handler                http.Handler
}

func NewServer(options ServerOptions) (*Server, error) {
	switch {
	case options.Authenticator == nil:
		return nil, errors.New("API authenticator is required")
	case options.Deployments == nil:
		return nil, errors.New("deployment service is required")
	case options.Accounts == nil:
		return nil, errors.New("account service is required")
	case options.MaximumBodyBytes < 0:
		return nil, errors.New("maximum request body bytes cannot be negative")
	case options.MaximumDeploymentBytes < 0:
		return nil, errors.New("maximum deployment source bytes cannot be negative")
	}
	maximumBodyBytes := options.MaximumBodyBytes
	if maximumBodyBytes == 0 {
		maximumBodyBytes = defaultMaximumBodyBytes
	}
	maximumDeploymentBytes := options.MaximumDeploymentBytes
	if maximumDeploymentBytes == 0 {
		maximumDeploymentBytes = deployment.DefaultMaxSourceBytes
	}
	server := &Server{
		deployments:            options.Deployments,
		accounts:               options.Accounts,
		ready:                  options.Ready,
		maximumBodyBytes:       maximumBodyBytes,
		maximumDeploymentBytes: maximumDeploymentBytes,
		apiKeys:                options.Authenticator,
		operators:              options.Operators,
		operatorCookieSecure:   options.OperatorCookieSecure,
	}
	server.handler = server.routes()
	return server, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	s.handler.ServeHTTP(w, request)
}

func (s *Server) routes() http.Handler {
	protected := http.NewServeMux()
	protected.Handle("/v1/deployments", http.HandlerFunc(s.deploymentsCollection))
	protected.Handle("/v1/projects", s.scoped(
		ScopeProjectsRead, s.only(http.MethodGet, http.HandlerFunc(s.listProjects)),
	))
	protected.Handle("/v1/projects/{project_id}", http.HandlerFunc(s.projectItem))
	protected.Handle("/v1/apps", http.HandlerFunc(s.appsCollection))
	protected.Handle("/v1/apps/{app_id}", http.HandlerFunc(s.appItem))
	protected.Handle("/v1/builds", s.scoped(
		ScopeBuildsRead, s.only(http.MethodGet, http.HandlerFunc(s.listBuilds)),
	))
	protected.Handle("/v1/builds/{build_id}", s.scoped(
		ScopeBuildsRead, s.only(http.MethodGet, http.HandlerFunc(s.getBuild)),
	))
	protected.Handle("/v1/executions", s.scoped(
		ScopeExecutionsRead, s.only(http.MethodGet, http.HandlerFunc(s.listExecutions)),
	))
	protected.Handle("/v1/users", http.HandlerFunc(s.usersCollection))
	protected.Handle("/v1/users/{user_id}", http.HandlerFunc(s.userItem))
	protected.Handle("/v1/api-keys", http.HandlerFunc(s.apiKeysCollection))
	protected.Handle("/v1/api-keys/{api_key_id}", s.scoped(
		ScopeAPIKeysWrite, s.only(http.MethodDelete, http.HandlerFunc(s.revokeAPIKey)),
	))
	protected.Handle("/v1/deployments/{deployment_id}", s.scoped(
		ScopeDeploymentsRead,
		s.only(http.MethodGet, http.HandlerFunc(s.getDeployment)),
	))
	protected.Handle("/v1/deployments/{deployment_id}/executions", http.HandlerFunc(s.deploymentRuns))
	protected.Handle("/v1/executions/{execution_id}", s.scoped(
		ScopeExecutionsRead,
		s.only(http.MethodGet, http.HandlerFunc(s.getRun)),
	))
	protected.Handle("/v1/executions/{execution_id}/rerun", s.scoped(
		ScopeExecutionsWrite,
		s.only(http.MethodPost, http.HandlerFunc(s.rerun)),
	))
	protected.Handle("/", http.HandlerFunc(notFound))

	root := http.NewServeMux()
	root.Handle("/healthz", s.only(http.MethodGet, http.HandlerFunc(s.health)))
	root.Handle("/readyz", s.only(http.MethodGet, http.HandlerFunc(s.readiness)))
	root.Handle("/version", s.only(http.MethodGet, http.HandlerFunc(s.version)))
	root.Handle("/v1/auth/login", s.only(http.MethodPost, http.HandlerFunc(s.operatorLogin)))
	root.Handle("/v1/auth/logout", s.only(http.MethodPost, http.HandlerFunc(s.operatorLogout)))
	root.Handle("/v1/auth/session", s.only(http.MethodGet, http.HandlerFunc(s.operatorSession)))
	root.Handle("/v1/", s.authenticate(protected))
	root.Handle("/", http.HandlerFunc(notFound))
	return RequestIDMiddleware(SecurityHeaders(Recoverer(root)))
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) readiness(w http.ResponseWriter, request *http.Request) {
	if s.ready != nil {
		if err := s.ready(request.Context()); err != nil {
			WriteProblem(w, request, http.StatusServiceUnavailable, Problem{
				Code: "not_ready", Message: "the server is not ready to accept work",
			})
			return
		}
	}
	WriteJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) version(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, buildinfo.Current())
}

func (s *Server) scoped(scope string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if !s.requireScope(w, request, scope) {
			return
		}
		next.ServeHTTP(w, request)
	})
}

func (s *Server) requireScope(w http.ResponseWriter, request *http.Request, scope string) bool {
	principal, ok := auth.FromContext(request.Context())
	if !ok {
		WriteProblem(w, request, http.StatusUnauthorized, Problem{
			Code: "authentication_failed", Message: "a valid bearer API key is required",
		})
		return false
	}
	if !principal.HasScope(scope) {
		WriteProblem(w, request, http.StatusForbidden, Problem{
			Code:    "permission_denied",
			Message: "the API key does not grant the required scope",
			Details: map[string]any{"required_scope": scope},
		})
		return false
	}
	return true
}

func (s *Server) only(method string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != method {
			methodNotAllowed(w, request, method)
			return
		}
		next.ServeHTTP(w, request)
	})
}

func (s *Server) pageLimit(w http.ResponseWriter, request *http.Request) (int, bool) {
	raw := strings.TrimSpace(request.URL.Query().Get("limit"))
	if raw == "" {
		return defaultPageSize, true
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > maximumPageSize {
		s.invalidQuery(w, request, "limit must be an integer between 1 and 200")
		return 0, false
	}
	return limit, true
}

func (s *Server) writeDomainError(w http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, deployment.ErrNotFound),
		errors.Is(err, deployment.ErrRunNotFound),
		errors.Is(err, deployment.ErrProjectNotFound),
		errors.Is(err, deployment.ErrAppNotFound):
		s.resourceNotFound(w, request, "resource")
	case errors.Is(err, deployment.ErrProjectConflict):
		WriteProblem(w, request, http.StatusConflict, Problem{
			Code:    "project_conflict",
			Message: "the project conflicts with an existing project",
		})
	case errors.Is(err, deployment.ErrAppConflict):
		WriteProblem(w, request, http.StatusConflict, Problem{
			Code:    "app_conflict",
			Message: "the app conflicts with an existing app",
		})
	case errors.Is(err, deployment.ErrRunConflict):
		WriteProblem(w, request, http.StatusConflict, Problem{
			Code:    "execution_conflict",
			Message: "the execution changed while the operation was in progress",
		})
	case errors.Is(err, deployment.ErrSourceTooLarge):
		WriteProblem(w, request, http.StatusRequestEntityTooLarge, Problem{
			Code:    "deployment_too_large",
			Message: "deployment upload exceeds the configured source limit",
		})
	case errors.Is(err, deployment.ErrInvalid),
		errors.Is(err, deployment.ErrNoReadyBuild):
		s.invalidRequest(w, request, err.Error())
	case errors.Is(err, context.Canceled):
		WriteProblem(w, request, http.StatusRequestTimeout, Problem{
			Code: "request_canceled", Message: "the request was canceled",
		})
	case errors.Is(err, context.DeadlineExceeded):
		WriteProblem(w, request, http.StatusGatewayTimeout, Problem{
			Code: "request_timeout", Message: "the request exceeded its deadline",
		})
	default:
		WriteProblem(w, request, http.StatusInternalServerError, Problem{
			Code: "internal_error", Message: "the server could not complete the request",
		})
	}
}

func (s *Server) invalidRequest(w http.ResponseWriter, request *http.Request, message string) {
	WriteProblem(w, request, http.StatusUnprocessableEntity, Problem{
		Code: "invalid_request", Message: message,
	})
}

func (s *Server) invalidQuery(w http.ResponseWriter, request *http.Request, message string) {
	WriteProblem(w, request, http.StatusBadRequest, Problem{
		Code: "invalid_request", Message: message,
	})
}

func (s *Server) resourceNotFound(w http.ResponseWriter, request *http.Request, kind string) {
	WriteProblem(w, request, http.StatusNotFound, Problem{
		Code: "resource_not_found", Message: "the requested " + kind + " was not found",
	})
}

func methodNotAllowed(w http.ResponseWriter, request *http.Request, allowed ...string) {
	w.Header().Set("Allow", strings.Join(allowed, ", "))
	WriteProblem(w, request, http.StatusMethodNotAllowed, Problem{
		Code:    "method_not_allowed",
		Message: "the request method is not supported for this resource",
	})
}

func notFound(w http.ResponseWriter, request *http.Request) {
	WriteProblem(w, request, http.StatusNotFound, Problem{
		Code: "resource_not_found", Message: "the requested resource was not found",
	})
}

func requireEmptyBody(request *http.Request) error {
	if request.Body == nil || request.ContentLength == 0 {
		return nil
	}
	return fmt.Errorf("request body must be empty")
}
