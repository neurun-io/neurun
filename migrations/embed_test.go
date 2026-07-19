package migrations

import (
	"io/fs"
	"strings"
	"testing"
)

func TestCoreMigrationIsEmbedded(t *testing.T) {
	t.Parallel()

	body, err := fs.ReadFile(FS, "000001_core.sql")
	if err != nil {
		t.Fatal(err)
	}

	required := []string{
		"CREATE TABLE projects",
		"CREATE TABLE jobs",
		"CREATE TABLE job_attempts",
		"CREATE TABLE function_versions",
		"CREATE TABLE function_invocations",
		"CREATE TABLE outbox",
		"CREATE TABLE sessions",
		"CREATE TABLE session_usage_samples",
		"CREATE TABLE artifacts",
		"CREATE TABLE audit_events",
	}
	for _, fragment := range required {
		if !strings.Contains(string(body), fragment) {
			t.Errorf("migration missing %q", fragment)
		}
	}
}
