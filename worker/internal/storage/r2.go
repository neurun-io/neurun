package storage

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dagflows/worker/internal/config"
)

const (
	cloudflareR2HostSuffix = ".r2.cloudflarestorage.com"
	awsService             = "s3"
	emptyPayloadHash       = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
)

type R2Store struct {
	endpoint  *url.URL
	client    *http.Client
	bucket    string
	region    string
	prefix    string
	accessKey string
	secretKey string
}

func NewR2(cfg config.R2Config) (*R2Store, error) {
	if !isR2Configured(cfg) {
		return nil, fmt.Errorf("R2 is not configured")
	}

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
		return nil, fmt.Errorf("R2_BUCKET, R2_ACCESS_KEY_ID, and R2_SECRET_ACCESS_KEY are required for direct R2 fetch")
	}

	return &R2Store{
		endpoint:  endpoint,
		client:    http.DefaultClient,
		bucket:    bucket,
		region:    region,
		prefix:    prefix,
		accessKey: accessKey,
		secretKey: secretKey,
	}, nil
}

func (s *R2Store) Fetch(ctx context.Context, ref, path string) error {
	key := s.objectKey(ref)
	objectURL, canonicalURI := s.objectURL(key)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, objectURL.String(), nil)
	if err != nil {
		return err
	}
	s.sign(req, canonicalURI, emptyPayloadHash, time.Now().UTC())

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("r2 get %s failed: %s: %s", key, resp.Status, strings.TrimSpace(string(body)))
	}
	return writeResponse(path, resp.Body)
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

func (s *R2Store) objectKey(ref string) string {
	key := strings.TrimLeft(strings.TrimSpace(ref), "/")
	if s.prefix == "" || key == s.prefix || strings.HasPrefix(key, s.prefix+"/") {
		return key
	}
	return s.prefix + "/" + key
}

func (s *R2Store) objectURL(key string) (*url.URL, string) {
	segments := pathSegments(s.endpoint.Path)
	if len(segments) == 0 {
		segments = append(segments, s.bucket)
	}
	segments = append(segments, pathSegments(key)...)

	path := "/" + strings.Join(segments, "/")
	escapedPath := "/" + strings.Join(escapePathSegments(segments), "/")

	objectURL := *s.endpoint
	objectURL.Path = path
	objectURL.RawPath = escapedPath
	objectURL.RawQuery = ""
	objectURL.Fragment = ""
	return &objectURL, escapedPath
}

func (s *R2Store) sign(req *http.Request, canonicalURI, payloadHash string, now time.Time) {
	amzDate := now.Format("20060102T150405Z")
	shortDate := now.Format("20060102")

	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)

	credentialScope := shortDate + "/" + s.region + "/" + awsService + "/aws4_request"
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonicalHeaders := "host:" + req.URL.Host + "\n" +
		"x-amz-content-sha256:" + payloadHash + "\n" +
		"x-amz-date:" + amzDate + "\n"
	canonicalRequest := strings.Join([]string{
		req.Method,
		canonicalURI,
		req.URL.RawQuery,
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		credentialScope,
		hexSHA256(canonicalRequest),
	}, "\n")

	signingKey := hmacSHA256([]byte("AWS4"+s.secretKey), shortDate)
	signingKey = hmacSHA256(signingKey, s.region)
	signingKey = hmacSHA256(signingKey, awsService)
	signingKey = hmacSHA256(signingKey, "aws4_request")
	signature := hex.EncodeToString(hmacSHA256(signingKey, stringToSign))

	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+s.accessKey+"/"+credentialScope+", SignedHeaders="+signedHeaders+", Signature="+signature)
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

func hexSHA256(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func hmacSHA256(key []byte, value string) []byte {
	hash := hmac.New(sha256.New, key)
	hash.Write([]byte(value))
	return hash.Sum(nil)
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

func isR2Configured(cfg config.R2Config) bool {
	return strings.TrimSpace(cfg.Bucket) != "" ||
		strings.TrimSpace(cfg.AccessKeyID) != "" ||
		strings.TrimSpace(cfg.SecretAccessKey) != "" ||
		strings.TrimSpace(cfg.Endpoint) != "" ||
		strings.TrimSpace(cfg.AccountID) != ""
}
