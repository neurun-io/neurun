// Package api serves the control plane over HTTP.
package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"github.com/neurun-io/neurun/internal/buildinfo"
	"github.com/neurun-io/neurun/internal/domain/account"
	appdomain "github.com/neurun-io/neurun/internal/domain/app"
	"github.com/neurun-io/neurun/internal/domain/auth"
	"github.com/neurun-io/neurun/internal/domain/deployment"
	"github.com/neurun-io/neurun/internal/domain/execution"
	"github.com/neurun-io/neurun/internal/domain/project"
	"github.com/neurun-io/neurun/internal/dto"
	"github.com/neurun-io/neurun/internal/service"
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
	ScopeExecutionsRead   = "executions:read"
	ScopeExecutionsWrite  = "executions:write"
	ScopeAppsRead         = "apps:read"
	ScopeBuildsRead       = "builds:read"
	ScopeAppsWrite        = "apps:write"
	// A display is a signed-in browser rendered as pixels, which is more than
	// the profile endpoints ever return.
	ScopeBrowserSessionsRead = "browser_sessions:read"
	// Reading a profile's state returns live cookies, so it takes the write
	// scope rather than the read one.
	ScopeBrowserProfilesRead  = "browser_profiles:read"
	ScopeBrowserProfilesWrite = "browser_profiles:write"

	defaultMaximumBodyBytes = int64(1_048_576)
	defaultPageSize         = 50
	maximumPageSize         = 200
)

type ReadyCheck func(context.Context) error

type ServerOptions struct {
	Projects            *service.ProjectService
	Apps                *service.AppService
	Builds              *service.BuildService
	Deployments         *service.DeploymentService
	Executions          *service.ExecutionService
	Accounts            *service.AccountService
	Sessions            *service.SessionService
	Organizations       *service.OrganizationService
	GitHub              *service.GitHubService
	Browsers            *service.BrowserService
	BrowserSessions     *service.BrowserSessionService
	AllowedOrigins      []string
	Ready               ReadyCheck
	MaximumBodyBytes    int64
	SessionCookieSecure bool
}

type Server struct {
	projects            *service.ProjectService
	apps                *service.AppService
	builds              *service.BuildService
	deployments         *service.DeploymentService
	executions          *service.ExecutionService
	accounts            *service.AccountService
	sessions            *service.SessionService
	organizations       *service.OrganizationService
	gitHub              *service.GitHubService
	browsers            *service.BrowserService
	browserSessions     *service.BrowserSessionService
	allowedOrigins      []string
	ready               ReadyCheck
	maximumBodyBytes    int64
	sessionCookieSecure bool
	engine              *gin.Engine
}

func NewServer(options ServerOptions) (*Server, error) {
	switch {
	case options.Projects == nil:
		return nil, errors.New("project service is required")
	case options.Apps == nil:
		return nil, errors.New("app service is required")
	case options.Builds == nil:
		return nil, errors.New("build service is required")
	case options.Deployments == nil:
		return nil, errors.New("deployment service is required")
	case options.Executions == nil:
		return nil, errors.New("execution service is required")
	case options.Accounts == nil:
		return nil, errors.New("account service is required")
	case options.Sessions == nil:
		return nil, errors.New("session service is required")
	case options.Organizations == nil:
		return nil, errors.New("organization service is required")
	case options.MaximumBodyBytes < 0:
		return nil, errors.New("maximum request body bytes cannot be negative")
	}
	if options.MaximumBodyBytes == 0 {
		options.MaximumBodyBytes = defaultMaximumBodyBytes
	}
	server := &Server{
		projects:            options.Projects,
		apps:                options.Apps,
		builds:              options.Builds,
		deployments:         options.Deployments,
		executions:          options.Executions,
		accounts:            options.Accounts,
		sessions:            options.Sessions,
		organizations:       options.Organizations,
		gitHub:              options.GitHub,
		browsers:            options.Browsers,
		browserSessions:     options.BrowserSessions,
		allowedOrigins:      options.AllowedOrigins,
		ready:               options.Ready,
		maximumBodyBytes:    options.MaximumBodyBytes,
		sessionCookieSecure: options.SessionCookieSecure,
	}
	server.engine = server.routes()
	return server, nil
}

func (server *Server) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	server.engine.ServeHTTP(writer, request)
}

