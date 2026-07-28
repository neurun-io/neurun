package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dagflows/neurun-io/internal/auth"
	"github.com/dagflows/neurun-io/internal/buildinfo"
	"github.com/dagflows/neurun-io/internal/function"
	"github.com/dagflows/neurun-io/internal/job"
)

const (
	ScopeFunctionsRead   = "functions:read"
	ScopeFunctionsInvoke = "functions:invoke"
	ScopeJobsRead        = "jobs:read"
	ScopeJobsWrite       = "jobs:write"

	defaultMaximumBodyBytes = int64(1 << 20)
	defaultPageSize         = 50
	maximumPageSize         = 200
)

// InvocationService is the direct invocation port used by the HTTP transport.
// function.Service satisfies this interface.
type InvocationService interface {
	Invoke(context.Context, function.InvocationRequest) (function.Invocation, error)
	Get(string) (function.Invocation, error)
	List() []function.Invocation
	Cancel(string) error
}

// JobService is the project-scoped durable job port used by the HTTP
// transport. job.Repository satisfies this interface.
type JobService interface {
	job.JobReader
	Accept(context.Context, job.AcceptCommand) (job.AcceptResult, error)
	Cancel(context.Context, string, string, string) (job.CancelResult, error)
}

type ReadyCheck func(context.Context) error

type JobDurability string

const (
	JobDurabilityDurable      JobDurability = "durable"
	JobDurabilityProcessLocal JobDurability = "process_local"
)

var ErrAsyncJobsUnavailable = errors.New("asynchronous jobs require a durable backend")

type ServerOptions struct {
	Authenticator    *auth.Authenticator
	Registry         *function.Registry
	Invocations      InvocationService
	Jobs             JobService
	Ready            ReadyCheck
	BundleVersion    string
	MaximumBodyBytes int64
	JobDurability    JobDurability
	AllowAsyncJobs   bool
}

// Server exposes the public health endpoints and authenticated v1 control
// plane. It implements http.Handler directly.
type Server struct {
	registry         *function.Registry
	invocations      InvocationService
	jobs             JobService
	ready            ReadyCheck
	bundleVersion    string
	maximumBodyBytes int64
	jobDurability    JobDurability
	allowAsyncJobs   bool
	handler          http.Handler
}

func NewServer(options ServerOptions) (*Server, error) {
	switch {
	case options.Authenticator == nil:
		return nil, errors.New("API authenticator is required")
	case options.Registry == nil:
		return nil, errors.New("function registry is required")
	case options.Invocations == nil:
		return nil, errors.New("invocation service is required")
	case options.Jobs == nil:
		return nil, errors.New("job service is required")
	case options.MaximumBodyBytes < 0:
		return nil, errors.New("maximum request body bytes cannot be negative")
	}

	bundleVersion := strings.TrimSpace(options.BundleVersion)
	if bundleVersion == "" {
		bundleVersion = function.BuiltinBundleVersion
	}
	if _, err := options.Registry.Bundle(bundleVersion); err != nil {
		return nil, fmt.Errorf("function bundle: %w", err)
	}
	maximumBodyBytes := options.MaximumBodyBytes
	if maximumBodyBytes == 0 {
		maximumBodyBytes = defaultMaximumBodyBytes
	}
	jobDurability := options.JobDurability
	if jobDurability == "" {
		// Fail closed when an embedding does not declare its persistence
		// guarantee. Process-local jobs require a second, explicit opt-in
		// through AllowAsyncJobs.
		jobDurability = JobDurabilityProcessLocal
	}
	if jobDurability != JobDurabilityDurable &&
		jobDurability != JobDurabilityProcessLocal {
		return nil, fmt.Errorf("unsupported job durability %q", jobDurability)
	}

	server := &Server{
		registry:         options.Registry,
		invocations:      options.Invocations,
		jobs:             options.Jobs,
		ready:            options.Ready,
		bundleVersion:    bundleVersion,
		maximumBodyBytes: maximumBodyBytes,
		jobDurability:    jobDurability,
		allowAsyncJobs: options.AllowAsyncJobs ||
			jobDurability == JobDurabilityDurable,
	}
	server.handler = server.routes(options.Authenticator)
	return server, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	s.handler.ServeHTTP(w, request)
}

