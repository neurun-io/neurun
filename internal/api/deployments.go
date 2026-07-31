package api

import (
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/neurun-io/neurun/internal/auth"
	"github.com/neurun-io/neurun/internal/deployment"
)

const deploymentMultipartOverhead = int64(1 << 20)

func (s *Server) deploymentsCollection(w http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case http.MethodGet:
		if s.requireScope(w, request, ScopeDeploymentsRead) {
			s.listDeployments(w, request)
		}
	case http.MethodPost:
		if s.requireScope(w, request, ScopeDeploymentsWrite) {
			s.createDeployment(w, request)
		}
	default:
		methodNotAllowed(w, request, http.MethodGet, http.MethodPost)
	}
}

func (s *Server) listExecutions(w http.ResponseWriter, request *http.Request) {
	limit, ok := s.pageLimit(w, request)
	if !ok {
		return
	}
	principal, _ := auth.FromContext(request.Context())
	executions, err := s.deployments.ListExecutions(
		request.Context(), principal.ProjectID,
		strings.TrimSpace(request.URL.Query().Get("deployment_id")), limit,
	)
	if err != nil {
		s.writeDomainError(w, request, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"executions": executions})
}

func (s *Server) createDeployment(w http.ResponseWriter, request *http.Request) {
	principal, _ := auth.FromContext(request.Context())
	request.Body = http.MaxBytesReader(
		w, request.Body, s.maximumDeploymentBytes+deploymentMultipartOverhead,
	)
	if err := request.ParseMultipartForm(1 << 20); err != nil {
		var maximum *http.MaxBytesError
		status, code := http.StatusBadRequest, "invalid_multipart"
		message := "request must be valid multipart/form-data"
		if errors.As(err, &maximum) || errors.Is(err, multipart.ErrMessageTooLarge) {
			status, code = http.StatusRequestEntityTooLarge, "deployment_too_large"
			message = "deployment upload exceeds the configured source limit"
		}
		WriteProblem(w, request, status, Problem{
			Code: code, Message: message, Details: map[string]any{"cause": err.Error()},
		})
		return
	}
	defer request.MultipartForm.RemoveAll()
	if problem := validateDeploymentForm(request.MultipartForm); problem != "" {
		s.invalidRequest(w, request, problem)
		return
	}
	runtime, err := deployment.ParseRuntime(request.MultipartForm.Value["runtime"][0])
	if err != nil {
		s.writeDomainError(w, request, err)
		return
	}
	entrypoint := ""
	if values := request.MultipartForm.Value["entrypoint"]; len(values) == 1 {
		entrypoint = strings.TrimSpace(values[0])
	}
	header := request.MultipartForm.File["source"][0]
	if header.Size > s.maximumDeploymentBytes {
		WriteProblem(w, request, http.StatusRequestEntityTooLarge, Problem{
			Code:    "deployment_too_large",
			Message: "deployment upload exceeds the configured source limit",
		})
		return
	}
	source, err := header.Open()
	if err != nil {
		WriteProblem(w, request, http.StatusBadRequest, Problem{
			Code: "invalid_multipart", Message: "could not open the uploaded source ZIP",
		})
		return
	}
	defer source.Close()
	created, err := s.deployments.Create(request.Context(), deployment.CreateRequest{
		ProjectID:  principal.ProjectID,
		AppID:      strings.TrimSpace(request.MultipartForm.Value["app_id"][0]),
		Runtime:    runtime,
		EntryPoint: entrypoint,
		SourceName: header.Filename,
		Source:     source,
	})
	if err != nil {
		s.writeDomainError(w, request, err)
		return
	}
	w.Header().Set("Location", "/v1/deployments/"+created.ID)
	WriteJSON(w, http.StatusCreated, created)
}

func (s *Server) listDeployments(w http.ResponseWriter, request *http.Request) {
	principal, _ := auth.FromContext(request.Context())
	limit, ok := s.pageLimit(w, request)
	if !ok {
		return
	}
	records, err := s.deployments.List(
		request.Context(), principal.ProjectID,
		request.URL.Query().Get("app_id"), limit,
	)
	if err != nil {
		s.writeDomainError(w, request, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"deployments": records})
}

