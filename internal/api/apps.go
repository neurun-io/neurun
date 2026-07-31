package api

import (
	"net/http"

	"github.com/neurun-io/neurun/internal/auth"
	"github.com/neurun-io/neurun/internal/deployment"
)

func (s *Server) appsCollection(w http.ResponseWriter, request *http.Request) {
	principal, _ := auth.FromContext(request.Context())
	switch request.Method {
	case http.MethodGet:
		if !s.requireScope(w, request, ScopeAppsRead) {
			return
		}
		limit, ok := s.pageLimit(w, request)
		if !ok {
			return
		}
		apps, err := s.deployments.ListApps(
			request.Context(), principal.ProjectID,
			request.URL.Query().Get("name"), limit,
		)
		if err != nil {
			s.writeDomainError(w, request, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"apps": apps})
	case http.MethodPost:
		if !s.requireScope(w, request, ScopeAppsWrite) {
			return
		}
		var body struct {
			Name string `json:"name"`
		}
		if !DecodeJSON(w, request, &body, s.maximumBodyBytes) {
			return
		}
		app, err := s.deployments.CreateApp(request.Context(), deployment.CreateAppRequest{
			ProjectID: principal.ProjectID, Name: body.Name,
		})
		if err != nil {
			s.writeDomainError(w, request, err)
			return
		}
		w.Header().Set("Location", "/v1/apps/"+app.ID)
		WriteJSON(w, http.StatusCreated, app)
	default:
		methodNotAllowed(w, request, http.MethodGet, http.MethodPost)
	}
}

func (s *Server) appItem(w http.ResponseWriter, request *http.Request) {
	principal, _ := auth.FromContext(request.Context())
	appID := request.PathValue("app_id")
	switch request.Method {
	case http.MethodGet:
		if !s.requireScope(w, request, ScopeAppsRead) {
			return
		}
		app, err := s.deployments.GetApp(request.Context(), principal.ProjectID, appID)
		if err != nil {
			s.writeDomainError(w, request, err)
			return
		}
		WriteJSON(w, http.StatusOK, app)
	case http.MethodPatch:
		if !s.requireScope(w, request, ScopeAppsWrite) {
			return
		}
		var body struct {
			Name *string `json:"name"`
		}
		if !DecodeJSON(w, request, &body, s.maximumBodyBytes) {
			return
		}
		if body.Name == nil {
			s.invalidRequest(w, request, "app update must include name")
			return
		}
		app, err := s.deployments.UpdateApp(
			request.Context(), principal.ProjectID, appID,
			deployment.UpdateAppRequest{Name: body.Name},
		)
		if err != nil {
			s.writeDomainError(w, request, err)
			return
		}
		WriteJSON(w, http.StatusOK, app)
	default:
		methodNotAllowed(w, request, http.MethodGet, http.MethodPatch)
	}
}
