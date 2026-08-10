package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadEnvFile(t *testing.T) {
	t.Cleanup(func() {
		os.Unsetenv("NEURUN_CONFIG_FILE_VALUE")
		os.Unsetenv("NEURUN_CONFIG_QUOTED_VALUE")
	})

	path := filepath.Join(t.TempDir(), ".env")
	contents := "NEURUN_CONFIG_FILE_VALUE=from-file\n" +
		"NEURUN_CONFIG_QUOTED_VALUE=\"with spaces\"\n" +
		"NEURUN_CONFIG_PROCESS_VALUE=from-file\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NEURUN_CONFIG_PROCESS_VALUE", "from-process")

	if err := loadEnvFile(path); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("NEURUN_CONFIG_FILE_VALUE"); got != "from-file" {
		t.Fatalf("file value = %q", got)
	}
	if got := os.Getenv("NEURUN_CONFIG_QUOTED_VALUE"); got != "with spaces" {
		t.Fatalf("quoted value = %q", got)
	}
	if got := os.Getenv("NEURUN_CONFIG_PROCESS_VALUE"); got != "from-process" {
		t.Fatalf("process value = %q", got)
	}
}

func TestLoadEnvFileRejectsMalformedLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("NEURUN_CONFIG_MALFORMED\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := loadEnvFile(path)
	if err == nil || !strings.Contains(err.Error(), "expected KEY=VALUE") {
		t.Fatalf("error = %v", err)
	}
}

// Configuration carries no credentials at all: accounts are made by registering
// through the API, so loading a minimal environment must succeed without any
// secret being set.
func TestLoadMinimalConfiguration(t *testing.T) {
	t.Setenv("NEURUN_DATA_DIRECTORY", t.TempDir())
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxRunInputBytes <= 0 || cfg.SessionCookieSecure ||
		cfg.DatabaseSchema == "" {
		t.Fatalf("configuration = %#v", cfg)
	}
}

func TestValidateRejectsInvalidRuntimeLimits(t *testing.T) {
	t.Setenv("NEURUN_EXECUTION_TIMEOUT", "0s")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "NEURUN_EXECUTION_TIMEOUT") {
		t.Fatalf("error = %v", err)
	}
}
