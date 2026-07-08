package config

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dagflows/worker/pkg"
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
)

type Config struct {
	Redis       RedisConfig
	Streams     StreamConfig
	R2          R2Config
	Worker      WorkerConfig
	Firecracker FirecrackerConfig
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
	MinFreeMemoryMB      int64
	OutputInlineMaxBytes int64
}

type FirecrackerConfig struct {
	RunnerCommand string
}

func Load(path string) (Config, error) {
	if err := pkg.LoadDotEnv(path); err != nil {
		return Config{}, err
	}

	cfg := Default()
	cfg.Redis.Addr = firstEnvOr(cfg.Redis.Addr, "WORKER_REDIS_ADDR", "REDIS_ADDR", "GOFLOW_REDIS_ADDR")
	cfg.Redis.Password = firstEnv("WORKER_REDIS_PASSWORD", "REDIS_PASSWORD", "GOFLOW_REDIS_PASSWORD")
	cfg.Redis.DB = envIntOr("WORKER_REDIS_DB", cfg.Redis.DB)

	cfg.Streams.RequestStream = firstEnvOr(cfg.Streams.RequestStream, "WORKER_REQUEST_STREAM", "REDIS_REQUEST_STREAM")
	cfg.Streams.RequestGroup = firstEnvOr(cfg.Streams.RequestGroup, "WORKER_REQUEST_GROUP", "REDIS_REQUEST_GROUP")
	cfg.Streams.ResponseStream = firstEnvOr(cfg.Streams.ResponseStream, "WORKER_RESPONSE_STREAM", "REDIS_RESPONSE_STREAM")
	cfg.Streams.MinIdle = envDurationOr("WORKER_STALE_ENTRY_RECLAIM_THRESHOLD", cfg.Streams.MinIdle)
	cfg.Streams.BlockDuration = envDurationOr("WORKER_BLOCKING_READ_TIMEOUT", cfg.Streams.BlockDuration)
	cfg.Streams.MaxLen = envInt64Or("WORKER_RESPONSE_MAX_LEN", cfg.Streams.MaxLen)

	cfg.R2.AccountID = firstEnv("WORKER_R2_ACCOUNT_ID", "R2_ACCOUNT_ID")
	cfg.R2.Endpoint = firstEnv("WORKER_R2_ENDPOINT", "R2_ENDPOINT")
	cfg.R2.Region = firstEnvOr(cfg.R2.Region, "WORKER_R2_REGION", "R2_REGION")
	cfg.R2.Bucket = firstEnv("WORKER_R2_BUCKET", "R2_BUCKET")
	cfg.R2.AccessKeyID = firstEnv("WORKER_R2_ACCESS_KEY_ID", "R2_ACCESS_KEY_ID")
	cfg.R2.SecretAccessKey = firstEnv("WORKER_R2_SECRET_ACCESS_KEY", "R2_SECRET_ACCESS_KEY")
	cfg.R2.Prefix = strings.Trim(firstEnvOr(cfg.R2.Prefix, "WORKER_R2_PREFIX", "R2_PREFIX"), "/")

	cfg.Worker.ID = firstEnvOr(cfg.Worker.ID, "WORKER_ID")
	cfg.Worker.WorkDir = firstEnvOr(cfg.Worker.WorkDir, "WORKER_WORK_DIR")
	cfg.Worker.MinFreeMemoryMB = envInt64Or("WORKER_MIN_FREE_MEMORY_MB", cfg.Worker.MinFreeMemoryMB)
	cfg.Worker.OutputInlineMaxBytes = envInt64Or("WORKER_OUTPUT_INLINE_MAX_BYTES", cfg.Worker.OutputInlineMaxBytes)

	cfg.Firecracker.RunnerCommand = firstEnv("FIRECRACKER_RUNNER_COMMAND", "WORKER_FIRECRACKER_RUNNER_COMMAND")
	if cfg.Worker.OutputInlineMaxBytes <= 0 {
		cfg.Worker.OutputInlineMaxBytes = DefaultInlineMaxBytes
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
			OutputInlineMaxBytes: DefaultInlineMaxBytes,
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
