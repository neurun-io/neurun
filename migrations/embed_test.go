package migrations

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

func upMigrations(t *testing.T) map[string]string {
	t.Helper()

	names, err := fs.Glob(FS, "*.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if len(names) == 0 {
		t.Fatal("no up migrations are embedded")
	}
	bodies := make(map[string]string, len(names))
	for _, name := range names {
		body, err := fs.ReadFile(FS, name)
		if err != nil {
			t.Fatal(err)
		}
		bodies[name] = string(body)
	}
	return bodies
}

func TestMigrationsCreateExactlyTheApplicationTables(t *testing.T) {
	t.Parallel()

	var combined strings.Builder
	for _, body := range upMigrations(t) {
		combined.WriteString(body)
	}
	sql := combined.String()

	matches := regexp.MustCompile(`(?m)^CREATE TABLE ([a-z_]+)`).FindAllStringSubmatch(sql, -1)
	if len(matches) != 10 {
		t.Fatalf("application table count = %d, want 10: %v", len(matches), matches)
	}
	for _, table := range []string{
		"users", "organizations", "organization_members", "organization_invites",
		"projects", "apps", "api_keys", "deployments", "builds", "executions",
	} {
		if !strings.Contains(sql, "CREATE TABLE "+table) {
			t.Errorf("migrations missing table %q", table)
		}
	}
	for _, retired := range []string{
		"operators", "operator_sessions", "functions", "jobs", "invocations",
		"workflows", "agents", "artifacts", "sessions", "audit_events",
	} {
		if strings.Contains(sql, "CREATE TABLE "+retired) {
			t.Errorf("migrations retained retired table %q", retired)
		}
	}
}

func TestEachMigrationCreatesOneTable(t *testing.T) {
	t.Parallel()

	pattern := regexp.MustCompile(`(?m)^CREATE TABLE ([a-z_]+)`)
	for name, body := range upMigrations(t) {
		if matches := pattern.FindAllStringSubmatch(body, -1); len(matches) != 1 {
			t.Errorf("%s creates %d tables, want 1: %v", name, len(matches), matches)
		}
	}
}

func TestEveryUpMigrationHasADownMigration(t *testing.T) {
	t.Parallel()

	for name := range upMigrations(t) {
		down := strings.TrimSuffix(name, ".up.sql") + ".down.sql"
		if _, err := fs.ReadFile(FS, down); err != nil {
			t.Errorf("%s has no matching down migration: %v", name, err)
		}
	}
}

// Transaction control belongs to golang-migrate, which wraps each migration in
// one. An explicit BEGIN would nest inside it.
func TestMigrationsDoNotManageTheirOwnTransactions(t *testing.T) {
	t.Parallel()

	for name, body := range upMigrations(t) {
		upper := strings.ToUpper(body)
		if strings.Contains(upper, "BEGIN;") || strings.Contains(upper, "COMMIT;") {
			t.Errorf("%s manages its own transaction", name)
		}
	}
}
