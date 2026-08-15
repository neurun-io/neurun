package build

import (
	"fmt"
	"strings"
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
