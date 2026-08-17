package builder

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/neurun-io/neurun/internal/domain/build"
)

type RubyOptions struct {
	Executable string
}

// RubyBuilder packages source and gems as two layers, the same shape as Python:
// the code layer is what the handler is, the install layer is what it loads.
type RubyBuilder struct {
	bundle string
}

func NewRubyBuilder(options RubyOptions) (*RubyBuilder, error) {
	if strings.TrimSpace(options.Executable) == "" {
		options.Executable = "bundle"
	}
	return &RubyBuilder{bundle: options.Executable}, nil
}

func (builder *RubyBuilder) Build(ctx context.Context, request Request) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	sourceDir := request.SourceDirectory
	codePath := filepath.Join(request.WorkDirectory, "code-layer.zip")
	if err := zipDirectory(sourceDir, codePath); err != nil {
		return Result{}, fmt.Errorf("builder: package code layer: %w", err)
	}
	result := Result{Layers: []Layer{{Name: build.LayerCode, Path: codePath}}}

	// No Gemfile is a handler on the standard library alone, which is a complete
	// app and needs no second layer.
	gemfile := filepath.Join(sourceDir, "Gemfile")
	if info, err := os.Lstat(gemfile); err != nil || !info.Mode().IsRegular() {
		return result, nil
	}
	installDir := filepath.Join(request.WorkDirectory, "install")
	if err := os.Mkdir(installDir, 0o700); err != nil {
		return Result{}, fmt.Errorf("builder: create install layer: %w", err)
	}
	// --deployment refuses to resolve anything Gemfile.lock did not pin.
	install := exec.CommandContext(
		ctx, builder.bundle, "install", "--deployment", "--path", installDir,
	)
	install.Dir = sourceDir
	cache := request.CacheDirectory
	if cache == "" {
		cache = request.WorkDirectory
	}
	// The gems still install into the layer; only the downloads are shared.
	install.Env = append(os.Environ(),
		"BUNDLE_PATH="+installDir,
		"GEM_SPEC_CACHE="+filepath.Join(cache, "gem-spec"),
		"BUNDLE_USER_CACHE="+filepath.Join(cache, "bundle"),
	)
	if err := request.run("install Ruby gems", install); err != nil {
		return Result{}, err
	}
	installPath := filepath.Join(request.WorkDirectory, "install-layer.zip")
	if err := zipDirectory(installDir, installPath); err != nil {
		return Result{}, fmt.Errorf("builder: package install layer: %w", err)
	}
	result.Layers = append(result.Layers, Layer{Name: build.LayerInstall, Path: installPath})
	return result, nil
}

var _ Builder = (*RubyBuilder)(nil)
