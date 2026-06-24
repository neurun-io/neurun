package grpc

import (
	"context"

	"github.com/dagflows/builder/internal/domain"
	builderv1 "github.com/dagflows/builder/proto/builder/v1"
	grpcserver "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Builder interface {
	Build(ctx context.Context, req domain.BuildRequest) (domain.BuildResult, error)
}

type BuilderHandler struct {
	builderv1.UnimplementedBuilderServiceServer

	builder Builder
}

func Register(server *grpcserver.Server, builder Builder) {
	builderv1.RegisterBuilderServiceServer(server, NewBuilderHandler(builder))
}

func NewBuilderHandler(builder Builder) *BuilderHandler {
	return &BuilderHandler{builder: builder}
}

func (h *BuilderHandler) Build(ctx context.Context, req *builderv1.BuildRequest) (*builderv1.BuildResponse, error) {
	domainReq, err := buildRequestFromDTO(req)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	result, err := h.builder.Build(ctx, domainReq)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return buildResponseToDTO(result), nil
}

func buildRequestFromDTO(req *builderv1.BuildRequest) (domain.BuildRequest, error) {
	if req.GetAppId() == "" {
		return domain.BuildRequest{}, errRequired("app_id")
	}
	if req.GetSourcePath() == "" {
		return domain.BuildRequest{}, errRequired("source_path")
	}

	runtime, err := runtimeFromDTO(req.GetRuntime())
	if err != nil {
		return domain.BuildRequest{}, err
	}

	return domain.BuildRequest{
		AppID:      req.GetAppId(),
		SourcePath: req.GetSourcePath(),
		Runtime:    runtime,
		EntryPoint: req.GetEntrypoint(),
	}, nil
}

func runtimeFromDTO(runtime builderv1.Runtime) (domain.Runtime, error) {
	switch runtime {
	case builderv1.Runtime_RUNTIME_PYTHON:
		return domain.RuntimePython, nil
	case builderv1.Runtime_RUNTIME_NODE:
		return domain.RuntimeNode, nil
	case builderv1.Runtime_RUNTIME_GO:
		return domain.RuntimeGo, nil
	default:
		return "", errRequired("runtime")
	}
}

func buildResponseToDTO(result domain.BuildResult) *builderv1.BuildResponse {
	resp := &builderv1.BuildResponse{BuildId: result.BuildID}
	for _, artifact := range result.Artifacts {
		dto := &builderv1.Artifact{
			Name:      artifact.Name,
			Bucket:    artifact.Bucket,
			Key:       artifact.Key,
			Sha256:    artifact.SHA256,
			SizeBytes: artifact.SizeBytes,
			MediaType: artifact.MediaType,
		}

		switch artifact.Kind {
		case domain.ArtifactInstallLayer:
			resp.InstallLayers = append(resp.InstallLayers, dto)
		case domain.ArtifactCodeLayer:
			resp.CodeLayers = append(resp.CodeLayers, dto)
		case domain.ArtifactDeployable:
			resp.Deployables = append(resp.Deployables, dto)
		}
	}
	return resp
}
