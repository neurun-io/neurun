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
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/dagflows/neurun-io/internal/agent"
	"github.com/dagflows/neurun-io/internal/api"
	"github.com/dagflows/neurun-io/internal/auth"
	"github.com/dagflows/neurun-io/internal/buildinfo"
	"github.com/dagflows/neurun-io/internal/config"
	"github.com/dagflows/neurun-io/internal/function"
	"github.com/dagflows/neurun-io/internal/httpruntime"
	"github.com/dagflows/neurun-io/internal/ids"
	"github.com/dagflows/neurun-io/internal/job"
	"github.com/dagflows/neurun-io/internal/netpolicy"
	"github.com/dagflows/neurun-io/internal/queue"
)

const maintenanceInterval = 100 * time.Millisecond

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
			return fmt.Errorf("unknown command %q (expected serve, doctor, or version)", os.Args[1])
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
	policy, err := netpolicy.NewPolicy(netpolicy.Options{
		AllowPrivateNetworks: !cfg.BlockPrivateNetworks,
	})
	if err != nil {
		return fmt.Errorf("configure outbound network policy: %w", err)
	}
	httpClient, err := netpolicy.NewClient(policy, netpolicy.ClientOptions{
		Timeout:          125 * time.Second,
		MaxResponseBytes: cfg.MaxResponseBytes,
	})
	if err != nil {
		return fmt.Errorf("configure outbound HTTP client: %w", err)
	}
	defer httpClient.CloseIdleConnections()

	registry := function.NewRegistry()
	if err := function.RegisterBuiltins(registry); err != nil {
		return fmt.Errorf("register foundation functions: %w", err)
	}
	fetchFunction, err := httpruntime.New(httpClient)
	if err != nil {
		return fmt.Errorf("construct HTTP runtime: %w", err)
	}
	if err := registry.Register(fetchFunction); err != nil {
		return fmt.Errorf("register HTTP runtime: %w", err)
	}

	invocations := function.NewService(registry, function.NewMemoryStore())
	jobs := job.NewMemoryRepository()
	broker := queue.NewMemoryBroker(cfg.JobLeaseDuration)

	dispatcherID, err := ids.New("dsp")
	if err != nil {
		return fmt.Errorf("allocate dispatcher ID: %w", err)
	}
	dispatcher := job.Dispatcher{
		Outbox:    jobs,
		Publisher: broker,
		Owner:     dispatcherID,
		BatchSize: 100,
		ClaimTTL:  cfg.JobLeaseDuration,
	}

	agentID, err := ids.New("agt")
	if err != nil {
		return fmt.Errorf("allocate agent ID: %w", err)
	}
	worker, err := agent.NewWorker(jobs, broker, invocations, agent.Options{
		AgentID:         agentID,
		Capabilities:    []string{"http"},
		Concurrency:     cfg.AgentConcurrency,
		LeaseTTL:        cfg.JobLeaseDuration,
		FinalizeTimeout: boundedFinalizeTimeout(cfg.ShutdownTimeout),
		Logger:          logger,
	})
	if err != nil {
		return fmt.Errorf("configure execution agent: %w", err)
	}

	authenticator, err := auth.New(auth.Credential{
		ID:        "key_configured",
		ProjectID: cfg.DefaultProjectID,
		RawKey:    cfg.APIKey,
		Scopes:    []string{"*"},
	})
	if err != nil {
		return fmt.Errorf("configure API authentication: %w", err)
	}
	controlAPI, err := api.NewServer(api.ServerOptions{
		Authenticator:    authenticator,
		Registry:         registry,
		Invocations:      invocations,
		Jobs:             jobs,
		BundleVersion:    function.BuiltinBundleVersion,
		MaximumBodyBytes: cfg.MaxRequestBodyBytes,
		JobDurability:    api.JobDurabilityProcessLocal,
		AllowAsyncJobs:   cfg.AllowVolatileJobs,
	})
	if err != nil {
		return fmt.Errorf("configure control API: %w", err)
	}

	runtimeCtx, stopRuntime := context.WithCancel(ctx)
	defer stopRuntime()

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           controlAPI,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      130 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    32 << 10,
		BaseContext: func(net.Listener) context.Context {
			return runtimeCtx
		},
	}

	errs := make(chan error, 2)
	var background sync.WaitGroup
	background.Add(2)
	go func() {
		defer background.Done()
		if err := worker.Run(runtimeCtx); err != nil {
			sendRuntimeError(errs, fmt.Errorf("execution agent: %w", err))
		}
	}()
	go func() {
		defer background.Done()
		runMaintenance(runtimeCtx, jobs, dispatcher, logger)
	}()

	go func() {
		logger.Info("neurun listening",
			"address", cfg.HTTPAddr,
			"version", buildinfo.Version,
			"agent_id", agentID,
		)
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

	// Cancel execution and request base contexts before waiting for handlers.
	// Cooperative in-process functions stop here; future side-effecting
	// runtimes must additionally own a killable process boundary.
	stopRuntime()
	shutdownCtx, cancelShutdown := context.WithTimeout(
		context.Background(),
		cfg.ShutdownTimeout,
	)
	shutdownErr := server.Shutdown(shutdownCtx)
	cancelShutdown()
	if shutdownErr != nil {
		shutdownErr = errors.Join(shutdownErr, server.Close())
	}

	background.Wait()
	return errors.Join(serveErr, shutdownErr)
}

func doctor(cfg config.Config) error {
	base, err := url.Parse(cfg.PublicURL)
	if err != nil {
		return err
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/healthz"
	base.RawQuery = ""
	base.Fragment = ""

	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Get(base.String())
	if err != nil {
		return fmt.Errorf("health check: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned %s", response.Status)
	}
	return nil
}

func runMaintenance(
	ctx context.Context,
	repository job.Repository,
	dispatcher job.Dispatcher,
	logger *slog.Logger,
) {
	ticker := time.NewTicker(maintenanceInterval)
	defer ticker.Stop()
	for {
		maintain(ctx, repository, dispatcher, logger)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func maintain(
	ctx context.Context,
	repository job.Repository,
	dispatcher job.Dispatcher,
	logger *slog.Logger,
) {
	if _, err := repository.RecoverExpiredLeases(ctx, 100); err != nil &&
		!errors.Is(err, context.Canceled) {
		logger.Error("recover expired job leases", "error", err)
	}
	if _, err := repository.EnqueueDueRetries(ctx, 100); err != nil &&
		!errors.Is(err, context.Canceled) {
		logger.Error("enqueue due job retries", "error", err)
	}
	report, err := dispatcher.DispatchOnce(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("dispatch job outbox",
			"claimed", report.Claimed,
			"published", report.Published,
			"failed", report.Failed,
			"error", err,
		)
	}
}

func sendRuntimeError(destination chan<- error, err error) {
	select {
	case destination <- err:
	default:
	}
}

func boundedFinalizeTimeout(shutdownTimeout time.Duration) time.Duration {
	timeout := 5 * time.Second
	if shutdownTimeout > 0 && shutdownTimeout/2 < timeout {
		timeout = shutdownTimeout / 2
	}
	if timeout <= 0 {
		return time.Second
	}
	return timeout
}
