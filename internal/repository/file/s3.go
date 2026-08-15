package file

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3Types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"

	"github.com/neurun-io/neurun/internal/domain/build"
	"github.com/neurun-io/neurun/internal/files"
)

type S3Options struct {
	Bucket    string
	Endpoint  string
	Region    string
	AccessKey string
	SecretKey string
	// PathStyle addresses the bucket in the path rather than the hostname.
	// R2 accepts either; a bucket whose name is not a valid DNS label needs it.
	PathStyle bool
}

// S3 keeps artifact payloads in an S3-compatible bucket — R2 in
// production, and anything speaking the same API elsewhere.
//
// Keys stay exactly as the rest of the system writes them, so a bucket is a
// byte-for-byte copy of a Local root and the two can be migrated between
// with a plain copy.
type S3 struct {
	client *s3.Client
	bucket string
}

func NewS3(options S3Options) (*S3, error) {
	bucket := strings.TrimSpace(options.Bucket)
	endpoint := strings.TrimSpace(options.Endpoint)
	switch {
	case bucket == "":
		return nil, errors.New("artifact: S3 bucket is required")
	case options.AccessKey == "" || options.SecretKey == "":
		return nil, errors.New("artifact: S3 credentials are required")
	}
	region := strings.TrimSpace(options.Region)
	if region == "" {
		// R2 has one region and names it this.
		region = "auto"
	}
	configuration := aws.Config{
		Region: region,
		Credentials: credentials.NewStaticCredentialsProvider(
			options.AccessKey, options.SecretKey, "",
		),
	}
	client := s3.NewFromConfig(configuration, func(settings *s3.Options) {
		if endpoint != "" {
			settings.BaseEndpoint = aws.String(endpoint)
		}
		settings.UsePathStyle = options.PathStyle
	})
	return &S3{client: client, bucket: bucket}, nil
}

// Put stores an object create-only.
//
// The body is spooled to a temporary file first: the digest is part of what a
// caller gets back, computing it means reading the stream, and the request needs
// a rewindable body to be signed and retried. Content addressing makes the
// head-then-put race harmless — two writers racing on one key are writing
// identical bytes.
func (store *S3) Put(
	ctx context.Context,
	storageKey string,
	source io.Reader,
	maxBytes int64,
) (Info, error) {
	if err := build.ValidateStorageKey(storageKey); err != nil {
		return Info{}, err
	}
	if source == nil {
		return Info{}, errors.New("artifact: source is nil")
	}
	if maxBytes < 0 {
		return Info{}, errors.New("artifact: byte limit cannot be negative")
	}
	ctx = orBackground(ctx)
	if err := ctx.Err(); err != nil {
		return Info{}, err
	}

	if exists, err := store.exists(ctx, storageKey); err != nil {
		return Info{}, err
	} else if exists {
		return Info{}, fmt.Errorf("%w: %s", ErrExists, storageKey)
	}

	// A caller that already holds the bytes on disk — the cache staging a fill,
	// the service publishing a build — is uploaded from where they are.
	if seekable, ok := source.(io.ReadSeeker); ok {
		return store.putSeekable(ctx, storageKey, seekable, maxBytes)
	}
	spooled, err := os.CreateTemp("", "neurun-s3-upload-*")
	if err != nil {
		return Info{}, fmt.Errorf("artifact: spool object: %w", err)
	}
	spooledPath := spooled.Name()
	defer os.Remove(spooledPath)
	defer spooled.Close()

	result, err := files.CopyAndHashContext(ctx, spooled, source, maxBytes)
	if err != nil {
		return Info{}, err
	}
	if _, err := spooled.Seek(0, io.SeekStart); err != nil {
		return Info{}, fmt.Errorf("artifact: rewind spooled object: %w", err)
	}
	if _, err := store.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(store.bucket),
		Key:           aws.String(storageKey),
		Body:          spooled,
		ContentLength: aws.Int64(result.SizeBytes),
	}); err != nil {
		return Info{}, fmt.Errorf("artifact: put object: %w", err)
	}
	return Info{
		storageKey: storageKey,
		sizeBytes:  result.SizeBytes,
		sha256:     result.SHA256,
	}, nil
}