func (s *Server) routes(authenticator *auth.Authenticator) http.Handler {
	protected := http.NewServeMux()
	protected.Handle("/v1/functions", s.scoped(
		ScopeFunctionsRead,
		s.only(http.MethodGet, http.HandlerFunc(s.listFunctions)),
	))
	protected.Handle("/v1/functions/{name}", s.scoped(
		ScopeFunctionsRead,
		s.only(http.MethodGet, http.HandlerFunc(s.getFunction)),
	))
	protected.Handle("/v1/functions/{name}/versions/{version}", s.scoped(
		ScopeFunctionsRead,
		s.only(http.MethodGet, http.HandlerFunc(s.getFunctionVersion)),
	))
	protected.Handle("/v1/functions/{name}/invoke", s.scoped(
		ScopeFunctionsInvoke,
		s.only(http.MethodPost, http.HandlerFunc(s.invokeFunction)),
	))
	protected.Handle("/v1/function-manifest-bundle", s.scoped(
		ScopeFunctionsRead,
		s.only(http.MethodGet, http.HandlerFunc(s.functionBundle)),
	))
	protected.Handle("/v1/function-invocations", s.scoped(
		ScopeFunctionsRead,
		s.only(http.MethodGet, http.HandlerFunc(s.listInvocations)),
	))
	protected.Handle("/v1/function-invocations/{invocation_id}", s.scoped(
		ScopeFunctionsRead,
		s.only(http.MethodGet, http.HandlerFunc(s.getInvocation)),
	))
	protected.Handle("/v1/function-invocations/{invocation_id}/cancel", s.scoped(
		ScopeFunctionsInvoke,
		s.only(http.MethodPost, http.HandlerFunc(s.cancelInvocation)),
	))
	protected.Handle("/v1/jobs", http.HandlerFunc(s.jobsCollection))
	protected.Handle("/v1/jobs/{job_id}", s.scoped(
		ScopeJobsRead,
		s.only(http.MethodGet, http.HandlerFunc(s.getJob)),
	))
	protected.Handle("/v1/jobs/{job_id}/events", s.scoped(
		ScopeJobsRead,
		s.only(http.MethodGet, http.HandlerFunc(s.getJobEvents)),
	))
	protected.Handle("/v1/jobs/{job_id}/attempts", s.scoped(
		ScopeJobsRead,
		s.only(http.MethodGet, http.HandlerFunc(s.getJobAttempts)),
	))
	protected.Handle("/v1/jobs/{job_id}/cancel", s.scoped(
		ScopeJobsWrite,
		s.only(http.MethodPost, http.HandlerFunc(s.cancelJob)),
	))
	protected.Handle("/v1/fetch", s.scoped(
		ScopeFunctionsInvoke,
		s.only(http.MethodPost, http.HandlerFunc(s.fetch)),
	))
	protected.Handle("/", http.HandlerFunc(notFound))

	root := http.NewServeMux()
	root.Handle("/healthz", s.only(http.MethodGet, http.HandlerFunc(s.health)))
	root.Handle("/readyz", s.only(http.MethodGet, http.HandlerFunc(s.readiness)))
	root.Handle("/version", s.only(http.MethodGet, http.HandlerFunc(s.version)))
	root.Handle("/v1/", authenticator.Middleware(protected))
	root.Handle("/", http.HandlerFunc(notFound))

	return RequestIDMiddleware(SecurityHeaders(Recoverer(root)))
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) readiness(w http.ResponseWriter, request *http.Request) {
	if s.ready != nil {
		if err := s.ready(request.Context()); err != nil {
			WriteProblem(w, request, http.StatusServiceUnavailable, Problem{
				Code:    "not_ready",
				Message: "the server is not ready to accept work",
			})
			return
		}
	}
	WriteJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) version(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, buildinfo.Current())
}

