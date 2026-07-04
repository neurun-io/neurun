package service

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/dagflows/builder/internal/domain"
	"github.com/dagflows/builder/pkg"
)

const (
	defaultNodeKey        = "main"
	defaultNodeType       = "task"
	defaultRetryCount     = 3
	defaultTimeoutSeconds = 300
)

type DeploymentService struct {
	buildService *BuildService
	github       *GitHubService
}

func NewDeploymentService(buildService *BuildService, github *GitHubService) *DeploymentService {
	return &DeploymentService{
		buildService: buildService,
		github:       github,
	}
}

func (s *DeploymentService) BuildDeployment(ctx context.Context, req domain.DeploymentRequest) (domain.DeploymentResponse, error) {
	if err := validateDeploymentRequest(req); err != nil {
		return domain.DeploymentResponse{}, err
	}

	checkout, err := s.github.FetchCode(ctx, req)
	if err != nil {
		return domain.DeploymentResponse{}, fmt.Errorf("fetch code: %w", err)
	}
	defer checkout.Close()

	nodes, err := s.resolveWorkflowNodes(ctx, checkout.Path)
	if err != nil {
		return domain.DeploymentResponse{}, err
	}

	if err := s.buildPackage(ctx, checkout, nodes); err != nil {
		return domain.DeploymentResponse{}, err
	}

	return domain.DeploymentResponse{
		DeploymentID: req.DeploymentID,
		Status:       domain.DeploymentStatusSuccess,
		ErrorMessage: "",
		Nodes:        nodes,
	}, nil
}

func validateDeploymentRequest(req domain.DeploymentRequest) error {
	switch {
	case strings.TrimSpace(req.DeploymentID) == "":
		return fmt.Errorf("deployment_id is required")
	case strings.TrimSpace(req.WorkflowID) == "":
		return fmt.Errorf("workflow_id is required")
	case strings.TrimSpace(req.GitURL) == "":
		return fmt.Errorf("git_url is required")
	default:
		return nil
	}
}

func (s *DeploymentService) resolveWorkflowNodes(ctx context.Context, repoDir string) ([]domain.WorkflowNode, error) {
	if isPythonProject(repoDir) {
		nodes, err := inspectPythonWorkflow(ctx, repoDir)
		if err != nil {
			return nil, err
		}
		return normalizeWorkflowNodes(nodes)
	}

	node, err := inferWorkflowNode(repoDir)
	if err != nil {
		return nil, err
	}
	return normalizeWorkflowNodes([]domain.WorkflowNode{node})
}

func normalizeWorkflowNodes(nodes []domain.WorkflowNode) ([]domain.WorkflowNode, error) {
	normalized := make([]domain.WorkflowNode, len(nodes))
	for i, node := range nodes {
		if strings.TrimSpace(node.Key) == "" {
			node.Key = defaultNodeKey
		}
		if strings.TrimSpace(node.Type) == "" {
			node.Type = defaultNodeType
		}
		node.Language = normalizeLanguage(node.Language)
		if node.Language == "" {
			return nil, fmt.Errorf("nodes[%d].language is required", i)
		}
		if node.EntryPoint == "" {
			node.EntryPoint = defaultEntryPoint(node.Language)
		}
		if node.Config == nil {
			node.Config = map[string]any{}
		}
		if node.Depends == nil {
			node.Depends = []string{}
		}
		if node.RetryCount <= 0 {
			node.RetryCount = defaultRetryCount
		}
		if node.TimeoutSeconds <= 0 {
			node.TimeoutSeconds = defaultTimeoutSeconds
		}
		normalized[i] = node
	}
	return normalized, nil
}

