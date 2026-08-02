package api

import (
	"net/http"
	"strings"

	"github.com/neurun-io/neurun/internal/domain/auth"
	"github.com/neurun-io/neurun/internal/domain/deployment"
)

func (s *Server) listProjects(w http.ResponseWriter, request *http.Request) {
	limit, ok := s.pageLimit(w, request)
	if !ok {
		return
	}
	principal, _ := auth.FromContext(request.Context())
	projects, err := s.deployments.ListProjects(
		request.Context(), principal.ProjectID, limit,
	)
	if err != nil {
		s.writeDomainError(w, request, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"projects": projects})
}

func (s *Server) projectItem(w http.ResponseWriter, request *http.Request) {
	principal, _ := auth.FromContext(request.Context())
	projectID := request.PathValue("project_id")
	if projectID != principal.ProjectID {
		s.resourceNotFound(w, request, "project")
		return
	}
	switch request.Method {
	case http.MethodGet:
		if !s.requireScope(w, request, ScopeProjectsRead) {
			return
		}
		project, err := s.deployments.GetProject(request.Context(), projectID)
		if err != nil {
			s.writeDomainError(w, request, err)
			return
		}
		WriteJSON(w, http.StatusOK, project)
	case http.MethodPatch:
		if !s.requireScope(w, request, ScopeProjectsWrite) {
			return
		}
		var body struct {
			Name *string `json:"name"`
		}
		if !DecodeJSON(w, request, &body, s.maximumBodyBytes) {
			return
		}
		project, err := s.deployments.UpdateProject(
			request.Context(), projectID,
			deployment.UpdateProjectRequest{Name: body.Name},
		)
		if err != nil {
			s.writeDomainError(w, request, err)
			return
		}
		WriteJSON(w, http.StatusOK, project)
	default:
		methodNotAllowed(w, request, http.MethodGet, http.MethodPatch)
	}
}

func (s *Server) listBuilds(w http.ResponseWriter, request *http.Request) {
	limit, ok := s.pageLimit(w, request)
	if !ok {
		return
	}
	principal, _ := auth.FromContext(request.Context())
	builds, err := s.deployments.ListBuilds(
		request.Context(), principal.ProjectID,
		strings.TrimSpace(request.URL.Query().Get("deployment_id")), limit,
	)
	if err != nil {
		s.writeDomainError(w, request, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"builds": builds})
}

func (s *Server) getBuild(w http.ResponseWriter, request *http.Request) {
	principal, _ := auth.FromContext(request.Context())
	build, err := s.deployments.GetBuild(
		request.Context(), principal.ProjectID, request.PathValue("build_id"),
	)
	if err != nil {
		s.writeDomainError(w, request, err)
		return
	}
	WriteJSON(w, http.StatusOK, build)
}