func (server *Server) routes() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(securityHeaders(), gin.Logger(), recovery())
	if len(server.allowedOrigins) > 0 {
		engine.Use(cors.New(cors.Config{
			AllowOrigins:     server.allowedOrigins,
			AllowMethods:     []string{"GET", "POST", "PATCH", "PUT", "DELETE"},
			AllowHeaders:     []string{"Content-Type"},
			ExposeHeaders:    []string{"Retry-After"},
			AllowCredentials: true,
			MaxAge:           10 * time.Minute,
		}))
	}
	engine.NoRoute(func(ctx *gin.Context) {
		notFound(ctx, "resource")
	})
	engine.NoMethod(func(ctx *gin.Context) {
		writeProblem(ctx, http.StatusMethodNotAllowed, dto.Problem{
			Code:    "method_not_allowed",
			Message: "the request method is not supported for this resource",
		})
	})

	engine.GET("/healthz", server.health)
	engine.GET("/readyz", server.readiness)
	engine.GET("/version", server.version)

	// Registration and sign-in are deliberately outside the authenticated
	// group: they are how a caller obtains the credential the group requires.
	// The invite lookup joins them, so a sign-up page can name the organization
	// it is about to add somebody to.
	engine.POST("/v1/auth/register", server.register)
	engine.POST("/v1/auth/login", server.login)
	engine.POST("/v1/auth/logout", server.logout)
	engine.GET("/v1/auth/session", server.currentSession)
	engine.GET("/v1/invites/lookup", server.lookupInvite)

	// GitHub holds no credential of ours to present: it signs the body with the
	// webhook secret instead, which the handler verifies before reading a word
	// of the payload.
	engine.POST("/v1/github/webhook", server.gitHubWebhook)

	v1 := engine.Group("/v1", server.authenticate())

	v1.GET("/deployments", server.scoped(ScopeDeploymentsRead), server.listDeployments)
	v1.GET("/deployments/:deployment_id", server.scoped(ScopeDeploymentsRead), server.getDeployment)
	v1.GET("/deployments/:deployment_id/executions", server.scoped(ScopeExecutionsRead), server.listDeploymentExecutions)
	v1.POST("/deployments/:deployment_id/executions", server.scoped(ScopeExecutionsWrite), server.createExecution)

	v1.GET("/builds", server.scoped(ScopeBuildsRead), server.listBuilds)
	v1.GET("/builds/:build_id", server.scoped(ScopeBuildsRead), server.getBuild)

	v1.GET("/executions", server.scoped(ScopeExecutionsRead), server.listExecutions)
	v1.GET("/executions/:execution_id", server.scoped(ScopeExecutionsRead), server.getExecution)
	v1.POST("/executions/:execution_id/rerun", server.scoped(ScopeExecutionsWrite), server.rerunExecution)

	v1.GET("/projects", server.scoped(ScopeProjectsRead), server.listProjects)
	v1.POST("/projects", server.scoped(ScopeProjectsWrite), server.createProject)
	v1.GET("/projects/:project_id", server.scoped(ScopeProjectsRead), server.getProject)
	v1.PATCH("/projects/:project_id", server.scoped(ScopeProjectsWrite), server.updateProject)
	v1.DELETE("/projects/:project_id", server.scoped(ScopeProjectsWrite), server.deleteProject)

	v1.GET("/apps", server.scoped(ScopeAppsRead), server.listApps)
	v1.POST("/apps", server.scoped(ScopeAppsWrite), server.createApp)
	v1.GET("/apps/:app_id", server.scoped(ScopeAppsRead), server.getApp)
	v1.PATCH("/apps/:app_id", server.scoped(ScopeAppsWrite), server.updateApp)
	v1.DELETE("/apps/:app_id", server.scoped(ScopeAppsWrite), server.deleteApp)

	v1.GET("/github/installation", server.scoped(ScopeAppsRead), server.getInstallation)
	v1.POST("/github/installation", server.scoped(ScopeAppsWrite), server.recordInstallation)
	v1.DELETE("/github/installation", server.scoped(ScopeAppsWrite), server.deleteInstallation)
	v1.GET("/github/repositories", server.scoped(ScopeAppsRead), server.listRepositories)
	v1.GET("/github/branches", server.scoped(ScopeAppsRead), server.listBranches)
	v1.PUT("/apps/:app_id/repository", server.scoped(ScopeAppsWrite), server.connectRepository)
	v1.POST("/github/deployments", server.scoped(ScopeDeploymentsWrite), server.deployRef)

	v1.GET("/browser-sessions", server.scoped(ScopeBrowserSessionsRead), server.listBrowserSessions)
	v1.GET("/browser-sessions/:session_id", server.scoped(ScopeBrowserSessionsRead), server.getBrowserSession)
	v1.DELETE("/browser-sessions/:session_id", server.scoped(ScopeBrowserSessionsRead), server.closeBrowserSession)
	// The upgrade happens inside the authenticated group, so the credential is
	// checked before a frame moves rather than after.
	v1.GET("/browser-sessions/:session_id/display", server.scoped(ScopeBrowserSessionsRead), server.streamBrowserDisplay)

	v1.GET("/organizations", sessionOnly(), server.listOrganizations)
	// Deliberately unscoped: an account with no organization holds no scopes,
	// and creating one is the only thing it may do.
	v1.POST("/organizations", sessionOnly(), server.createOrganization)
	v1.GET("/organization", server.scoped(ScopeProjectsRead), server.getOrganization)
	v1.PATCH("/organization", server.scoped(ScopeUsersWrite), server.updateOrganization)

	v1.GET("/members", server.scoped(ScopeUsersRead), server.listMembers)
	v1.PATCH("/members/:user_id", server.scoped(ScopeUsersWrite), server.updateMember)
	v1.DELETE("/members/:user_id", server.scoped(ScopeUsersWrite), server.removeMember)

	v1.GET("/invites", server.scoped(ScopeUsersRead), server.listInvites)
	v1.POST("/invites", server.scoped(ScopeUsersWrite), server.createInvite)
	v1.DELETE("/invites/:invite_id", server.scoped(ScopeUsersWrite), server.revokeInvite)
	v1.POST("/invites/accept", sessionOnly(), server.acceptInvite)

	v1.GET("/users", server.scoped(ScopeUsersRead), server.listUsers)
	v1.GET("/users/:user_id", server.scoped(ScopeUsersRead), server.getUser)
	v1.PATCH("/users/:user_id", server.scoped(ScopeUsersWrite), server.updateUser)
	v1.DELETE("/users/:user_id", server.scoped(ScopeUsersWrite), server.deleteUser)

	v1.GET("/identity-catalog", server.scoped(ScopeBrowserProfilesRead), server.identityCatalog)
	v1.GET("/browser-profiles", server.scoped(ScopeBrowserProfilesRead), server.listBrowserProfiles)
	v1.POST("/browser-profiles", server.scoped(ScopeBrowserProfilesWrite), server.createBrowserProfile)
	v1.GET("/browser-profiles/:browser_profile_id", server.scoped(ScopeBrowserProfilesRead), server.getBrowserProfile)
	v1.PATCH("/browser-profiles/:browser_profile_id", server.scoped(ScopeBrowserProfilesWrite), server.updateBrowserProfile)
	v1.DELETE("/browser-profiles/:browser_profile_id", server.scoped(ScopeBrowserProfilesWrite), server.deleteBrowserProfile)
	v1.GET("/browser-profiles/:browser_profile_id/state", server.scoped(ScopeBrowserProfilesWrite), server.getBrowserProfileState)
	v1.PUT("/browser-profiles/:browser_profile_id/state", server.scoped(ScopeBrowserProfilesWrite), server.saveBrowserProfileState)

	v1.GET("/api-keys", server.scoped(ScopeAPIKeysRead), server.listAPIKeys)
	v1.POST("/api-keys", server.scoped(ScopeAPIKeysWrite), server.createAPIKey)
	v1.DELETE("/api-keys/:api_key_id", server.scoped(ScopeAPIKeysWrite), server.revokeAPIKey)

	return engine
}

