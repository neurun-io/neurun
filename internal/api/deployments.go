package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/neurun-io/neurun/internal/dto"
)

func (server *Server) listDeployments(ctx *gin.Context) {
	limit, ok := server.pageLimit(ctx)
	if !ok {
		return
	}
	records, err := server.deployments.List(
		ctx.Request.Context(), principalOf(ctx).OrganizationID,
		strings.TrimSpace(ctx.Query("project_id")),
		strings.TrimSpace(ctx.Query("app_id")),
		limit,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"deployments": dto.NewDeploymentResponses(records)})
}

func (server *Server) getDeployment(ctx *gin.Context) {
	record, err := server.deployments.Get(
		ctx.Request.Context(), principalOf(ctx).OrganizationID, ctx.Param("deployment_id"),
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, dto.NewDeploymentResponse(record))
}

// retryDeployment builds the same commit again, as a new deployment: the one
// that failed keeps its logs and its failure.
func (server *Server) retryDeployment(ctx *gin.Context) {
	record, err := server.gitHub.Retry(
		ctx.Request.Context(), principalOf(ctx).OrganizationID,
		ctx.Param("deployment_id"),
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusAccepted, dto.NewDeploymentResponse(record))
}
