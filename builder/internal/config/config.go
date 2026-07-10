package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/dagflows/builder/pkg"
)

const (
	DefaultAWSRegion                   = "us-east-1"
	DefaultSQSWaitTimeSeconds          = int32(20)
	DefaultSQSVisibilityTimeoutSeconds = int32(900)
	DefaultR2Region                    = "auto"
	DefaultR2Prefix                    = "builds"
	DefaultGitBranch                   = "main"
)

type Config struct {
	AWS AWSConfig
	SQS SQSConfig
	R2  R2Config
}

type AWSConfig struct {
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
}

type SQSConfig struct {
	RequestQueueURL          string
	ResponseQueueURL         string
	WaitTimeSeconds          int32
	VisibilityTimeoutSeconds int32
}

type R2Config struct {
	AccountID       string
	Endpoint        string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	Prefix          string
}

func Load(path string) (Config, error) {
	if err := pkg.LoadDotEnv(path); err != nil {
		return Config{}, err
	}

	cfg := Default()
	cfg.AWS.Region = firstEnvOr(cfg.AWS.Region, "SQS_REGION", "AWS_REGION", "AWS_DEFAULT_REGION")
	cfg.AWS.AccessKeyID = env("AWS_ACCESS_KEY_ID")
	cfg.AWS.SecretAccessKey = env("AWS_SECRET_ACCESS_KEY")
	cfg.AWS.SessionToken = env("AWS_SESSION_TOKEN")

	cfg.SQS.RequestQueueURL = firstEnv("SQS_REQUEST_QUEUE_URL", "SQS_QUEUE_URL")
	cfg.SQS.ResponseQueueURL = firstEnv("SQS_RESPONSE_QUEUE_URL", "SQS_RESULT_QUEUE_URL")
	var err error
	if cfg.SQS.WaitTimeSeconds, err = envInt32("SQS_WAIT_TIME_SECONDS", cfg.SQS.WaitTimeSeconds, 20); err != nil {
		return Config{}, err
	}
	if cfg.SQS.VisibilityTimeoutSeconds, err = envInt32("SQS_VISIBILITY_TIMEOUT_SECONDS", cfg.SQS.VisibilityTimeoutSeconds, 43200); err != nil {
		return Config{}, err
	}

	cfg.R2.AccountID = env("R2_ACCOUNT_ID")
	cfg.R2.Endpoint = env("R2_ENDPOINT")
	cfg.R2.Region = envOr("R2_REGION", cfg.R2.Region)
	cfg.R2.Bucket = env("R2_BUCKET")
	cfg.R2.AccessKeyID = env("R2_ACCESS_KEY_ID")
	cfg.R2.SecretAccessKey = env("R2_SECRET_ACCESS_KEY")
	cfg.R2.Prefix = strings.Trim(envOr("R2_PREFIX", cfg.R2.Prefix), "/")

	return cfg, nil
}

func Default() Config {
	return Config{
		AWS: AWSConfig{
			Region: DefaultAWSRegion,
		},
		SQS: SQSConfig{
			WaitTimeSeconds:          DefaultSQSWaitTimeSeconds,
			VisibilityTimeoutSeconds: DefaultSQSVisibilityTimeoutSeconds,
		},
		R2: R2Config{
			Region: DefaultR2Region,
			Prefix: DefaultR2Prefix,
		},
	}
}

func firstEnv(names ...string) string {
	return firstEnvOr("", names...)
}

func firstEnvOr(fallback string, names ...string) string {
	for _, name := range names {
		if value := env(name); value != "" {
			return value
		}
	}
	return fallback
}

func envOr(name, fallback string) string {
	if value := env(name); value != "" {
		return value
	}
	return fallback
}

func env(name string) string {
	return strings.TrimSpace(os.Getenv(name))
}

func envInt32(name string, fallback, max int32) (int32, error) {
	value := env(name)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil || parsed < 0 || parsed > int64(max) {
		return 0, fmt.Errorf("%s must be an integer between 0 and %d", name, max)
	}
	return int32(parsed), nil
}