func (server *Server) health(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (server *Server) readiness(ctx *gin.Context) {
	if server.ready != nil {
		if err := server.ready(ctx.Request.Context()); err != nil {
			writeProblem(ctx, http.StatusServiceUnavailable, dto.Problem{
				Code: "not_ready", Message: "the server is not ready to accept work",
			})
			return
		}
	}
	ctx.JSON(http.StatusOK, gin.H{"status": "ready"})
}

func (server *Server) version(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, buildinfo.Current())
}

// pageLimit reads the shared limit query parameter, defaulting when absent.
func (server *Server) pageLimit(ctx *gin.Context) (int, bool) {
	raw := strings.TrimSpace(ctx.Query("limit"))
	if raw == "" {
		return defaultPageSize, true
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > maximumPageSize {
		invalidQuery(ctx, "limit must be an integer between 1 and 200")
		return 0, false
	}
	return limit, true
}

func principalOf(ctx *gin.Context) auth.Principal {
	principal, _ := auth.FromContext(ctx.Request.Context())
	return principal
}

func writeProblem(ctx *gin.Context, status int, problem dto.Problem) {
	ctx.AbortWithStatusJSON(status, dto.ErrorResponse{Error: problem})
}

func invalidRequest(ctx *gin.Context, message string) {
	writeProblem(ctx, http.StatusUnprocessableEntity, dto.Problem{
		Code: "invalid_request", Message: message,
	})
}

func invalidQuery(ctx *gin.Context, message string) {
	writeProblem(ctx, http.StatusBadRequest, dto.Problem{
		Code: "invalid_request", Message: message,
	})
}

func notFound(ctx *gin.Context, kind string) {
	writeProblem(ctx, http.StatusNotFound, dto.Problem{
		Code: "resource_not_found", Message: "the requested " + kind + " was not found",
	})
}

// writeError maps a domain error onto its HTTP shape. Anything unrecognised is
// a 500 with no detail, so an internal message cannot leak through.
func writeError(ctx *gin.Context, err error) {
	if writeOrganizationError(ctx, err) {
		return
	}
	if writeGitHubError(ctx, err) {
		return
	}
	if writeBrowserError(ctx, err) {
		return
	}
	if writeBrowserSessionError(ctx, err) {
		return
	}
	switch {
	case errors.Is(err, deployment.ErrNotFound),
		errors.Is(err, project.ErrNotFound),
		errors.Is(err, appdomain.ErrNotFound),
		errors.Is(err, execution.ErrNotFound),
		errors.Is(err, account.ErrNotFound):
		notFound(ctx, "resource")
	case errors.Is(err, project.ErrConflict):
		writeProblem(ctx, http.StatusConflict, dto.Problem{
			Code:    "project_conflict",
			Message: "the project conflicts with an existing project",
		})
	case errors.Is(err, appdomain.ErrConflict):
		writeProblem(ctx, http.StatusConflict, dto.Problem{
			Code:    "app_conflict",
			Message: "the app conflicts with an existing app",
		})
	case errors.Is(err, execution.ErrConflict):
		writeProblem(ctx, http.StatusConflict, dto.Problem{
			Code:    "execution_conflict",
			Message: "the execution changed while the operation was in progress",
		})
	case errors.Is(err, account.ErrConflict):
		writeProblem(ctx, http.StatusConflict, dto.Problem{
			Code:    "resource_conflict",
			Message: "the resource conflicts with an existing record",
		})
	case errors.Is(err, deployment.ErrSourceTooLarge):
		writeProblem(ctx, http.StatusRequestEntityTooLarge, dto.Problem{
			Code:    "deployment_too_large",
			Message: "deployment upload exceeds the configured source limit",
		})
	case errors.Is(err, deployment.ErrInvalid),
		errors.Is(err, deployment.ErrNotReady),
		errors.Is(err, execution.ErrInvalid),
		errors.Is(err, account.ErrInvalid):
		invalidRequest(ctx, err.Error())
	case errors.Is(err, context.Canceled):
		writeProblem(ctx, http.StatusRequestTimeout, dto.Problem{
			Code: "request_canceled", Message: "the request was canceled",
		})
	case errors.Is(err, context.DeadlineExceeded):
		writeProblem(ctx, http.StatusGatewayTimeout, dto.Problem{
			Code: "request_timeout", Message: "the request exceeded its deadline",
		})
	default:
		// Nothing recognised it, so the response cannot say what went wrong
		// without leaking internals. Log it instead — an unmapped 500 with no
		// trace is the one failure nobody can diagnose.
		slog.Error("unhandled error",
			"method", ctx.Request.Method,
			"path", ctx.FullPath(),
			"error", err,
		)
		writeProblem(ctx, http.StatusInternalServerError, dto.Problem{
			Code: "internal_error", Message: "the server could not complete the request",
		})
	}
}
