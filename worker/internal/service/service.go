package service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/dagflows/worker/internal/artifact"
	"github.com/dagflows/worker/internal/domain"
	"github.com/dagflows/worker/internal/dto"
	"github.com/dagflows/worker/internal/vm"
)

type NodeRunService struct {
	runner               vm.Runner
	fetcher              artifact.Fetcher
	outputInlineMaxBytes int64
}

func NewNodeRunService(runner vm.Runner, fetcher artifact.Fetcher, outputInlineMaxBytes int64) *NodeRunService {
	return &NodeRunService{
		runner:               runner,
		fetcher:              fetcher,
		outputInlineMaxBytes: outputInlineMaxBytes,
	}
}

func (s *NodeRunService) Execute(ctx context.Context, req dto.WorkflowNodeRunRequest) dto.WorkflowNodeRunResponse {
	start := time.Now()
	base := dto.WorkflowNodeRunResponse{
		WorkflowRunID: req.WorkflowRunID,
		NodeKey:       req.NodeKey,
	}

	if err := validateRequest(req); err != nil {
		return failedResponse(base, err, time.Since(start))
	}

	log.Printf("node run stage=prepare run=%s node=%s", req.WorkflowRunID, req.NodeKey)
	prepared, cleanup, err := artifact.Prepare(ctx, s.fetcher, req)
	if err != nil {
		return failedResponse(base, infrastructure(err.Error()), time.Since(start))
	}
	defer cleanup()

	log.Printf("node run stage=vm_start run=%s node=%s", req.WorkflowRunID, req.NodeKey)
	result, err := s.runner.Run(ctx, prepared)
	if err != nil {
		return failedResponse(base, classifyRunError(err), time.Since(start))
	}

	normalized, err := normalizeOutput(result.Output, s.outputInlineMaxBytes)
	if err != nil {
		return failedResponse(base, permanent(err.Error()), time.Since(start))
	}
	if normalized.Status == domain.WorkflowNodeRunAttemptStatusFailed {
		resp := base
		resp.Status = domain.WorkflowNodeRunAttemptStatusFailed
		resp.ErrorMessage = normalized.ErrorMessage
		resp.ErrorCategory = normalized.ErrorCategory
		resp.Retryable = normalized.Retryable
		resp.DurationMs = durationMs(result.Duration, time.Since(start))
		return resp
	}

	resp := base
	resp.Status = domain.WorkflowNodeRunAttemptStatusSuccess
	resp.RouteTo = normalized.RouteTo
	resp.OutputType = normalized.OutputType
	resp.OutputRef = normalized.OutputRef
	resp.OutputSize = normalized.OutputSize
	resp.InlineOutput = normalized.InlineOutput
	resp.DurationMs = durationMs(result.Duration, time.Since(start))
	return resp
}

func validateRequest(req dto.WorkflowNodeRunRequest) error {
	switch {
	case strings.TrimSpace(req.WorkflowRunID) == "":
		return permanent("workflow_run_id is required")
	case strings.TrimSpace(req.NodeKey) == "":
		return permanent("node_key is required")
	case strings.TrimSpace(req.ArtifactKey) == "":
		return permanent("artifact_key is required")
	}
	return nil
}

func failedResponse(base dto.WorkflowNodeRunResponse, err error, duration time.Duration) dto.WorkflowNodeRunResponse {
	category := "permanent"
	retryable := false
	if execErr, ok := err.(executionError); ok {
		category = execErr.category
		retryable = execErr.retryable
	}
	return dto.WorkflowNodeRunResponse{
		WorkflowRunID: base.WorkflowRunID,
		NodeKey:       base.NodeKey,
		Status:        domain.WorkflowNodeRunAttemptStatusFailed,
		ErrorMessage:  err.Error(),
		ErrorCategory: category,
		Retryable:     retryable,
		DurationMs:    int(duration / time.Millisecond),
	}
}

func classifyRunError(err error) error {
	if runErr, ok := err.(vm.RunError); ok {
		return executionError{
			message:   runErr.Error(),
			category:  runErr.Category,
			retryable: runErr.Retryable,
		}
	}
	return permanent(fmt.Sprintf("runtime failed: %s", err))
}

func durationMs(resultDuration, fallback time.Duration) int {
	if resultDuration <= 0 {
		resultDuration = fallback
	}
	return int(resultDuration / time.Millisecond)
}
