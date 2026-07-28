package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultHTTPAddr         = ":8080"
	defaultPublicURL        = "http://localhost:8080"
	defaultProjectID        = "prj_local"
	defaultShutdownTimeout  = 10 * time.Second
	defaultRequestBodyBytes = 1 << 20
	defaultResponseBytes    = 4 << 20
	defaultLeaseDuration    = 30 * time.Second
	defaultAgentConcurrency = 8
)

type Config struct {
	HTTPAddr             string
	PublicURL            string
	LogLevel             string
	APIKey               string
	DefaultProjectID     string
	BlockPrivateNetworks bool
	ShutdownTimeout      time.Duration
	MaxRequestBodyBytes  int64
	MaxResponseBytes     int64
	JobLeaseDuration     time.Duration
	AgentConcurrency     int
	AllowVolatileJobs    bool
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:             value("NEURUN_HTTP_ADDR", defaultHTTPAddr),
		PublicURL:            value("NEURUN_PUBLIC_URL", defaultPublicURL),
		LogLevel:             value("NEURUN_LOG_LEVEL", "info"),
		APIKey:               strings.TrimSpace(os.Getenv("NEURUN_API_KEY")),
		DefaultProjectID:     value("NEURUN_DEFAULT_PROJECT_ID", defaultProjectID),
		BlockPrivateNetworks: true,
		ShutdownTimeout:      defaultShutdownTimeout,
		MaxRequestBodyBytes:  defaultRequestBodyBytes,
		MaxResponseBytes:     defaultResponseBytes,
		JobLeaseDuration:     defaultLeaseDuration,
		AgentConcurrency:     defaultAgentConcurrency,
		AllowVolatileJobs:    false,
	}

	var err error
	if cfg.BlockPrivateNetworks, err = boolValue("NEURUN_BLOCK_PRIVATE_NETWORKS", cfg.BlockPrivateNetworks); err != nil {
		return Config{}, err
	}
	if cfg.ShutdownTimeout, err = durationValue("NEURUN_SHUTDOWN_TIMEOUT", cfg.ShutdownTimeout); err != nil {
		return Config{}, err
	}
	if cfg.MaxRequestBodyBytes, err = int64Value("NEURUN_MAX_REQUEST_BODY_BYTES", cfg.MaxRequestBodyBytes); err != nil {
		return Config{}, err
	}
	if cfg.MaxResponseBytes, err = int64Value("NEURUN_MAX_RESPONSE_BYTES", cfg.MaxResponseBytes); err != nil {
		return Config{}, err
	}
	if cfg.JobLeaseDuration, err = durationValue("NEURUN_JOB_LEASE_DURATION", cfg.JobLeaseDuration); err != nil {
		return Config{}, err
	}
	if cfg.AgentConcurrency, err = intValue("NEURUN_AGENT_CONCURRENCY", cfg.AgentConcurrency); err != nil {
		return Config{}, err
	}
	if cfg.AllowVolatileJobs, err = boolValue("NEURUN_ALLOW_VOLATILE_JOBS", cfg.AllowVolatileJobs); err != nil {
		return Config{}, err
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	var problems []error
	if strings.TrimSpace(c.HTTPAddr) == "" {
		problems = append(problems, errors.New("NEURUN_HTTP_ADDR is required"))
	}
	publicURL, err := url.Parse(c.PublicURL)
	if err != nil || publicURL.Host == "" || (publicURL.Scheme != "http" && publicURL.Scheme != "https") {
		problems = append(problems, errors.New("NEURUN_PUBLIC_URL must be an absolute http or https URL"))
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		problems = append(problems, errors.New("NEURUN_LOG_LEVEL must be debug, info, warn, or error"))
	}
	if c.APIKey == "" {
		problems = append(problems, errors.New("NEURUN_API_KEY is required"))
	} else if !strings.HasPrefix(c.APIKey, "neu_") || !strings.Contains(c.APIKey, ".") {
		problems = append(problems, errors.New("NEURUN_API_KEY must use the neu_<environment>_<prefix>.<secret> form"))
	}
	if strings.TrimSpace(c.DefaultProjectID) == "" {
		problems = append(problems, errors.New("NEURUN_DEFAULT_PROJECT_ID is required"))
	}
	if c.ShutdownTimeout <= 0 {
		problems = append(problems, errors.New("NEURUN_SHUTDOWN_TIMEOUT must be positive"))
	}
	if c.MaxRequestBodyBytes <= 0 {
		problems = append(problems, errors.New("NEURUN_MAX_REQUEST_BODY_BYTES must be positive"))
	}
	if c.MaxResponseBytes <= 0 {
		problems = append(problems, errors.New("NEURUN_MAX_RESPONSE_BYTES must be positive"))
	}
	if c.JobLeaseDuration <= 0 {
		problems = append(problems, errors.New("NEURUN_JOB_LEASE_DURATION must be positive"))
	}
	if c.AgentConcurrency <= 0 {
		problems = append(problems, errors.New("NEURUN_AGENT_CONCURRENCY must be positive"))
	}
	return errors.Join(problems...)
}

func value(key, fallback string) string {
	if raw := strings.TrimSpace(os.Getenv(key)); raw != "" {
		return raw
	}
	return fallback
}

func boolValue(key string, fallback bool) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s: %w", key, err)
	}
	return parsed, nil
}

func durationValue(key string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return parsed, nil
}

func int64Value(key string, fallback int64) (int64, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return parsed, nil
}

func intValue(key string, fallback int) (int, error) {
	parsed, err := int64Value(key, int64(fallback))
	if err != nil {
		return 0, err
	}
	if int64(int(parsed)) != parsed {
		return 0, fmt.Errorf("%s is outside the supported integer range", key)
	}
	return int(parsed), nil
}