func (s *Server) listFunctions(w http.ResponseWriter, request *http.Request) {
	category := strings.TrimSpace(request.URL.Query().Get("category"))
	capability := strings.TrimSpace(request.URL.Query().Get("capability"))
	status := strings.TrimSpace(request.URL.Query().Get("status"))
	if status != "" && status != "available" {
		s.invalidQuery(w, request, "status must be available")
		return
	}
	manifests := s.registry.List()
	filtered := make([]function.Manifest, 0, len(manifests))
	for _, manifest := range manifests {
		if category != "" && manifest.Category != category {
			continue
		}
		if capability != "" && !contains(manifest.Capabilities, capability) {
			continue
		}
		filtered = append(filtered, manifest)
	}
	WriteJSON(w, http.StatusOK, map[string]any{"functions": filtered})
}

func (s *Server) getFunction(w http.ResponseWriter, request *http.Request) {
	name := request.PathValue("name")
	manifests := s.registry.List()
	versions := make([]function.Manifest, 0)
	for _, manifest := range manifests {
		if manifest.Name == name {
			versions = append(versions, manifest)
		}
	}
	if len(versions) == 0 {
		s.resourceNotFound(w, request, "function")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"name":     name,
		"versions": versions,
	})
}

func (s *Server) getFunctionVersion(w http.ResponseWriter, request *http.Request) {
	manifest, err := s.registry.Manifest(
		request.PathValue("name"),
		request.PathValue("version"),
	)
	if err != nil {
		s.writeDomainError(w, request, err)
		return
	}
	WriteJSON(w, http.StatusOK, manifest)
}

func (s *Server) functionBundle(w http.ResponseWriter, request *http.Request) {
	bundle, err := s.registry.Bundle(s.bundleVersion)
	if err != nil {
		s.writeDomainError(w, request, err)
		return
	}
	WriteJSON(w, http.StatusOK, bundle)
}

type invokeRequest struct {
	ProjectID   string                     `json:"project_id,omitempty"`
	Version     string                     `json:"version,omitempty"`
	Digest      string                     `json:"digest,omitempty"`
	Execution   string                     `json:"execution,omitempty"`
	Context     *function.ExecutionContext `json:"context,omitempty"`
	Input       json.RawMessage            `json:"input"`
	TimeoutMS   int64                      `json:"timeout_ms,omitempty"`
	MaxAttempts int                        `json:"max_attempts,omitempty"`
}

func (s *Server) invokeFunction(w http.ResponseWriter, request *http.Request) {
	var payload invokeRequest
	if !DecodeJSON(w, request, &payload, s.maximumBodyBytes) {
		return
	}
	s.executeInvocation(w, request, request.PathValue("name"), payload, false)
}

func (s *Server) executeInvocation(
	w http.ResponseWriter,
	request *http.Request,
	name string,
	payload invokeRequest,
	trustedContext bool,
) {
	principal, ok := s.scopedProject(w, request, payload.ProjectID)
	if !ok {
		return
	}
	if len(bytes.TrimSpace(payload.Input)) == 0 {
		s.invalidRequest(w, request, "input is required")
		return
	}
	executionContext, ok := s.executionContext(
		w,
		request,
		principal.ProjectID,
		payload.Context,
		trustedContext,
	)
	if !ok {
		return
	}
	execution := strings.TrimSpace(payload.Execution)
	if execution == "" {
		execution = "sync"
	}

	switch execution {
	case "sync":
		invocation, err := s.invocations.Invoke(request.Context(), function.InvocationRequest{
			ProjectID: principal.ProjectID,
			Function: function.FunctionRef{
				Name:    name,
				Version: payload.Version,
				Digest:  payload.Digest,
			},
			Context:   executionContext,
			Input:     payload.Input,
			TimeoutMS: payload.TimeoutMS,
		})
		if err != nil {
			var invocationError *function.InvocationError
			if errors.As(err, &invocationError) {
				WriteProblem(w, request, invocationFailureStatus(invocationError.Failure), Problem{
					Code:    invocationFailureCode(invocationError.Failure),
					Message: invocationError.Failure.Message,
					Details: map[string]any{"invocation": invocation},
				})
				return
			}
			s.writeDomainError(w, request, err)
			return
		}
		s.writeInvocation(w, request, http.StatusOK, invocation)
	case "async":
		if !s.requireScope(w, request, ScopeJobsWrite) {
			return
		}
		if payload.Context != nil || payload.TimeoutMS != 0 {
			s.invalidRequest(
				w,
				request,
				"context and timeout_ms are not supported by the current durable job request contract",
			)
			return
		}
		result, err := s.acceptJob(
			request,
			principal.ProjectID,
			function.FunctionRef{Name: name, Version: payload.Version, Digest: payload.Digest},
			payload.Input,
			payload.MaxAttempts,
		)
		if err != nil {
			s.writeDomainError(w, request, err)
			return
		}
		s.writeAcceptedJob(w, request, result)
	default:
		s.invalidRequest(w, request, "execution must be sync or async")
	}
}