func (store *S3) putSeekable(
	ctx context.Context,
	storageKey string,
	source io.ReadSeeker,
	maxBytes int64,
) (Info, error) {
	start, err := source.Seek(0, io.SeekCurrent)
	if err != nil {
		return Info{}, fmt.Errorf("artifact: locate object: %w", err)
	}
	result, err := files.CopyAndHashContext(ctx, io.Discard, source, maxBytes)
	if err != nil {
		return Info{}, err
	}
	if _, err := source.Seek(start, io.SeekStart); err != nil {
		return Info{}, fmt.Errorf("artifact: rewind object: %w", err)
	}
	if _, err := store.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(store.bucket),
		Key:           aws.String(storageKey),
		Body:          source,
		ContentLength: aws.Int64(result.SizeBytes),
	}); err != nil {
		return Info{}, fmt.Errorf("artifact: put object: %w", err)
	}
	return Info{
		storageKey: storageKey,
		sizeBytes:  result.SizeBytes,
		sha256:     result.SHA256,
	}, nil
}

// Open streams an object. The digest is not recomputed here — that would mean
// downloading twice — and the caller verifies it against the metadata it already
// holds.
func (store *S3) Open(
	ctx context.Context,
	storageKey string,
) (io.ReadCloser, Info, error) {
	if err := build.ValidateStorageKey(storageKey); err != nil {
		return nil, Info{}, err
	}
	ctx = orBackground(ctx)
	if err := ctx.Err(); err != nil {
		return nil, Info{}, err
	}
	object, err := store.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(store.bucket),
		Key:    aws.String(storageKey),
	})
	if err != nil {
		if notFound(err) {
			return nil, Info{}, fmt.Errorf("%w: %s", ErrNotFound, storageKey)
		}
		return nil, Info{}, fmt.Errorf("artifact: get object: %w", err)
	}
	info := Info{storageKey: storageKey}
	if object.ContentLength != nil {
		info.sizeBytes = *object.ContentLength
	}
	return object.Body, info, nil
}

func (store *S3) Delete(ctx context.Context, storageKey string) error {
	if err := build.ValidateStorageKey(storageKey); err != nil {
		return err
	}
	ctx = orBackground(ctx)
	if err := ctx.Err(); err != nil {
		return err
	}
	if exists, err := store.exists(ctx, storageKey); err != nil {
		return err
	} else if !exists {
		return fmt.Errorf("%w: %s", ErrNotFound, storageKey)
	}
	if _, err := store.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(store.bucket),
		Key:    aws.String(storageKey),
	}); err != nil {
		return fmt.Errorf("artifact: delete object: %w", err)
	}
	return nil
}

// Check proves the credentials reach the bucket, which is what readiness is
// asking. HeadBucket is the cheapest call that answers it.
func (store *S3) Check(ctx context.Context) error {
	if store == nil || store.client == nil {
		return errors.New("artifact: S3 store is not configured")
	}
	ctx = orBackground(ctx)
	if _, err := store.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(store.bucket),
	}); err != nil {
		return fmt.Errorf("artifact: bucket is not reachable: %w", err)
	}
	return nil
}

func (store *S3) exists(ctx context.Context, storageKey string) (bool, error) {
	_, err := store.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(store.bucket),
		Key:    aws.String(storageKey),
	})
	if err == nil {
		return true, nil
	}
	if notFound(err) {
		return false, nil
	}
	return false, fmt.Errorf("artifact: head object: %w", err)
}

// notFound reads both shapes the API returns: GetObject answers NoSuchKey, while
// HeadObject has no body to carry one and answers a bare 404.
func notFound(err error) bool {
	var noSuchKey *s3Types.NoSuchKey
	if errors.As(err, &noSuchKey) {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NoSuchKey", "NotFound", "404":
			return true
		}
	}
	return false
}

func orBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

var _ Repository = (*S3)(nil)
