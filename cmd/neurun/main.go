package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/neurun-io/neurun/internal/domain/account"
	"github.com/neurun-io/neurun/internal/api"
	"github.com/neurun-io/neurun/internal/artifact"
	"github.com/neurun-io/neurun/internal/domain/builder"
	"github.com/neurun-io/neurun/internal/buildinfo"
	"github.com/neurun-io/neurun/internal/config"
	"github.com/neurun-io/neurun/internal/domain/deployment"
	"github.com/neurun-io/neurun/internal/domain/operator"
	"github.com/neurun-io/neurun/internal/domain/worker"
	"github.com/neurun-io/neurun/migrations"
)

func main() {
	if err := run(); err != nil {
		slog.Error("neurun stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) > 1 && os.Args[1] == "hash-password" {
		return hashPassword(os.Args[2:])
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "doctor":
			return doctor(cfg)
		case "version":
			return json.NewEncoder(os.Stdout).Encode(buildinfo.Current())
		case "serve":
		default:
			return fmt.Errorf("unknown command %q (expected serve, doctor, version, or hash-password)", os.Args[1])
		}
	}
	level := slog.LevelInfo
	switch cfg.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return serve(ctx, cfg, logger)
}

func serve(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	if !cfg.TrustedCodeExecution {
		return errors.New("local Python execution is disabled; set NEURUN_TRUSTED_CODE_EXECUTION=true only when uploaded code is trusted")
	}
	if err := migrations.Apply(cfg.DatabaseURL, cfg.DatabaseSchema); err != nil {
		return err
	}
	dsn, err := cfg.DatabaseDSN()
	if err != nil {
		return err
	}
	database, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open PostgreSQL: %w", err)
	}
	defer database.Close()
	database.SetMaxOpenConns(cfg.DatabaseMaxConns)
	database.SetMaxIdleConns(cfg.DatabaseMaxConns)
	database.SetConnMaxLifetime(cfg.DatabaseConnMaxLifetime)
	database.SetConnMaxIdleTime(cfg.DatabaseConnMaxIdleTime)
	metadataStore, err := deployment.NewPostgresStore(database)
	if err != nil {
		return fmt.Errorf("configure PostgreSQL metadata: %w", err)
	}
	accounts, err := account.NewStore(database)
	if err != nil {
		return fmt.Errorf("configure account storage: %w", err)
	}
	blobStore, err := artifact.NewLocalStore(filepath.Join(cfg.DataDirectory, "blobs"))
	if err != nil {
		return fmt.Errorf("configure artifact storage: %w", err)
	}
	deploymentBuilder, err := builder.NewPython(builder.PythonOptions{
		MaxArchiveEntries:       cfg.MaxDeploymentArchiveEntries,
		MaxArchiveExpandedBytes: cfg.MaxDeploymentExpandedBytes,
		PythonExecutable:        cfg.PythonExecutable,
	})
	if err != nil {
		return fmt.Errorf("configure Python builder: %w", err)
	}
	deployments, err := deployment.NewService(metadataStore, blobStore, deploymentBuilder,
		deployment.ServiceOptions{
			MaxSourceBytes:   cfg.MaxDeploymentSourceBytes,
			MaxArtifactBytes: cfg.MaxDeploymentArtifactBytes,
			MaxRunInputBytes: cfg.MaxRunInputBytes,
			BuildTimeout:     cfg.DeploymentBuildTimeout,
		})
	if err != nil {
		return fmt.Errorf("configure deployment service: %w", err)
	}
	if _, err := deployments.EnsureProject(
		ctx, cfg.DefaultProjectID, cfg.DefaultProjectID,
	); err != nil {
		return fmt.Errorf("bootstrap default project: %w", err)
	}
	if err := accounts.EnsureConfiguredKey(
		ctx, cfg.DefaultProjectID, cfg.APIKey, []string{"*"},
	); err != nil {
		return fmt.Errorf("bootstrap configured API key: %w", err)
	}
	for _, configured := range cfg.OperatorAccounts {
		if err := accounts.EnsureConfiguredUser(ctx, configured); err != nil {
			return fmt.Errorf("bootstrap configured user %q: %w", configured.Username, err)
		}
	}
	recoveredBuilds, err := deployments.RecoverInterruptedBuilds(ctx)
	if err != nil {
		return fmt.Errorf("recover interrupted builds: %w", err)
	}
	if recoveredBuilds > 0 {
		logger.Warn("marked interrupted builds failed", "builds", recoveredBuilds)
	}
	pythonRunner, err := worker.NewPythonRunner(worker.PythonOptions{Executable: cfg.PythonExecutable})
	if err != nil {
		return fmt.Errorf("configure Python runner: %w", err)
	}
	executor, err := worker.New(metadataStore, blobStore, pythonRunner, worker.Options{
		PollInterval: cfg.WorkerPollInterval, RunTimeout: cfg.RunTimeout,
		MaxResultBytes: cfg.MaxRunResultBytes, MaxLogBytes: cfg.MaxRunLogBytes,
		MaxArtifactBytes:        cfg.MaxDeploymentArtifactBytes,
		MaxArchiveEntries:       cfg.MaxDeploymentArchiveEntries,
		MaxArchiveExpandedBytes: cfg.MaxDeploymentExpandedBytes,
	})
	if err != nil {
		return fmt.Errorf("configure execution worker: %w", err)
	}
	recoveredExecutions, err := executor.Recover(ctx)
	if err != nil {
		return fmt.Errorf("recover interrupted executions: %w", err)
	}
	if recoveredExecutions > 0 {
		logger.Warn("marked interrupted executions failed", "executions", recoveredExecutions)
	}
	operators, err := operatorAuthenticator(cfg, accounts)
	if err != nil {
		return err
	}
	controlAPI, err := api.NewServer(api.ServerOptions{
		Authenticator: accounts, Accounts: accounts, Deployments: deployments,
		Operators: operators, OperatorCookieSecure: cfg.OperatorCookieSecure,
		Ready: func(readyCtx context.Context) error {
			return errors.Join(
				database.PingContext(readyCtx), blobStore.Check(readyCtx),
				metadataStore.Check(readyCtx),
			)
		},
		MaximumBodyBytes:       cfg.MaxRequestBodyBytes,
		MaximumDeploymentBytes: cfg.MaxDeploymentSourceBytes,
	})
	if err != nil {
		return fmt.Errorf("configure control API: %w", err)
	}
	runtimeCtx, stopRuntime := context.WithCancel(ctx)
	defer stopRuntime()
	server := &http.Server{
		Addr: cfg.HTTPAddr, Handler: controlAPI,
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second,
		WriteTimeout: maximumDuration(30*time.Second, cfg.DeploymentBuildTimeout+10*time.Second),
		IdleTimeout:  60 * time.Second, MaxHeaderBytes: 32 << 10,
		BaseContext: func(net.Listener) context.Context { return runtimeCtx },
	}
	errs := make(chan error, 2)
	var background sync.WaitGroup
	background.Add(2)
	go func() {
		defer background.Done()
		if err := executor.Run(runtimeCtx); err != nil && !errors.Is(err, context.Canceled) {
			sendRuntimeError(errs, fmt.Errorf("execution worker: %w", err))
		}
	}()
	go func() {
		defer background.Done()
		pruneSessions(runtimeCtx, operators, logger)
	}()
	go func() {
		logger.Info("neurun listening", "address", cfg.HTTPAddr,
			"version", buildinfo.Version, "runtime", "python")
		errs <- server.ListenAndServe()
	}()
	var serveErr error
	select {
	case err := <-errs:
		if !errors.Is(err, http.ErrServerClosed) {
			serveErr = err
		}
	case <-ctx.Done():
	}
	stopRuntime()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	shutdownErr := server.Shutdown(shutdownCtx)
	cancel()
	if shutdownErr != nil {
		shutdownErr = errors.Join(shutdownErr, server.Close())
	}
	background.Wait()
	return errors.Join(serveErr, shutdownErr)
}

