package config

import (
	"bufio"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const defaultEnvFile = ".env"

const (
	defaultHTTPAddr                 = ":1267"
	defaultPublicURL                = "http://localhost:1267"
	defaultProjectID                = "prj_local"
	defaultDataDirectory            = "data"
	defaultShutdownTimeout          = 10 * time.Second
	defaultRequestBodyBytes         = 1 << 20
	defaultDeploymentSourceBytes    = 32 << 20
	defaultDeploymentExpandedBytes  = 256 << 20
	defaultDeploymentArtifactBytes  = 256 << 20
	defaultDeploymentArchiveEntries = 1_000
	defaultDeploymentBuildTimeout   = 5 * time.Minute
	defaultRunInputBytes            = 1 << 20
	defaultRunResultBytes           = 4 << 20
	defaultRunLogBytes              = 256 << 10
	defaultRunTimeout               = 5 * time.Minute
	defaultWorkerPollInterval       = 250 * time.Millisecond
	defaultDatabaseURL              = "postgres://neurun:neurun-local-change-me@localhost:5432/neurun?sslmode=disable"
	defaultSessionTTL               = 12 * time.Hour
	defaultDatabaseSchema           = "neurun"
	defaultDatabaseMaxConns         = 25
	defaultDatabaseConnMaxLifetime  = 5 * time.Minute
	defaultDatabaseConnMaxIdleTime  = time.Minute
)

type Config struct {
	HTTPAddr                    string
	PublicURL                   string
	LogLevel                    string
	DefaultProjectID            string
	DataDirectory               string
	PythonExecutable            string
	ShutdownTimeout             time.Duration
	MaxRequestBodyBytes         int64
	MaxDeploymentSourceBytes    int64
	MaxDeploymentExpandedBytes  int64
	MaxDeploymentArtifactBytes  int64
	MaxDeploymentArchiveEntries int
	DeploymentBuildTimeout      time.Duration
	MaxRunInputBytes            int64
	MaxRunResultBytes           int64
	MaxRunLogBytes              int64
	RunTimeout                  time.Duration
	WorkerPollInterval          time.Duration
	DatabaseURL                 string
	DatabaseSchema              string
	DatabaseMaxConns            int
	DatabaseConnMaxLifetime     time.Duration
	DatabaseConnMaxIdleTime     time.Duration
	SessionTTL                  time.Duration
	SessionCookieSecure         bool
	GitHubAppID                 int64
	GitHubPrivateKey            []byte
}

func Load() (Config, error) {
	if err := loadEnvFile(defaultEnvFile); err != nil {
		return Config{}, err
	}

	cfg := Config{
		HTTPAddr:                    value("NEURUN_HTTP_ADDR", defaultHTTPAddr),
		PublicURL:                   value("NEURUN_PUBLIC_URL", defaultPublicURL),
		LogLevel:                    value("NEURUN_LOG_LEVEL", "info"),
		DefaultProjectID:            value("NEURUN_DEFAULT_PROJECT_ID", defaultProjectID),
		DataDirectory:               value("NEURUN_DATA_DIRECTORY", defaultDataDirectory),
		PythonExecutable:            strings.TrimSpace(os.Getenv("NEURUN_PYTHON_EXECUTABLE")),
		ShutdownTimeout:             defaultShutdownTimeout,
		MaxRequestBodyBytes:         defaultRequestBodyBytes,
		MaxDeploymentSourceBytes:    defaultDeploymentSourceBytes,
		MaxDeploymentExpandedBytes:  defaultDeploymentExpandedBytes,
		MaxDeploymentArtifactBytes:  defaultDeploymentArtifactBytes,
		MaxDeploymentArchiveEntries: defaultDeploymentArchiveEntries,
		DeploymentBuildTimeout:      defaultDeploymentBuildTimeout,
		MaxRunInputBytes:            defaultRunInputBytes,
		MaxRunResultBytes:           defaultRunResultBytes,
		MaxRunLogBytes:              defaultRunLogBytes,
		RunTimeout:                  defaultRunTimeout,
		WorkerPollInterval:          defaultWorkerPollInterval,
		DatabaseURL:                 value("NEURUN_DATABASE_URL", defaultDatabaseURL),
		DatabaseSchema:              value("NEURUN_DATABASE_SCHEMA", defaultDatabaseSchema),
		DatabaseMaxConns:            defaultDatabaseMaxConns,
		DatabaseConnMaxLifetime:     defaultDatabaseConnMaxLifetime,
		DatabaseConnMaxIdleTime:     defaultDatabaseConnMaxIdleTime,
		SessionTTL:                  defaultSessionTTL,
		SessionCookieSecure:         false,
	}

	var err error
	if cfg.GitHubAppID, err = int64Value("NEURUN_GITHUB_APP_ID", 0); err != nil {
		return Config{}, err
	}
	if encoded := value("NEURUN_GITHUB_PRIVATE_KEY", ""); encoded != "" {
		cfg.GitHubPrivateKey, err = base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return Config{}, fmt.Errorf(
				"NEURUN_GITHUB_PRIVATE_KEY must be the base64 of the app's PEM: %w", err,
			)
		}
	}
	if cfg.SessionCookieSecure, err = boolValue(
		"NEURUN_SESSION_COOKIE_SECURE", cfg.SessionCookieSecure,
	); err != nil {
		return Config{}, err
	}
	durationFields := []struct {
		name string
		dst  *time.Duration
	}{
		{"NEURUN_SHUTDOWN_TIMEOUT", &cfg.ShutdownTimeout},
		{"NEURUN_DEPLOYMENT_BUILD_TIMEOUT", &cfg.DeploymentBuildTimeout},
		{"NEURUN_EXECUTION_TIMEOUT", &cfg.RunTimeout},
		{"NEURUN_WORKER_POLL_INTERVAL", &cfg.WorkerPollInterval},
		{"NEURUN_SESSION_TTL", &cfg.SessionTTL},
	}
	for _, field := range durationFields {
		if *field.dst, err = durationValue(field.name, *field.dst); err != nil {
			return Config{}, err
		}
	}
	byteFields := []struct {
		name string
		dst  *int64
	}{
		{"NEURUN_MAX_REQUEST_BODY_BYTES", &cfg.MaxRequestBodyBytes},
		{"NEURUN_MAX_DEPLOYMENT_SOURCE_BYTES", &cfg.MaxDeploymentSourceBytes},
		{"NEURUN_MAX_DEPLOYMENT_EXPANDED_BYTES", &cfg.MaxDeploymentExpandedBytes},
		{"NEURUN_MAX_DEPLOYMENT_ARTIFACT_BYTES", &cfg.MaxDeploymentArtifactBytes},
		{"NEURUN_MAX_EXECUTION_INPUT_BYTES", &cfg.MaxRunInputBytes},
		{"NEURUN_MAX_EXECUTION_RESULT_BYTES", &cfg.MaxRunResultBytes},
		{"NEURUN_MAX_EXECUTION_LOG_BYTES", &cfg.MaxRunLogBytes},
	}
	for _, field := range byteFields {
		if *field.dst, err = int64Value(field.name, *field.dst); err != nil {
			return Config{}, err
		}
	}
	if cfg.MaxDeploymentArchiveEntries, err = intValue(
		"NEURUN_MAX_DEPLOYMENT_ARCHIVE_ENTRIES", cfg.MaxDeploymentArchiveEntries,
	); err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// loadEnvFile loads development defaults without replacing values explicitly
// supplied by the process environment.
func loadEnvFile(path string) error {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open environment file %q: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024), 1<<20)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, rawValue, found := strings.Cut(line, "=")
		if !found {
			return fmt.Errorf("parse environment file %q line %d: expected KEY=VALUE", path, lineNumber)
		}
		key = strings.TrimSpace(strings.TrimPrefix(key, "export "))
		if !validEnvKey(key) {
			return fmt.Errorf("parse environment file %q line %d: invalid key %q", path, lineNumber, key)
		}
		value, err := parseEnvValue(strings.TrimSpace(rawValue))
		if err != nil {
			return fmt.Errorf("parse environment file %q line %d: %w", path, lineNumber, err)
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("set environment variable %q: %w", key, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read environment file %q: %w", path, err)
	}
	return nil
}

