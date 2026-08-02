package api

import (
	"errors"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/neurun-io/neurun/internal/domain/deployment"
	"github.com/neurun-io/neurun/internal/dto"
)

// deploymentMultipartOverhead is the slack allowed for the form's own framing
// on top of the source archive itself.
const deploymentMultipartOverhead = int64(1 << 20)

func (server *Server) createDeployment(ctx *gin.Context) {
	ctx.Request.Body = http.MaxBytesReader(
		ctx.Writer, ctx.Request.Body,
		server.maximumDeploymentBytes+deploymentMultipartOverhead,
	)
	if err := ctx.Request.ParseMultipartForm(1 << 20); err != nil {
		status, code := http.StatusBadRequest, "invalid_multipart"
		message := "request must be valid multipart/form-data"
		var maximum *http.MaxBytesError
		if errors.As(err, &maximum) || errors.Is(err, multipart.ErrMessageTooLarge) {
			status, code = http.StatusRequestEntityTooLarge, "deployment_too_large"
			message = "deployment upload exceeds the configured source limit"
		}
		writeProblem(ctx, status, dto.Problem{
			Code: code, Message: message,
			Details: map[string]any{"cause": err.Error()},
		})
		return
	}
	defer ctx.Request.MultipartForm.RemoveAll()

	if problem := validateDeploymentForm(ctx.Request.MultipartForm); problem != "" {
		invalidRequest(ctx, problem)
		return
	}
	runtime, err := deployment.ParseRuntime(ctx.Request.MultipartForm.Value["runtime"][0])
	if err != nil {
		writeError(ctx, err)
		return
	}
	entrypoint := ""
	if values := ctx.Request.MultipartForm.Value["entrypoint"]; len(values) == 1 {
		entrypoint = strings.TrimSpace(values[0])
	}
	header := ctx.Request.MultipartForm.File["source"][0]
	if header.Size > server.maximumDeploymentBytes {
		writeProblem(ctx, http.StatusRequestEntityTooLarge, dto.Problem{
			Code:    "deployment_too_large",
			Message: "deployment upload exceeds the configured source limit",
		})
		return
	}
	source, err := header.Open()
	if err != nil {
		writeProblem(ctx, http.StatusBadRequest, dto.Problem{
			Code: "invalid_multipart", Message: "could not open the uploaded source ZIP",
		})
		return
	}
	defer source.Close()

	// app_id alone decides the project. An app that was not created first is
	// refused, so an SDK cannot bring one into being by deploying to it.
	created, err := server.deployments.Create(ctx.Request.Context(), dto.CreateDeploymentRequest{
		AppID:      strings.TrimSpace(ctx.Request.MultipartForm.Value["app_id"][0]),
		Runtime:    runtime,
		EntryPoint: entrypoint,
		SourceName: header.Filename,
		Source:     source,
	})
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Header("Location", "/v1/deployments/"+created.ID)
	ctx.JSON(http.StatusCreated, dto.NewDeploymentResponse(created))
}

func (server *Server) listDeployments(ctx *gin.Context) {
	limit, ok := server.pageLimit(ctx)
	if !ok {
		return
	}
	records, err := server.deployments.List(
		ctx.Request.Context(),
		strings.TrimSpace(ctx.Query("project_id")),
		strings.TrimSpace(ctx.Query("app_id")),
		limit,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"deployments": dto.NewDeploymentResponses(records)})
}

func (server *Server) getDeployment(ctx *gin.Context) {
	record, err := server.deployments.Get(
		ctx.Request.Context(), ctx.Param("deployment_id"),
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, dto.NewDeploymentResponse(record))
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
