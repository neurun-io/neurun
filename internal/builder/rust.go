package builder

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/neurun-io/neurun/internal/domain/build"
)

type RustOptions struct {
	Executable string
}

// RustBuilder compiles a crate into one executable.
//
// It produces a code layer and never an install layer: cargo resolves and links
// dependencies into the binary, so there is nothing left to ship beside it. That
// is the whole difference from Python, where the install layer is a second
// directory the handler imports from at run time.
type RustBuilder struct {
	cargo string
}

func NewRustBuilder(options RustOptions) (*RustBuilder, error) {
	if strings.TrimSpace(options.Executable) == "" {
		options.Executable = "cargo"
	}
	return &RustBuilder{cargo: options.Executable}, nil
}

func (builder *RustBuilder) Build(ctx context.Context, request Request) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	sourceDir := request.SourceDirectory
	manifest := filepath.Join(sourceDir, "Cargo.toml")
	if info, err := os.Lstat(manifest); err != nil || !info.Mode().IsRegular() {
		return Result{}, errors.New("builder: Cargo.toml was not found at the source root")
	}

	// --locked refuses to update Cargo.lock, so a build compiles the dependency
	// versions the commit pinned rather than whatever resolves today.
	compile := exec.CommandContext(
		ctx, builder.cargo, "build", "--release", "--locked",
	)
	compile.Dir = sourceDir
	cache := request.CacheDirectory
	if cache == "" {
		cache = request.WorkDirectory
	}
	compile.Env = append(os.Environ(),
		// What the cache is for: the registry cargo resolves against and the
		// intermediate objects it fingerprints, so the next deployment of this
		// crate compiles what changed instead of everything. The binary is
		// copied out of it — the cache holds the environment, not the release.
		"CARGO_HOME="+filepath.Join(cache, "cargo"),
		"CARGO_TARGET_DIR="+filepath.Join(cache, "target"),
		"CARGO_TERM_COLOR=never",
	)
	if err := request.run("compile Rust source", compile); err != nil {
		return Result{}, err
	}

	releaseDir := filepath.Join(cache, "target", "release")
	binary, err := builder.locateBinary(ctx, sourceDir, releaseDir, compile.Env)
	if err != nil {
		return Result{}, err
	}
	codePath := filepath.Join(request.WorkDirectory, "code-layer.zip")
	if err := zipFileAs(binary, CompiledBinaryName, codePath); err != nil {
		return Result{}, fmt.Errorf("builder: package code layer: %w", err)
	}
	return Result{Layers: []Layer{{Name: build.LayerCode, Path: codePath}}}, nil
}

// locateBinary finds what this crate builds by asking cargo, not by reading the
// release directory: the target directory is kept warm between builds, so what
// is lying in it includes binaries earlier builds left behind.
//
// The crate has to declare exactly one, since nothing else says which of
// several a deployment meant.
func (builder *RustBuilder) locateBinary(
	ctx context.Context,
	sourceDir string,
	releaseDir string,
	environment []string,
) (string, error) {
	metadata := exec.CommandContext(
		ctx, builder.cargo, "metadata", "--no-deps", "--format-version", "1",
	)
	metadata.Dir = sourceDir
	metadata.Env = environment
	encoded, err := metadata.Output()
	if err != nil {
		return "", fmt.Errorf("builder: read cargo metadata: %w", err)
	}
	var described struct {
		Packages []struct {
			Targets []struct {
				Name string   `json:"name"`
				Kind []string `json:"kind"`
			} `json:"targets"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(encoded, &described); err != nil {
		return "", fmt.Errorf("builder: decode cargo metadata: %w", err)
	}
	var names []string
	for _, pkg := range described.Packages {
		for _, target := range pkg.Targets {
			if slices.Contains(target.Kind, "bin") {
				names = append(names, target.Name)
			}
		}
	}
	switch len(names) {
	case 1:
	case 0:
		return "", errors.New("builder: the crate declares no binary")
	default:
		return "", fmt.Errorf(
			"builder: the crate declares several binaries: %s",
			strings.Join(names, ", "),
		)
	}
	binary := filepath.Join(releaseDir, names[0]+executableExtension())
	if info, err := os.Lstat(binary); err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("builder: cargo produced no %s", filepath.Base(binary))
	}
	return binary, nil
}