func (s *Server) listInvocations(w http.ResponseWriter, request *http.Request) {
	principal, _ := auth.FromContext(request.Context())
	query := request.URL.Query()
	status := function.InvocationStatus(strings.TrimSpace(query.Get("status")))
	if status != "" && !validInvocationStatus(status) {
		s.invalidQuery(w, request, "status is not a recognized invocation status")
		return
	}
	limit, ok := s.pageLimit(w, request)
	if !ok {
		return
	}
	name := strings.TrimSpace(query.Get("function"))
	version := strings.TrimSpace(query.Get("version"))

	filtered := make([]function.Invocation, 0)
	for _, invocation := range s.invocations.List() {
		if invocation.ProjectID != principal.ProjectID {
			continue
		}
		if name != "" && invocation.Function.Name != name {
			continue
		}
		if version != "" && invocation.Function.Version != version {
			continue
		}
		if status != "" && invocation.Status != status {
			continue
		}
		filtered = append(filtered, invocation)
	}

	start := 0
	if cursor := strings.TrimSpace(query.Get("cursor")); cursor != "" {
		found := false
		for index := range filtered {
			if filtered[index].ID == cursor {
				start = index + 1
				found = true
				break
			}
		}
		if !found {
			s.invalidQuery(w, request, "cursor is invalid or no longer visible")
			return
		}
	}
	end := min(start+limit, len(filtered))
	page := filtered[start:end]
	nextCursor := ""
	if end < len(filtered) && len(page) != 0 {
		nextCursor = page[len(page)-1].ID
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"invocations": page,
		"next_cursor": nextCursor,
	})
}

func (s *Server) getInvocation(w http.ResponseWriter, request *http.Request) {
	invocation, ok := s.projectInvocation(w, request, request.PathValue("invocation_id"))
	if !ok {
		return
	}
	WriteJSON(w, http.StatusOK, invocation)
}

func (s *Server) cancelInvocation(w http.ResponseWriter, request *http.Request) {
	invocationID := request.PathValue("invocation_id")
	if _, ok := s.projectInvocation(w, request, invocationID); !ok {
		return
	}
	if err := s.invocations.Cancel(invocationID); err != nil {
		s.writeDomainError(w, request, err)
		return
	}
	WriteJSON(w, http.StatusAccepted, map[string]string{
		"invocation_id": invocationID,
		"request_id":    RequestID(request.Context()),
		"status":        "cancel_requested",
	})
}

func (s *Server) projectInvocation(
	w http.ResponseWriter,
	request *http.Request,
	invocationID string,
) (function.Invocation, bool) {
	invocation, err := s.invocations.Get(invocationID)
	if err != nil {
		s.writeDomainError(w, request, err)
		return function.Invocation{}, false
	}
	principal, _ := auth.FromContext(request.Context())
	if invocation.ProjectID != principal.ProjectID {
		s.resourceNotFound(w, request, "invocation")
		return function.Invocation{}, false
	}
	return invocation, true
}

type createJobRequest struct {
	ProjectID   string               `json:"project_id,omitempty"`
	Function    function.FunctionRef `json:"function"`
	Input       json.RawMessage      `json:"input"`
	MaxAttempts int                  `json:"max_attempts,omitempty"`
}

func (s *Server) jobsCollection(w http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case http.MethodGet:
		if !s.requireScope(w, request, ScopeJobsRead) {
			return
		}
		s.listJobs(w, request)
	case http.MethodPost:
		if !s.requireScope(w, request, ScopeJobsWrite) {
			return
		}
		s.createJob(w, request)
	default:
		methodNotAllowed(w, request, http.MethodGet, http.MethodPost)
	}
}

