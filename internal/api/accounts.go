package api

import (
	"errors"
	"net/http"

	"github.com/neurun-io/neurun/internal/domain/account"
	"github.com/neurun-io/neurun/internal/domain/auth"
)

type updateUserBody struct {
	DisplayName *string `json:"display_name"`
	Role        *string `json:"role"`
	Disabled    *bool   `json:"disabled"`
}

func (s *Server) usersCollection(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.FromContext(r.Context())
	switch r.Method {
	case http.MethodGet:
		if !s.requireScope(w, r, ScopeUsersRead) {
			return
		}
		limit, ok := s.pageLimit(w, r)
		if !ok {
			return
		}
		users, err := s.accounts.ListUsers(r.Context(), principal.ProjectID, limit)
		if err != nil {
			s.writeAccountError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"users": users})
	case http.MethodPost:
		if !s.requireScope(w, r, ScopeUsersWrite) {
			return
		}
		var body struct {
			Username    string `json:"username"`
			DisplayName string `json:"display_name"`
			Role        string `json:"role"`
			Password    string `json:"password"`
		}
		if !DecodeJSON(w, r, &body, s.maximumBodyBytes) {
			return
		}
		user, err := s.accounts.CreateUser(r.Context(), account.CreateUserRequest{
			ProjectID: principal.ProjectID, Username: body.Username,
			DisplayName: body.DisplayName, Role: body.Role, Password: body.Password,
		})
		if err != nil {
			s.writeAccountError(w, r, err)
			return
		}
		w.Header().Set("Location", "/v1/users/"+user.ID)
		WriteJSON(w, http.StatusCreated, user)
	default:
		methodNotAllowed(w, r, http.MethodGet, http.MethodPost)
	}
}

func (s *Server) userItem(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.FromContext(r.Context())
	switch r.Method {
	case http.MethodGet:
		if !s.requireScope(w, r, ScopeUsersRead) {
			return
		}
		user, err := s.accounts.GetUser(r.Context(), principal.ProjectID, r.PathValue("user_id"))
		if err != nil {
			s.writeAccountError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, user)
	case http.MethodPatch:
		if !s.requireScope(w, r, ScopeUsersWrite) {
			return
		}
		var body updateUserBody
		if !DecodeJSON(w, r, &body, s.maximumBodyBytes) {
			return
		}
		if body.DisplayName == nil && body.Role == nil && body.Disabled == nil {
			s.invalidRequest(w, r, "user update must include at least one field")
			return
		}
		user, err := s.accounts.UpdateUser(r.Context(), principal.ProjectID,
			r.PathValue("user_id"), account.UpdateUserRequest{
				DisplayName: body.DisplayName, Role: body.Role, Disabled: body.Disabled,
			})
		if err != nil {
			s.writeAccountError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, user)
	default:
		methodNotAllowed(w, r, http.MethodGet, http.MethodPatch)
	}
}

func (s *Server) apiKeysCollection(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.FromContext(r.Context())
	switch r.Method {
	case http.MethodGet:
		if !s.requireScope(w, r, ScopeAPIKeysRead) {
			return
		}
		limit, ok := s.pageLimit(w, r)
		if !ok {
			return
		}
		keys, err := s.accounts.ListKeys(r.Context(), principal.ProjectID, limit)
		if err != nil {
			s.writeAccountError(w, r, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"api_keys": keys})
	case http.MethodPost:
		if !s.requireScope(w, r, ScopeAPIKeysWrite) {
			return
		}
		var body struct {
			Name   string   `json:"name"`
			UserID string   `json:"user_id"`
			Scopes []string `json:"scopes"`
		}
		if !DecodeJSON(w, r, &body, s.maximumBodyBytes) {
			return
		}
		key, err := s.accounts.CreateKey(r.Context(), account.CreateKeyRequest{
			ProjectID: principal.ProjectID, UserID: body.UserID,
			Name: body.Name, Scopes: body.Scopes,
		})
		if err != nil {
			s.writeAccountError(w, r, err)
			return
		}
		w.Header().Set("Location", "/v1/api-keys/"+key.ID)
		WriteJSON(w, http.StatusCreated, key)
	default:
		methodNotAllowed(w, r, http.MethodGet, http.MethodPost)
	}
}

func (s *Server) revokeAPIKey(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.FromContext(r.Context())
	key, err := s.accounts.RevokeKey(r.Context(), principal.ProjectID, r.PathValue("api_key_id"))
	if err != nil {
		s.writeAccountError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, key)
}

func (s *Server) writeAccountError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, account.ErrNotFound):
		s.resourceNotFound(w, r, "resource")
	case errors.Is(err, account.ErrInvalid):
		s.invalidRequest(w, r, err.Error())
	case errors.Is(err, account.ErrConflict):
		WriteProblem(w, r, http.StatusConflict, Problem{
			Code: "resource_conflict", Message: "the resource conflicts with an existing record",
		})
	default:
		WriteProblem(w, r, http.StatusInternalServerError, Problem{
			Code: "internal_error", Message: "the server could not complete the request",
		})
	}
}
