package service

import (
	"context"
	"os"
	"path/filepath"

	"github.com/dagflows/builder/internal/domain"
	"github.com/dagflows/builder/pkg"
)

func (s *BuildService) buildPython(ctx context.Context, srcDir, outDir string) ([]pkg.Artifact, error) {
	installDir := filepath.Join(outDir, "python-install")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return nil, err
	}

	requirements := filepath.Join(srcDir, "requirements.txt")
	if pkg.FileExists(requirements) {
		if err := pkg.Run(ctx, srcDir, nil, "python", "-m", "pip", "install", "-r", requirements, "-t", installDir); err != nil {
			return nil, err
		}
	}
	if err := pkg.Run(ctx, srcDir, nil, "python", "-m", "compileall", "-q", srcDir); err != nil {
		return nil, err
	}

	installZip, err := pkg.ZipDirectory(domain.ArtifactInstallLayer, "install-layer.zip", installDir, outDir)
	if err != nil {
		return nil, err
	}
	codeZip, err := pkg.ZipDirectory(domain.ArtifactCodeLayer, "code-layer.zip", srcDir, outDir)
	if err != nil {
		return nil, err
	}
	return []pkg.Artifact{installZip, codeZip}, nil
}
