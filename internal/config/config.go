package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dagflows/neurun-io/internal/operator"
)

const (
	defaultHTTPAddr           = ":1267"
	defaultPublicURL          = "http://localhost:1267"
	defaultProjectID          = "prj_local"
	defaultArtifactDirectory  = "data/artifacts"
	defaultShutdownTimeout    = 10 * time.Second
	defaultRequestBodyBytes   = 1 << 20
	defaultResponseBytes      = 4 << 20
	defaultLeaseDuration      = 30 * time.Second
	defaultAgentConcurrency   = 8
	defaultOperatorSessionTTL = 12 * time.Hour
)

type Config struct {
	HTTPAddr             string
	PublicURL            string
	LogLevel             string
	APIKey               string
	DefaultProjectID     string
	ArtifactDirectory    string
	BlockPrivateNetworks bool
	ShutdownTimeout      time.Duration
	MaxRequestBodyBytes  int64
	MaxResponseBytes     int64
	JobLeaseDuration     time.Duration
	AgentConcurrency     int
	AllowVolatileJobs    bool

	// OperatorAccounts are the human logins for the dashboard. Empty means
	// operator sign-in is unavailable and only API-key access works.
	OperatorAccounts []operator.Account
	// OperatorSessionTTL is the absolute lifetime of an operator session.
	OperatorSessionTTL time.Duration
	// OperatorCookieSecure sets the Secure attribute on the session cookie. It
	// must stay true anywhere but a local plain-HTTP development server.
	OperatorCookieSecure bool
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:             value("NEURUN_HTTP_ADDR", defaultHTTPAddr),
		PublicURL:            value("NEURUN_PUBLIC_URL", defaultPublicURL),
		LogLevel:             value("NEURUN_LOG_LEVEL", "info"),
		APIKey:               strings.TrimSpace(os.Getenv("NEURUN_API_KEY")),
		DefaultProjectID:     value("NEURUN_DEFAULT_PROJECT_ID", defaultProjectID),
		ArtifactDirectory:    value("NEURUN_ARTIFACT_DIRECTORY", defaultArtifactDirectory),
		BlockPrivateNetworks: true,
		ShutdownTimeout:      defaultShutdownTimeout,
		MaxRequestBodyBytes:  defaultRequestBodyBytes,
		MaxResponseBytes:     defaultResponseBytes,
		JobLeaseDuration:     defaultLeaseDuration,
		AgentConcurrency:     defaultAgentConcurrency,
		AllowVolatileJobs:    false,
		OperatorSessionTTL:   defaultOperatorSessionTTL,
		OperatorCookieSecure: true,
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
	if cfg.OperatorSessionTTL, err = durationValue("NEURUN_OPERATOR_SESSION_TTL", cfg.OperatorSessionTTL); err != nil {
		return Config{}, err
	}
	if cfg.OperatorCookieSecure, err = boolValue("NEURUN_OPERATOR_COOKIE_SECURE", cfg.OperatorCookieSecure); err != nil {
		return Config{}, err
	}
	if cfg.OperatorAccounts, err = parseOperatorAccounts(
		os.Getenv("NEURUN_OPERATOR_ACCOUNTS"),
		cfg.DefaultProjectID,
	); err != nil {
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
	if strings.TrimSpace(c.ArtifactDirectory) == "" {
		problems = append(problems, errors.New("NEURUN_ARTIFACT_DIRECTORY is required"))
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
	if c.OperatorSessionTTL <= 0 {
		problems = append(problems, errors.New("NEURUN_OPERATOR_SESSION_TTL must be positive"))
	}
	return errors.Join(problems...)
}

// OperatorSignInEnabled reports whether any human login is configured.
func (c Config) OperatorSignInEnabled() bool {
	return len(c.OperatorAccounts) > 0
}

// parseOperatorAccounts reads NEURUN_OPERATOR_ACCOUNTS.
//
// Format is `username:role:hash`, with entries separated by `;` or newlines:
//
//	daniel:admin:pbkdf2-sha256$650000$c2FsdA$a2V5;view:viewer:pbkdf2-sha256$...
//
// `:` is safe as the field separator because the encoded hash uses `$` plus
// unpadded base64, whose alphabet contains no colon. Generate a hash with
// `neurun hash-password`.
//
// Accounts are validated here so a typo fails at startup rather than at an
// operator's first sign-in attempt.
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
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		username, rest, found := strings.Cut(entry, ":")
		if !found {
			return nil, fmt.Errorf(
				"NEURUN_OPERATOR_ACCOUNTS entry %q must use username:role:hash form", entry)
		}
		roleName, hash, found := strings.Cut(rest, ":")
		if !found {
			return nil, fmt.Errorf(
				"NEURUN_OPERATOR_ACCOUNTS entry for %q must use username:role:hash form", username)
		}

		username = strings.ToLower(strings.TrimSpace(username))
		hash = strings.TrimSpace(hash)
		if username == "" {
			return nil, errors.New("NEURUN_OPERATOR_ACCOUNTS contains an entry with an empty username")
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
			return nil, fmt.Errorf(
				"NEURUN_OPERATOR_ACCOUNTS entry for %q: %w (generate one with `neurun hash-password`)",
				username, err)
		}

		accounts = append(accounts, operator.Account{
			Username:     username,
			Role:         role,
			ProjectID:    projectID,
			PasswordHash: hash,
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
