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
	"github.com/neurun-io/neurun/internal/browsergrpc"
	"github.com/neurun-io/neurun/internal/builder"
	"github.com/neurun-io/neurun/internal/buildinfo"
	"github.com/neurun-io/neurun/internal/config"
	"github.com/neurun-io/neurun/internal/domain/deployment"
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

// buildCacheDirectory is where toolchain caches persist between builds. It
// defaults inside the data directory rather than off: a cold cargo or npm build
// is minutes that the build timeout has to cover.
func buildCacheDirectory(cfg config.Config) string {
	if cfg.BuildCacheDirectory != "" {
		return cfg.BuildCacheDirectory
	}
	return filepath.Join(cfg.DataDirectory, "build-cache")
}

// sessionCache holds issued sessions.
//
// Redis is required, not preferred. Sessions in process die with it, so every
// restart signs everybody out and a second replica sees none of them — which is
// a broken control plane, not a degraded one. It fails here the same way a
// missing database does.
func sessionCache(cfg config.Config, logger *slog.Logger) (repository.Cache, error) {
	cache, err := repository.NewRedisCache(cfg.RedisURL, "neurun")
	if err != nil {
		return nil, err
	}
	probe, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := cache.Check(probe); err != nil {
		cache.Close()
		return nil, err
	}
	logger.Info("sessions are cached in redis")
	return cache, nil
}

// artifactStore builds the payload store the whole plane shares.
//
// Local is the appliance shape: one host owning its own disk. S3 is the hosted
// one, and it is always wrapped in a read-through cache, because the worker
// materializes a build's layers on every execution and that would otherwise be a
// download per run.
func artifactStore(cfg config.Config, logger *slog.Logger) (artifact.BlobStore, error) {
	if cfg.ArtifactStore != "s3" {
		logger.Info("artifact storage is local", "directory", cfg.DataDirectory)
		return artifact.NewLocalStore(filepath.Join(cfg.DataDirectory, "blobs"))
	}
	remote, err := artifact.NewS3Store(artifact.S3Options{
		Bucket:    cfg.S3Bucket,
		Endpoint:  cfg.S3Endpoint,
		Region:    cfg.S3Region,
		AccessKey: cfg.S3AccessKeyID,
		SecretKey: cfg.S3SecretAccessKey,
		PathStyle: cfg.S3PathStyle,
	})
	if err != nil {
		return nil, err
	}
	cached, err := artifact.NewCacheStore(remote, artifact.CacheOptions{
		Directory: filepath.Join(cfg.DataDirectory, "cache"),
		MaxBytes:  cfg.ArtifactCacheBytes,
	})
	if err != nil {
		return nil, err
	}
	logger.Info("artifact storage is s3",
		"bucket", cfg.S3Bucket, "cache_bytes", cfg.ArtifactCacheBytes,
	)
	return cached, nil
}

