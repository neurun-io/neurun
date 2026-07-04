package service

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dagflows/builder/internal/domain"
	"github.com/dagflows/builder/pkg"
)

func (s *BuildService) buildPython(ctx context.Context, sourceDir, srcDir, outDir string) ([]pkg.Artifact, error) {
	installDir := filepath.Join(outDir, "python-install")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return nil, err
	}

	if err := installPythonRequirements(ctx, sourceDir, installDir, outDir); err != nil {
		return nil, err
	}
	if err := pkg.Run(ctx, srcDir, pythonTempEnv(outDir), "python", "-m", "compileall", "-q", srcDir); err != nil {
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

func installPythonRequirements(ctx context.Context, sourceDir, installDir, workDir string) error {
	requirements := filepath.Join(sourceDir, "requirements.txt")
	if !pkg.FileExists(requirements) {
		return nil
	}

	prepared, err := preparePythonRequirements(requirements, sourceDir, workDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return err
	}
	return pkg.Run(ctx, sourceDir, pythonTempEnv(workDir), "python", "-m", "pip", "install", "-r", prepared, "-t", installDir)
}

func preparePythonRequirements(requirements, sourceDir, workDir string) (string, error) {
	source, err := os.Open(requirements)
	if err != nil {
		return "", err
	}
	defer source.Close()

	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return "", err
	}
	prepared := filepath.Join(workDir, "requirements.resolved.txt")
	target, err := os.Create(prepared)
	if err != nil {
		return "", err
	}
	defer target.Close()

	baseDir := filepath.Dir(requirements)
	sourceRoot, err := filepath.Abs(sourceDir)
	if err != nil {
		return "", err
	}
	scanner := bufio.NewScanner(source)
	for scanner.Scan() {
		line, err := rewriteEditableRequirement(scanner.Text(), baseDir, sourceRoot)
		if err != nil {
			return "", err
		}
		if _, err := fmt.Fprintln(target, line); err != nil {
			return "", err
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return prepared, nil
}

func rewriteEditableRequirement(line, baseDir, sourceRoot string) (string, error) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return line, nil
	}

	var spec string
	switch {
	case strings.HasPrefix(trimmed, "-e "):
		spec = strings.TrimSpace(strings.TrimPrefix(trimmed, "-e "))
	case strings.HasPrefix(trimmed, "--editable "):
		spec = strings.TrimSpace(strings.TrimPrefix(trimmed, "--editable "))
	default:
		return line, nil
	}

	if !isLocalPythonRequirement(spec) {
		return line, nil
	}

	absolute, err := filepath.Abs(filepath.Join(baseDir, filepath.FromSlash(spec)))
	if err != nil {
		return "", err
	}
	if !isPathInside(sourceRoot, absolute) {
		return "", fmt.Errorf("editable requirement %q resolves outside the checked-out source to %s; use a package name, a git URL, or a path inside the repository", spec, absolute)
	}
	return filepath.ToSlash(absolute), nil
}

func isLocalPythonRequirement(spec string) bool {
	if strings.Contains(spec, "://") || strings.Contains(spec, "+") {
		return false
	}
	return spec == "." ||
		strings.HasPrefix(spec, "./") ||
		strings.HasPrefix(spec, "../") ||
		strings.HasPrefix(spec, ".\\") ||
		strings.HasPrefix(spec, "..\\") ||
		filepath.IsAbs(spec)
}

func pythonTempEnv(workDir string) []string {
	tempDir := filepath.Join(workDir, "pip-tmp")
	cacheDir := filepath.Join(workDir, "pip-cache")
	_ = os.MkdirAll(tempDir, 0o755)
	_ = os.MkdirAll(cacheDir, 0o755)
	return []string{
		"TEMP=" + tempDir,
		"TMP=" + tempDir,
		"PIP_CACHE_DIR=" + cacheDir,
	}
}

func isPathInside(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
}
