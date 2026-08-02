package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/neurun-io/neurun/internal/config"
	"github.com/neurun-io/neurun/internal/dto"
	"github.com/neurun-io/neurun/internal/repository"
	"github.com/neurun-io/neurun/internal/repository/storage"
	"github.com/neurun-io/neurun/internal/service"
	"github.com/neurun-io/neurun/migrations"
)

// createUser is the only way an account comes into being.
//
// The server never creates one. A service that invents an administrator on boot
// leaves behind a credential nobody asked for and makes a fresh install
// indistinguishable from a misconfigured one. Everything after the first
// account — more users, keys, projects, apps — goes through the API.
func createUser(ctx context.Context, cfg config.Config, args []string) error {
	if len(args) < 1 || len(args) > 2 {
		return errors.New("usage: neurun user create <username> [admin|operator|viewer]")
	}
	role := "admin"
	if len(args) == 2 {
		role = args[1]
	}
	password, err := readPassword(os.Stdin, args[0])
	if err != nil {
		return err
	}

	if err := migrations.Apply(cfg.DatabaseURL, cfg.DatabaseSchema); err != nil {
		return err
	}
	dsn, err := cfg.DatabaseDSN()
	if err != nil {
		return err
	}
	pool, err := storage.PostgresConnect(ctx, storage.PostgresConfig{
		DSN:             dsn,
		MaxConns:        cfg.DatabaseMaxConns,
		ConnMaxLifetime: cfg.DatabaseConnMaxLifetime,
		ConnMaxIdleTime: cfg.DatabaseConnMaxIdleTime,
	})
	if err != nil {
		return err
	}
	defer pool.Close()

	users, err := repository.NewUserRepository(pool)
	if err != nil {
		return err
	}
	keys, err := repository.NewAPIKeyRepository(pool)
	if err != nil {
		return err
	}
	accounts, err := service.NewAccountService(users, keys, nil, nil)
	if err != nil {
		return err
	}
	record, err := accounts.CreateUser(ctx, dto.CreateUserRequest{
		Username: args[0], DisplayName: args[0],
		Role: role, Password: password,
	})
	if err != nil {
		return err
	}
	fmt.Printf("%s\t%s\t%s\n", record.ID, record.Username, record.Role)
	return nil
}

// readPassword takes the password from stdin so it never reaches a process
// listing or shell history.
func readPassword(input *os.File, username string) (string, error) {
	if stat, err := input.Stat(); err == nil && stat.Mode()&os.ModeCharDevice != 0 {
		fmt.Fprintf(os.Stderr,
			"Password for %s (at least 12 characters, input is visible): ", username)
	}
	line, err := bufio.NewReader(io.LimitReader(input, 4096)).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	password := strings.TrimRight(line, "\r\n")
	if password == "" {
		return "", errors.New("no password supplied")
	}
	return password, nil
}
