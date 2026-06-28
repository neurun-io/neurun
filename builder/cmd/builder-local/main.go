package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/dagflows/builder/internal/config"
	"github.com/dagflows/builder/internal/domain"
	"github.com/dagflows/builder/internal/service"
	"github.com/dagflows/builder/internal/storage"
)

func main() {
	envPath := flag.String("env", ".env", "path to env file")
	deploymentID := flag.String("deployment-id", "", "deployment id")
	workflowID := flag.String("workflow-id", "", "workflow id")
	gitURL := flag.String("git-url", "", "Git repository URL")
	gitBranch := flag.String("git-branch", "", "Git branch; defaults to GIT_DEFAULT_BRANCH")
	gitCommitHash := flag.String("git-commit-hash", "", "Git commit hash")
	flag.Parse()

	if *deploymentID == "" || *workflowID == "" || *gitURL == "" {
		flag.Usage()
		log.Fatal("--deployment-id, --workflow-id, and --git-url are required")
	}

	cfg, err := config.Load(*envPath)
	if err != nil {
		log.Fatal(err)
	}

	branch := *gitBranch
	if branch == "" {
		branch = cfg.GitHub.DefaultBranch
	}
	request := domain.DeploymentRequest{
		DeploymentID:  *deploymentID,
		WorkflowID:    *workflowID,
		GitURL:        *gitURL,
		GitBranch:     branch,
		GitCommitHash: *gitCommitHash,
	}

	store, err := storage.NewR2(cfg.R2)
	if err != nil {
		log.Fatal(err)
	}

	buildService := service.NewBuildService(store)
	github := service.NewGitHubService(cfg.GitHub)
	deploymentService := service.NewDeploymentService(buildService, github)

	response, err := deploymentService.BuildDeployment(context.Background(), request)
	if err != nil {
		response = domain.DeploymentResponse{
			DeploymentID: request.DeploymentID,
			Status:       domain.DeploymentStatusFailed,
			ErrorMessage: err.Error(),
			Nodes:        []domain.WorkflowNode{},
		}
	}

	if err := writeResponse(response); err != nil {
		log.Fatal(err)
	}
}

func writeResponse(response domain.DeploymentResponse) error {
	encoded, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return fmt.Errorf("encode response: %w", err)
	}
	_, err = os.Stdout.Write(append(encoded, '\n'))
	return err
}