func operatorAuthenticator(
	cfg config.Config, accounts *account.Store,
) (*operator.Authenticator, error) {
	store, err := account.NewOperatorStore(accounts)
	if err != nil {
		return nil, fmt.Errorf("configure operator storage: %w", err)
	}
	authenticator, err := operator.NewAuthenticator(store, cfg.OperatorSessionTTL)
	if err != nil {
		return nil, fmt.Errorf("configure operator sign-in: %w", err)
	}
	return authenticator, nil
}

func pruneSessions(ctx context.Context, authenticator *operator.Authenticator, logger *slog.Logger) {
	if authenticator == nil {
		<-ctx.Done()
		return
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		if _, err := authenticator.PruneSessions(ctx); err != nil &&
			!errors.Is(err, context.Canceled) {
			logger.Error("prune expired sessions", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func hashPassword(args []string) error {
	if len(args) != 0 {
		return errors.New("hash-password takes no arguments; pipe the password on stdin")
	}
	password, err := bufio.NewReader(io.LimitReader(os.Stdin, 4097)).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	password = strings.TrimRight(password, "\r\n")
	if err := operator.ValidatePassword(password); err != nil {
		return err
	}
	hash, err := operator.HashPassword(password)
	if err != nil {
		return err
	}
	fmt.Println(hash)
	return nil
}

func doctor(cfg config.Config) error {
	base, err := url.Parse(cfg.PublicURL)
	if err != nil {
		return err
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/healthz"
	base.RawQuery, base.Fragment = "", ""
	response, err := (&http.Client{Timeout: 3 * time.Second}).Get(base.String())
	if err != nil {
		return fmt.Errorf("health check: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned %s", response.Status)
	}
	return nil
}

func maximumDuration(left, right time.Duration) time.Duration {
	if left > right {
		return left
	}
	return right
}

func sendRuntimeError(destination chan<- error, err error) {
	select {
	case destination <- err:
	default:
	}
}
