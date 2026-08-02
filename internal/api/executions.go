package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/neurun-io/neurun/internal/dto"
)

func (server *Server) createExecution(ctx *gin.Context) {
	var payload map[string]json.RawMessage
	if !server.bindJSON(ctx, &payload) {
		return
	}
	input, exists := payload["input"]
	if !exists || len(payload) != 1 {
		invalidRequest(ctx, `request must contain exactly the "input" field`)
		return
	}
	record, err := server.executions.Create(ctx.Request.Context(), dto.CreateExecutionRequest{
		DeploymentID: ctx.Param("deployment_id"),
		Input:        input,
	})
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Header("Location", "/v1/executions/"+record.ID)
	ctx.JSON(http.StatusAccepted, dto.NewExecutionResponse(record))
}

func (server *Server) listDeploymentExecutions(ctx *gin.Context) {
	limit, ok := server.pageLimit(ctx)
	if !ok {
		return
	}
	records, err := server.executions.ListForDeployment(
		ctx.Request.Context(), ctx.Param("deployment_id"), limit,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"executions": dto.NewExecutionResponses(records)})
}

func (server *Server) listExecutions(ctx *gin.Context) {
	limit, ok := server.pageLimit(ctx)
	if !ok {
		return
	}
	records, err := server.executions.List(
		ctx.Request.Context(),
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
		ctx.Request.Context(), ctx.Param("execution_id"),
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
		ctx.Request.Context(), ctx.Param("execution_id"),
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Header("Location", "/v1/executions/"+record.ID)
	ctx.JSON(http.StatusAccepted, dto.NewExecutionResponse(record))
}
