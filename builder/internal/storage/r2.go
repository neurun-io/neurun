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
	"strings"
	"time"

	"github.com/dagflows/builder/internal/domain"
)

const (
	cloudflareR2HostSuffix = ".r2.cloudflarestorage.com"
	awsRegion              = "auto"
	awsService             = "s3"
)

type R2Store struct {
	endpoint  *url.URL
	client    *http.Client
	bucket    string
	prefix    string
	accessKey string
	secretKey string
}

func NewR2FromEnv() (*R2Store, error) {
	endpoint, err := endpointFromEnv()
	if err != nil {
		return nil, err
	}

	bucket := strings.TrimSpace(os.Getenv("R2_BUCKET"))
	accessKey := strings.TrimSpace(os.Getenv("R2_ACCESS_KEY_ID"))
	secretKey := strings.TrimSpace(os.Getenv("R2_SECRET_ACCESS_KEY"))
	prefix := strings.Trim(os.Getenv("R2_PREFIX"), "/")

	if bucket == "" || accessKey == "" || secretKey == "" {
		return nil, fmt.Errorf("R2_BUCKET, R2_ACCESS_KEY_ID, and R2_SECRET_ACCESS_KEY are required")
	}

	return &R2Store{
		endpoint:  endpoint,
		client:    http.DefaultClient,
		bucket:    bucket,
		prefix:    prefix,
		accessKey: accessKey,
		secretKey: secretKey,
	}, nil
}

func endpointFromEnv() (*url.URL, error) {
	if endpoint := strings.TrimSpace(os.Getenv("R2_ENDPOINT")); endpoint != "" {
		return normalizeEndpoint(endpoint)
	}

	accountID := strings.TrimSpace(os.Getenv("R2_ACCOUNT_ID"))
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

	objectURL, canonicalURI := s.objectURL(key)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, objectURL.String(), file)
	if err != nil {
		return domain.UploadedArtifact{}, err
	}
	req.ContentLength = info.Size()
	req.Header.Set("Content-Type", mediaType)
	s.sign(req, canonicalURI, payloadHash, time.Now().UTC())

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

	credentialScope := shortDate + "/" + awsRegion + "/" + awsService + "/aws4_request"
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
	signingKey = hmacSHA256(signingKey, awsRegion)
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

func hashPayload(reader io.Reader) (string, error) {
	hash := sha256.New()
	if _, err := io.Copy(hash, reader); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
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
