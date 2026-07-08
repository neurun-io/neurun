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
	DefaultMaxConcurrency = 2
	DefaultRuntimeMode    = "firecracker"
	DefaultInlineMaxBytes = int64(256 * 1024)
)

type Config struct {
	Redis       RedisConfig
	Streams     StreamConfig
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

type WorkerConfig struct {
	ID                   string
	MaxConcurrency       int
	WorkDir              string
	MinFreeMemoryMB      int64
	RuntimeMode          string
	HostPythonBinary     string
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

	cfg.Worker.ID = firstEnvOr(cfg.Worker.ID, "WORKER_ID")
	cfg.Worker.MaxConcurrency = envIntOr("WORKER_MAX_CONCURRENCY", cfg.Worker.MaxConcurrency)
	cfg.Worker.WorkDir = firstEnvOr(cfg.Worker.WorkDir, "WORKER_WORK_DIR")
	cfg.Worker.MinFreeMemoryMB = envInt64Or("WORKER_MIN_FREE_MEMORY_MB", cfg.Worker.MinFreeMemoryMB)
	cfg.Worker.RuntimeMode = strings.ToLower(firstEnvOr(cfg.Worker.RuntimeMode, "WORKER_RUNTIME_MODE"))
	cfg.Worker.HostPythonBinary = firstEnvOr(cfg.Worker.HostPythonBinary, "WORKER_HOST_PYTHON", "PYTHON")
	cfg.Worker.OutputInlineMaxBytes = envInt64Or("WORKER_OUTPUT_INLINE_MAX_BYTES", cfg.Worker.OutputInlineMaxBytes)

	cfg.Firecracker.RunnerCommand = firstEnv("FIRECRACKER_RUNNER_COMMAND", "WORKER_FIRECRACKER_RUNNER_COMMAND")
	if cfg.Worker.MaxConcurrency <= 0 {
		cfg.Worker.MaxConcurrency = DefaultMaxConcurrency
	}
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
		Worker: WorkerConfig{
			MaxConcurrency:       DefaultMaxConcurrency,
			RuntimeMode:          DefaultRuntimeMode,
			HostPythonBinary:     "python",
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
