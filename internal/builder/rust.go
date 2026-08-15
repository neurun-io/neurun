package builder

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/neurun-io/neurun/internal/domain/build"
	"github.com/neurun-io/neurun/internal/files"
)

type RustOptions struct {
	CargoExecutable         string
	MaxArchiveEntries       int
	MaxArchiveExpandedBytes int64
}

// RustBuilder compiles a crate into one executable.
//
// It produces a code layer and never an install layer: cargo resolves and links
// dependencies into the binary, so there is nothing left to ship beside it. That
// is the whole difference from Python, where the install layer is a second
// directory the handler imports from at run time.
type RustBuilder struct {
	cargo  string
	limits files.ArchiveLimits
}

func NewRust(options RustOptions) (*RustBuilder, error) {
	if strings.TrimSpace(options.CargoExecutable) == "" {
		options.CargoExecutable = "cargo"
	}
	if options.MaxArchiveEntries < 0 || options.MaxArchiveExpandedBytes < 0 {
		return nil, errors.New("builder: archive limits cannot be negative")
	}
	return &RustBuilder{
		cargo: options.CargoExecutable,
		limits: files.ArchiveLimits{
			MaxEntries:       options.MaxArchiveEntries,
			MaxExpandedBytes: options.MaxArchiveExpandedBytes,
		},
	}, nil
}

func (builder *RustBuilder) Build(ctx context.Context, request Request) (Result, error) {
	if request.Runtime != build.RuntimeRust {
		return Result{}, fmt.Errorf("builder: unsupported runtime %q", request.Runtime)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(request.SourceArchivePath) == "" ||
		strings.TrimSpace(request.WorkDirectory) == "" {
		return Result{}, errors.New("builder: source archive and work directory are required")
	}
	sourceDir := filepath.Join(request.WorkDirectory, "source")
	if _, err := files.ExtractZIPFile(
		request.SourceArchivePath, sourceDir, builder.limits,
	); err != nil {
		return Result{}, fmt.Errorf("builder: extract source: %w", err)
	}
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
	binary, err := locateBinary(releaseDir, "")
	if err != nil {
		return Result{}, err
	}
	codePath := filepath.Join(request.WorkDirectory, "code-layer.zip")
	if err := zipFileAs(binary, CompiledBinaryName, codePath); err != nil {
		return Result{}, fmt.Errorf("builder: package code layer: %w", err)
	}
	return Result{Layers: []Layer{{Name: build.LayerCode, Path: codePath}}}, nil
}

// locateBinary finds what cargo produced. A named entrypoint must exist under
// that name; without one, the crate has to produce exactly one executable, so a
// workspace of several binaries is refused rather than guessed at.
func locateBinary(releaseDir, entryPoint string) (string, error) {
	if entryPoint != "" {
		candidate := filepath.Join(releaseDir, entryPoint)
		if info, err := os.Lstat(candidate); err == nil && info.Mode().IsRegular() {
			return candidate, nil
		}
		return "", fmt.Errorf("builder: cargo produced no binary named %q", entryPoint)
	}
	entries, err := os.ReadDir(releaseDir)
	if err != nil {
		return "", fmt.Errorf("builder: read cargo output: %w", err)
	}
	var found []string
	for _, entry := range entries {
		if entry.IsDir() || !executableOutput(entry) {
			continue
		}
		found = append(found, filepath.Join(releaseDir, entry.Name()))
	}
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return "", errors.New("builder: cargo produced no binary")
	default:
		return "", errors.New(
			"builder: cargo produced several binaries; name one as the entrypoint",
		)
	}
}

// executableOutput separates a linked binary from the build metadata cargo
// leaves beside it — .d dependency files, .pdb symbols, and the like.
func executableOutput(entry fs.DirEntry) bool {
	name := entry.Name()
	if strings.HasPrefix(name, ".") || strings.Contains(name, ".") &&
		!strings.HasSuffix(name, ".exe") {
		return false
	}
	info, err := entry.Info()
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	return true
}
