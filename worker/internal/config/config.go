package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const (
	DefaultRedisAddr      = "localhost:6379"
	DefaultRequestStream  = "goflow:node_run_requests"
	DefaultRequestGroup   = "goflow:node_run_requests:group"
	DefaultResponseStream = "goflow:node_run_responses"
	DefaultMinIdle        = 2 * time.Minute
	DefaultBlockDuration  = 2 * time.Second
	DefaultInlineMaxBytes = int64(256 * 1024)
	DefaultR2Region       = "auto"
	DefaultMaxConcurrency = 1
)

type Config struct {
	Redis   RedisConfig
	Streams StreamConfig
	R2      R2Config
	Worker  WorkerConfig
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

type StreamConfig struct {
	RequestStream  string
	RequestGroup   string
	ResponseStream string
	MinIdle        time.Duration
	BlockDuration  time.Duration
	MaxLen         int64
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

type WorkerConfig struct {
	ID                   string
	WorkDir              string
	MaxConcurrency       int
	OutputInlineMaxBytes int64
}

func Load(path string) (Config, error) {
	if err := godotenv.Load(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, err
	}

	cfg := Default()
	cfg.Redis.Addr = envOr("REDIS_ADDR", cfg.Redis.Addr)
	cfg.Redis.Password = env("REDIS_PASSWORD")
	cfg.Redis.DB = envIntOr("REDIS_DB", cfg.Redis.DB)

	cfg.Streams.RequestStream = envOr("WORKER_REQUEST_STREAM", cfg.Streams.RequestStream)
	cfg.Streams.RequestGroup = envOr("WORKER_REQUEST_GROUP", cfg.Streams.RequestGroup)
	cfg.Streams.ResponseStream = envOr("WORKER_RESPONSE_STREAM", cfg.Streams.ResponseStream)
	cfg.Streams.MinIdle = envDurationOr("WORKER_STALE_ENTRY_RECLAIM_THRESHOLD", cfg.Streams.MinIdle)
	cfg.Streams.BlockDuration = envDurationOr("WORKER_BLOCKING_READ_TIMEOUT", cfg.Streams.BlockDuration)
	cfg.Streams.MaxLen = envInt64Or("WORKER_RESPONSE_MAX_LEN", cfg.Streams.MaxLen)

	cfg.R2.AccountID = env("R2_ACCOUNT_ID")
	cfg.R2.Endpoint = env("R2_ENDPOINT")
	cfg.R2.Region = envOr("R2_REGION", cfg.R2.Region)
	cfg.R2.Bucket = env("R2_BUCKET")
	cfg.R2.AccessKeyID = env("R2_ACCESS_KEY_ID")
	cfg.R2.SecretAccessKey = env("R2_SECRET_ACCESS_KEY")
	cfg.R2.Prefix = strings.Trim(env("R2_PREFIX"), "/")

	cfg.Worker.ID = env("WORKER_ID")
	cfg.Worker.WorkDir = env("WORKER_WORK_DIR")
	cfg.Worker.MaxConcurrency = envIntOr("WORKER_MAX_CONCURRENCY", cfg.Worker.MaxConcurrency)
	cfg.Worker.OutputInlineMaxBytes = envInt64Or("WORKER_OUTPUT_INLINE_MAX_BYTES", cfg.Worker.OutputInlineMaxBytes)

	if cfg.Worker.OutputInlineMaxBytes <= 0 {
		cfg.Worker.OutputInlineMaxBytes = DefaultInlineMaxBytes
	}
	if cfg.Worker.MaxConcurrency <= 0 {
		cfg.Worker.MaxConcurrency = DefaultMaxConcurrency
	}
	return cfg, nil
}

func Default() Config {
	return Config{
		Redis: RedisConfig{
			Addr: DefaultRedisAddr,
		},
		Streams: StreamConfig{
			RequestStream:  DefaultRequestStream,
			RequestGroup:   DefaultRequestGroup,
			ResponseStream: DefaultResponseStream,
			MinIdle:        DefaultMinIdle,
			BlockDuration:  DefaultBlockDuration,
			MaxLen:         10000,
		},
		R2: R2Config{
			Region: DefaultR2Region,
		},
		Worker: WorkerConfig{
			MaxConcurrency:       DefaultMaxConcurrency,
			OutputInlineMaxBytes: DefaultInlineMaxBytes,
		},
	}
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

func envIntOr(name string, fallback int) int {
	value := env(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envInt64Or(name string, fallback int64) int64 {
	value := env(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func envDurationOr(name string, fallback time.Duration) time.Duration {
	value := env(name)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err == nil {
		return parsed
	}
	seconds, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return time.Duration(seconds) * time.Second
}
