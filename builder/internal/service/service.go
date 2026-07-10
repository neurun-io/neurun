package service

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"os"
	objectpath "path"
	"path/filepath"
	"strings"

	"github.com/dagflows/builder/internal/domain"
	"github.com/dagflows/builder/internal/storage"
	"github.com/dagflows/builder/pkg"
	"github.com/google/uuid"
)

type BuildService struct {
	store storage.Store
}

func NewBuildService(store storage.Store) *BuildService {
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

	buildID := strings.Trim(req.BuildID, "/")
	if buildID == "" {
		buildID = pkg.NewBuildID()
	}
	if pkg.SafeName(buildID) != buildID {
		return domain.BuildResult{}, fmt.Errorf("build_id must contain only letters, numbers, dots, dashes, or underscores")
	}
	log.Printf("build app=%s build=%s runtime=%s started", req.AppID, buildID, req.Runtime)

	workDir, err := os.MkdirTemp("", "dagflows-build-*")
	if err != nil {
		log.Printf("build app=%s build=%s stage=create_work_dir failed reason=%s", req.AppID, buildID, err)
		return domain.BuildResult{}, fmt.Errorf("create work dir: %w", err)
	}
	defer os.RemoveAll(workDir)

	srcDir := filepath.Join(workDir, "src")
	outDir := filepath.Join(workDir, "out")
	log.Printf("build app=%s build=%s stage=copy_source started", req.AppID, buildID)
	if err := pkg.CopyDir(sourcePath, srcDir, skipSourceEntry); err != nil {
		log.Printf("build app=%s build=%s stage=copy_source failed reason=%s", req.AppID, buildID, err)
		return domain.BuildResult{}, fmt.Errorf("copy source: %w", err)
	}
	log.Printf("build app=%s build=%s stage=copy_source completed", req.AppID, buildID)

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		log.Printf("build app=%s build=%s stage=create_output_dir failed reason=%s", req.AppID, buildID, err)
		return domain.BuildResult{}, fmt.Errorf("create output dir: %w", err)
	}

	var artifacts []pkg.Artifact
	log.Printf("build app=%s build=%s stage=compile_runtime started runtime=%s", req.AppID, buildID, req.Runtime)
	switch req.Runtime {
	case domain.RuntimePython:
		artifacts, err = s.buildPython(ctx, sourcePath, srcDir, outDir)
	case domain.RuntimeNode:
		artifacts, err = s.buildNode(ctx, srcDir, outDir)
	case domain.RuntimeGo:
		artifacts, err = s.buildGo(ctx, srcDir, outDir, req.EntryPoint, req.AppID)
	default:
		log.Printf("build app=%s build=%s stage=compile_runtime failed reason=unsupported runtime %q", req.AppID, buildID, req.Runtime)
		return domain.BuildResult{}, fmt.Errorf("unsupported runtime %q", req.Runtime)
	}
	if err != nil {
		log.Printf("build app=%s build=%s stage=compile_runtime failed reason=%s", req.AppID, buildID, err)
		return domain.BuildResult{}, fmt.Errorf("build failed: %w", err)
	}
	log.Printf("build app=%s build=%s stage=compile_runtime completed artifacts=%d", req.AppID, buildID, len(artifacts))

	result := domain.BuildResult{BuildID: buildID}
	for _, artifact := range artifacts {
		key := objectpath.Join(pkg.SafeName(req.AppID), buildID, artifact.Name)
		log.Printf("build app=%s build=%s stage=upload_artifact started artifact=%s key=%s", req.AppID, buildID, artifact.Name, key)
		uploaded, err := s.store.PutFile(ctx, key, artifact.Path, artifact.MediaType)
		if err != nil {
			log.Printf("build app=%s build=%s stage=upload_artifact failed artifact=%s reason=%s", req.AppID, buildID, artifact.Name, err)
			return domain.BuildResult{}, fmt.Errorf("upload %s: %w", artifact.Name, err)
		}
		log.Printf("build app=%s build=%s stage=upload_artifact completed artifact=%s bucket=%s key=%s", req.AppID, buildID, artifact.Name, uploaded.Bucket, uploaded.Key)

		result.Artifacts = append(result.Artifacts, domain.Artifact{
			ID:        uuid.NewString(),
			Kind:      artifact.Kind,
			Name:      artifact.Name,
			Bucket:    uploaded.Bucket,
			Key:       uploaded.Key,
			SHA256:    artifact.SHA256,
			SizeBytes: artifact.SizeBytes,
			MediaType: artifact.MediaType,
		})
	}

	log.Printf("build app=%s build=%s completed artifacts=%d", req.AppID, buildID, len(result.Artifacts))
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
