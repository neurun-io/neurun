package builder

import (
	"archive/zip"
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
	source := filepath.Join(root, "source.zip")
	work := filepath.Join(root, "work")
	if err := os.Mkdir(work, 0o700); err != nil {
		t.Fatal(err)
	}
	writeZIP(t, source, map[string]string{"main.py": "def handler(event):\n    return event\n", "requirements.txt": ""})
	builder, err := NewPython(PythonOptions{PythonExecutable: python})
	if err != nil {
		t.Fatal(err)
	}
	result, err := builder.Build(context.Background(), Request{Runtime: build.RuntimePython, SourceArchivePath: source, WorkDirectory: work})
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

func writeZIP(t *testing.T, target string, files map[string]string) {
	t.Helper()
	output, err := os.Create(target)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(output)
	for name, contents := range files {
		writer, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write([]byte(contents)); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
}
