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

	"github.com/neurun-io/neurun/internal/domain/build"
)

type PythonOptions struct {
	Executable string
}

type PythonBuilder struct {
	python string
}

func NewPythonBuilder(options PythonOptions) (*PythonBuilder, error) {
	if strings.TrimSpace(options.Executable) == "" {
		options.Executable = "python"
	}
	return &PythonBuilder{python: options.Executable}, nil
}

func (builder *PythonBuilder) Build(ctx context.Context, request Request) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	sourceDir := request.SourceDirectory
	compile := exec.CommandContext(ctx, builder.python, "-m", "compileall", "-q", sourceDir)
	compile.Env = append(os.Environ(), "PYTHONPYCACHEPREFIX="+filepath.Join(request.WorkDirectory, "pycache"))
	if err := request.run("compile Python source", compile); err != nil {
		return Result{}, err
	}
	codePath := filepath.Join(request.WorkDirectory, "code-layer.zip")
	if err := zipDirectory(sourceDir, codePath); err != nil {
		return Result{}, fmt.Errorf("builder: package code layer: %w", err)
	}
	result := Result{Layers: []Layer{{Name: build.LayerCode, Path: codePath}}}
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
	cache := request.CacheDirectory
	if cache == "" {
		cache = request.WorkDirectory
	}
	// The wheel cache is the whole win here: without it every build re-downloads
	// and, for anything without a wheel, recompiles.
	pip.Env = append(os.Environ(),
		"PIP_NO_INPUT=1",
		"PIP_CACHE_DIR="+filepath.Join(cache, "pip"),
	)
	if err := request.run("install Python requirements", pip); err != nil {
		return Result{}, err
	}
	installPath := filepath.Join(request.WorkDirectory, "install-layer.zip")
	if err := zipDirectory(installDir, installPath); err != nil {
		return Result{}, fmt.Errorf("builder: package install layer: %w", err)
	}
	result.Layers = append(result.Layers, Layer{Name: build.LayerInstall, Path: installPath})
	return result, nil
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