func (s *Server) createJob(w http.ResponseWriter, request *http.Request) {
	var payload createJobRequest
	if !DecodeJSON(w, request, &payload, s.maximumBodyBytes) {
		return
	}
	principal, ok := s.scopedProject(w, request, payload.ProjectID)
	if !ok {
		return
	}
	if len(bytes.TrimSpace(payload.Input)) == 0 {
		s.invalidRequest(w, request, "input is required")
		return
	}
	result, err := s.acceptJob(
		request,
		principal.ProjectID,
		payload.Function,
		payload.Input,
		payload.MaxAttempts,
	)
	if err != nil {
		s.writeDomainError(w, request, err)
		return
	}
	s.writeAcceptedJob(w, request, result)
}

func (s *Server) acceptJob(
	request *http.Request,
	projectID string,
	requested function.FunctionRef,
	input json.RawMessage,
	maxAttempts int,
) (job.AcceptResult, error) {
	if !s.allowAsyncJobs {
		return job.AcceptResult{}, ErrAsyncJobsUnavailable
	}
	resolved, atomicFunction, err := s.registry.ResolveRef(requested)
	if err != nil {
		return job.AcceptResult{}, err
	}
	manifest := atomicFunction.Manifest()
	if err := manifest.InputSchema.ValidateJSON(input); err != nil {
		return job.AcceptResult{}, fmt.Errorf("%w: input schema: %v", job.ErrInvalid, err)
	}
	switch manifest.ExecutionContext {
	case function.ExecutionContextNone, function.ExecutionContextHTTPAttempt:
	default:
		return job.AcceptResult{}, fmt.Errorf(
			"%w: durable execution context %q is not supported",
			job.ErrInvalid,
			manifest.ExecutionContext,
		)
	}
	durableRequest, err := job.NewRequest(
		projectID,
		job.FunctionRef{
			Name:    resolved.Name,
			Version: resolved.Version,
			Digest:  resolved.Digest,
		},
		input,
		job.RequestOptions{MaxAttempts: maxAttempts},
	)
	if err != nil {
		return job.AcceptResult{}, err
	}
	idempotencyKey := strings.TrimSpace(request.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		return job.AcceptResult{}, fmt.Errorf(
			"%w: Idempotency-Key header is required for durable job creation",
			job.ErrInvalid,
		)
	}
	return s.jobs.Accept(request.Context(), job.AcceptCommand{
		Request:        durableRequest,
		IdempotencyKey: idempotencyKey,
	})
}

func (s *Server) writeAcceptedJob(
	w http.ResponseWriter,
	request *http.Request,
	result job.AcceptResult,
) {
	w.Header().Set("Location", "/v1/jobs/"+result.Job.ID)
	w.Header().Set("Neurun-Job-Durability", string(s.jobDurability))
	if result.Duplicate {
		w.Header().Set("Idempotent-Replayed", "true")
	}
	WriteJSON(w, http.StatusAccepted, map[string]any{
		"job":        result.Job,
		"job_id":     result.Job.ID,
		"duplicate":  result.Duplicate,
		"durability": string(s.jobDurability),
		"request_id": RequestID(request.Context()),
	})
}

func (s *Server) listJobs(w http.ResponseWriter, request *http.Request) {
	principal, _ := auth.FromContext(request.Context())
	query := request.URL.Query()
	limit, ok := s.pageLimit(w, request)
	if !ok {
		return
	}
	var createdAfter time.Time
	if raw := strings.TrimSpace(query.Get("created_after")); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			s.invalidQuery(w, request, "created_after must be RFC3339")
			return
		}
		createdAfter = parsed
	}
	if query.Get("tag") != "" {
		s.invalidQuery(w, request, "tag filtering is not available in this release")
		return
	}
	states, ok := s.jobStates(w, request)
	if !ok {
		return
	}
	page, err := s.jobs.List(request.Context(), principal.ProjectID, job.ListOptions{
		States:       states,
		CreatedAfter: createdAfter,
		Limit:        limit,
		Cursor:       strings.TrimSpace(query.Get("cursor")),
	})
	if err != nil {
		s.writeDomainError(w, request, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"jobs":        page.Jobs,
		"next_cursor": page.NextCursor,
	})
}

func (s *Server) getJob(w http.ResponseWriter, request *http.Request) {
	jobSnapshot, err := s.projectJob(request, request.PathValue("job_id"))
	if err != nil {
		s.writeDomainError(w, request, err)
		return
	}
	WriteJSON(w, http.StatusOK, jobSnapshot)
}

