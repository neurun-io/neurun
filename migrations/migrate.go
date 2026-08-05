package migrations

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"
)

const schemaTimeout = 10 * time.Second

func Apply(databaseURL, schema string) (err error) {
	if err := ensureSchema(databaseURL, schema); err != nil {
		return err
	}
	source, err := iofs.New(FS, ".")
	if err != nil {
		return fmt.Errorf("read embedded migrations: %w", err)
	}
	target, err := pgxURL(databaseURL, schema)
	if err != nil {
		return err
	}
	migrator, err := migrate.NewWithSourceInstance("iofs", source, target)
	if err != nil {
		return fmt.Errorf("open migration target: %w", err)
	}
	migrator.Log = stepLogger{}
	defer func() {
		if sourceErr, databaseErr := migrator.Close(); err == nil {
			err = errors.Join(sourceErr, databaseErr)
		}
	}()

	before, dirty, err := migrator.Version()
	if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
		return fmt.Errorf("read schema version: %w", err)
	}
	slog.Info("migrating schema", "schema", schema, "version", version(before), "dirty", dirty)

	switch err := migrator.Up(); {
	case errors.Is(err, migrate.ErrNoChange):
		slog.Info("schema already current", "schema", schema, "version", version(before))
	case err != nil:
		return fmt.Errorf("apply migrations: %w", err)
	default:
		after, _, _ := migrator.Version()
		slog.Info("schema migrated",
			"schema", schema, "from", version(before), "to", version(after))
	}
	return nil
}

// stepLogger routes golang-migrate's per-migration output into slog, so each
// version that runs is named rather than the whole batch reporting once.
type stepLogger struct{}

func (stepLogger) Printf(format string, arguments ...any) {
	slog.Info("migration " + strings.TrimSpace(fmt.Sprintf(format, arguments...)))
}

func (stepLogger) Verbose() bool { return true }

func version(value uint) string {
	if value == 0 {
		return "none"
	}
	return strconv.FormatUint(uint64(value), 10)
}

func ensureSchema(databaseURL, schema string) error {
	if schema == "" {
		return errors.New("database schema is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), schemaTimeout)
	defer cancel()

	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("connect to create schema: %w", err)
	}
	defer conn.Close(ctx)

	name := pgx.Identifier{schema}.Sanitize()
	if _, err := conn.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS "+name); err != nil {
		return fmt.Errorf("create schema %s: %w", name, err)
	}
	return nil
}

func pgxURL(databaseURL, schema string) (string, error) {
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return "", fmt.Errorf("parse database URL: %w", err)
	}
	switch parsed.Scheme {
	case "postgres", "postgresql", "pgx", "pgx5":
		parsed.Scheme = "pgx5"
	default:
		return "", fmt.Errorf("unsupported database URL scheme %q", parsed.Scheme)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}
