package artifact

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/dagflows/worker/internal/domain"
	"github.com/dagflows/worker/pkg"
)

const (
	guestRoot     = "/srv/dagflows"
	guestCodeDir  = guestRoot + "/code"
	guestDepsDir  = guestRoot + "/deps"
	guestInput    = guestRoot + "/input.json"
	guestManifest = guestRoot + "/manifest.json"
)

type PreparedWorkload struct {
	Request      domain.WorkflowNodeRunRequest
	WorkDir      string
	CodeDir      string
	DepsDir      string
	InputPath    string
	ManifestPath string
}

type RuntimePayload struct {
	Ctx       map[string]any    `json:"ctx"`
	Inputs    map[string]any    `json:"inputs"`
	InputRefs map[string]string `json:"input_refs,omitempty"`
}

type Manifest struct {
	WorkflowRunID  string            `json:"workflow_run_id"`
	NodeKey        string            `json:"node_key"`
	ExecutionToken int64             `json:"execution_token"`
	Language       string            `json:"language"`
	Entrypoint     string            `json:"entrypoint"`
	TimeoutSeconds int               `json:"timeout_seconds"`
	Host           ManifestPaths     `json:"host"`
	Guest          ManifestPaths     `json:"guest"`
	Env            map[string]string `json:"env,omitempty"`
	Command        []string          `json:"command,omitempty"`
}

type ManifestPaths struct {
	WorkDir      string `json:"work_dir"`
	CodeDir      string `json:"code_dir"`
	DepsDir      string `json:"deps_dir"`
	InputPath    string `json:"input_path"`
	ManifestPath string `json:"manifest_path"`
}

func Prepare(ctx context.Context, baseDir string, req domain.WorkflowNodeRunRequest) (*PreparedWorkload, func(), error) {
	if strings.TrimSpace(req.ArtifactURL) == "" {
		return nil, nil, fmt.Errorf("artifact_url is required")
	}
	if baseDir == "" {
		baseDir = os.TempDir()
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

	if req.DepsArtifactURL != "" {
		depsArtifact := filepath.Join(workDir, "deps.artifact")
		if err := Download(ctx, req.DepsArtifactURL, depsArtifact); err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("download deps artifact: %w", err)
		}
		if err := ExpandArtifact(depsArtifact, prepared.DepsDir, "deps"); err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("expand deps artifact: %w", err)
		}
	}

	codeArtifact := filepath.Join(workDir, "code.artifact")
	if err := Download(ctx, req.ArtifactURL, codeArtifact); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("download code artifact: %w", err)
	}
	if err := ExpandArtifact(codeArtifact, prepared.CodeDir, "deployable"); err != nil {
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

func Download(ctx context.Context, rawURL, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("download failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(file, resp.Body)
	return err
}

func ExpandArtifact(path, destDir, rawName string) error {
	if isZip(path) {
		return Unzip(path, destDir)
	}

	name := pkg.SafeName(rawName)
	if name == "" {
		name = "artifact"
	}
	target := filepath.Join(destDir, name)
	if err := pkg.CopyFile(path, target); err != nil {
		return err
	}
	return os.Chmod(target, 0o755)
}

func Unzip(path, destDir string) error {
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
		if !pkg.PathInside(destRoot, target) {
			return fmt.Errorf("unsafe zip entry %q", file.Name)
		}

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

func RuntimeInput(req domain.WorkflowNodeRunRequest) (RuntimePayload, error) {
	inputs := make(map[string]any, len(req.InputData))
	for key, raw := range req.InputData {
		var value any
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &value); err != nil {
				return RuntimePayload{}, fmt.Errorf("decode input %s: %w", key, err)
			}
		}
		inputs[key] = value
	}

	return RuntimePayload{
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

func BuildManifest(workload *PreparedWorkload) Manifest {
	req := workload.Request
	env := map[string]string{}
	command := []string{}

	switch strings.ToLower(strings.TrimSpace(req.Language)) {
	case "python", "py":
		env["PYTHONPATH"] = guestCodeDir + ":" + guestDepsDir
		command = []string{"python", "-m", "dagflows", "invoke", "--node", req.NodeKey}
	case "node", "nodejs", "javascript", "typescript", "js", "ts":
		env["NODE_PATH"] = guestDepsDir + "/node_modules"
		command = []string{"node", "/opt/dagflows-node-runner.js", "--entrypoint", req.Entrypoint}
	case "go", "golang":
		command = []string{guestCodeDir + "/deployable"}
	}

	return Manifest{
		WorkflowRunID:  req.WorkflowRunID,
		NodeKey:        req.NodeKey,
		ExecutionToken: req.ExecutionToken,
		Language:       req.Language,
		Entrypoint:     req.Entrypoint,
		TimeoutSeconds: req.TimeoutSeconds,
		Host: ManifestPaths{
			WorkDir:      workload.WorkDir,
			CodeDir:      workload.CodeDir,
			DepsDir:      workload.DepsDir,
			InputPath:    workload.InputPath,
			ManifestPath: workload.ManifestPath,
		},
		Guest: ManifestPaths{
			WorkDir:      guestRoot,
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
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
