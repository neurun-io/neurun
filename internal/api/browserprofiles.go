package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/neurun-io/neurun/internal/domain/browser"
	"github.com/neurun-io/neurun/internal/dto"
)

func (server *Server) listBrowserProfiles(ctx *gin.Context) {
	if !server.browsersConfigured(ctx) {
		return
	}
	limit, ok := server.pageLimit(ctx)
	if !ok {
		return
	}
	records, err := server.browsers.List(
		ctx.Request.Context(), principalOf(ctx).OrganizationID, limit,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{
		"browser_profiles": dto.NewBrowserProfileResponses(records),
	})
}

func (server *Server) createBrowserProfile(ctx *gin.Context) {
	if !server.browsersConfigured(ctx) {
		return
	}
	var body dto.CreateBrowserProfileRequest
	if !server.bindJSON(ctx, &body) {
		return
	}
	kind, err := browser.ParseKind(body.Browser)
	if err != nil {
		writeError(ctx, err)
		return
	}
	record, err := server.browsers.Create(
		ctx.Request.Context(), principalOf(ctx).OrganizationID,
		strings.TrimSpace(body.Name), kind, body.Identity,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Header("Location", "/v1/browser-profiles/"+record.ID)
	ctx.JSON(http.StatusCreated, dto.NewBrowserProfileResponse(record))
}

func (server *Server) getBrowserProfile(ctx *gin.Context) {
	if !server.browsersConfigured(ctx) {
		return
	}
	record, err := server.browsers.Get(
		ctx.Request.Context(), principalOf(ctx).OrganizationID,
		ctx.Param("browser_profile_id"),
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, dto.NewBrowserProfileResponse(record))
}

func (server *Server) updateBrowserProfile(ctx *gin.Context) {
	if !server.browsersConfigured(ctx) {
		return
	}
	var body dto.UpdateBrowserProfileRequest
	if !server.bindJSON(ctx, &body) {
		return
	}
	organizationID := principalOf(ctx).OrganizationID
	profileID := ctx.Param("browser_profile_id")

	record, err := server.browsers.Get(ctx.Request.Context(), organizationID, profileID)
	if err != nil {
		writeError(ctx, err)
		return
	}
	if body.Name != nil {
		record, err = server.browsers.Rename(
			ctx.Request.Context(), organizationID, profileID, *body.Name,
		)
		if err != nil {
			writeError(ctx, err)
			return
		}
	}
	if body.Identity != nil {
		record, err = server.browsers.SetIdentity(
			ctx.Request.Context(), organizationID, profileID, *body.Identity,
		)
		if err != nil {
			writeError(ctx, err)
			return
		}
	}
	ctx.JSON(http.StatusOK, dto.NewBrowserProfileResponse(record))
}

func (server *Server) deleteBrowserProfile(ctx *gin.Context) {
	if !server.browsersConfigured(ctx) {
		return
	}
	err := server.browsers.Delete(
		ctx.Request.Context(), principalOf(ctx).OrganizationID,
		ctx.Param("browser_profile_id"),
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
}

// getBrowserProfileState returns cookie values and storage contents in the
// clear. It sits behind the write scope rather than the read one because
// exporting a profile's state is exporting live credentials.
//
// This is what the SDK reads before it opens a browser on loopback.
func (server *Server) getBrowserProfileState(ctx *gin.Context) {
	if !server.browsersConfigured(ctx) {
		return
	}
	record, err := server.browsers.Get(
		ctx.Request.Context(), principalOf(ctx).OrganizationID,
		ctx.Param("browser_profile_id"),
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, dto.NewBrowserProfileStateResponse(record))
}

// saveBrowserProfileState stores what a closed session captured.
//
// A whole-state replace, not a merge: the browser returns its entire cookie jar,
// so a cookie missing from the body was deleted, and merging would resurrect a
// login the site had already ended.
func (server *Server) saveBrowserProfileState(ctx *gin.Context) {
	if !server.browsersConfigured(ctx) {
		return
	}
	var body dto.SaveBrowserProfileStateRequest
	if !server.bindJSON(ctx, &body) {
		return
	}
	record, err := server.browsers.SaveState(
		ctx.Request.Context(), principalOf(ctx).OrganizationID,
		ctx.Param("browser_profile_id"),
		body.Cookies, body.LocalStorage, body.SessionStorage,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, dto.NewBrowserProfileResponse(record))
}

func (server *Server) browsersConfigured(ctx *gin.Context) bool {
	if server.browsers != nil {
		return true
	}
	writeProblem(ctx, http.StatusServiceUnavailable, dto.Problem{
		Code:    "browser_profiles_unavailable",
		Message: "browser profiles are not configured on this server",
	})
	return false
}

func writeBrowserError(ctx *gin.Context, err error) bool {
	switch {
	case errors.Is(err, browser.ErrNotFound):
		notFound(ctx, "browser profile")
	case errors.Is(err, browser.ErrConflict):
		writeProblem(ctx, http.StatusConflict, dto.Problem{
			Code:    "browser_profile_conflict",
			Message: "the browser profile conflicts with an existing profile",
		})
	case errors.Is(err, browser.ErrInvalid):
		invalidRequest(ctx, err.Error())
	default:
		return false
	}
	return true
}
