package build

import (
	"fmt"
	"path"
	"strings"
)

// markers are the files a toolchain is named by. Order decides ties: a Rust
// crate with a helper script in it is still a Rust crate, and requirements.txt
// is last because a repository of any runtime may carry one for its tooling.
var markers = []struct {
	name    string
	runtime Runtime
}{
	{"Cargo.toml", RuntimeRust},
	{"go.mod", RuntimeGo},
	{"Gemfile", RuntimeRuby},
	{"package.json", RuntimeNode},
	{"requirements.txt", RuntimePython},
}

// DetectRuntime reads the runtime off the source itself, which is the only
// signal a push carries: a repository says what it is by what is at its root,
// and nothing in the delivery does.
func DetectRuntime(names []string) (Runtime, error) {
	roots := make(map[string]struct{}, len(names))
	extensions := make(map[string]struct{}, 4)
	for _, name := range names {
		cleaned := strings.TrimPrefix(path.Clean(strings.ReplaceAll(name, `\`, "/")), "./")
		if strings.Contains(cleaned, "/") {
			continue
		}
		roots[cleaned] = struct{}{}
		if dot := strings.LastIndex(cleaned, "."); dot >= 0 {
			extensions[cleaned[dot:]] = struct{}{}
		}
	}
	for _, marker := range markers {
		if _, found := roots[marker.name]; found {
			return marker.runtime, nil
		}
	}
	// No manifest, so the source is a loose script or nothing buildable.
	for extension, runtime := range map[string]Runtime{
		".py": RuntimePython,
		".rb": RuntimeRuby,
		".ts": RuntimeNode,
		".js": RuntimeNode,
	} {
		if _, found := extensions[extension]; found {
			return runtime, nil
		}
	}
	return "", fmt.Errorf(
		"%w: no Cargo.toml, go.mod, Gemfile, package.json or requirements.txt at the repository root",
		ErrInvalid,
	)
}
