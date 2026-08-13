// Package github fetches application source from GitHub for a build.
package github

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v88/github"
)

var (
	ErrNotConfigured = errors.New("github integration is not configured")
	ErrNotFound      = errors.New("github repository or ref not found")
	ErrForbidden     = errors.New("the installation does not grant access to this repository")
	ErrSourceTooBig  = errors.New("github source exceeds the configured limit")
)

// Repo names a repository the way GitHub does.
type Repo struct {
	Owner string
	Name  string
}

func ParseRepo(raw string) (Repo, error) {
	owner, name, found := strings.Cut(strings.TrimSpace(raw), "/")
	if !found || owner == "" || name == "" || strings.Contains(name, "/") {
		return Repo{}, fmt.Errorf("%w: repository must be owner/name", ErrNotFound)
	}
	return Repo{Owner: owner, Name: name}, nil
}

func (repo Repo) String() string { return repo.Owner + "/" + repo.Name }

type Limits struct {
	MaxArchiveBytes   int64
	MaxArchiveEntries int
}

// Client talks to GitHub as a GitHub App. Every call is made with an
// installation token, minted and cached by ghinstallation, so nothing here
// holds a long-lived credential.
type Client struct {
	appID         int64
	privateKey    []byte
	webhookSecret []byte
	limits        Limits
	transport     http.RoundTripper
}

type Options struct {
	AppID         int64
	PrivateKey    []byte
	WebhookSecret []byte
	Limits        Limits
	Transport     http.RoundTripper
}

func New(options Options) (*Client, error) {
	if options.AppID == 0 || len(options.PrivateKey) == 0 {
		return nil, ErrNotConfigured
	}
	if options.Transport == nil {
		options.Transport = http.DefaultTransport
	}
	if options.Limits.MaxArchiveBytes <= 0 {
		options.Limits.MaxArchiveBytes = 64 << 20
	}
	if options.Limits.MaxArchiveEntries <= 0 {
		options.Limits.MaxArchiveEntries = 20_000
	}
	return &Client{
		appID:         options.AppID,
		privateKey:    options.PrivateKey,
		webhookSecret: options.WebhookSecret,
		limits:        options.Limits,
		transport:     options.Transport,
	}, nil
}

func (client *Client) forInstallation(installationID int64) (*github.Client, error) {
	transport, err := ghinstallation.New(
		client.transport, client.appID, installationID, client.privateKey,
	)
	if err != nil {
		return nil, fmt.Errorf("github: installation transport: %w", err)
	}
	api, err := github.NewClient(
		github.WithTransport(transport),
		github.WithTimeout(2*time.Minute),
	)
	if err != nil {
		return nil, fmt.Errorf("github: client: %w", err)
	}
	return api, nil
}

// ResolveRef turns a branch, tag or SHA into the commit SHA it names.
func (client *Client) ResolveRef(
	ctx context.Context,
	installationID int64,
	repo Repo,
	ref string,
) (string, error) {
	api, err := client.forInstallation(installationID)
	if err != nil {
		return "", err
	}
	sha, response, err := api.Repositories.GetCommitSHA1(
		ctx, repo.Owner, repo.Name, ref, "",
	)
	if err != nil {
		return "", classify(response, err)
	}
	return sha, nil
}

// Source downloads the repository at commit and writes it as a ZIP, which is
// what the builder consumes. GitHub serves a gzipped tar, so it is repacked
// here rather than teaching the builder a second archive format.
func (client *Client) Source(
	ctx context.Context,
	installationID int64,
	repo Repo,
	commit string,
	target string,
) (int64, error) {
	api, err := client.forInstallation(installationID)
	if err != nil {
		return 0, err
	}
	url, response, err := api.Repositories.GetArchiveLink(
		ctx, repo.Owner, repo.Name, github.Tarball,
		&github.RepositoryContentGetOptions{Ref: commit}, 10,
	)
	if err != nil {
		return 0, classify(response, err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url.String(), nil)
	if err != nil {
		return 0, fmt.Errorf("github: build archive request: %w", err)
	}
	archive, err := (&http.Client{Timeout: 5 * time.Minute}).Do(request)
	if err != nil {
		return 0, fmt.Errorf("github: download archive: %w", err)
	}
	defer archive.Body.Close()
	if archive.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("github: download archive: %s", archive.Status)
	}
	return client.repack(archive.Body, target)
}

// repack streams the tar.gz into a ZIP, stripping the single directory GitHub
// wraps every archive in, and refusing anything that walks outside it.
func (client *Client) repack(source io.Reader, target string) (int64, error) {
	file, err := os.Create(target)
	if err != nil {
		return 0, fmt.Errorf("github: create source archive: %w", err)
	}
	defer file.Close()

	decompressed, err := gzip.NewReader(io.LimitReader(source, client.limits.MaxArchiveBytes+1))
	if err != nil {
		return 0, fmt.Errorf("github: read archive: %w", err)
	}
	defer decompressed.Close()

	writer := zip.NewWriter(file)
	reader := tar.NewReader(decompressed)
	var entries int
	var written int64

	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("github: read archive: %w", err)
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		name, ok := strip(header.Name)
		if !ok {
			continue
		}
		entries++
		if entries > client.limits.MaxArchiveEntries {
			return 0, ErrSourceTooBig
		}
		written += header.Size
		if written > client.limits.MaxArchiveBytes {
			return 0, ErrSourceTooBig
		}
		entry, err := writer.CreateHeader(&zip.FileHeader{
			Name:     name,
			Method:   zip.Deflate,
			Modified: header.ModTime,
		})
		if err != nil {
			return 0, fmt.Errorf("github: write archive: %w", err)
		}
		if _, err := io.Copy(entry, io.LimitReader(reader, header.Size)); err != nil {
			return 0, fmt.Errorf("github: write archive: %w", err)
		}
	}
	if err := writer.Close(); err != nil {
		return 0, fmt.Errorf("github: finish archive: %w", err)
	}
	return written, nil
}

// strip removes the owner-repo-sha/ prefix GitHub adds, and rejects any path
// that would escape it.
func strip(name string) (string, bool) {
	cleaned := path.Clean(strings.ReplaceAll(name, `\`, "/"))
	_, rest, found := strings.Cut(cleaned, "/")
	if !found || rest == "" {
		return "", false
	}
	if strings.HasPrefix(rest, "../") || rest == ".." || path.IsAbs(rest) {
		return "", false
	}
	return rest, true
}

func classify(response *github.Response, err error) error {
	if response != nil {
		switch response.StatusCode {
		case http.StatusNotFound:
			return fmt.Errorf("%w: %v", ErrNotFound, err)
		case http.StatusForbidden, http.StatusUnauthorized:
			return fmt.Errorf("%w: %v", ErrForbidden, err)
		}
	}
	return fmt.Errorf("github: %w", err)
}
