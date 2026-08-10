package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

	"github.com/neurun-io/neurun/internal/api"
	"github.com/neurun-io/neurun/internal/artifact"
	"github.com/neurun-io/neurun/internal/builder"
	"github.com/neurun-io/neurun/internal/buildinfo"
	"github.com/neurun-io/neurun/internal/config"
	"github.com/neurun-io/neurun/internal/github"
	"github.com/neurun-io/neurun/internal/repository"
	"github.com/neurun-io/neurun/internal/repository/storage"
	"github.com/neurun-io/neurun/internal/service"
	"github.com/neurun-io/neurun/internal/worker"
	"github.com/neurun-io/neurun/migrations"
)

func main() {
	if err := run(); err != nil {
		slog.Error("neurun stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
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
			return fmt.Errorf(
				"unknown command %q (expected serve, doctor, or version)",
				os.Args[1],
			)
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

	projects, err := repository.NewProjectRepository(pool)
	if err != nil {
		return err
	}
	apps, err := repository.NewAppRepository(pool)
	if err != nil {
		return err
	}
	deployments, err := repository.NewDeploymentRepository(pool)
	if err != nil {
		return err
	}
	executions, err := repository.NewExecutionRepository(pool)
	if err != nil {
		return err
	}
	users, err := repository.NewUserRepository(pool)
	if err != nil {
		return err
	}
	organizations, err := repository.NewOrganizationRepository(pool)
	if err != nil {
		return err
	}
	apiKeys, err := repository.NewAPIKeyRepository(pool)
	if err != nil {
		return err
	}
	cache := repository.NewCacheRepository("neurun")
	defer cache.Close()
	sessions, err := repository.NewSessionRepository(cache, users)
	if err != nil {
		return err
	}

	blobStore, err := artifact.NewLocalStore(filepath.Join(cfg.DataDirectory, "blobs"))
	if err != nil {
		return fmt.Errorf("configure artifact storage: %w", err)
	}
	pythonBuilder, err := builder.NewPython(builder.PythonOptions{
		MaxArchiveEntries:       cfg.MaxDeploymentArchiveEntries,
		MaxArchiveExpandedBytes: cfg.MaxDeploymentExpandedBytes,
		PythonExecutable:        cfg.PythonExecutable,
	})
	if err != nil {
		return fmt.Errorf("configure Python builder: %w", err)
	}

	deploymentService, err := service.NewDeploymentService(
		projects, apps, deployments, blobStore, pythonBuilder,
		service.DeploymentOptions{
			MaxSourceBytes:   cfg.MaxDeploymentSourceBytes,
			MaxArtifactBytes: cfg.MaxDeploymentArtifactBytes,
			BuildTimeout:     cfg.DeploymentBuildTimeout,
		},
	)
	if err != nil {
		return fmt.Errorf("configure deployment service: %w", err)
	}
	executionService, err := service.NewExecutionService(
		executions, deployments,
		service.ExecutionOptions{MaxInputBytes: cfg.MaxRunInputBytes},
	)
	if err != nil {
		return fmt.Errorf("configure execution service: %w", err)
	}
	accountService, err := service.NewAccountService(users, apiKeys, nil, nil)
	if err != nil {
		return fmt.Errorf("configure account service: %w", err)
	}
	sessionService, err := service.NewSessionService(
		users, organizations, sessions, cfg.SessionTTL, nil,
	)
	if err != nil {
		return fmt.Errorf("configure sign-in: %w", err)
	}
	organizationService, err := service.NewOrganizationService(
		organizations, users, nil, nil,
	)
	if err != nil {
		return fmt.Errorf("configure organizations: %w", err)
	}
	installations, err := repository.NewGitHubInstallationRepository(pool)
	if err != nil {
		return err
	}
	// Absent credentials are not a failure: the GitHub routes refuse with a
	// documented error and uploads keep working.
	var gitHubClient *github.Client
	if cfg.GitHubAppID != 0 && len(cfg.GitHubPrivateKey) > 0 {
		gitHubClient, err = github.New(github.Options{
			AppID:      cfg.GitHubAppID,
			PrivateKey: cfg.GitHubPrivateKey,
			Limits: github.Limits{
				MaxArchiveBytes:   cfg.MaxDeploymentSourceBytes,
				MaxArchiveEntries: cfg.MaxDeploymentArchiveEntries,
			},
		})
		if err != nil {
			return fmt.Errorf("configure GitHub app: %w", err)
		}
		logger.Info("github app configured", "app_id", cfg.GitHubAppID)
	} else {
		logger.Warn("github app is not configured; repository deployments are unavailable")
	}
	gitHubService, err := service.NewGitHubService(
		gitHubClient, installations, apps, deploymentService, nil, nil,
	)
	if err != nil {
		return fmt.Errorf("configure GitHub deployments: %w", err)
	}

	recoveredBuilds, err := deploymentService.RecoverInterruptedBuilds(ctx)
	if err != nil {
		return fmt.Errorf("recover interrupted builds: %w", err)
	}
	if recoveredBuilds > 0 {
		logger.Warn("marked interrupted builds failed", "builds", recoveredBuilds)
	}

	pythonRunner, err := worker.NewPythonRunner(
		worker.PythonOptions{Executable: cfg.PythonExecutable},
	)
	if err != nil {
		return fmt.Errorf("configure Python runner: %w", err)
	}
	executor, err := worker.New(
		executions, deployments, blobStore, pythonRunner,
		worker.Options{
			PollInterval: cfg.WorkerPollInterval, RunTimeout: cfg.RunTimeout,
			MaxResultBytes: cfg.MaxRunResultBytes, MaxLogBytes: cfg.MaxRunLogBytes,
			MaxArtifactBytes:        cfg.MaxDeploymentArtifactBytes,
			MaxArchiveEntries:       cfg.MaxDeploymentArchiveEntries,
			MaxArchiveExpandedBytes: cfg.MaxDeploymentExpandedBytes,
		},
	)
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

	controlAPI, err := api.NewServer(api.ServerOptions{
		Deployments:   deploymentService,
		Executions:    executionService,
		Accounts:      accountService,
		Sessions:      sessionService,
		Organizations: organizationService,
		GitHub:        gitHubService,
		Ready: func(readyCtx context.Context) error {
			return errors.Join(
				pool.Ping(readyCtx), blobStore.Check(readyCtx),
				deployments.Check(readyCtx),
			)
		},
		MaximumBodyBytes:       cfg.MaxRequestBodyBytes,
		MaximumDeploymentBytes: cfg.MaxDeploymentSourceBytes,
		SessionCookieSecure:    cfg.SessionCookieSecure,
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
		pruneSessions(runtimeCtx, sessionService, logger)
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

func pruneSessions(
	ctx context.Context,
	sessions *service.SessionService,
	logger *slog.Logger,
) {
	if sessions == nil {
		<-ctx.Done()
		return
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		if _, err := sessions.PruneSessions(ctx); err != nil &&
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
