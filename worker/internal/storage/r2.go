package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/dagflows/worker/internal/config"
)

const cloudflareR2HostSuffix = ".r2.cloudflarestorage.com"

type R2Store struct {
	client *s3.Client
	bucket string
	prefix string
}

func NewR2(cfg config.R2Config) (*R2Store, error) {
	bucket := strings.TrimSpace(cfg.Bucket)
	accessKey := strings.TrimSpace(cfg.AccessKeyID)
	secretKey := strings.TrimSpace(cfg.SecretAccessKey)
	if bucket == "" || accessKey == "" || secretKey == "" {
		return nil, fmt.Errorf("R2_BUCKET, R2_ACCESS_KEY_ID, and R2_SECRET_ACCESS_KEY are required")
	}

	endpoint := strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/")
	if endpoint == "" {
		accountID := strings.TrimSpace(cfg.AccountID)
		if accountID == "" {
			return nil, fmt.Errorf("R2_ENDPOINT or R2_ACCOUNT_ID is required")
		}
		endpoint = "https://" + accountID + cloudflareR2HostSuffix
	}

	region := strings.TrimSpace(cfg.Region)
	if region == "" {
		region = config.DefaultR2Region
	}
	client := s3.New(s3.Options{
		BaseEndpoint: aws.String(endpoint),
		Credentials:  credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
		DisableLogOutputChecksumValidationSkipped: true,
		Region:       region,
		UsePathStyle: true,
	})
	return &R2Store{client: client, bucket: bucket, prefix: strings.Trim(cfg.Prefix, "/")}, nil
}

func (s *R2Store) Fetch(ctx context.Context, ref, path string) error {
	key := s.objectKey(ref)
	output, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("get artifact %s: %w", key, err)
	}
	defer output.Body.Close()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(file, output.Body)
	return errors.Join(copyErr, file.Close())
}

func (s *R2Store) objectKey(ref string) string {
	key := strings.TrimLeft(strings.TrimSpace(ref), "/")
	if s.prefix == "" || key == s.prefix || strings.HasPrefix(key, s.prefix+"/") {
		return key
	}
	return s.prefix + "/" + key
}
