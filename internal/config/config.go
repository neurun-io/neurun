package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/neurun-io/neurun/internal/domain/operator"
)

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
	defaultOperatorSessionTTL       = 12 * time.Hour
	defaultDatabaseSchema           = "neurun"
	defaultDatabaseMaxConns         = 25
	defaultDatabaseConnMaxLifetime  = 5 * time.Minute
	defaultDatabaseConnMaxIdleTime  = time.Minute
)

type Config struct {
	HTTPAddr                    string
	PublicURL                   string
	LogLevel                    string
	APIKey                      string
	DefaultProjectID            string
	DataDirectory               string
	PythonExecutable            string
	TrustedCodeExecution        bool
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
	OperatorAccounts            []operator.Account
	OperatorSessionTTL          time.Duration
	OperatorCookieSecure        bool
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:                    value("NEURUN_HTTP_ADDR", defaultHTTPAddr),
		PublicURL:                   value("NEURUN_PUBLIC_URL", defaultPublicURL),
		LogLevel:                    value("NEURUN_LOG_LEVEL", "info"),
		APIKey:                      strings.TrimSpace(os.Getenv("NEURUN_API_KEY")),
		DefaultProjectID:            value("NEURUN_DEFAULT_PROJECT_ID", defaultProjectID),
		DataDirectory:               value("NEURUN_DATA_DIRECTORY", defaultDataDirectory),
		PythonExecutable:            strings.TrimSpace(os.Getenv("NEURUN_PYTHON_EXECUTABLE")),
		TrustedCodeExecution:        false,
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
		OperatorSessionTTL:          defaultOperatorSessionTTL,
		OperatorCookieSecure:        false,
	}
	var err error
	if cfg.TrustedCodeExecution, err = boolValue(
		"NEURUN_TRUSTED_CODE_EXECUTION", cfg.TrustedCodeExecution,
	); err != nil {
		return Config{}, err
	}
	if cfg.OperatorCookieSecure, err = boolValue(
		"NEURUN_OPERATOR_COOKIE_SECURE", cfg.OperatorCookieSecure,
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
		{"NEURUN_OPERATOR_SESSION_TTL", &cfg.OperatorSessionTTL},
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
	if cfg.OperatorAccounts, err = parseOperatorAccounts(
		os.Getenv("NEURUN_OPERATOR_ACCOUNTS"), cfg.DefaultProjectID,
	); err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
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
	if c.APIKey == "" {
		problems = append(problems, errors.New("NEURUN_API_KEY is required"))
	} else if !strings.HasPrefix(c.APIKey, "neu_") || !strings.Contains(c.APIKey, ".") {
		problems = append(problems,
			errors.New("NEURUN_API_KEY must use the neu_<environment>_<prefix>.<secret> form"))
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
		{"NEURUN_OPERATOR_SESSION_TTL", c.OperatorSessionTTL},
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

func (c Config) OperatorSignInEnabled() bool {
	return len(c.OperatorAccounts) > 0
}

func parseOperatorAccounts(raw, projectID string) ([]operator.Account, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	entries := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ';' || r == '\n' || r == '\r'
	})
	accounts := make([]operator.Account, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		username, rest, found := strings.Cut(strings.TrimSpace(entry), ":")
		if !found {
			return nil, fmt.Errorf("NEURUN_OPERATOR_ACCOUNTS entry must use username:role:hash")
		}
		roleName, hash, found := strings.Cut(rest, ":")
		if !found {
			return nil, fmt.Errorf("NEURUN_OPERATOR_ACCOUNTS entry must use username:role:hash")
		}
		username = strings.ToLower(strings.TrimSpace(username))
		hash = strings.TrimSpace(hash)
		if username == "" {
			return nil, errors.New("NEURUN_OPERATOR_ACCOUNTS contains an empty username")
		}
		if _, duplicate := seen[username]; duplicate {
			return nil, fmt.Errorf("NEURUN_OPERATOR_ACCOUNTS defines %q more than once", username)
		}
		seen[username] = struct{}{}
		role, err := operator.ParseRole(roleName)
		if err != nil {
			return nil, fmt.Errorf("NEURUN_OPERATOR_ACCOUNTS entry for %q: %w", username, err)
		}
		if err := operator.ValidateHash(hash); err != nil {
			return nil, fmt.Errorf("NEURUN_OPERATOR_ACCOUNTS entry for %q: %w", username, err)
		}
		accounts = append(accounts, operator.Account{
			Username: username, Role: role, ProjectID: projectID, PasswordHash: hash,
		})
	}
	return accounts, nil
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
