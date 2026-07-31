package migrations

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

func TestCoreMigrationContainsExactlyTheSevenApplicationConcepts(t *testing.T) {
	t.Parallel()
	body, err := fs.ReadFile(FS, "000001_core.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(body)
	matches := regexp.MustCompile(`(?m)^CREATE TABLE ([a-z_]+)`).FindAllStringSubmatch(sql, -1)
	if len(matches) != 7 {
		t.Fatalf("application table count = %d, want 7: %v", len(matches), matches)
	}
	for _, table := range []string{
		"projects", "apps", "users", "api_keys", "deployments", "builds", "executions",
	} {
		if !strings.Contains(sql, "CREATE TABLE "+table) {
			t.Errorf("migration missing table %q", table)
		}
	}
	for _, retired := range []string{
		"operators", "operator_sessions", "functions", "jobs", "invocations",
		"workflows", "agents", "artifacts", "sessions", "audit_events",
	} {
		if strings.Contains(sql, "CREATE TABLE "+retired) {
			t.Errorf("migration retained retired table %q", retired)
		}
	}
}
