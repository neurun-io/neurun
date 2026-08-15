package build

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Runtime identifies a pluggable builder/runner pair.
type Runtime string

const (
	RuntimePython Runtime = "python"
	RuntimeRust   Runtime = "rust"
	RuntimeGo     Runtime = "go"
	RuntimeRuby   Runtime = "ruby"
	RuntimeNode   Runtime = "node"
)

// Compiled runtimes link their dependencies into one executable, so a build
// produces a code layer and never an install layer.
func (runtime Runtime) Compiled() bool {
	return runtime == RuntimeRust || runtime == RuntimeGo
}

func (runtime Runtime) Valid() bool {
	switch runtime {
	case RuntimePython, RuntimeRust, RuntimeGo, RuntimeRuby, RuntimeNode:
		return true
	}
	return false
}

func ParseRuntime(raw string) (Runtime, error) {
	runtime := Runtime(strings.ToLower(strings.TrimSpace(raw)))
	if !runtime.Valid() {
		return "", fmt.Errorf("%w: runtime must be python, rust, go, ruby or node", ErrInvalid)
	}
	return runtime, nil
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
