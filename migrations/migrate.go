package migrations

import (
	"errors"
	"fmt"
	"net/url"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

func Apply(databaseURL string) (err error) {
	source, err := iofs.New(FS, ".")
	if err != nil {
		return fmt.Errorf("read embedded migrations: %w", err)
	}
	target, err := pgxURL(databaseURL)
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

func pgxURL(databaseURL string) (string, error) {
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
	return parsed.String(), nil
}
