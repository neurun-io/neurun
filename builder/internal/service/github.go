package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dagflows/builder/internal/config"
	"github.com/dagflows/builder/internal/domain"
	"github.com/dagflows/builder/pkg"
)

type GitHubService struct {
	tempDir string
	run     commandRunner
}

type GitHubCheckout struct {
	Path    string
	workDir string
}

func (c GitHubCheckout) Close() error {
	if c.workDir == "" {
		return nil
	}
	return os.RemoveAll(c.workDir)
}

type commandRunner func(ctx context.Context, dir string, env []string, name string, args ...string) error

func NewGitHubService(cfg config.GitHubConfig) *GitHubService {
	return &GitHubService{
		tempDir: cfg.TempDir,
		run:     pkg.Run,
	}
}

func (s *GitHubService) FetchCode(ctx context.Context, req domain.DeploymentRequest) (GitHubCheckout, error) {
	gitURL := strings.TrimSpace(req.GitURL)
	if gitURL == "" {
		return GitHubCheckout{}, fmt.Errorf("git_url is required")
	}

	workDir, err := os.MkdirTemp(s.tempDir, "dagflows-github-*")
	if err != nil {
		return GitHubCheckout{}, fmt.Errorf("create github checkout dir: %w", err)
	}

	checkout := GitHubCheckout{
		Path:    filepath.Join(workDir, "repo"),
		workDir: workDir,
	}
	if err := s.clone(ctx, gitURL, req.GitBranch, checkout.Path); err != nil {
		_ = checkout.Close()
		return GitHubCheckout{}, err
	}
	if commit := strings.TrimSpace(req.GitCommitHash); commit != "" {
		if err := s.run(ctx, checkout.Path, nil, "git", "checkout", "--detach", commit); err != nil {
			_ = checkout.Close()
			return GitHubCheckout{}, fmt.Errorf("checkout commit %s: %w", commit, err)
		}
	}

	return checkout, nil
}

func (s *GitHubService) clone(ctx context.Context, gitURL, branch, dst string) error {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		branch = config.DefaultGitBranch
	}

	if err := s.run(ctx, "", nil, "git", "clone", "--branch", branch, "--single-branch", gitURL, dst); err != nil {
		return fmt.Errorf("clone %s branch %s: %w", gitURL, branch, err)
	}
	return nil
}