// storeReady asks a store whether it is reachable, when it can answer.
func storeReady(ctx context.Context, store any) error {
	checker, ok := store.(interface{ Check(context.Context) error })
	if !ok {
		return nil
	}
	return checker.Check(ctx)
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
	cache, err := sessionCache(cfg, logger)
	if err != nil {
		return fmt.Errorf("configure session cache: %w", err)
	}
	defer cache.Close()
	sessions, err := repository.NewSessionRepository(cache, users)
	if err != nil {
		return err
	}

	blobStore, err := artifactStore(cfg, logger)
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
	rustBuilder, err := builder.NewRust(builder.RustOptions{
		MaxArchiveEntries:       cfg.MaxDeploymentArchiveEntries,
		MaxArchiveExpandedBytes: cfg.MaxDeploymentExpandedBytes,
		CargoExecutable:         cfg.CargoExecutable,
	})
	if err != nil {
		return fmt.Errorf("configure Rust builder: %w", err)
	}
	goBuilder, err := builder.NewGo(builder.GoOptions{
		MaxArchiveEntries:       cfg.MaxDeploymentArchiveEntries,
		MaxArchiveExpandedBytes: cfg.MaxDeploymentExpandedBytes,
		GoExecutable:            cfg.GoExecutable,
	})
	if err != nil {
		return fmt.Errorf("configure Go builder: %w", err)
	}
	rubyBuilder, err := builder.NewRuby(builder.RubyOptions{
		MaxArchiveEntries:       cfg.MaxDeploymentArchiveEntries,
		MaxArchiveExpandedBytes: cfg.MaxDeploymentExpandedBytes,
		BundleExecutable:        cfg.BundleExecutable,
	})
	if err != nil {
		return fmt.Errorf("configure Ruby builder: %w", err)
	}
	nodeBuilder, err := builder.NewNode(builder.NodeOptions{
		MaxArchiveEntries:       cfg.MaxDeploymentArchiveEntries,
		MaxArchiveExpandedBytes: cfg.MaxDeploymentExpandedBytes,
		NPMExecutable:           cfg.NPMExecutable,
	})
	if err != nil {
		return fmt.Errorf("configure Node builder: %w", err)
	}
	// A runtime is available when its toolchain is in the image. Nothing here
	// checks that; a missing toolchain fails the build it was asked for.
	toolchains := map[deployment.Runtime]builder.Builder{
		deployment.RuntimePython: pythonBuilder,
		deployment.RuntimeRust:   rustBuilder,
		deployment.RuntimeGo:     goBuilder,
		deployment.RuntimeRuby:   rubyBuilder,
		deployment.RuntimeNode:   nodeBuilder,
	}

	deploymentService, err := service.NewDeploymentService(
		projects, apps, deployments, blobStore, toolchains,
		service.DeploymentOptions{
			BuildCacheDirectory: buildCacheDirectory(cfg),
			MaxSourceBytes:      cfg.MaxDeploymentSourceBytes,
			MaxArtifactBytes:    cfg.MaxDeploymentArtifactBytes,
			BuildTimeout:        cfg.DeploymentBuildTimeout,
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
	browserProfiles, err := repository.NewBrowserProfileRepository(pool)
	if err != nil {
		return err
	}
	browserService, err := service.NewBrowserService(browserProfiles, nil, nil)
	if err != nil {
		return fmt.Errorf("configure browser profiles: %w", err)
	}

	browserSessions, err := repository.NewBrowserSessionRepository(cache)
	if err != nil {
		return err
	}
	browserSessionService, err := service.NewBrowserSessionService(browserSessions, nil, nil)
	if err != nil {
		return fmt.Errorf("configure browser sessions: %w", err)
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
			AppID:         cfg.GitHubAppID,
			PrivateKey:    cfg.GitHubPrivateKey,
			WebhookSecret: cfg.GitHubWebhookSecret,
			Limits: github.Limits{
				MaxArchiveBytes:   cfg.MaxDeploymentSourceBytes,
				MaxArchiveEntries: cfg.MaxDeploymentArchiveEntries,
			},
		})
		if err != nil {
			return fmt.Errorf("configure GitHub app: %w", err)
		}
		logger.Info("github app configured", "app_id", cfg.GitHubAppID)
		if len(cfg.GitHubWebhookSecret) == 0 {
			logger.Warn("github webhook secret is not set; pushes will not deploy")
		}
	} else {
		logger.Warn("github app is not configured; repository deployments are unavailable")
	}
	gitHubService, err := service.NewGitHubService(
		gitHubClient, installations, apps, deploymentService, nil, nil,
	)
	if err != nil {
		return fmt.Errorf("configure GitHub deployments: %w", err)
	}

	executionTokens, err := service.NewExecutionTokenService(cache, apps, cfg.RunTimeout*2)
	if err != nil {
		return fmt.Errorf("configure execution tokens: %w", err)
	}
	browserSupervisor := browsergrpc.NewSupervisor(cfg.BrowserService)
	defer browserSupervisor.Close()
	browserRPC, err := browsergrpc.NewServer(
		browserSessionService, browserService, executionTokens, browserSupervisor,
	)
	if err != nil {
		return fmt.Errorf("configure browser grpc: %w", err)
	}

	recoveredBuilds, err := deploymentService.RecoverInterruptedBuilds(ctx)
	if err != nil {
		return fmt.Errorf("recover interrupted builds: %w", err)
	}
	if recoveredBuilds > 0 {
		logger.Warn("marked interrupted builds failed", "builds", recoveredBuilds)
	}

	if cfg.BrowserService == "" {
		logger.Warn("no browser service configured; apps that drive a browser will fail")
	}
	pythonRunner, err := worker.NewPythonRunner(
		worker.PythonOptions{
			Executable:     cfg.PythonExecutable,
			BrowserService: cfg.BrowserService,
		},
	)
	if err != nil {
		return fmt.Errorf("configure Python runner: %w", err)
	}
	rubyRunner, err := worker.NewRubyRunner(
		worker.RubyOptions{
			Executable:     cfg.RubyExecutable,
			BrowserService: cfg.BrowserService,
		},
	)
	if err != nil {
		return fmt.Errorf("configure Ruby runner: %w", err)
	}
	nodeRunner, err := worker.NewNodeRunner(
		worker.NodeOptions{
			Executable:     cfg.NodeExecutable,
			BrowserService: cfg.BrowserService,
		},
	)
	if err != nil {
		return fmt.Errorf("configure Node runner: %w", err)
	}
	// Rust and Go differ only in what produced the binary, so they share one.
	binaryRunner, err := worker.NewBinaryRunner(
		worker.BinaryOptions{BrowserService: cfg.BrowserService},
	)
	if err != nil {
		return fmt.Errorf("configure compiled runner: %w", err)
	}
	runners := map[deployment.Runtime]worker.Runner{
		deployment.RuntimePython: pythonRunner,
		deployment.RuntimeRuby:   rubyRunner,
		deployment.RuntimeNode:   nodeRunner,
		deployment.RuntimeRust:   binaryRunner,
		deployment.RuntimeGo:     binaryRunner,
	}
	executor, err := worker.New(
		executions, deployments, blobStore, runners,
		worker.Options{
			PollInterval: cfg.WorkerPollInterval, RunTimeout: cfg.RunTimeout,
			MaxResultBytes: cfg.MaxRunResultBytes, MaxLogBytes: cfg.MaxRunLogBytes,
			MaxArtifactBytes:        cfg.MaxDeploymentArtifactBytes,
			MaxArchiveEntries:       cfg.MaxDeploymentArchiveEntries,
			MaxArchiveExpandedBytes: cfg.MaxDeploymentExpandedBytes,
			CallbackAddress:         cfg.GRPCAddress,
			Tokens:                  executionTokens,
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
		Deployments:     deploymentService,
		Executions:      executionService,
		Accounts:        accountService,
		Sessions:        sessionService,
		Organizations:   organizationService,
		GitHub:          gitHubService,
		Browsers:        browserService,
		BrowserSessions: browserSessionService,
		AllowedOrigins:  cfg.AllowedOrigins,
		Ready: func(readyCtx context.Context) error {
			return errors.Join(
				pool.Ping(readyCtx), storeReady(readyCtx, blobStore),
				storeReady(readyCtx, cache),
				deployments.Check(readyCtx),
			)
		},
		MaximumBodyBytes:    cfg.MaxRequestBodyBytes,
		SessionCookieSecure: cfg.SessionCookieSecure,
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
	errs := make(chan error, 3)
	var background sync.WaitGroup
	background.Add(3)
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
	// Loopback only, and the handlers it serves are the ones this process
	// started. It carries no TLS because nothing off this host can reach it.
	go func() {
		defer background.Done()
		if err := browserRPC.Serve(runtimeCtx, cfg.GRPCAddress); err != nil &&
			!errors.Is(err, context.Canceled) {
			sendRuntimeError(errs, fmt.Errorf("browser grpc: %w", err))
		}
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
