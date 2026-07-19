package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dagflows/builder/internal/domain"
	"github.com/dagflows/builder/pkg"
)

func inspectPythonWorkflow(ctx context.Context, repoDir string) ([]domain.WorkflowNode, error) {
	workDir, err := os.MkdirTemp("", "dagflows-inspect-*")
	if err != nil {
		return nil, fmt.Errorf("create inspect work dir: %w", err)
	}
	defer os.RemoveAll(workDir)

	depsDir := filepath.Join(workDir, "deps")
	if err := installPythonRequirements(ctx, repoDir, depsDir, workDir); err != nil {
		return nil, fmt.Errorf("install inspect dependencies: %w", err)
	}

	output, err := pkg.Output(ctx, repoDir, pythonInspectEnv(repoDir, depsDir, workDir), "python", "-m", "dagflows", "inspect")
	if err != nil {
		return nil, fmt.Errorf("inspect Python workflow: %w", err)
	}

	var workflow struct {
		Nodes []domain.WorkflowNode `json:"nodes"`
	}
	if err := json.Unmarshal([]byte(output), &workflow); err != nil {
		return nil, fmt.Errorf("decode dagflows inspect output: %w", err)
	}
	if len(workflow.Nodes) == 0 {
		return nil, fmt.Errorf("dagflows inspect returned no nodes")
	}
	return workflow.Nodes, nil
}

func pythonInspectEnv(repoDir, depsDir, workDir string) []string {
	pythonPath := []string{depsDir, repoDir}
	if existing := os.Getenv("PYTHONPATH"); existing != "" {
		pythonPath = append(pythonPath, existing)
	}

	env := pythonTempEnv(workDir)
	env = append(env, "PYTHONPATH="+strings.Join(pythonPath, string(os.PathListSeparator)))
	return env
}
