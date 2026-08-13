package deployment

import (
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/neurun-io/neurun/internal/artifact"
	"github.com/neurun-io/neurun/internal/ids"
)

func ValidateIdentifier(field, value string) error {
	if err := ids.Validate(field, value); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return nil
}

func (record Deployment) Validate() error {
	if err := ValidateIdentifier("project_id", record.ProjectID); err != nil {
		return err
	}
	if err := ValidateIdentifier("deployment_id", record.ID); err != nil {
		return err
	}
	if err := ValidateIdentifier("app_id", record.AppID); err != nil {
		return err
	}
	if !record.Runtime.Valid() {
		return fmt.Errorf("%w: deployment runtime is invalid", ErrInvalid)
	}
	normalized, err := NormalizeEntryPoint(record.Runtime, record.EntryPoint)
	if err != nil || normalized != record.EntryPoint {
		return fmt.Errorf("%w: deployment entrypoint is not normalized", ErrInvalid)
	}
	if !record.Status.Valid() {
		return fmt.Errorf("%w: deployment status is invalid", ErrInvalid)
	}
	if record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() ||
		record.UpdatedAt.Before(record.CreatedAt) {
		return fmt.Errorf("%w: deployment timestamps are invalid", ErrInvalid)
	}
	if err := validateArtifact(record.Source, ArtifactSource); err != nil {
		return err
	}
	for index, build := range record.Builds {
		if err := validateBuild(build, index+1, record); err != nil {
			return err
		}
	}
	if len(record.Builds) == 0 {
		if record.Status != StatusUploaded {
			return fmt.Errorf("%w: deployment without a build must be uploaded", ErrInvalid)
		}
	} else if record.Status != record.Builds[len(record.Builds)-1].Status {
		return fmt.Errorf("%w: deployment status differs from latest build", ErrInvalid)
	}
	return nil
}

func validateBuild(build Build, expectedNumber int, record Deployment) error {
	if err := ValidateIdentifier("build_id", build.ID); err != nil {
		return err
	}
	if build.ProjectID != record.ProjectID || build.DeploymentID != record.ID {
		return fmt.Errorf("%w: build ownership is inconsistent", ErrInvalid)
	}
	if build.Number != expectedNumber {
		return fmt.Errorf("%w: build numbers must be contiguous", ErrInvalid)
	}
	if !build.Status.Valid() || build.Status == StatusUploaded ||
		build.Runtime != record.Runtime ||
		build.EntryPoint != record.EntryPoint ||
		build.SourceSHA256 != record.Source.SHA256 {
		return fmt.Errorf("%w: build metadata is inconsistent", ErrInvalid)
	}
	if build.StartedAt.IsZero() || build.StartedAt.Before(record.CreatedAt) {
		return fmt.Errorf("%w: build start time is invalid", ErrInvalid)
	}
	if build.FinishedAt != nil && build.FinishedAt.Before(build.StartedAt) {
		return fmt.Errorf("%w: build finish time is invalid", ErrInvalid)
	}
	switch build.Status {
	case StatusBuilding:
		if build.FinishedAt != nil || build.Failure != nil {
			return fmt.Errorf("%w: building build cannot be finished", ErrInvalid)
		}
	case StatusReady:
		if build.FinishedAt == nil || build.Failure != nil || len(build.Artifacts) == 0 {
			return fmt.Errorf("%w: ready build is incomplete", ErrInvalid)
		}
	case StatusFailed:
		if build.FinishedAt == nil || build.Failure == nil {
			return fmt.Errorf("%w: failed build requires failure metadata", ErrInvalid)
		}
	}
	kinds := make(map[string]struct{}, len(build.Artifacts))
	for _, stored := range build.Artifacts {
		if err := validateArtifact(stored, ""); err != nil {
			return err
		}
		if stored.Kind != ArtifactCodeLayer && stored.Kind != ArtifactInstallLayer {
			return fmt.Errorf("%w: build artifact kind is invalid", ErrInvalid)
		}
		if _, duplicate := kinds[stored.Kind]; duplicate {
			return fmt.Errorf("%w: build artifact kinds must be unique", ErrInvalid)
		}
		kinds[stored.Kind] = struct{}{}
	}
	if build.Status == StatusReady {
		if _, exists := kinds[ArtifactCodeLayer]; !exists {
			return fmt.Errorf("%w: ready build requires a code layer", ErrInvalid)
		}
	}
	return validateFailure(build.Failure)
}

