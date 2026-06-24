package service

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/dagflows/builder/internal/domain"
	"github.com/dagflows/builder/pkg"
)

func (s *BuildService) buildGo(ctx context.Context, srcDir, outDir, entrypoint, appID string) ([]pkg.Artifact, error) {
	target := "."
	if strings.TrimSpace(entrypoint) != "" {
		target = entrypoint
	}

	binName := pkg.SafeName(appID)
	if s.goos == "windows" && !strings.HasSuffix(strings.ToLower(binName), ".exe") {
		binName += ".exe"
	}
	outPath := filepath.Join(outDir, binName)
	env := []string{"GOOS=" + s.goos, "GOARCH=" + s.goarch, "CGO_ENABLED=" + s.cgo}
	if err := pkg.Run(ctx, srcDir, env, "go", "build", "-o", outPath, target); err != nil {
		return nil, err
	}

	artifact, err := pkg.FileArtifact(domain.ArtifactDeployable, binName, outPath, "application/octet-stream")
	if err != nil {
		return nil, err
	}
	return []pkg.Artifact{artifact}, nil
}
