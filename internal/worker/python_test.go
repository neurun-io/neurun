package worker

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestPythonRunnerSupportsSyncAndAsyncHandlers(t *testing.T) {
	python := pythonForTest(t)
	code := t.TempDir()
	writeFile(t, filepath.Join(code, "sync_handler.py"), "def handler(event):\n    print('worker log')\n    return {'value': event['value'] + 1}\n")
	writeFile(t, filepath.Join(code, "async_handler.py"), "async def handler(event):\n    return {'value': event['value'] + 2}\n")
	runner, err := NewPythonRunner(PythonOptions{Executable: python})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		entrypoint string
		want       string
	}{{"sync_handler:handler", `{"value":2}`}, {"async_handler:handler", `{"value":3}`}} {
		result, err := runner.Execute(context.Background(), ExecuteRequest{CodeDirectory: code, Entrypoint: test.entrypoint, Input: json.RawMessage(`{"value":1}`), MaxResultBytes: 1024, MaxLogBytes: 1024})
		if err != nil {
			t.Fatal(err)
		}
		if string(result.Output) != test.want {
			t.Fatalf("got %s, want %s", result.Output, test.want)
		}
	}
}

func TestPythonRunnerInstantiatesCallableClassEntrypoint(t *testing.T) {
	python := pythonForTest(t)
	code := t.TempDir()
	writeFile(t, filepath.Join(code, "class_handler.py"), "class Scraper:\n    def __call__(self, event):\n        return {'value': event['value'] + 3}\n")
	runner, err := NewPythonRunner(PythonOptions{Executable: python})
	if err != nil {
		t.Fatal(err)
	}
	result, err := runner.Execute(context.Background(), ExecuteRequest{
		CodeDirectory:  code,
		Entrypoint:     "class_handler:Scraper",
		Input:          json.RawMessage(`{"value":1}`),
		MaxResultBytes: 1024,
		MaxLogBytes:    1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(result.Output) != `{"value":4}` {
		t.Fatalf("got %s, want class handler output", result.Output)
	}
}

func TestPythonRunnerRejectsOversizedResult(t *testing.T) {
	python := pythonForTest(t)
	code := t.TempDir()
	writeFile(t, filepath.Join(code, "main.py"), "def handler(event):\n    return 'x' * 100\n")
	runner, _ := NewPythonRunner(PythonOptions{Executable: python})
	_, err := runner.Execute(context.Background(), ExecuteRequest{CodeDirectory: code, Entrypoint: "main.py:handler", Input: json.RawMessage(`null`), MaxResultBytes: 10, MaxLogBytes: 1024})
	if !errors.Is(err, ErrResultTooLarge) {
		t.Fatalf("expected oversized result failure, got %v", err)
	}
}

func pythonForTest(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("python")
	if err != nil {
		t.Skip("python is not installed")
	}
	return path
}
func writeFile(t *testing.T, path, value string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
}
