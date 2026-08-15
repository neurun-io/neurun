package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/neurun-io/neurun/internal/dto"
)

func (server *Server) listProjects(ctx *gin.Context) {
	limit, ok := server.pageLimit(ctx)
	if !ok {
		return
	}
	records, err := server.projects.List(
		ctx.Request.Context(), principalOf(ctx).OrganizationID, limit,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"projects": dto.NewProjectResponses(records)})
}

func (server *Server) createProject(ctx *gin.Context) {
	var body dto.CreateProjectRequest
	if !server.bindJSON(ctx, &body) {
		return
	}
	record, err := server.projects.Create(
		ctx.Request.Context(), principalOf(ctx).OrganizationID, body.Name,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusCreated, dto.NewProjectResponse(record))
}

func (server *Server) getProject(ctx *gin.Context) {
	record, err := server.projects.Get(
		ctx.Request.Context(), principalOf(ctx).OrganizationID, ctx.Param("project_id"),
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, dto.NewProjectResponse(record))
}

func (server *Server) updateProject(ctx *gin.Context) {
	var body dto.UpdateProjectRequest
	if !server.bindJSON(ctx, &body) {
		return
	}
	record, err := server.projects.Update(
		ctx.Request.Context(), principalOf(ctx).OrganizationID, ctx.Param("project_id"), body,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, dto.NewProjectResponse(record))
}

// deleteProject destroys a project and every app, deployment, build and
// execution beneath it. Users and API keys are not touched — they belong to the
// install, not to a project.
func (server *Server) deleteProject(ctx *gin.Context) {
	if err := server.projects.Delete(
		ctx.Request.Context(), principalOf(ctx).OrganizationID, ctx.Param("project_id"),
	); err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
}
