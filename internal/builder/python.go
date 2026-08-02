package builder

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/neurun-io/neurun/internal/artifact"
	"github.com/neurun-io/neurun/internal/domain/deployment"
)

type PythonOptions struct {
	PythonExecutable        string
	MaxArchiveEntries       int
	MaxArchiveExpandedBytes int64
}

type PythonBuilder struct {
	python string
	limits artifact.ArchiveLimits
}

func NewPython(options PythonOptions) (*PythonBuilder, error) {
	if strings.TrimSpace(options.PythonExecutable) == "" {
		options.PythonExecutable = "python"
	}
	if options.MaxArchiveEntries < 0 || options.MaxArchiveExpandedBytes < 0 {
		return nil, errors.New("builder: archive limits cannot be negative")
	}
	return &PythonBuilder{python: options.PythonExecutable, limits: artifact.ArchiveLimits{
		MaxEntries: options.MaxArchiveEntries, MaxExpandedBytes: options.MaxArchiveExpandedBytes,
	}}, nil
}

func (builder *PythonBuilder) Build(ctx context.Context, request Request) (Result, error) {
	if request.Runtime != deployment.RuntimePython {
		return Result{}, fmt.Errorf("builder: unsupported runtime %q", request.Runtime)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(request.SourceArchivePath) == "" || strings.TrimSpace(request.WorkDirectory) == "" {
		return Result{}, errors.New("builder: source archive and work directory are required")
	}
	sourceDir := filepath.Join(request.WorkDirectory, "source")
	if _, err := artifact.ExtractZIPFile(request.SourceArchivePath, sourceDir, builder.limits); err != nil {
		return Result{}, fmt.Errorf("builder: extract source: %w", err)
	}
	if err := validateEntrypoint(sourceDir, request.EntryPoint); err != nil {
		return Result{}, err
	}
	compile := exec.CommandContext(ctx, builder.python, "-m", "compileall", "-q", sourceDir)
	compile.Env = append(os.Environ(), "PYTHONPYCACHEPREFIX="+filepath.Join(request.WorkDirectory, "pycache"))
	if output, err := compile.CombinedOutput(); err != nil {
		return Result{}, commandError("compile Python source", output, err)
	}
	codePath := filepath.Join(request.WorkDirectory, "code-layer.zip")
	if err := zipDirectory(sourceDir, codePath); err != nil {
		return Result{}, fmt.Errorf("builder: package code layer: %w", err)
	}
	result := Result{Artifacts: []Output{{Kind: deployment.ArtifactCodeLayer, Name: "code-layer.zip", MediaType: "application/zip", Path: codePath}}}
	requirements := filepath.Join(sourceDir, "requirements.txt")
	info, err := os.Stat(requirements)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return Result{}, fmt.Errorf("builder: inspect requirements.txt: %w", err)
	}
	if !info.Mode().IsRegular() {
		return Result{}, errors.New("builder: requirements.txt must be a regular file")
	}
	contents, err := os.ReadFile(requirements)
	if err != nil {
		return Result{}, fmt.Errorf("builder: read requirements.txt: %w", err)
	}
	if strings.TrimSpace(string(contents)) == "" {
		return result, nil
	}
	installDir := filepath.Join(request.WorkDirectory, "install")
	if err := os.Mkdir(installDir, 0o700); err != nil {
		return Result{}, fmt.Errorf("builder: create install layer: %w", err)
	}
	pip := exec.CommandContext(ctx, builder.python, "-m", "pip", "install", "--disable-pip-version-check", "--no-input", "--target", installDir, "-r", requirements)
	pip.Env = append(os.Environ(), "PIP_NO_INPUT=1")
	if output, err := pip.CombinedOutput(); err != nil {
		return Result{}, commandError("install Python requirements", output, err)
	}
	installPath := filepath.Join(request.WorkDirectory, "install-layer.zip")
	if err := zipDirectory(installDir, installPath); err != nil {
		return Result{}, fmt.Errorf("builder: package install layer: %w", err)
	}
	result.Artifacts = append(result.Artifacts, Output{Kind: deployment.ArtifactInstallLayer, Name: "install-layer.zip", MediaType: "application/zip", Path: installPath})
	return result, nil
}

func validateEntrypoint(sourceDir, entrypoint string) error {
	subject, handler, ok := strings.Cut(entrypoint, ":")
	if !ok || subject == "" || handler == "" {
		return errors.New("builder: entrypoint must use module_or_file:handler")
	}
	var candidates []string
	if strings.HasSuffix(subject, ".py") || strings.Contains(subject, "/") {
		candidates = []string{filepath.FromSlash(subject)}
	} else {
		modulePath := filepath.FromSlash(strings.ReplaceAll(subject, ".", "/"))
		candidates = []string{modulePath + ".py", filepath.Join(modulePath, "__init__.py")}
	}
	for _, candidate := range candidates {
		info, err := os.Lstat(filepath.Join(sourceDir, candidate))
		if err == nil && info.Mode().IsRegular() {
			return nil
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("builder: inspect entrypoint: %w", err)
		}
	}
	return fmt.Errorf("builder: entrypoint module %q was not found", subject)
}

func zipDirectory(root, target string) error {
	var names []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root || entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return fmt.Errorf("non-regular build input %q", path)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		names = append(names, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(names)
	output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	archive := zip.NewWriter(output)
	for _, name := range names {
		input, err := os.Open(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			archive.Close()
			output.Close()
			return err
		}
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetMode(0o644)
		header.Modified = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)
		writer, err := archive.CreateHeader(header)
		if err == nil {
			_, err = io.Copy(writer, input)
		}
		closeErr := input.Close()
		if err != nil {
			archive.Close()
			output.Close()
			return err
		}
		if closeErr != nil {
			archive.Close()
			output.Close()
			return closeErr
		}
	}
	if err := archive.Close(); err != nil {
		output.Close()
		return err
	}
	return output.Close()
}

func commandError(action string, output []byte, err error) error {
	message := strings.TrimSpace(string(output))
	if len(message) > 4096 {
		message = message[len(message)-4096:]
	}
	if message == "" {
		return fmt.Errorf("builder: %s: %w", action, err)
	}
	return fmt.Errorf("builder: %s: %w: %s", action, err, message)
}

var _ Builder = (*PythonBuilder)(nil)
