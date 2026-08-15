package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/neurun-io/neurun/internal/dto"
)

func (server *Server) listApps(ctx *gin.Context) {
	limit, ok := server.pageLimit(ctx)
	if !ok {
		return
	}
	records, err := server.apps.List(
		ctx.Request.Context(), principalOf(ctx).OrganizationID,
		strings.TrimSpace(ctx.Query("project_id")),
		ctx.Query("name"), limit,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"apps": dto.NewAppResponses(records)})
}

func (server *Server) createApp(ctx *gin.Context) {
	var body dto.CreateAppRequest
	if !server.bindJSON(ctx, &body) {
		return
	}
	// An app is only ever created from a repository, so creation goes through
	// the integration that can prove the installation reads it.
	record, err := server.gitHub.CreateApp(ctx.Request.Context(), principalOf(ctx).OrganizationID, body)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusCreated, dto.NewAppResponse(record))
}

func (server *Server) getApp(ctx *gin.Context) {
	record, err := server.apps.Get(
		ctx.Request.Context(), principalOf(ctx).OrganizationID, ctx.Param("app_id"),
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, dto.NewAppResponse(record))
}

func (server *Server) updateApp(ctx *gin.Context) {
	var body dto.UpdateAppRequest
	if !server.bindJSON(ctx, &body) {
		return
	}
	if body.Name == nil {
		invalidRequest(ctx, "app update must include name")
		return
	}
	record, err := server.apps.Update(
		ctx.Request.Context(), principalOf(ctx).OrganizationID, ctx.Param("app_id"), body,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, dto.NewAppResponse(record))
}

// deleteApp destroys an app and the deployments, builds and executions under it.
func (server *Server) deleteApp(ctx *gin.Context) {
	if err := server.apps.Delete(
		ctx.Request.Context(), principalOf(ctx).OrganizationID, ctx.Param("app_id"),
	); err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
}
