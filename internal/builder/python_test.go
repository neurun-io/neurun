package builder

import (
	"archive/zip"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/neurun-io/neurun/internal/domain/deployment"
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

func TestPythonBuilderRejectsMissingEntrypoint(t *testing.T) {
	python := pythonForTest(t)
	root := t.TempDir()
	source := filepath.Join(root, "source.zip")
	writeZIP(t, source, map[string]string{"other.py": "value = 1\n"})
	builder, err := NewPython(PythonOptions{PythonExecutable: python})
	if err != nil {
		t.Fatal(err)
	}
	_, err = builder.Build(context.Background(), Request{Runtime: deployment.RuntimePython, EntryPoint: "main.py:handler", SourceArchivePath: source, WorkDirectory: filepath.Join(root, "work")})
	if err == nil {
		t.Fatal("expected missing entrypoint error")
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
	result, err := builder.Build(context.Background(), Request{Runtime: deployment.RuntimePython, EntryPoint: "main.py:handler", SourceArchivePath: source, WorkDirectory: work})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0].Kind != deployment.ArtifactCodeLayer {
		t.Fatalf("unexpected artifacts: %#v", result.Artifacts)
	}
	return result.Artifacts[0].Path
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
