package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/neurun-io/neurun/internal/domain/deployment"
	"github.com/neurun-io/neurun/internal/dto"
	"github.com/neurun-io/neurun/internal/github"
)

type installRequest struct {
	InstallationID string `json:"installation_id"`
	AccountLogin   string `json:"account_login"`
}

// recordInstallation stores the installation GitHub redirects back with after
// somebody installs the app on their account.
func (server *Server) recordInstallation(ctx *gin.Context) {
	var body installRequest
	if !server.bindJSON(ctx, &body) {
		return
	}
	installationID, err := strconv.ParseInt(strings.TrimSpace(body.InstallationID), 10, 64)
	if err != nil || installationID <= 0 {
		invalidRequest(ctx, "installation_id must be a positive integer")
		return
	}
	record, err := server.gitHub.Install(
		ctx.Request.Context(), principalOf(ctx).OrganizationID,
		installationID, body.AccountLogin,
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
		body.AppID, body.Ref, deployment.RuntimePython, "",
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Header("Location", "/v1/deployments/"+record.ID)
	ctx.JSON(http.StatusCreated, dto.NewDeploymentResponse(record))
}

// writeGitHubError maps integration failures onto their HTTP shape.
func writeGitHubError(ctx *gin.Context, err error) bool {
	switch {
	case errors.Is(err, github.ErrNotConfigured):
		writeProblem(ctx, http.StatusServiceUnavailable, dto.Problem{
			Code:    "github_not_configured",
			Message: "this control plane holds no GitHub App credentials",
		})
	case errors.Is(err, deployment.ErrNoInstallation):
		writeProblem(ctx, http.StatusConflict, dto.Problem{
			Code:    "github_not_installed",
			Message: "install the GitHub App on this organization first",
		})
	case errors.Is(err, deployment.ErrNotConnected):
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
