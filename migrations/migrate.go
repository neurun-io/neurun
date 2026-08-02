package migrations

import (
	"context"
	"errors"
	"fmt"
	"net/url"
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
	defer func() {
		if sourceErr, databaseErr := migrator.Close(); err == nil {
			err = errors.Join(sourceErr, databaseErr)
		}
	}()
	if err := migrator.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
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