func validEnvKey(key string) bool {
	if key == "" {
		return false
	}
	for index, char := range key {
		if char == '_' || char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' ||
			index > 0 && char >= '0' && char <= '9' {
			continue
		}
		return false
	}
	return true
}

func parseEnvValue(value string) (string, error) {
	if value == "" || value[0] != '"' && value[0] != '\'' {
		return strings.TrimSpace(strings.SplitN(value, " #", 2)[0]), nil
	}
	if len(value) < 2 || value[len(value)-1] != value[0] {
		return "", errors.New("unterminated quoted value")
	}
	if value[0] == '\'' {
		return value[1 : len(value)-1], nil
	}
	parsed, err := strconv.Unquote(value)
	if err != nil {
		return "", fmt.Errorf("unquote value: %w", err)
	}
	return parsed, nil
}

// DatabaseDSN is DatabaseURL pinned to the configured schema, so unqualified
// table names resolve there rather than in public.
func (c Config) DatabaseDSN() (string, error) {
	parsed, err := url.Parse(c.DatabaseURL)
	if err != nil {
		return "", fmt.Errorf("parse database URL: %w", err)
	}
	query := parsed.Query()
	query.Set("search_path", c.DatabaseSchema)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func (c Config) Validate() error {
	var problems []error
	if strings.TrimSpace(c.HTTPAddr) == "" {
		problems = append(problems, errors.New("NEURUN_HTTP_ADDR is required"))
	}
	publicURL, err := url.Parse(c.PublicURL)
	if err != nil || publicURL.Host == "" ||
		(publicURL.Scheme != "http" && publicURL.Scheme != "https") {
		problems = append(problems,
			errors.New("NEURUN_PUBLIC_URL must be an absolute http or https URL"))
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		problems = append(problems,
			errors.New("NEURUN_LOG_LEVEL must be debug, info, warn, or error"))
	}
	if strings.TrimSpace(c.DefaultProjectID) == "" {
		problems = append(problems, errors.New("NEURUN_DEFAULT_PROJECT_ID is required"))
	}
	if strings.TrimSpace(c.DataDirectory) == "" {
		problems = append(problems, errors.New("NEURUN_DATA_DIRECTORY is required"))
	}
	databaseURL, databaseErr := url.Parse(c.DatabaseURL)
	if databaseErr != nil || databaseURL.Host == "" ||
		(databaseURL.Scheme != "postgres" && databaseURL.Scheme != "postgresql") {
		problems = append(problems,
			errors.New("NEURUN_DATABASE_URL must be an absolute postgres URL"))
	}
	positiveDurations := []struct {
		name  string
		value time.Duration
	}{
		{"NEURUN_SHUTDOWN_TIMEOUT", c.ShutdownTimeout},
		{"NEURUN_DEPLOYMENT_BUILD_TIMEOUT", c.DeploymentBuildTimeout},
		{"NEURUN_EXECUTION_TIMEOUT", c.RunTimeout},
		{"NEURUN_WORKER_POLL_INTERVAL", c.WorkerPollInterval},
		{"NEURUN_SESSION_TTL", c.SessionTTL},
	}
	for _, field := range positiveDurations {
		if field.value <= 0 {
			problems = append(problems, fmt.Errorf("%s must be positive", field.name))
		}
	}
	positiveSizes := []struct {
		name  string
		value int64
	}{
		{"NEURUN_MAX_REQUEST_BODY_BYTES", c.MaxRequestBodyBytes},
		{"NEURUN_MAX_DEPLOYMENT_SOURCE_BYTES", c.MaxDeploymentSourceBytes},
		{"NEURUN_MAX_DEPLOYMENT_EXPANDED_BYTES", c.MaxDeploymentExpandedBytes},
		{"NEURUN_MAX_DEPLOYMENT_ARTIFACT_BYTES", c.MaxDeploymentArtifactBytes},
		{"NEURUN_MAX_EXECUTION_INPUT_BYTES", c.MaxRunInputBytes},
		{"NEURUN_MAX_EXECUTION_RESULT_BYTES", c.MaxRunResultBytes},
		{"NEURUN_MAX_EXECUTION_LOG_BYTES", c.MaxRunLogBytes},
	}
	for _, field := range positiveSizes {
		if field.value <= 0 {
			problems = append(problems, fmt.Errorf("%s must be positive", field.name))
		}
	}
	if c.MaxRunLogBytes > 256<<10 {
		problems = append(problems,
			errors.New("NEURUN_MAX_EXECUTION_LOG_BYTES cannot exceed 262144"))
	}
	if c.MaxDeploymentArchiveEntries <= 0 {
		problems = append(problems,
			errors.New("NEURUN_MAX_DEPLOYMENT_ARCHIVE_ENTRIES must be positive"))
	}
	return errors.Join(problems...)
}

func value(key, fallback string) string {
	if raw := strings.TrimSpace(os.Getenv(key)); raw != "" {
		return raw
	}
	return fallback
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
