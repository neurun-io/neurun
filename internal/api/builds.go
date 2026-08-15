package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/neurun-io/neurun/internal/dto"
)

func (server *Server) listBuilds(ctx *gin.Context) {
	limit, ok := server.pageLimit(ctx)
	if !ok {
		return
	}
	records, err := server.builds.List(
		ctx.Request.Context(), principalOf(ctx).OrganizationID,
		strings.TrimSpace(ctx.Query("app_id")),
		strings.TrimSpace(ctx.Query("deployment_id")), limit,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"builds": dto.NewBuildResponses(records)})
}

func (server *Server) getBuild(ctx *gin.Context) {
	record, err := server.builds.Get(
		ctx.Request.Context(), principalOf(ctx).OrganizationID, ctx.Param("build_id"),
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, dto.NewBuildResponse(record))
}
