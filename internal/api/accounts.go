package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/neurun-io/neurun/internal/dto"
)

func (server *Server) listUsers(ctx *gin.Context) {
	limit, ok := server.pageLimit(ctx)
	if !ok {
		return
	}
	records, err := server.accounts.ListUsers(ctx.Request.Context(), limit)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"users": dto.NewUserResponses(records)})
}

func (server *Server) createUser(ctx *gin.Context) {
	var body dto.CreateUserRequest
	if !server.bindJSON(ctx, &body) {
		return
	}
	record, err := server.accounts.CreateUser(ctx.Request.Context(), body)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Header("Location", "/v1/users/"+record.ID)
	ctx.JSON(http.StatusCreated, dto.NewUserResponse(record))
}

func (server *Server) getUser(ctx *gin.Context) {
	record, err := server.accounts.GetUser(ctx.Request.Context(), ctx.Param("user_id"))
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, dto.NewUserResponse(record))
}

func (server *Server) updateUser(ctx *gin.Context) {
	var body dto.UpdateUserRequest
	if !server.bindJSON(ctx, &body) {
		return
	}
	if body.DisplayName == nil && body.Role == nil && body.Disabled == nil {
		invalidRequest(ctx, "user update must include at least one field")
		return
	}
	record, err := server.accounts.UpdateUser(
		ctx.Request.Context(), ctx.Param("user_id"), body,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, dto.NewUserResponse(record))
}

// deleteUser removes a person and nothing else. Keys they minted keep working
// with their attribution cleared, and every project resource stands.
func (server *Server) deleteUser(ctx *gin.Context) {
	if err := server.accounts.DeleteUser(ctx.Request.Context(), ctx.Param("user_id")); err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (server *Server) listAPIKeys(ctx *gin.Context) {
	limit, ok := server.pageLimit(ctx)
	if !ok {
		return
	}
	records, err := server.accounts.ListKeys(ctx.Request.Context(), limit)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"api_keys": dto.NewKeyResponses(records)})
}

func (server *Server) createAPIKey(ctx *gin.Context) {
	var body dto.CreateKeyRequest
	if !server.bindJSON(ctx, &body) {
		return
	}
	// A key may only be granted scopes the caller already holds, so a limited
	// key cannot mint an unlimited one.
	principal := principalOf(ctx)
	for _, scope := range body.Scopes {
		if !principal.HasScope(scope) {
			writeProblem(ctx, http.StatusForbidden, dto.Problem{
				Code:    "permission_denied",
				Message: "a key cannot be granted a scope the caller does not hold",
				Details: map[string]any{"scope": scope},
			})
			return
		}
	}
	record, err := server.accounts.CreateKey(ctx.Request.Context(), body)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Header("Location", "/v1/api-keys/"+record.ID)
	ctx.JSON(http.StatusCreated, dto.NewCreatedKeyResponse(record))
}

func (server *Server) revokeAPIKey(ctx *gin.Context) {
	record, err := server.accounts.RevokeKey(
		ctx.Request.Context(), ctx.Param("api_key_id"),
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, dto.NewKeyResponse(record))
}
