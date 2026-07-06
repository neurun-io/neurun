package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/dagflows/builder/pkg"
)

const (
	DefaultAWSRegion                   = "us-east-1"
	DefaultSQSMaxMessages              = int32(1)
	DefaultSQSWaitTimeSeconds          = int32(20)
	DefaultSQSVisibilityTimeoutSeconds = int32(900)
	DefaultR2Prefix                    = "builds"
	DefaultGitBranch                   = "main"
)

type Config struct {
	AWS    AWSConfig
	SQS    SQSConfig
	R2     R2Config
	GitHub GitHubConfig
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
	MaxMessages              int32
	WaitTimeSeconds          int32
	VisibilityTimeoutSeconds int32
}

type R2Config struct {
	AccountID       string
	Endpoint        string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	Prefix          string
}

type GitHubConfig struct {
	TempDir string
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
	cfg.SQS.MaxMessages = envInt32Or("SQS_MAX_MESSAGES", cfg.SQS.MaxMessages)
	cfg.SQS.WaitTimeSeconds = envInt32Or("SQS_WAIT_TIME_SECONDS", cfg.SQS.WaitTimeSeconds)
	cfg.SQS.VisibilityTimeoutSeconds = envInt32Or("SQS_VISIBILITY_TIMEOUT_SECONDS", cfg.SQS.VisibilityTimeoutSeconds)
	if cfg.SQS.MaxMessages > 10 {
		cfg.SQS.MaxMessages = 10
	}

	cfg.R2.AccountID = env("R2_ACCOUNT_ID")
	cfg.R2.Endpoint = env("R2_ENDPOINT")
	cfg.R2.Bucket = env("R2_BUCKET")
	cfg.R2.AccessKeyID = env("R2_ACCESS_KEY_ID")
	cfg.R2.SecretAccessKey = env("R2_SECRET_ACCESS_KEY")
	cfg.R2.Prefix = strings.Trim(envOr("R2_PREFIX", cfg.R2.Prefix), "/")

	cfg.GitHub.TempDir = env("GIT_TEMP_DIR")

	return cfg, nil
}

func Default() Config {
	return Config{
		AWS: AWSConfig{
			Region: DefaultAWSRegion,
		},
		SQS: SQSConfig{
			MaxMessages:              DefaultSQSMaxMessages,
			WaitTimeSeconds:          DefaultSQSWaitTimeSeconds,
			VisibilityTimeoutSeconds: DefaultSQSVisibilityTimeoutSeconds,
		},
		R2: R2Config{
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

func envInt32Or(name string, fallback int32) int32 {
	value := env(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return int32(parsed)
}
