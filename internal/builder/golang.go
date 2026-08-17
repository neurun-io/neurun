package builder

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/neurun-io/neurun/internal/domain/build"
)

type GoOptions struct {
	Executable string
}

// GoBuilder compiles a module into one executable. Like Rust and unlike Python,
// it emits no install layer: the dependencies are linked in.
type GoBuilder struct {
	golang string
}

func NewGoBuilder(options GoOptions) (*GoBuilder, error) {
	if strings.TrimSpace(options.Executable) == "" {
		options.Executable = "go"
	}
	return &GoBuilder{golang: options.Executable}, nil
}

func (builder *GoBuilder) Build(ctx context.Context, request Request) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	sourceDir := request.SourceDirectory
	if info, err := os.Lstat(filepath.Join(sourceDir, "go.mod")); err != nil ||
		!info.Mode().IsRegular() {
		return Result{}, errors.New("builder: go.mod was not found at the source root")
	}

	binary := filepath.Join(request.WorkDirectory, CompiledBinaryName)
	// -mod=readonly refuses to edit go.mod, so a build compiles what the commit
	// pinned; -trimpath keeps build paths out of the binary.
	compile := exec.CommandContext(
		ctx, builder.golang, "build", "-mod=readonly", "-trimpath", "-o", binary, ".",
	)
	compile.Dir = sourceDir
	cache := request.CacheDirectory
	if cache == "" {
		cache = request.WorkDirectory
	}
	compile.Env = append(os.Environ(),
		"GOCACHE="+filepath.Join(cache, "go-build"),
		"GOMODCACHE="+filepath.Join(cache, "go-mod"),
		"CGO_ENABLED=0",
	)
	if err := request.run("compile Go source", compile); err != nil {
		return Result{}, err
	}
	if info, err := os.Lstat(binary); err != nil || !info.Mode().IsRegular() {
		return Result{}, errors.New("builder: go build produced no binary")
	}

	codePath := filepath.Join(request.WorkDirectory, "code-layer.zip")
	if err := zipFileAs(binary, CompiledBinaryName, codePath); err != nil {
		return Result{}, fmt.Errorf("builder: package code layer: %w", err)
	}
	return Result{Layers: []Layer{{Name: build.LayerCode, Path: codePath}}}, nil
}

var _ Builder = (*GoBuilder)(nil)
