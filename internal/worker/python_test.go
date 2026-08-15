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
	runner, err := NewPythonRunner(PythonOptions{Executable: python})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		source string
		want   string
	}{
		{
			"sync",
			"def handler(event):\n    print('worker log')\n    return {'value': event['value'] + 1}\n",
			`{"value":2}`,
		},
		{
			"async",
			"async def handler(event):\n    return {'value': event['value'] + 2}\n",
			`{"value":3}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			code := t.TempDir()
			writeFile(t, filepath.Join(code, "main.py"), test.source)
			result, err := runner.Execute(context.Background(), ExecuteRequest{CodeDirectory: code, Input: json.RawMessage(`{"value":1}`), MaxResultBytes: 1024, MaxLogBytes: 1024})
			if err != nil {
				t.Fatal(err)
			}
			if string(result.Output) != test.want {
				t.Fatalf("got %s, want %s", result.Output, test.want)
			}
		})
	}
}

func TestPythonRunnerInstantiatesCallableClassHandler(t *testing.T) {
	python := pythonForTest(t)
	code := t.TempDir()
	writeFile(t, filepath.Join(code, "main.py"), "class handler:\n    def __call__(self, event):\n        return {'value': event['value'] + 3}\n")
	runner, err := NewPythonRunner(PythonOptions{Executable: python})
	if err != nil {
		t.Fatal(err)
	}
	result, err := runner.Execute(context.Background(), ExecuteRequest{
		CodeDirectory:  code,
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
	_, err := runner.Execute(context.Background(), ExecuteRequest{CodeDirectory: code, Input: json.RawMessage(`null`), MaxResultBytes: 10, MaxLogBytes: 1024})
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
