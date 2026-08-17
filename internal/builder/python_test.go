package builder

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/neurun-io/neurun/internal/domain/build"
)

func TestPythonBuilderProducesStableCodeLayer(t *testing.T) {
	python := pythonForTest(t)
	first := buildFixture(t, python)
	second := buildFixture(t, python)
	left, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	right, err := os.ReadFile(second)
	if err != nil {
		t.Fatal(err)
	}
	if string(left) != string(right) {
		t.Fatal("equivalent sources produced different code layers")
	}
}

func buildFixture(t *testing.T, python string) string {
	t.Helper()
	root := t.TempDir()
	source := filepath.Join(root, "source")
	work := filepath.Join(root, "work")
	for _, directory := range []string{source, work} {
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	writeSource(t, source, map[string]string{
		"main.py":          "def handler(event):\n    return event\n",
		"requirements.txt": "",
	})
	builder, err := NewPythonBuilder(PythonOptions{Executable: python})
	if err != nil {
		t.Fatal(err)
	}
	result, err := builder.Build(context.Background(), Request{
		SourceDirectory: source, WorkDirectory: work,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Layers) != 1 || result.Layers[0].Name != build.LayerCode {
		t.Fatalf("unexpected artifacts: %#v", result.Layers)
	}
	return result.Layers[0].Path
}

func pythonForTest(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("python")
	if err != nil {
		t.Skip("python is not installed")
	}
	return path
}

func writeSource(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, contents := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
