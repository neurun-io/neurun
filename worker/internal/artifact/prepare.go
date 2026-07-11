package artifact

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/dagflows/worker/internal/dto"
	"github.com/dagflows/worker/internal/protocol"
)

const (
	guestRoot     = "/srv/dagflows"
	guestCodeDir  = guestRoot + "/code"
	guestDepsDir  = guestRoot + "/deps"
	guestRuntime  = guestRoot + "/runtime"
	guestInput    = guestRoot + "/input.json"
	guestManifest = guestRoot + "/manifest.json"
)

type PreparedWorkload struct {
	Request      dto.WorkflowNodeRunRequest
	WorkDir      string
	CodeDir      string
	DepsDir      string
	InputPath    string
	ManifestPath string
}

type Fetcher interface {
	Fetch(ctx context.Context, ref, path string) error
}

func Prepare(ctx context.Context, baseDir string, fetcher Fetcher, req dto.WorkflowNodeRunRequest) (*PreparedWorkload, func(), error) {
	if fetcher == nil {
		return nil, nil, fmt.Errorf("artifact fetcher is required")
	}
	if strings.TrimSpace(req.CodeArtifactRef()) == "" {
		return nil, nil, fmt.Errorf("artifact_key or artifact_url is required")
	}
	if baseDir == "" {
		baseDir = os.TempDir()
	}
	if err := os.MkdirAll(baseDir, 0o700); err != nil {
		return nil, nil, fmt.Errorf("create work root: %w", err)
	}

	workDir, err := os.MkdirTemp(baseDir, "dagflows-node-*")
	if err != nil {
		return nil, nil, fmt.Errorf("create work dir: %w", err)
	}
	cleanup := func() {
		_ = os.RemoveAll(workDir)
	}

	prepared := &PreparedWorkload{
		Request:      req,
		WorkDir:      workDir,
		CodeDir:      filepath.Join(workDir, "code"),
		DepsDir:      filepath.Join(workDir, "deps"),
		InputPath:    filepath.Join(workDir, "input.json"),
		ManifestPath: filepath.Join(workDir, "manifest.json"),
	}

	if err := os.MkdirAll(prepared.CodeDir, 0o755); err != nil {
		cleanup()
		return nil, nil, err
	}
	if err := os.MkdirAll(prepared.DepsDir, 0o755); err != nil {
		cleanup()
		return nil, nil, err
	}

	if depsRef := req.DepsArtifactRef(); depsRef != "" {
		depsArtifact := filepath.Join(workDir, "deps.artifact")
		if err := fetcher.Fetch(ctx, depsRef, depsArtifact); err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("fetch deps artifact: %w", err)
		}
		if err := expandArtifact(depsArtifact, prepared.DepsDir, "deps"); err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("expand deps artifact: %w", err)
		}
	}

	codeArtifact := filepath.Join(workDir, "code.artifact")
	if err := fetcher.Fetch(ctx, req.CodeArtifactRef(), codeArtifact); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("fetch code artifact: %w", err)
	}
	if err := expandArtifact(codeArtifact, prepared.CodeDir, "deployable"); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("expand code artifact: %w", err)
	}

	payload, err := RuntimeInput(req)
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("prepare runtime input: %w", err)
	}
	if err := writeJSON(prepared.InputPath, payload); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("write input: %w", err)
	}
	if err := writeJSON(prepared.ManifestPath, BuildManifest(prepared)); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("write manifest: %w", err)
	}

	return prepared, cleanup, nil
}

func expandArtifact(path, destDir, rawName string) error {
	if isZip(path) {
		return unzip(path, destDir)
	}

	target := filepath.Join(destDir, rawName)
	source, err := os.Open(path)
	if err != nil {
		return err
	}
	defer source.Close()
	dest, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(dest, source)
	return errors.Join(copyErr, dest.Close())
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
			if err := os.MkdirAll(target, mode.Perm()); err != nil {
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
		targetFile, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
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
			"execution_token": req.ExecutionToken,
			"config":          req.Config,
			"entrypoint":      req.Entrypoint,
			"language":        req.Language,
			"timeout_seconds": req.TimeoutSeconds,
		},
		Inputs:    inputs,
		InputRefs: req.InputRefs,
	}, nil
}

func BuildManifest(workload *PreparedWorkload) protocol.Manifest {
	req := workload.Request
	env := map[string]string{}
	command := []string{}

	switch strings.ToLower(strings.TrimSpace(req.Language)) {
	case "python", "py":
		env["PYTHONPATH"] = guestRuntime + ":" + guestCodeDir + ":" + guestDepsDir
		command = []string{"python3", "-m", "dagflows", "invoke", "--node", req.NodeKey}
	case "node", "nodejs", "javascript", "typescript", "js", "ts":
		env["NODE_PATH"] = guestRuntime + "/node_modules:" + guestDepsDir + "/node_modules"
		entrypoint := strings.ReplaceAll(strings.TrimSpace(req.Entrypoint), "\\", "/")
		command = []string{"node", path.Join(guestRuntime, entrypoint)}
	case "go", "golang":
		command = []string{guestRuntime + "/deployable"}
	}

	return protocol.Manifest{
		WorkflowRunID:  req.WorkflowRunID,
		NodeKey:        req.NodeKey,
		ExecutionToken: req.ExecutionToken,
		Language:       req.Language,
		Entrypoint:     req.Entrypoint,
		TimeoutSeconds: req.TimeoutSeconds,
		MemoryLimitMB:  req.RequiredMemoryMB(),
		Guest: protocol.Paths{
			WorkDir:      guestRuntime,
			CodeDir:      guestCodeDir,
			DepsDir:      guestDepsDir,
			InputPath:    guestInput,
			ManifestPath: guestManifest,
		},
		Env:     env,
		Command: command,
	}
}

func isZip(path string) bool {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return false
	}
	_ = reader.Close()
	return true
}

func writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
