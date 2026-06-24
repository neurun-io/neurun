package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/dagflows/builder/internal/domain"
	"github.com/dagflows/builder/pkg"
)

func (s *BuildService) buildNode(ctx context.Context, srcDir, outDir string) ([]pkg.Artifact, error) {
	installDir := filepath.Join(outDir, "node-install")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return nil, err
	}

	packageJSON := filepath.Join(srcDir, "package.json")
	if pkg.FileExists(packageJSON) {
		installArgs := []string{"install"}
		if pkg.FileExists(filepath.Join(srcDir, "package-lock.json")) {
			installArgs = []string{"ci"}
		}
		if err := pkg.Run(ctx, srcDir, nil, "npm", installArgs...); err != nil {
			return nil, err
		}

		hasBuild, err := hasNpmBuildScript(packageJSON)
		if err != nil {
			return nil, err
		}
		if hasBuild {
			if err := pkg.Run(ctx, srcDir, nil, "npm", "run", "build"); err != nil {
				return nil, err
			}
		}
		if err := pkg.Run(ctx, srcDir, nil, "npm", "prune", "--omit=dev"); err != nil {
			return nil, err
		}

		nodeModules := filepath.Join(srcDir, "node_modules")
		if pkg.FileExists(nodeModules) {
			if err := pkg.CopyDir(nodeModules, filepath.Join(installDir, "node_modules"), nil); err != nil {
				return nil, err
			}
			if err := os.RemoveAll(nodeModules); err != nil {
				return nil, err
			}
		}
		for _, name := range []string{"package.json", "package-lock.json", "npm-shrinkwrap.json", "yarn.lock", "pnpm-lock.yaml"} {
			if pkg.FileExists(filepath.Join(srcDir, name)) {
				if err := pkg.CopyFile(filepath.Join(srcDir, name), filepath.Join(installDir, name)); err != nil {
					return nil, err
				}
			}
		}
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

func hasNpmBuildScript(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()

	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.NewDecoder(file).Decode(&pkg); err != nil {
		return false, err
	}
	return strings.TrimSpace(pkg.Scripts["build"]) != "", nil
}
