package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/neurun-io/neurun/internal/dto"
)

// createExecution runs an app. An execution belongs to the app, not to the
// deployment that happened to build what it runs.
func (server *Server) createExecution(ctx *gin.Context) {
	var body dto.CreateExecutionRequest
	if !server.bindJSON(ctx, &body) {
		return
	}
	if len(body.Input) == 0 {
		invalidRequest(ctx, "input is required")
		return
	}
	record, err := server.executions.Create(
		ctx.Request.Context(), principalOf(ctx).OrganizationID, body,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusAccepted, dto.NewExecutionResponse(record))
}

func (server *Server) listExecutions(ctx *gin.Context) {
	limit, ok := server.pageLimit(ctx)
	if !ok {
		return
	}
	records, err := server.executions.List(
		ctx.Request.Context(), principalOf(ctx).OrganizationID,
		strings.TrimSpace(ctx.Query("project_id")),
		strings.TrimSpace(ctx.Query("deployment_id")),
		limit,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"executions": dto.NewExecutionResponses(records)})
}

func (server *Server) getExecution(ctx *gin.Context) {
	record, err := server.executions.Get(
		ctx.Request.Context(), principalOf(ctx).OrganizationID, ctx.Param("execution_id"),
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, dto.NewExecutionResponse(record))
}

func (server *Server) rerunExecution(ctx *gin.Context) {
	if !requireEmptyBody(ctx) {
		return
	}
	record, err := server.executions.Rerun(
		ctx.Request.Context(), principalOf(ctx).OrganizationID, ctx.Param("execution_id"),
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusAccepted, dto.NewExecutionResponse(record))
}