func inferWorkflowNode(repoDir string) (domain.WorkflowNode, error) {
	switch {
	case pkg.FileExists(filepath.Join(repoDir, "package.json")):
		return domain.WorkflowNode{Key: defaultNodeKey, Type: defaultNodeType, Language: string(domain.RuntimeNode)}, nil
	case pkg.FileExists(filepath.Join(repoDir, "go.mod")):
		return domain.WorkflowNode{Key: defaultNodeKey, Type: defaultNodeType, Language: string(domain.RuntimeGo)}, nil
	default:
		return domain.WorkflowNode{}, fmt.Errorf("no workflow nodes found")
	}
}

func isPythonProject(repoDir string) bool {
	return pkg.FileExists(filepath.Join(repoDir, "requirements.txt")) ||
		pkg.FileExists(filepath.Join(repoDir, "pyproject.toml")) ||
		pkg.FileExists(filepath.Join(repoDir, "main.py")) ||
		hasFileWithExt(repoDir, ".py")
}

func hasFileWithExt(root, ext string) bool {
	found := false
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || found {
			return nil
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", ".venv", "venv":
				return filepath.SkipDir
			default:
				return nil
			}
		}
		if strings.EqualFold(filepath.Ext(path), ext) {
			found = true
		}
		return nil
	})
	return found
}

func (s *DeploymentService) buildPackage(ctx context.Context, checkout GitHubCheckout, nodes []domain.WorkflowNode) error {
	runtime, err := runtimeFromNodes(nodes)
	if err != nil {
		return err
	}

	result, err := s.buildService.Build(ctx, domain.BuildRequest{
		AppID:      checkout.RepoSequence,
		BuildID:    checkout.CommitHash,
		SourcePath: checkout.Path,
		Runtime:    runtime,
		EntryPoint: nodes[0].EntryPoint,
	})
	if err != nil {
		return fmt.Errorf("build package: %w", err)
	}

	artifactID := ""
	depsArtifactID := ""
	for _, artifact := range result.Artifacts {
		id := artifact.ID
		if id == "" {
			id = artifact.Key
		}
		switch artifact.Kind {
		case domain.ArtifactInstallLayer:
			depsArtifactID = id
		case domain.ArtifactCodeLayer, domain.ArtifactDeployable:
			artifactID = id
		}
	}

	for i := range nodes {
		nodes[i].ArtifactID = artifactID
		nodes[i].DepsArtifactID = depsArtifactID
	}
	return nil
}

func runtimeFromNodes(nodes []domain.WorkflowNode) (domain.Runtime, error) {
	if len(nodes) == 0 {
		return "", fmt.Errorf("no workflow nodes found")
	}

	runtime, err := runtimeFromLanguage(nodes[0].Language)
	if err != nil {
		return "", err
	}
	for _, node := range nodes[1:] {
		next, err := runtimeFromLanguage(node.Language)
		if err != nil {
			return "", err
		}
		if next != runtime {
			return "", fmt.Errorf("single-package deployments cannot mix node languages: %s and %s", runtime, next)
		}
	}
	return runtime, nil
}

func normalizeLanguage(language string) string {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "python", "py":
		return string(domain.RuntimePython)
	case "node", "nodejs", "javascript", "typescript", "js", "ts":
		return string(domain.RuntimeNode)
	case "go", "golang":
		return string(domain.RuntimeGo)
	default:
		return ""
	}
}

func runtimeFromLanguage(language string) (domain.Runtime, error) {
	switch normalizeLanguage(language) {
	case string(domain.RuntimePython):
		return domain.RuntimePython, nil
	case string(domain.RuntimeNode):
		return domain.RuntimeNode, nil
	case string(domain.RuntimeGo):
		return domain.RuntimeGo, nil
	default:
		return "", fmt.Errorf("unsupported language %q", language)
	}
}

func defaultEntryPoint(language string) string {
	switch normalizeLanguage(language) {
	case string(domain.RuntimePython):
		return "main.py:handler"
	case string(domain.RuntimeNode):
		return "index.js:handler"
	case string(domain.RuntimeGo):
		return "."
	default:
		return ""
	}
}
