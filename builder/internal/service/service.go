package service

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	objectpath "path"
	"path/filepath"

	"github.com/dagflows/builder/internal/domain"
	"github.com/dagflows/builder/pkg"
)

type Store interface {
	PutFile(ctx context.Context, key, path, mediaType string) (domain.UploadedArtifact, error)
}

type BuildService struct {
	store Store
}

func NewBuildService(store Store) *BuildService {
	return &BuildService{
		store: store,
	}
}

func (s *BuildService) Build(ctx context.Context, req domain.BuildRequest) (domain.BuildResult, error) {
	sourcePath, err := filepath.Abs(req.SourcePath)
	if err != nil {
		return domain.BuildResult{}, fmt.Errorf("invalid source_path: %w", err)
	}
	info, err := os.Stat(sourcePath)
	if err != nil {
		return domain.BuildResult{}, fmt.Errorf("source_path not readable: %w", err)
	}
	if !info.IsDir() {
		return domain.BuildResult{}, fmt.Errorf("source_path must be a directory")
	}

	buildID := pkg.NewBuildID()
	workDir, err := os.MkdirTemp("", "dagflows-build-*")
	if err != nil {
		return domain.BuildResult{}, fmt.Errorf("create work dir: %w", err)
	}
	defer os.RemoveAll(workDir)

	srcDir := filepath.Join(workDir, "src")
	outDir := filepath.Join(workDir, "out")
	if err := pkg.CopyDir(sourcePath, srcDir, skipSourceEntry); err != nil {
		return domain.BuildResult{}, fmt.Errorf("copy source: %w", err)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return domain.BuildResult{}, fmt.Errorf("create output dir: %w", err)
	}

	var artifacts []pkg.Artifact
	switch req.Runtime {
	case domain.RuntimePython:
		artifacts, err = s.buildPython(ctx, sourcePath, srcDir, outDir)
	case domain.RuntimeNode:
		artifacts, err = s.buildNode(ctx, srcDir, outDir)
	case domain.RuntimeGo:
		artifacts, err = s.buildGo(ctx, srcDir, outDir, req.EntryPoint, req.AppID)
	default:
		return domain.BuildResult{}, fmt.Errorf("unsupported runtime %q", req.Runtime)
	}
	if err != nil {
		return domain.BuildResult{}, fmt.Errorf("build failed: %w", err)
	}

	result := domain.BuildResult{BuildID: buildID}
	for _, artifact := range artifacts {
		artifactID := pkg.NewUUID()
		key := objectpath.Join(pkg.SafeName(req.AppID), buildID, artifactID, artifact.Name)
		uploaded, err := s.store.PutFile(ctx, key, artifact.Path, artifact.MediaType)
		if err != nil {
			return domain.BuildResult{}, fmt.Errorf("upload %s: %w", artifact.Name, err)
		}

		result.Artifacts = append(result.Artifacts, domain.Artifact{
			ID:        artifactID,
			Kind:      artifact.Kind,
			Name:      artifact.Name,
			Bucket:    uploaded.Bucket,
			Key:       uploaded.Key,
			SHA256:    artifact.SHA256,
			SizeBytes: artifact.SizeBytes,
			MediaType: artifact.MediaType,
		})
	}

	return result, nil
}

func skipSourceEntry(_ string, entry fs.DirEntry) bool {
	switch entry.Name() {
	case ".git", "node_modules", ".venv", "venv":
		return true
	default:
		return false
	}
}
