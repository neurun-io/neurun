package build

import (
	"errors"
	"testing"
)

// GitHub wraps every archive in one directory, so the manifest a repository is
// named by arrives one level down. Reading it as a nested file is how a Rust
// crate ends up built as Python.
func TestDetectRuntimeReadsTheRepositoryRoot(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		names   []string
		runtime Runtime
	}{
		{"rust", []string{"Cargo.toml", "src/main.rs"}, RuntimeRust},
		{"go", []string{"go.mod", "main.go"}, RuntimeGo},
		{"ruby", []string{"Gemfile", "main.rb"}, RuntimeRuby},
		{"node", []string{"package.json", "src/handler.ts"}, RuntimeNode},
		{"python", []string{"requirements.txt", "main.py"}, RuntimePython},
		{"a lone script", []string{"main.py"}, RuntimePython},
		// A crate that also ships tooling requirements is still a crate.
		{"manifest wins", []string{"Cargo.toml", "requirements.txt"}, RuntimeRust},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			runtime, err := DetectRuntime(test.names)
			if err != nil || runtime != test.runtime {
				t.Fatalf("DetectRuntime(%v) = %q, %v", test.names, runtime, err)
			}
		})
	}
}

// A manifest buried in a subdirectory belongs to something the repository
// contains, not to the repository.
func TestDetectRuntimeIgnoresNestedManifests(t *testing.T) {
	t.Parallel()
	if _, err := DetectRuntime([]string{
		"vendor/thing/Cargo.toml", "docs/go.mod",
	}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("nested manifests were detected: %v", err)
	}
}

func TestDetectRuntimeRefusesSourceItCannotName(t *testing.T) {
	t.Parallel()
	if _, err := DetectRuntime([]string{"README.md", "LICENSE"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unbuildable source error = %v", err)
	}
}
