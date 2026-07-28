package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadDefaultsAndOverrides(t *testing.T) {
	t.Setenv("NEURUN_API_KEY", "neu_live_test.secret")
	t.Setenv("NEURUN_HTTP_ADDR", "127.0.0.1:9090")
	t.Setenv("NEURUN_BLOCK_PRIVATE_NETWORKS", "false")
	t.Setenv("NEURUN_SHUTDOWN_TIMEOUT", "3s")
	t.Setenv("NEURUN_AGENT_CONCURRENCY", "3")
	t.Setenv("NEURUN_ALLOW_VOLATILE_JOBS", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != "127.0.0.1:9090" {
		t.Fatalf("HTTPAddr = %q", cfg.HTTPAddr)
	}
	if cfg.BlockPrivateNetworks {
		t.Fatal("private networks should be allowed by explicit override")
	}
	if cfg.ShutdownTimeout != 3*time.Second {
		t.Fatalf("ShutdownTimeout = %s", cfg.ShutdownTimeout)
	}
	if cfg.AgentConcurrency != 3 {
		t.Fatalf("AgentConcurrency = %d", cfg.AgentConcurrency)
	}
	if cfg.MaxResponseBytes != defaultResponseBytes {
		t.Fatalf("MaxResponseBytes = %d", cfg.MaxResponseBytes)
	}
	if !cfg.AllowVolatileJobs {
		t.Fatal("volatile jobs should be enabled by explicit override")
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	t.Setenv("NEURUN_API_KEY", "not-a-neurun-key")
	t.Setenv("NEURUN_PUBLIC_URL", "javascript:alert(1)")
	t.Setenv("NEURUN_AGENT_CONCURRENCY", "0")

	_, err := Load()
	if err == nil {
		t.Fatal("expected validation error")
	}
	for _, want := range []string{
		"NEURUN_API_KEY",
		"NEURUN_PUBLIC_URL",
		"NEURUN_AGENT_CONCURRENCY",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}

func TestLoadRejectsInvalidVolatileJobsFlag(t *testing.T) {
	t.Setenv("NEURUN_API_KEY", "neu_live_test.secret")
	t.Setenv("NEURUN_ALLOW_VOLATILE_JOBS", "sometimes")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "NEURUN_ALLOW_VOLATILE_JOBS") {
		t.Fatalf("Load() error = %v, want volatile jobs flag error", err)
	}
}
