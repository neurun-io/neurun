package config

import (
	"strings"
	"testing"
)

// Configuration carries no credentials at all: accounts are made with
// `neurun user create`, so loading a minimal environment must succeed without
// any secret being set.
func TestLoadMinimalConfiguration(t *testing.T) {
	t.Setenv("NEURUN_DATA_DIRECTORY", t.TempDir())
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxRunInputBytes <= 0 || cfg.OperatorCookieSecure ||
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