func (s *Server) getDeployment(w http.ResponseWriter, request *http.Request) {
	principal, _ := auth.FromContext(request.Context())
	record, err := s.deployments.Get(
		request.Context(), principal.ProjectID, request.PathValue("deployment_id"),
	)
	if err != nil {
		s.writeDomainError(w, request, err)
		return
	}
	WriteJSON(w, http.StatusOK, record)
}

func (s *Server) deploymentRuns(w http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case http.MethodGet:
		if s.requireScope(w, request, ScopeExecutionsRead) {
			s.listRuns(w, request)
		}
	case http.MethodPost:
		if s.requireScope(w, request, ScopeExecutionsWrite) {
			s.createRun(w, request)
		}
	default:
		methodNotAllowed(w, request, http.MethodGet, http.MethodPost)
	}
}

func (s *Server) createRun(w http.ResponseWriter, request *http.Request) {
	var payload map[string]json.RawMessage
	if !DecodeJSON(w, request, &payload, s.maximumBodyBytes) {
		return
	}
	input, exists := payload["input"]
	if !exists || len(payload) != 1 {
		s.invalidRequest(w, request, `request must contain exactly the "input" field`)
		return
	}
	principal, _ := auth.FromContext(request.Context())
	run, err := s.deployments.CreateExecution(request.Context(), deployment.CreateExecutionRequest{
		ProjectID:    principal.ProjectID,
		DeploymentID: request.PathValue("deployment_id"),
		Input:        input,
	})
	if err != nil {
		s.writeDomainError(w, request, err)
		return
	}
	w.Header().Set("Location", "/v1/executions/"+run.ID)
	WriteJSON(w, http.StatusAccepted, run)
}

func (s *Server) listRuns(w http.ResponseWriter, request *http.Request) {
	limit, ok := s.pageLimit(w, request)
	if !ok {
		return
	}
	principal, _ := auth.FromContext(request.Context())
	runs, err := s.deployments.ListDeploymentExecutions(
		request.Context(), principal.ProjectID, request.PathValue("deployment_id"), limit,
	)
	if err != nil {
		s.writeDomainError(w, request, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"executions": runs})
}

func (s *Server) getRun(w http.ResponseWriter, request *http.Request) {
	principal, _ := auth.FromContext(request.Context())
	run, err := s.deployments.GetExecution(
		request.Context(), principal.ProjectID, request.PathValue("execution_id"),
	)
	if err != nil {
		s.writeDomainError(w, request, err)
		return
	}
	WriteJSON(w, http.StatusOK, run)
}

func (s *Server) rerun(w http.ResponseWriter, request *http.Request) {
	if err := requireEmptyBody(request); err != nil {
		s.invalidRequest(w, request, err.Error())
		return
	}
	principal, _ := auth.FromContext(request.Context())
	run, err := s.deployments.RerunExecution(
		request.Context(), principal.ProjectID, request.PathValue("execution_id"),
	)
	if err != nil {
		s.writeDomainError(w, request, err)
		return
	}
	w.Header().Set("Location", "/v1/executions/"+run.ID)
	WriteJSON(w, http.StatusAccepted, run)
}

func validateDeploymentForm(form *multipart.Form) string {
	if form == nil {
		return "multipart form is required"
	}
	for field := range form.Value {
		if field != "app_id" && field != "runtime" && field != "entrypoint" {
			return "multipart form contains an unknown text field"
		}
	}
	for field := range form.File {
		if field != "source" {
			return "multipart form contains an unknown file field"
		}
	}
	if len(form.Value["app_id"]) != 1 ||
		strings.TrimSpace(form.Value["app_id"][0]) == "" {
		return "app_id is required exactly once"
	}
	if len(form.Value["runtime"]) != 1 ||
		strings.TrimSpace(form.Value["runtime"][0]) == "" {
		return "runtime is required exactly once"
	}
	if len(form.Value["entrypoint"]) > 1 {
		return "entrypoint may be supplied at most once"
	}
	if len(form.File["source"]) != 1 {
		return "source ZIP is required exactly once"
	}
	return ""
}
