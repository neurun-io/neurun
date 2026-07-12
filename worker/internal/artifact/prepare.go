package artifact

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	objectpath "path"
	"path/filepath"
	"strings"

	"github.com/dagflows/worker/internal/dto"
)

type PreparedWorkload struct {
	Request dto.WorkflowNodeRunRequest
	CodeDir string
	DepsDir string
	Input   json.RawMessage
}

type Fetcher interface {
	Fetch(ctx context.Context, ref, path string) error
}

func Prepare(ctx context.Context, fetcher Fetcher, req dto.WorkflowNodeRunRequest) (*PreparedWorkload, func(), error) {
	if fetcher == nil {
		return nil, nil, fmt.Errorf("artifact fetcher is required")
	}
	codeKey := strings.Join(strings.Fields(req.ArtifactKey), "")
	if codeKey == "" {
		return nil, nil, fmt.Errorf("artifact_key is required")
	}
	if objectpath.Base(codeKey) != "code-layer.zip" {
		return nil, nil, fmt.Errorf("artifact_key must point to code-layer.zip")
	}
	req.ArtifactKey = codeKey
	workDir, err := os.MkdirTemp("", "dagflows-node-*")
	if err != nil {
		return nil, nil, fmt.Errorf("create work dir: %w", err)
	}
	cleanup := func() {
		_ = os.RemoveAll(workDir)
	}

	prepared := &PreparedWorkload{
		Request: req,
		CodeDir: filepath.Join(workDir, "code"),
		DepsDir: filepath.Join(workDir, "deps"),
	}

	if err := os.MkdirAll(prepared.CodeDir, 0o755); err != nil {
		cleanup()
		return nil, nil, err
	}
	if err := os.MkdirAll(prepared.DepsDir, 0o755); err != nil {
		cleanup()
		return nil, nil, err
	}

	codeArtifact := filepath.Join(workDir, "code.artifact")
	if err := fetcher.Fetch(ctx, codeKey, codeArtifact); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("fetch code artifact: %w", err)
	}
	if err := unzip(codeArtifact, prepared.CodeDir); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("expand code artifact: %w", err)
	}

	depsArtifact := filepath.Join(workDir, "deps.artifact")
	depsKey := objectpath.Join(objectpath.Dir(codeKey), "install-layer.zip")
	if err := fetcher.Fetch(ctx, depsKey, depsArtifact); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("fetch deps artifact: %w", err)
	}
	if err := unzip(depsArtifact, prepared.DepsDir); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("expand deps artifact: %w", err)
	}

	payload, err := RuntimeInput(req)
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("prepare runtime input: %w", err)
	}
	prepared.Input, err = json.Marshal(payload)
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("encode runtime input: %w", err)
	}

	return prepared, cleanup, nil
}

func unzip(path, destDir string) error {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer reader.Close()

	destRoot, err := filepath.Abs(destDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(destRoot, 0o755); err != nil {
		return err
	}

	for _, file := range reader.File {
		name := filepath.Clean(filepath.FromSlash(file.Name))
		if name == "." || filepath.IsAbs(name) || strings.HasPrefix(name, ".."+string(filepath.Separator)) || name == ".." {
			return fmt.Errorf("unsafe zip entry %q", file.Name)
		}
		target := filepath.Join(destRoot, name)
		mode := file.FileInfo().Mode()
		if mode&os.ModeSymlink != 0 {
			return fmt.Errorf("zip symlink entries are not supported: %q", file.Name)
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if !mode.IsRegular() {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		source, err := file.Open()
		if err != nil {
			return err
		}
		targetFile, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			source.Close()
			return err
		}
		_, copyErr := io.Copy(targetFile, source)
		closeErr := targetFile.Close()
		sourceErr := source.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if sourceErr != nil {
			return sourceErr
		}
	}
	return nil
}

func RuntimeInput(req dto.WorkflowNodeRunRequest) (dto.RuntimePayload, error) {
	inputs := make(map[string]any, len(req.InputData))
	for key, raw := range req.InputData {
		var value any
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &value); err != nil {
				return dto.RuntimePayload{}, fmt.Errorf("decode input %s: %w", key, err)
			}
		}
		inputs[key] = value
	}

	return dto.RuntimePayload{
		Ctx: map[string]any{
			"workflow_run_id": req.WorkflowRunID,
			"node_key":        req.NodeKey,
			"config":          req.Config,
			"timeout_seconds": req.TimeoutSeconds,
		},
		Inputs: inputs,
	}, nil
}
