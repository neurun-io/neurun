package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	githubdomain "github.com/neurun-io/neurun/internal/domain/github"
	"github.com/neurun-io/neurun/internal/dto"
	"github.com/neurun-io/neurun/internal/github"
)

const (
	// GitHub caps a delivery at 25MB, but a push event carries at most twenty
	// commits and never approaches that.
	maximumWebhookBytes = int64(5_242_880)
	// A ceiling on a deploy that outlived its delivery, so a wedged build
	// cannot leave a goroutine running for the life of the process.
	webhookDeployTimeout = 30 * time.Minute
)

// recordInstallation stores the installation GitHub redirects back with after
// somebody installs the app on their account.
func (server *Server) recordInstallation(ctx *gin.Context) {
	var body dto.InstallRequest
	if !server.bindJSON(ctx, &body) {
		return
	}
	installationID, err := strconv.ParseInt(strings.TrimSpace(body.InstallationID), 10, 64)
	if err != nil || installationID <= 0 {
		invalidRequest(ctx, "installation_id must be a positive integer")
		return
	}
	record, err := server.gitHub.Install(
		ctx.Request.Context(), principalOf(ctx).OrganizationID, installationID,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusCreated, dto.NewInstallationResponse(record))
}

func (server *Server) getInstallation(ctx *gin.Context) {
	record, err := server.gitHub.Installation(
		ctx.Request.Context(), principalOf(ctx).OrganizationID,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, dto.NewInstallationResponse(record))
}

func (server *Server) deleteInstallation(ctx *gin.Context) {
	if err := server.gitHub.Uninstall(
		ctx.Request.Context(), principalOf(ctx).OrganizationID,
	); err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
}

// listRepositories serves what the installation grants, so an app is created
// from a repository that is known to exist rather than one typed from memory.
func (server *Server) listRepositories(ctx *gin.Context) {
	records, err := server.gitHub.Repositories(
		ctx.Request.Context(), principalOf(ctx).OrganizationID,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"repositories": records})
}

func (server *Server) listBranches(ctx *gin.Context) {
	repository := strings.TrimSpace(ctx.Query("repository"))
	if repository == "" {
		invalidRequest(ctx, "repository is required")
		return
	}
	names, err := server.gitHub.Branches(
		ctx.Request.Context(), principalOf(ctx).OrganizationID, repository,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"branches": names})
}

// connectRepository points an app at a repository, or disconnects it when the
// repository is empty.
func (server *Server) connectRepository(ctx *gin.Context) {
	var body dto.ConnectRepositoryRequest
	if !server.bindJSON(ctx, &body) {
		return
	}
	record, err := server.gitHub.Connect(
		ctx.Request.Context(), principalOf(ctx).OrganizationID,
		ctx.Param("app_id"), body,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, dto.NewAppResponse(record))
}

// deployRef builds a commit from the app's connected repository. An absent ref
// uses the app's production ref.
func (server *Server) deployRef(ctx *gin.Context) {
	var body dto.DeployRefRequest
	if !server.bindJSON(ctx, &body) {
		return
	}
	if strings.TrimSpace(body.AppID) == "" {
		invalidRequest(ctx, "app_id is required")
		return
	}
	record, err := server.gitHub.Deploy(
		ctx.Request.Context(), principalOf(ctx).OrganizationID,
		body.AppID, body.Ref,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusCreated, dto.NewDeploymentResponse(record))
}

// gitHubWebhook takes a delivery from GitHub. It carries neither a session nor
// an API key: the signature over the body is the credential, so this route sits
// outside the authenticated group.
func (server *Server) gitHubWebhook(ctx *gin.Context) {
	ctx.Request.Body = http.MaxBytesReader(
		ctx.Writer, ctx.Request.Body, maximumWebhookBytes,
	)
	push, deployable, err := server.gitHub.ParsePush(ctx.Request)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeProblem(ctx, http.StatusRequestEntityTooLarge, dto.Problem{
				Code:    "request_too_large",
				Message: "the delivery exceeds the configured limit",
			})
			return
		}
		writeError(ctx, err)
		return
	}
	if !deployable {
		// Signed, understood, and nothing to build: a ping, an event the app is
		// subscribed to but this server does not act on, or a deleted branch.
		ctx.Status(http.StatusNoContent)
		return
	}

	// A build runs for minutes and GitHub abandons a delivery after ten
	// seconds, so the deploy outlives the request that started it. Detaching
	// the context rather than reusing it keeps the build from being cancelled
	// the moment this handler returns.
	deployCtx, cancel := context.WithTimeout(
		context.WithoutCancel(ctx.Request.Context()), webhookDeployTimeout,
	)
	go func() {
		defer cancel()
		deployed, err := server.gitHub.Push(deployCtx, push)
		attributes := []any{
			"delivery", push.Delivery, "repository", push.Repository,
			"ref", push.Ref, "commit", push.Commit, "deployments", len(deployed),
		}
		if err != nil {
			slog.Error("github push deploy failed", append(attributes, "error", err)...)
			return
		}
		slog.Info("github push deployed", attributes...)
	}()
	ctx.Status(http.StatusAccepted)
}

// writeGitHubError maps integration failures onto their HTTP shape.
func writeGitHubError(ctx *gin.Context, err error) bool {
	switch {
	case errors.Is(err, github.ErrNotConfigured):
		writeProblem(ctx, http.StatusServiceUnavailable, dto.Problem{
			Code:    "github_not_configured",
			Message: "this control plane holds no GitHub App credentials",
		})
	case errors.Is(err, github.ErrNoWebhookSecret):
		writeProblem(ctx, http.StatusServiceUnavailable, dto.Problem{
			Code:    "github_not_configured",
			Message: "this control plane holds no GitHub webhook secret",
		})
	case errors.Is(err, github.ErrSignature):
		// GitHub's own guidance for a missing or mismatched signature is 403,
		// and GitHub is the only caller this route has.
		writeProblem(ctx, http.StatusForbidden, dto.Problem{
			Code:    "invalid_signature",
			Message: "the delivery signature did not match",
		})
	case errors.Is(err, githubdomain.ErrInvalid):
		invalidRequest(ctx, err.Error())
	case errors.Is(err, githubdomain.ErrNoInstallation):
		writeProblem(ctx, http.StatusConflict, dto.Problem{
			Code:    "github_not_installed",
			Message: "install the GitHub App on this organization first",
		})
	case errors.Is(err, githubdomain.ErrNotConnected):
		writeProblem(ctx, http.StatusConflict, dto.Problem{
			Code:    "app_not_connected",
			Message: "connect this app to a repository first",
		})
	case errors.Is(err, github.ErrNotFound):
		writeProblem(ctx, http.StatusNotFound, dto.Problem{
			Code:    "repository_not_found",
			Message: "the repository or ref does not exist, or the installation cannot see it",
		})
	case errors.Is(err, github.ErrForbidden):
		writeProblem(ctx, http.StatusForbidden, dto.Problem{
			Code:    "repository_forbidden",
			Message: "the installation does not grant access to this repository",
		})
	case errors.Is(err, github.ErrSourceTooBig):
		writeProblem(ctx, http.StatusRequestEntityTooLarge, dto.Problem{
			Code:    "deployment_too_large",
			Message: "the repository exceeds the configured source limit",
		})
	default:
		return false
	}
	return true
}
