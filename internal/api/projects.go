package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/neurun-io/neurun/internal/dto"
)

// confirmedByName gates an irreversible cascade behind typing the resource's
// name, the way GitHub and AWS do.
//
// The name, not the identifier: the identifier is already in the URL, so
// echoing it proves nothing about intent. The expected value is deliberately
// absent from the error — a response that told you what to type would remove
// the only friction this has.
func confirmedByName(ctx *gin.Context, name string) bool {
	if strings.TrimSpace(ctx.Query("confirm")) == name {
		return true
	}
	writeProblem(ctx, http.StatusUnprocessableEntity, dto.Problem{
		Code: "confirmation_required",
		Message: "this deletion cannot be undone; repeat the resource's exact " +
			`name in the "confirm" query parameter to proceed`,
		Details: map[string]any{"expected_parameter": "confirm"},
	})
	return false
}

func (server *Server) listProjects(ctx *gin.Context) {
	limit, ok := server.pageLimit(ctx)
	if !ok {
		return
	}
	records, err := server.deployments.ListProjects(ctx.Request.Context(), limit)
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
	record, err := server.deployments.CreateProject(ctx.Request.Context(), body.Name)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Header("Location", "/v1/projects/"+record.ID)
	ctx.JSON(http.StatusCreated, dto.NewProjectResponse(record))
}

func (server *Server) getProject(ctx *gin.Context) {
	record, err := server.deployments.GetProject(
		ctx.Request.Context(), ctx.Param("project_id"),
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
	record, err := server.deployments.UpdateProject(
		ctx.Request.Context(), ctx.Param("project_id"), body,
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
	projectID := ctx.Param("project_id")
	record, err := server.deployments.GetProject(ctx.Request.Context(), projectID)
	if err != nil {
		writeError(ctx, err)
		return
	}
	if !confirmedByName(ctx, record.Name) {
		return
	}
	if err := server.deployments.DeleteProject(ctx.Request.Context(), projectID); err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (server *Server) listApps(ctx *gin.Context) {
	limit, ok := server.pageLimit(ctx)
	if !ok {
		return
	}
	records, err := server.deployments.ListApps(
		ctx.Request.Context(),
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
	record, err := server.deployments.CreateApp(ctx.Request.Context(), body)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Header("Location", "/v1/apps/"+record.ID)
	ctx.JSON(http.StatusCreated, dto.NewAppResponse(record))
}

func (server *Server) getApp(ctx *gin.Context) {
	record, err := server.deployments.GetApp(ctx.Request.Context(), ctx.Param("app_id"))
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
	record, err := server.deployments.UpdateApp(
		ctx.Request.Context(), ctx.Param("app_id"), body,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, dto.NewAppResponse(record))
}

// deleteApp destroys an app and the deployments, builds and executions under it.
func (server *Server) deleteApp(ctx *gin.Context) {
	appID := ctx.Param("app_id")
	record, err := server.deployments.GetApp(ctx.Request.Context(), appID)
	if err != nil {
		writeError(ctx, err)
		return
	}
	if !confirmedByName(ctx, record.Name) {
		return
	}
	if err := server.deployments.DeleteApp(ctx.Request.Context(), appID); err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (server *Server) listBuilds(ctx *gin.Context) {
	limit, ok := server.pageLimit(ctx)
	if !ok {
		return
	}
	records, err := server.deployments.ListBuilds(
		ctx.Request.Context(),
		strings.TrimSpace(ctx.Query("deployment_id")), limit,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"builds": dto.NewBuildResponses(records)})
}

func (server *Server) getBuild(ctx *gin.Context) {
	record, err := server.deployments.GetBuild(
		ctx.Request.Context(), ctx.Param("build_id"),
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, dto.NewBuildResponse(record))
}
