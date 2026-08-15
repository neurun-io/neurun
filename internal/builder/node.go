package builder

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	esbuild "github.com/evanw/esbuild/pkg/api"

	"github.com/neurun-io/neurun/internal/domain/build"
	"github.com/neurun-io/neurun/internal/files"
)

// BundleName is the single file a Node build ships. Like the compiled runtimes,
// the name is fixed so the runner needs no manifest to find it.
const BundleName = "handler.js"

type NodeOptions struct {
	NPMExecutable           string
	MaxArchiveEntries       int
	MaxArchiveExpandedBytes int64
}

// NodeBuilder installs dependencies and bundles them into one file.
//
// It ships no install layer, which is not a stylistic choice: a node_modules
// tree is tens of thousands of files, and both packaging it and re-extracting it
// on every execution would dwarf the work the handler does. Bundling trades that
// for the two things it cannot do — native .node addons and require() of a
// computed path — neither of which a handler needs here, because the browser is
// a separate service rather than an in-process dependency.
//
// esbuild is linked in rather than shelled out to, so the version is the
// server's and an image without it is not a class of failure.
type NodeBuilder struct {
	npm    string
	limits files.ArchiveLimits
}

func NewNode(options NodeOptions) (*NodeBuilder, error) {
	if strings.TrimSpace(options.NPMExecutable) == "" {
		options.NPMExecutable = "npm"
	}
	if options.MaxArchiveEntries < 0 || options.MaxArchiveExpandedBytes < 0 {
		return nil, errors.New("builder: archive limits cannot be negative")
	}
	return &NodeBuilder{
		npm: options.NPMExecutable,
		limits: files.ArchiveLimits{
			MaxEntries:       options.MaxArchiveEntries,
			MaxExpandedBytes: options.MaxArchiveExpandedBytes,
		},
	}, nil
}

func (builder *NodeBuilder) Build(ctx context.Context, request Request) (Result, error) {
	if request.Runtime != build.RuntimeNode {
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
	if info, err := os.Lstat(filepath.Join(sourceDir, "package.json")); err != nil ||
		!info.Mode().IsRegular() {
		return Result{}, errors.New("builder: package.json was not found at the source root")
	}
	entry, _, _ := strings.Cut(request.EntryPoint, ":")
	entryPath := filepath.Join(sourceDir, filepath.FromSlash(entry))
	if info, err := os.Lstat(entryPath); err != nil || !info.Mode().IsRegular() {
		return Result{}, fmt.Errorf("builder: entrypoint file %q was not found", entry)
	}

	cache := request.CacheDirectory
	if cache == "" {
		cache = request.WorkDirectory
	}
	// npm ci refuses to resolve anything the lockfile did not pin, and refuses
	// outright without one — the same contract as cargo --locked.
	if info, err := os.Lstat(filepath.Join(sourceDir, "package-lock.json")); err == nil &&
		info.Mode().IsRegular() {
		install := exec.CommandContext(
			ctx, builder.npm, "ci", "--no-audit", "--no-fund",
		)
		install.Dir = sourceDir
		install.Env = append(os.Environ(),
			"npm_config_cache="+filepath.Join(cache, "npm"),
			"npm_config_update_notifier=false",
		)
		if err := request.run("install Node dependencies", install); err != nil {
			return Result{}, err
		}
	} else if hasDependencies(sourceDir) {
		return Result{}, errors.New(
			"builder: package-lock.json is required to install dependencies",
		)
	}

	bundlePath := filepath.Join(request.WorkDirectory, BundleName)
	// Inline maps rather than a second artifact: a stack trace that points into
	// a bundle nobody can map is a log entry nobody can act on.
	result := esbuild.Build(esbuild.BuildOptions{
		EntryPoints:   []string{entryPath},
		Outfile:       bundlePath,
		Bundle:        true,
		Write:         true,
		Platform:      esbuild.PlatformNode,
		Format:        esbuild.FormatCommonJS,
		Target:        esbuild.ESNext,
		Sourcemap:     esbuild.SourceMapInline,
		LogLevel:      esbuild.LogLevelSilent,
		AbsWorkingDir: sourceDir,
	})
	if len(result.Errors) > 0 {
		return Result{}, fmt.Errorf(
			"builder: bundle Node source: %s", bundleFailure(result.Errors),
		)
	}

	codePath := filepath.Join(request.WorkDirectory, "code-layer.zip")
	if err := zipFileAs(bundlePath, BundleName, codePath); err != nil {
		return Result{}, fmt.Errorf("builder: package code layer: %w", err)
	}
	return Result{Artifacts: []Output{{
		Kind:      build.ArtifactCodeLayer,
		Name:      "code-layer.zip",
		MediaType: "application/zip",
		Path:      codePath,
	}}}, nil
}

// hasDependencies reports whether the manifest asks for anything, so a handler
// on the standard library alone is not refused for lacking a lockfile.
func hasDependencies(sourceDir string) bool {
	contents, err := os.ReadFile(filepath.Join(sourceDir, "package.json"))
	if err != nil {
		return false
	}
	text := string(contents)
	return strings.Contains(text, `"dependencies"`) ||
		strings.Contains(text, `"optionalDependencies"`)
}

func bundleFailure(messages []esbuild.Message) string {
	reasons := make([]string, 0, len(messages))
	for index, message := range messages {
		if index == 8 {
			break
		}
		if message.Location != nil {
			reasons = append(reasons, fmt.Sprintf(
				"%s:%d: %s", message.Location.File, message.Location.Line, message.Text,
			))
			continue
		}
		reasons = append(reasons, message.Text)
	}
	return strings.Join(reasons, "; ")
}

var _ Builder = (*NodeBuilder)(nil)
