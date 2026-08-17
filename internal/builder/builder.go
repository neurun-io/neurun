// Package builder implements language-specific deployment build adapters.
package builder

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
)

type Request struct {
	// SourceDirectory holds the commit, already extracted. Unpacking is the
	// deployment's job — a builder is handed source and runs a toolchain over
	// it, the way a runner is handed a build and runs it.
	SourceDirectory string
	// WorkDirectory is scratch for one build and is deleted after it.
	WorkDirectory string
	// CacheDirectory outlives the build. Toolchain caches belong here and
	// nowhere else: a compiler cache under WorkDirectory is deleted before it is
	// ever read again, which makes every build a cold one.
	CacheDirectory string
	// Logs takes what the toolchain prints, as it prints it. A build runs for
	// minutes and somebody is watching it; handing the output back at the end
	// would be handing it over after the interesting part.
	Logs io.Writer
}

// run executes one toolchain command, streaming its output to whoever is
// watching and keeping a copy for the error, where the tail is what explains
// a failure.
func (request Request) run(action string, command *exec.Cmd) error {
	var captured bytes.Buffer
	sink := io.Writer(&captured)
	if request.Logs != nil {
		fmt.Fprintf(request.Logs, "$ %s\n", action)
		sink = io.MultiWriter(&captured, request.Logs)
	}
	command.Stdout = sink
	command.Stderr = sink
	if err := command.Run(); err != nil {
		return commandError(action, captured.Bytes(), err)
	}
	return nil
}

// Layer is one ZIP a build produced, still on local disk. Name is what it is
// to the runtime — the directory a runner unpacks it into.
type Layer struct {
	Name string
	Path string
}

// Result is what a build produced. The service hashes and stores each layer;
// the builder only says what it made and where it put it.
type Result struct {
	Layers []Layer
}

// Builder is the runtime-neutral boundary around language toolchains. Only
// Python is wired today; the seam is what lets a second runtime arrive without
// the deployment service learning anything about it.
type Builder interface {
	Build(context.Context, Request) (Result, error)
}