func validateArtifact(record Artifact, expectedKind string) error {
	if err := ValidateIdentifier("artifact_id", record.ID); err != nil {
		return err
	}
	if expectedKind != "" && record.Kind != expectedKind {
		return fmt.Errorf("%w: artifact kind is invalid", ErrInvalid)
	}
	if record.Kind == "" || record.Name == "" || record.MediaType == "" ||
		record.SizeBytes < 0 || len(record.SHA256) != 64 ||
		record.CreatedAt.IsZero() {
		return fmt.Errorf("%w: artifact metadata is incomplete", ErrInvalid)
	}
	for _, character := range record.SHA256 {
		if !((character >= '0' && character <= '9') ||
			(character >= 'a' && character <= 'f')) {
			return fmt.Errorf("%w: artifact digest is invalid", ErrInvalid)
		}
	}
	if err := artifact.ValidateStorageKey(record.StorageKey); err != nil {
		return fmt.Errorf("%w: artifact storage handle: %v", ErrInvalid, err)
	}
	return nil
}

func validateFailure(failure *Failure) error {
	if failure == nil {
		return nil
	}
	if failure.Code == "" || len(failure.Code) > 128 ||
		strings.TrimSpace(failure.Message) == "" || len(failure.Message) > 4_096 {
		return fmt.Errorf("%w: failure metadata is invalid", ErrInvalid)
	}
	return nil
}

func ValidateRecovery(now time.Time, failure Failure) error {
	if now.IsZero() {
		return fmt.Errorf("%w: recovery time is required", ErrInvalid)
	}
	return validateFailure(&failure)
}

func defaultEntryPoint(runtime Runtime) string {
	switch runtime {
	case RuntimePython:
		return "main.py:handler"
	case RuntimeRuby:
		return "main.rb:handler"
	case RuntimeNode:
		return "src/handler.ts:handler"
	case RuntimeRust, RuntimeGo:
		// Empty: a compiled crate or module has one obvious target, and the
		// builder takes it when nothing selects between several.
		return ""
	}
	return ""
}

// NormalizeEntryPoint settles what a build will invoke. Python names a callable
// inside the source; Rust names a compiled binary, because there is nothing to
// import into.
func NormalizeEntryPoint(runtime Runtime, raw string) (string, error) {
	if !runtime.Valid() {
		return "", fmt.Errorf(
			"%w: runtime must be python, rust, go, ruby or node", ErrInvalid,
		)
	}
	switch runtime {
	case RuntimeRust:
		return normalizeBinaryName(strings.TrimSpace(raw))
	case RuntimeGo:
		return normalizePackagePath(strings.TrimSpace(raw))
	case RuntimeRuby:
		return normalizeScriptEntryPoint(strings.TrimSpace(raw), ".rb")
	case RuntimeNode:
		return normalizeNodeEntryPoint(strings.TrimSpace(raw))
	}
	entryPoint := strings.TrimSpace(raw)
	if entryPoint == "" {
		entryPoint = defaultEntryPoint(runtime)
	}
	if len(entryPoint) > 512 ||
		!utf8.ValidString(entryPoint) ||
		strings.ContainsRune(entryPoint, '\x00') ||
		strings.Contains(entryPoint, "\\") {
		return "", fmt.Errorf("%w: entrypoint is malformed", ErrInvalid)
	}
	subject, handler, ok := strings.Cut(entryPoint, ":")
	if !ok || subject == "" || handler == "" || strings.Contains(handler, ":") {
		return "", fmt.Errorf(
			"%w: entrypoint must use module_or_file:handler",
			ErrInvalid,
		)
	}
	if err := validateHandler(handler); err != nil {
		return "", err
	}
	if strings.HasSuffix(subject, ".py") || strings.Contains(subject, "/") {
		if err := validateRelativeSlashPath("entrypoint", subject); err != nil {
			return "", err
		}
		if !strings.HasSuffix(subject, ".py") {
			return "", fmt.Errorf("%w: file entrypoint must end in .py", ErrInvalid)
		}
		return subject + ":" + handler, nil
	}
	for component := range strings.SplitSeq(subject, ".") {
		if !pythonIdentifier(component) {
			return "", fmt.Errorf("%w: entrypoint module is invalid", ErrInvalid)
		}
	}
	return subject + ":" + handler, nil
}