func (s *Server) getJobEvents(w http.ResponseWriter, request *http.Request) {
	principal, _ := auth.FromContext(request.Context())
	events, err := s.jobs.Events(
		request.Context(),
		principal.ProjectID,
		request.PathValue("job_id"),
	)
	if err != nil {
		s.writeDomainError(w, request, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"events": events})
}

func (s *Server) getJobAttempts(w http.ResponseWriter, request *http.Request) {
	principal, _ := auth.FromContext(request.Context())
	attempts, err := s.jobs.Attempts(
		request.Context(),
		principal.ProjectID,
		request.PathValue("job_id"),
	)
	if err != nil {
		s.writeDomainError(w, request, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"attempts": attempts})
}

type cancelJobRequest struct {
	Reason string `json:"reason,omitempty"`
}

func (s *Server) cancelJob(w http.ResponseWriter, request *http.Request) {
	var payload cancelJobRequest
	if request.ContentLength != 0 {
		if !DecodeJSON(w, request, &payload, s.maximumBodyBytes) {
			return
		}
	}
	if len(payload.Reason) > 512 {
		s.invalidRequest(w, request, "reason cannot exceed 512 bytes")
		return
	}
	principal, _ := auth.FromContext(request.Context())
	result, err := s.jobs.Cancel(
		request.Context(),
		principal.ProjectID,
		request.PathValue("job_id"),
		payload.Reason,
	)
	if err != nil {
		s.writeDomainError(w, request, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"job":        result.Job,
		"duplicate":  result.Duplicate,
		"request_id": RequestID(request.Context()),
	})
}

func (s *Server) projectJob(request *http.Request, jobID string) (job.Job, error) {
	principal, _ := auth.FromContext(request.Context())
	return s.jobs.Get(request.Context(), principal.ProjectID, jobID)
}

type fetchRequest struct {
	ProjectID   string          `json:"project_id,omitempty"`
	Mode        string          `json:"mode,omitempty"`
	Version     string          `json:"version,omitempty"`
	Execution   string          `json:"execution,omitempty"`
	Request     json.RawMessage `json:"request"`
	TimeoutMS   int64           `json:"timeout_ms,omitempty"`
	MaxAttempts int             `json:"max_attempts,omitempty"`
}

func (s *Server) fetch(w http.ResponseWriter, request *http.Request) {
	var payload fetchRequest
	if !DecodeJSON(w, request, &payload, s.maximumBodyBytes) {
		return
	}
	if len(bytes.TrimSpace(payload.Request)) == 0 {
		s.invalidRequest(w, request, "request is required")
		return
	}
	switch payload.Mode {
	case "", "auto", "http":
	default:
		s.invalidRequest(w, request, "mode must be auto or http in this release")
		return
	}
	execution := payload.Execution
	if execution == "" {
		execution = "sync"
	}
	var executionContext *function.ExecutionContext
	if execution == "sync" {
		executionContext = &function.ExecutionContext{
			EphemeralHTTP: true,
			Capabilities:  []string{"http"},
		}
	}
	s.executeInvocation(w, request, "http.fetch", invokeRequest{
		ProjectID:   payload.ProjectID,
		Version:     payload.Version,
		Execution:   execution,
		Context:     executionContext,
		Input:       payload.Request,
		TimeoutMS:   payload.TimeoutMS,
		MaxAttempts: payload.MaxAttempts,
	}, true)
}

func (s *Server) executionContext(
	w http.ResponseWriter,
	request *http.Request,
	projectID string,
	supplied *function.ExecutionContext,
	trusted bool,
) (*function.ExecutionContext, bool) {
	if supplied == nil {
		return &function.ExecutionContext{ProjectID: projectID}, true
	}
	if supplied.ProjectID != "" && supplied.ProjectID != projectID {
		WriteProblem(w, request, http.StatusForbidden, Problem{
			Code:    "permission_denied",
			Message: "execution context belongs to a different project",
		})
		return nil, false
	}
	if !trusted &&
		(supplied.JobID != "" ||
			supplied.AttemptID != "" ||
			supplied.SessionID != "" ||
			supplied.EphemeralHTTP ||
			supplied.EphemeralBrowser ||
			len(supplied.Capabilities) != 0) {
		WriteProblem(w, request, http.StatusUnprocessableEntity, Problem{
			Code: "server_owned_context",
			Message: "job, attempt, session, ephemeral runtime, and capability " +
				"context is assigned by Neurun",
		})
		return nil, false
	}
	cloned := *supplied
	cloned.ProjectID = projectID
	cloned.Capabilities = append([]string(nil), supplied.Capabilities...)
	return &cloned, true
}

