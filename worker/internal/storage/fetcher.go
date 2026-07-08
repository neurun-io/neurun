package storage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/dagflows/worker/internal/config"
)

type Fetcher interface {
	Fetch(ctx context.Context, ref, path string) error
}

type fallbackFetcher struct {
	http Fetcher
	r2   Fetcher
}

type HTTPFetcher struct{}

func NewFetcher(cfg config.Config) (Fetcher, error) {
	httpFetcher := HTTPFetcher{}
	r2Fetcher, err := NewR2(cfg.R2)
	if err != nil {
		if isR2Configured(cfg.R2) {
			return nil, err
		}
		return fallbackFetcher{http: httpFetcher}, nil
	}
	return fallbackFetcher{http: httpFetcher, r2: r2Fetcher}, nil
}

func (f fallbackFetcher) Fetch(ctx context.Context, ref, path string) error {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return fmt.Errorf("artifact reference is required")
	}
	if isHTTPURL(ref) {
		return f.http.Fetch(ctx, ref, path)
	}
	if f.r2 == nil {
		return fmt.Errorf("artifact reference %q is not a URL and R2 is not configured", ref)
	}
	return f.r2.Fetch(ctx, ref, path)
}

func (HTTPFetcher) Fetch(ctx context.Context, rawURL, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("download failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	return writeResponse(path, resp.Body)
}

func writeResponse(path string, body io.Reader) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(file, body)
	return err
}

func isHTTPURL(ref string) bool {
	u, err := url.Parse(ref)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func isR2Configured(cfg config.R2Config) bool {
	return strings.TrimSpace(cfg.Bucket) != "" ||
		strings.TrimSpace(cfg.AccessKeyID) != "" ||
		strings.TrimSpace(cfg.SecretAccessKey) != "" ||
		strings.TrimSpace(cfg.Endpoint) != "" ||
		strings.TrimSpace(cfg.AccountID) != ""
}
