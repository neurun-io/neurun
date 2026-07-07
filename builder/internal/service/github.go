package service

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/dagflows/builder/internal/config"
	"github.com/dagflows/builder/internal/domain"
	"github.com/dagflows/builder/pkg"
)

type GitHubService struct{}

type GitHubCheckout struct {
	Path         string
	RepoSequence string
	CommitHash   string
	workDir      string
}

func (c GitHubCheckout) Close() error {
	if c.workDir == "" {
		return nil
	}
	return os.RemoveAll(c.workDir)
}

func NewGitHubService() *GitHubService {
	return &GitHubService{}
}

func (s *GitHubService) FetchCode(ctx context.Context, req domain.DeploymentRequest) (GitHubCheckout, error) {
	gitURL := strings.TrimSpace(req.GitURL)
	if gitURL == "" {
		return GitHubCheckout{}, fmt.Errorf("git_url is required")
	}

	workDir, err := os.MkdirTemp("", "dagflows-github-*")
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
		if err := pkg.Run(ctx, checkout.Path, nil, "git", "checkout", "--detach", commit); err != nil {
			_ = checkout.Close()
			return GitHubCheckout{}, fmt.Errorf("checkout commit %s: %w", commit, err)
		}
	}
	resolvedCommit, err := pkg.Output(ctx, checkout.Path, nil, "git", "rev-parse", "HEAD")
	if err != nil {
		_ = checkout.Close()
		return GitHubCheckout{}, fmt.Errorf("resolve commit hash: %w", err)
	}
	checkout.CommitHash = strings.TrimSpace(resolvedCommit)
	checkout.RepoSequence = repoSequence(gitURL)

	return checkout, nil
}

func (s *GitHubService) clone(ctx context.Context, gitURL, branch, dst string) error {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		branch = config.DefaultGitBranch
	}

	if err := pkg.Run(ctx, "", nil, "git", "clone", "--branch", branch, "--single-branch", gitURL, dst); err != nil {
		return fmt.Errorf("clone %s branch %s: %w", gitURL, branch, err)
	}
	return nil
}

func repoSequence(gitURL string) string {
	canonical := canonicalRepoIdentity(gitURL)
	sequence := pkg.SafeName(strings.ReplaceAll(canonical, "/", "-"))
	if sequence == "" {
		return "repo"
	}
	return strings.ToLower(sequence)
}

func canonicalRepoIdentity(gitURL string) string {
	raw := strings.TrimSpace(strings.TrimSuffix(gitURL, ".git"))
	if raw == "" {
		return "repo"
	}

	if strings.HasPrefix(raw, "git@") {
		withoutUser := strings.TrimPrefix(raw, "git@")
		host, path, ok := strings.Cut(withoutUser, ":")
		if ok {
			return strings.ToLower(strings.Trim(host, "/")) + "/" + strings.Trim(strings.TrimSuffix(path, ".git"), "/")
		}
	}

	parsed, err := url.Parse(raw)
	if err == nil && parsed.Host != "" {
		path := strings.Trim(strings.TrimSuffix(parsed.Path, ".git"), "/")
		if parsed.Scheme == "file" {
			return "local/" + path
		}
		return strings.ToLower(parsed.Host) + "/" + path
	}

	return raw
}