type invocationResponse struct {
	function.Invocation
	RequestID string `json:"request_id"`
}

func (s *Server) writeInvocation(
	w http.ResponseWriter,
	request *http.Request,
	status int,
	invocation function.Invocation,
) {
	WriteJSON(w, status, invocationResponse{
		Invocation: invocation,
		RequestID:  RequestID(request.Context()),
	})
}

func (s *Server) scopedProject(
	w http.ResponseWriter,
	request *http.Request,
	requestedProjectID string,
) (auth.Principal, bool) {
	principal, ok := auth.FromContext(request.Context())
	if !ok {
		WriteProblem(w, request, http.StatusUnauthorized, Problem{
			Code:    "authentication_failed",
			Message: "a valid bearer API key is required",
		})
		return auth.Principal{}, false
	}
	if requestedProjectID != "" && requestedProjectID != principal.ProjectID {
		WriteProblem(w, request, http.StatusForbidden, Problem{
			Code:    "permission_denied",
			Message: "the API key cannot access the requested project",
		})
		return auth.Principal{}, false
	}
	return principal, true
}

func (s *Server) scoped(scope string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if !s.requireScope(w, request, scope) {
			return
		}
		next.ServeHTTP(w, request)
	})
}

func (s *Server) requireScope(w http.ResponseWriter, request *http.Request, scope string) bool {
	principal, ok := auth.FromContext(request.Context())
	if !ok {
		WriteProblem(w, request, http.StatusUnauthorized, Problem{
			Code:    "authentication_failed",
			Message: "a valid bearer API key is required",
		})
		return false
	}
	if !principal.HasScope(scope) && !principal.HasScope("admin") {
		WriteProblem(w, request, http.StatusForbidden, Problem{
			Code:    "permission_denied",
			Message: "the API key does not grant the required scope",
			Details: map[string]any{"required_scope": scope},
		})
		return false
	}
	return true
}

func (s *Server) only(method string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != method {
			methodNotAllowed(w, request, method)
			return
		}
		next.ServeHTTP(w, request)
	})
}

func (s *Server) pageLimit(w http.ResponseWriter, request *http.Request) (int, bool) {
	raw := strings.TrimSpace(request.URL.Query().Get("limit"))
	if raw == "" {
		return defaultPageSize, true
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > maximumPageSize {
		s.invalidQuery(w, request, "limit must be an integer between 1 and 200")
		return 0, false
	}
	return limit, true
}

func (s *Server) jobStates(w http.ResponseWriter, request *http.Request) ([]job.State, bool) {
	rawValues := request.URL.Query()["status"]
	states := make([]job.State, 0)
	seen := make(map[job.State]struct{})
	for _, raw := range rawValues {
		for _, item := range strings.Split(raw, ",") {
			if strings.TrimSpace(item) == "" {
				continue
			}
			state := job.State(strings.TrimSpace(item))
			if !state.Valid() {
				s.invalidQuery(w, request, "status contains an unknown job state")
				return nil, false
			}
			if _, exists := seen[state]; !exists {
				states = append(states, state)
				seen[state] = struct{}{}
			}
		}
	}
	return states, true
}

func (s *Server) writeDomainError(w http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, function.ErrFunctionNotFound),
		errors.Is(err, function.ErrAliasNotFound),
		errors.Is(err, function.ErrInvocationNotFound),
		errors.Is(err, job.ErrNotFound):
		s.resourceNotFound(w, request, "resource")
	case errors.Is(err, function.ErrDigestPinMismatch),
		errors.Is(err, job.ErrIdempotencyConflict):
		code := "idempotency_conflict"
		message := "the idempotency key was already used for different work"
		if errors.Is(err, function.ErrDigestPinMismatch) {
			code = "digest_pin_mismatch"
			message = "the requested function digest does not match the resolved version"
		}
		WriteProblem(w, request, http.StatusConflict, Problem{Code: code, Message: message})
	case errors.Is(err, function.ErrInvocationNotLive),
		errors.Is(err, job.ErrInvalidTransition):
		WriteProblem(w, request, http.StatusConflict, Problem{
			Code:    "invalid_state",
			Message: "the resource is not in a state that permits this operation",
		})
	case errors.Is(err, ErrAsyncJobsUnavailable):
		WriteProblem(w, request, http.StatusServiceUnavailable, Problem{
			Code: "durable_backend_unavailable",
			Message: "asynchronous jobs are disabled because this process has " +
				"no durable job backend",
		})
	case errors.Is(err, job.ErrInvalid),
		errors.Is(err, job.ErrInvalidCursor),
		errors.Is(err, function.ErrInvalidJSON),
		errors.Is(err, function.ErrSchemaMismatch):
		s.invalidRequest(w, request, err.Error())
	case errors.Is(err, context.Canceled):
		WriteProblem(w, request, http.StatusRequestTimeout, Problem{
			Code:    "request_canceled",
			Message: "the request was canceled",
		})
	case errors.Is(err, context.DeadlineExceeded):
		WriteProblem(w, request, http.StatusGatewayTimeout, Problem{
			Code:    "request_timeout",
			Message: "the request exceeded its deadline",
		})
	default:
		WriteProblem(w, request, http.StatusInternalServerError, Problem{
			Code:    "internal_error",
			Message: "the server could not complete the request",
		})
	}
}