// normalizeBinaryName accepts what cargo will accept as a [[bin]] target, so an
// entrypoint that cannot name a binary is refused before a build spends minutes
// discovering the same thing.
func normalizeBinaryName(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	if len(raw) > 64 {
		return "", fmt.Errorf("%w: entrypoint is malformed", ErrInvalid)
	}
	for _, character := range raw {
		if character == '_' || character == '-' ||
			(character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') {
			continue
		}
		return "", fmt.Errorf(
			"%w: entrypoint must name a binary target", ErrInvalid,
		)
	}
	return raw, nil
}

// normalizePackagePath accepts the main package a Go build targets, relative to
// the module root. Empty builds the root itself.
func normalizePackagePath(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	if len(raw) > 256 || strings.Contains(raw, "..") {
		return "", fmt.Errorf("%w: entrypoint is malformed", ErrInvalid)
	}
	if err := validateRelativeSlashPath("entrypoint", raw); err != nil {
		return "", err
	}
	return raw, nil
}

// normalizeNodeEntryPoint accepts file:export across every extension esbuild
// compiles, because TypeScript and JavaScript are one runtime here — the loader
// is chosen by the file, not by the deployment.
func normalizeNodeEntryPoint(raw string) (string, error) {
	if raw == "" {
		raw = defaultEntryPoint(RuntimeNode)
	}
	file, _, _ := strings.Cut(raw, ":")
	for _, extension := range []string{".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs"} {
		if strings.HasSuffix(file, extension) {
			return normalizeScriptEntryPoint(raw, extension)
		}
	}
	return "", fmt.Errorf(
		"%w: entrypoint must be a .ts, .tsx, .js, .jsx, .mjs or .cjs file", ErrInvalid,
	)
}

// normalizeScriptEntryPoint accepts file.ext:method for an interpreted runtime
// whose handler lives in a file rather than a module tree.
func normalizeScriptEntryPoint(raw, extension string) (string, error) {
	if raw == "" {
		raw = "main" + extension + ":handler"
	}
	if len(raw) > 512 || !utf8.ValidString(raw) ||
		strings.ContainsRune(raw, '\x00') || strings.Contains(raw, "\\") {
		return "", fmt.Errorf("%w: entrypoint is malformed", ErrInvalid)
	}
	file, handler, ok := strings.Cut(raw, ":")
	if !ok || file == "" || handler == "" || strings.Contains(handler, ":") {
		return "", fmt.Errorf("%w: entrypoint must use file:handler", ErrInvalid)
	}
	if err := validateRelativeSlashPath("entrypoint", file); err != nil {
		return "", err
	}
	if !strings.HasSuffix(file, extension) {
		return "", fmt.Errorf(
			"%w: file entrypoint must end in %s", ErrInvalid, extension,
		)
	}
	if err := validateHandler(handler); err != nil {
		return "", err
	}
	return file + ":" + handler, nil
}

func validateHandler(handler string) error {
	for component := range strings.SplitSeq(handler, ".") {
		if !pythonIdentifier(component) {
			return fmt.Errorf("%w: entrypoint handler is invalid", ErrInvalid)
		}
	}
	return nil
}

func pythonIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for index, character := range value {
		if character == '_' || unicode.IsLetter(character) ||
			(index > 0 && unicode.IsDigit(character)) {
			continue
		}
		return false
	}
	return true
}

func validateRelativeSlashPath(field, value string) error {
	if value == "" || strings.HasPrefix(value, "/") ||
		strings.HasSuffix(value, "/") || strings.Contains(value, "\\") {
		return fmt.Errorf("%w: %s must be a relative slash path", ErrInvalid, field)
	}
	for component := range strings.SplitSeq(value, "/") {
		if component == "" || component == "." || component == ".." {
			return fmt.Errorf("%w: %s contains an unsafe path component", ErrInvalid, field)
		}
	}
	return nil
}
