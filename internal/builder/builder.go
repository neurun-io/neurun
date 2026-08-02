// Package builder implements language-specific deployment build adapters.
package builder

import (
	"context"

	"github.com/neurun-io/neurun/internal/domain/deployment"
)

type Request struct {
	Runtime           deployment.Runtime
	EntryPoint        string
	SourceArchivePath string
	WorkDirectory     string
}

// Output is one file a build produced, still on local disk. The service hashes
// and stores it; the builder only says what it is and where it put it.
type Output struct {
	Kind      string
	Name      string
	MediaType string
	Path      string
}

type Result struct {
	Artifacts []Output
}

// Builder is the runtime-neutral boundary around language toolchains. Only
// Python is wired today; the seam is what lets a second runtime arrive without
// the deployment service learning anything about it.
type Builder interface {
	Build(context.Context, Request) (Result, error)
}
