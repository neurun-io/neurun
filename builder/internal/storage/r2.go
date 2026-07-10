package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/dagflows/builder/internal/config"
	"github.com/dagflows/builder/internal/domain"
)

const (
	cloudflareR2HostSuffix = ".r2.cloudflarestorage.com"
	awsService             = "s3"
)

type R2Store struct {
	endpoint    *url.URL
	client      *http.Client
	bucket      string
	region      string
	prefix      string
	credentials aws.Credentials
	signer      *v4.Signer
}

func NewR2(cfg config.R2Config) (*R2Store, error) {
	endpoint, err := endpointFromConfig(cfg)
	if err != nil {
		return nil, err
	}

	bucket := strings.TrimSpace(cfg.Bucket)
	region := strings.TrimSpace(cfg.Region)
	accessKey := strings.TrimSpace(cfg.AccessKeyID)
	secretKey := strings.TrimSpace(cfg.SecretAccessKey)
	prefix := strings.Trim(cfg.Prefix, "/")
	if region == "" {
		region = config.DefaultR2Region
	}

	if bucket == "" || accessKey == "" || secretKey == "" {
		return nil, fmt.Errorf("R2_BUCKET, R2_ACCESS_KEY_ID, and R2_SECRET_ACCESS_KEY are required")
	}

	return &R2Store{
		endpoint:    endpoint,
		client:      &http.Client{Timeout: 15 * time.Minute},
		bucket:      bucket,
		region:      region,
		prefix:      prefix,
		credentials: aws.Credentials{AccessKeyID: accessKey, SecretAccessKey: secretKey},
		signer:      v4.NewSigner(),
	}, nil
}

func endpointFromConfig(cfg config.R2Config) (*url.URL, error) {
	if endpoint := strings.TrimSpace(cfg.Endpoint); endpoint != "" {
		return normalizeEndpoint(endpoint)
	}

	accountID := strings.TrimSpace(cfg.AccountID)
	if accountID == "" {
		return nil, fmt.Errorf("R2_ENDPOINT or R2_ACCOUNT_ID is required")
	}
	return normalizeEndpoint("https://" + accountID + cloudflareR2HostSuffix)
}

func (s *R2Store) PutFile(ctx context.Context, key, filePath, mediaType string) (domain.UploadedArtifact, error) {
	if s.prefix != "" {
		key = s.prefix + "/" + strings.TrimLeft(key, "/")
	}

	file, err := os.Open(filePath)
	if err != nil {
		return domain.UploadedArtifact{}, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return domain.UploadedArtifact{}, err
	}
	payloadHash, err := hashPayload(file)
	if err != nil {
		return domain.UploadedArtifact{}, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return domain.UploadedArtifact{}, err
	}

	objectURL := s.objectURL(key)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, objectURL.String(), file)
	if err != nil {
		return domain.UploadedArtifact{}, err
	}
	req.ContentLength = info.Size()
	req.Header.Set("Content-Type", mediaType)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	if err := s.signer.SignHTTP(ctx, s.credentials, req, payloadHash, awsService, s.region, time.Now().UTC()); err != nil {
		return domain.UploadedArtifact{}, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return domain.UploadedArtifact{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return domain.UploadedArtifact{}, fmt.Errorf("r2 put failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	return domain.UploadedArtifact{
		Bucket:    s.bucket,
		Key:       key,
		SizeBytes: info.Size(),
	}, nil
}

func (s *R2Store) objectURL(key string) *url.URL {
	segments := append(pathSegments(s.endpoint.Path), s.bucket)
	segments = append(segments, pathSegments(key)...)

	path := "/" + strings.Join(segments, "/")
	escapedPath := "/" + strings.Join(escapePathSegments(segments), "/")

	objectURL := *s.endpoint
	objectURL.Path = path
	objectURL.RawPath = escapedPath
	objectURL.RawQuery = ""
	objectURL.Fragment = ""
	return &objectURL
}

func normalizeEndpoint(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("R2 endpoint is required")
	}
	if strings.HasPrefix(raw, "://") {
		raw = "https" + raw
	} else if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}

	endpoint, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if endpoint.Host == "" {
		return nil, fmt.Errorf("invalid R2 endpoint %q", raw)
	}
	if endpoint.Scheme != "https" && endpoint.Scheme != "http" {
		return nil, fmt.Errorf("unsupported R2 endpoint scheme %q", endpoint.Scheme)
	}
	endpoint.RawQuery = ""
	endpoint.Fragment = ""
	return endpoint, nil
}

func hashPayload(reader io.Reader) (string, error) {
	hash := sha256.New()
	if _, err := io.Copy(hash, reader); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func pathSegments(value string) []string {
	parts := strings.Split(strings.Trim(value, "/"), "/")
	segments := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			segments = append(segments, part)
		}
	}
	return segments
}

func escapePathSegments(segments []string) []string {
	escaped := make([]string, 0, len(segments))
	for _, segment := range segments {
		escaped = append(escaped, url.PathEscape(segment))
	}
	return escaped
}
