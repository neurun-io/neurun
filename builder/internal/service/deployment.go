package service

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

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

	for i := range nodes {
		if err := s.buildNode(ctx, req, checkout.Path, &nodes[i]); err != nil {
			return domain.DeploymentResponse{}, fmt.Errorf("build node %q: %w", nodes[i].Key, err)
		}
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
		return normalizeWorkflowNodes(repoDir, nodes)
	}

	node, err := inferWorkflowNode(repoDir)
	if err != nil {
		return nil, err
	}
	return normalizeWorkflowNodes(repoDir, []domain.WorkflowNode{node})
}

func normalizeWorkflowNodes(repoDir string, nodes []domain.WorkflowNode) ([]domain.WorkflowNode, error) {
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
		if _, err := nodeSourcePath(repoDir, node); err != nil {
			return nil, fmt.Errorf("nodes[%d].config.source_path: %w", i, err)
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

func (s *DeploymentService) buildNode(ctx context.Context, req domain.DeploymentRequest, repoDir string, node *domain.WorkflowNode) error {
	sourcePath, err := nodeSourcePath(repoDir, *node)
	if err != nil {
		return err
	}

	runtime, err := runtimeFromLanguage(node.Language)
	if err != nil {
		return err
	}

	timeout := time.Duration(node.TimeoutSeconds) * time.Second
	nodeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, err := s.buildService.Build(nodeCtx, domain.BuildRequest{
		AppID:      req.DeploymentID + "-" + node.Key,
		SourcePath: sourcePath,
		Runtime:    runtime,
		EntryPoint: node.EntryPoint,
	})
	if err != nil {
		return err
	}

	for _, artifact := range result.Artifacts {
		id := artifact.ID
		if id == "" {
			id = artifact.Key
		}
		switch artifact.Kind {
		case domain.ArtifactInstallLayer:
			node.DepsArtifactID = id
		case domain.ArtifactCodeLayer, domain.ArtifactDeployable:
			node.ArtifactID = id
		}
	}
	return nil
}

func nodeSourcePath(repoDir string, node domain.WorkflowNode) (string, error) {
	sourcePath := "."
	for _, key := range []string{"source_path", "sourcePath", "path"} {
		value, ok := node.Config[key]
		if !ok {
			continue
		}
		raw, ok := value.(string)
		if !ok {
			return "", fmt.Errorf("%s must be a string", key)
		}
		if strings.TrimSpace(raw) != "" {
			sourcePath = raw
			break
		}
	}

	joined, err := filepath.Abs(filepath.Join(repoDir, filepath.FromSlash(sourcePath)))
	if err != nil {
		return "", err
	}
	repoAbs, err := filepath.Abs(repoDir)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(repoAbs, joined)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("path escapes repository")
	}
	return joined, nil
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
