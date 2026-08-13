package github

import (
	"context"

	"github.com/google/go-github/v88/github"
)

// pageSize is GitHub's maximum, and maxPages bounds an installation that grants
// thousands of repositories so one dropdown cannot walk the whole API.
const (
	pageSize = 100
	maxPages = 10
)

// Repository is one repository an installation grants, as an app picker needs
// it. Nothing here is chosen by the caller: the list is what GitHub says the
// installation may read.
type Repository struct {
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
	Private       bool   `json:"private"`
}

// Repositories lists what the installation can read.
func (client *Client) Repositories(
	ctx context.Context,
	installationID int64,
) ([]Repository, error) {
	api, err := client.forInstallation(installationID)
	if err != nil {
		return nil, err
	}
	options := &github.ListOptions{PerPage: pageSize}
	repositories := make([]Repository, 0, pageSize)
	for page := 0; page < maxPages; page++ {
		listed, response, err := api.Apps.ListRepos(ctx, options)
		if err != nil {
			return nil, classify(response, err)
		}
		for _, record := range listed.Repositories {
			repositories = append(repositories, Repository{
				FullName:      record.GetFullName(),
				DefaultBranch: record.GetDefaultBranch(),
				Private:       record.GetPrivate(),
			})
		}
		if response.NextPage == 0 {
			break
		}
		options.Page = response.NextPage
	}
	return repositories, nil
}

// Branches lists a repository's branches, so a production ref is chosen from
// what exists rather than typed and resolved later.
func (client *Client) Branches(
	ctx context.Context,
	installationID int64,
	repo Repo,
) ([]string, error) {
	api, err := client.forInstallation(installationID)
	if err != nil {
		return nil, err
	}
	options := &github.BranchListOptions{
		ListOptions: github.ListOptions{PerPage: pageSize},
	}
	branches := make([]string, 0, pageSize)
	for page := 0; page < maxPages; page++ {
		listed, response, err := api.Repositories.ListBranches(
			ctx, repo.Owner, repo.Name, options,
		)
		if err != nil {
			return nil, classify(response, err)
		}
		for _, branch := range listed {
			branches = append(branches, branch.GetName())
		}
		if response.NextPage == 0 {
			break
		}
		options.Page = response.NextPage
	}
	return branches, nil
}