func (s *Server) invalidRequest(w http.ResponseWriter, request *http.Request, message string) {
	WriteProblem(w, request, http.StatusUnprocessableEntity, Problem{
		Code:    "invalid_request",
		Message: message,
	})
}

func (s *Server) invalidQuery(w http.ResponseWriter, request *http.Request, message string) {
	WriteProblem(w, request, http.StatusBadRequest, Problem{
		Code:    "invalid_request",
		Message: message,
	})
}

func (s *Server) resourceNotFound(w http.ResponseWriter, request *http.Request, kind string) {
	WriteProblem(w, request, http.StatusNotFound, Problem{
		Code:    "resource_not_found",
		Message: "the requested " + kind + " was not found",
	})
}

func invocationFailureStatus(failure function.Failure) int {
	switch failure.Category {
	case function.FailureFunctionNotFound:
		return http.StatusNotFound
	case function.FailureInvalidRequest,
		function.FailureInputSchema,
		function.FailureOutputSchema,
		function.FailureContextIncompatible,
		function.FailureCapabilityMissing,
		function.FailureValidation:
		return http.StatusUnprocessableEntity
	case function.FailureTimeout:
		return http.StatusGatewayTimeout
	case function.FailureCanceled:
		return http.StatusConflict
	case function.FailureTransientNetwork,
		function.FailureBrowserCrash,
		function.FailureAgentLost:
		return http.StatusBadGateway
	default:
		return http.StatusInternalServerError
	}
}

func invocationFailureCode(failure function.Failure) string {
	if failure.Code != "" {
		return failure.Code
	}
	if failure.Category != "" {
		return string(failure.Category)
	}
	return "invocation_failed"
}

func validInvocationStatus(status function.InvocationStatus) bool {
	switch status {
	case function.InvocationAccepted,
		function.InvocationRunning,
		function.InvocationSucceeded,
		function.InvocationRejected,
		function.InvocationFailed,
		function.InvocationTimedOut,
		function.InvocationCanceled:
		return true
	default:
		return false
	}
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func methodNotAllowed(
	w http.ResponseWriter,
	request *http.Request,
	allowed ...string,
) {
	w.Header().Set("Allow", strings.Join(allowed, ", "))
	WriteProblem(w, request, http.StatusMethodNotAllowed, Problem{
		Code:    "method_not_allowed",
		Message: "the request method is not supported for this resource",
	})
}

func notFound(w http.ResponseWriter, request *http.Request) {
	WriteProblem(w, request, http.StatusNotFound, Problem{
		Code:    "resource_not_found",
		Message: "the requested resource was not found",
	})
}
