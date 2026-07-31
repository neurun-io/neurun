package config

import (
	"strings"
	"testing"
)

func TestLoadMinimalConfiguration(t *testing.T) {
	t.Setenv("NEURUN_API_KEY", "neu_test.local-secret")
	t.Setenv("NEURUN_DATA_DIRECTORY", t.TempDir())
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultProjectID != "prj_local" || cfg.MaxRunInputBytes <= 0 ||
		cfg.OperatorCookieSecure {
		t.Fatalf("configuration = %#v", cfg)
	}
}

func TestValidateRejectsInvalidRuntimeLimits(t *testing.T) {
	t.Setenv("NEURUN_API_KEY", "neu_test.local-secret")
	t.Setenv("NEURUN_EXECUTION_TIMEOUT", "0s")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "NEURUN_EXECUTION_TIMEOUT") {
		t.Fatalf("error = %v", err)
	}
}
