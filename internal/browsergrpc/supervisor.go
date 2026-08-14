// Package browsergrpc serves the SDK on loopback and brokers for it.
package browsergrpc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os/exec"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/neurun-io/neurun/internal/browserservicepb"
)

// ErrNoBrowserService is the executable not being configured. Every browser
// call refuses with it rather than failing halfway through opening one.
var ErrNoBrowserService = errors.New("no browser service is configured")

const (
	// startTimeout bounds waiting for a freshly spawned service to listen.
	startTimeout = 30 * time.Second
	// probeInterval is how often the port is tried while it starts.
	probeInterval = 100 * time.Millisecond
)

// Supervisor owns the browser service process for this host.
//
// One service, many sessions: spawning per execution would mean a process
// launch on the critical path of every run and a connection torn down as often
// as it is made. It starts on first use rather than at boot, so a plane that
// never opens a browser never pays for one.
type Supervisor struct {
	executable string

	mu         sync.Mutex
	command    *exec.Cmd
	connection *grpc.ClientConn
	address    string
}

func NewSupervisor(executable string) *Supervisor {
	return &Supervisor{executable: strings.TrimSpace(executable)}
}

// Client returns a client to the running service, starting it if it is not.
func (supervisor *Supervisor) Client(
	ctx context.Context,
) (browserservicepb.BrowserServiceClient, error) {
	if supervisor.executable == "" {
		return nil, ErrNoBrowserService
	}
	supervisor.mu.Lock()
	defer supervisor.mu.Unlock()

	if supervisor.connection != nil && supervisor.alive() {
		return browserservicepb.NewBrowserServiceClient(supervisor.connection), nil
	}
	if err := supervisor.start(ctx); err != nil {
		return nil, err
	}
	return browserservicepb.NewBrowserServiceClient(supervisor.connection), nil
}

// Address is where the service listens, for anything that needs to record it.
func (supervisor *Supervisor) Address() string {
	supervisor.mu.Lock()
	defer supervisor.mu.Unlock()
	return supervisor.address
}

// Close stops the service. Sessions do not outlive it, which is the point: the
// browsers were its children.
func (supervisor *Supervisor) Close() {
	supervisor.mu.Lock()
	defer supervisor.mu.Unlock()
	if supervisor.connection != nil {
		supervisor.connection.Close()
		supervisor.connection = nil
	}
	if supervisor.command != nil && supervisor.command.Process != nil {
		_ = supervisor.command.Process.Kill()
		_ = supervisor.command.Wait()
		supervisor.command = nil
	}
}

func (supervisor *Supervisor) alive() bool {
	return supervisor.command != nil &&
		supervisor.command.Process != nil &&
		supervisor.command.ProcessState == nil
}

// start spawns the service on a port the kernel picked and waits for it.
//
// Binding loopback is not hardening, it is the design: nothing outside this
// host is meant to reach a browser, and the control plane is the only thing
// that knows the port at all.
func (supervisor *Supervisor) start(ctx context.Context) error {
	if supervisor.connection != nil {
		supervisor.connection.Close()
		supervisor.connection = nil
	}
	port, err := freeLoopbackPort()
	if err != nil {
		return err
	}
	address := fmt.Sprintf("127.0.0.1:%d", port)

	command := exec.Command(supervisor.executable, "--listen", address)
	if err := command.Start(); err != nil {
		return fmt.Errorf("start browser service: %w", err)
	}
	if err := waitForListener(ctx, address); err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return err
	}
	connection, err := grpc.NewClient(
		address, grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return fmt.Errorf("dial browser service: %w", err)
	}
	supervisor.command, supervisor.connection, supervisor.address = command, connection, address
	slog.Info("browser service started", "address", address)
	return nil
}

// freeLoopbackPort asks the kernel for one rather than guessing, so two planes
// on a host do not fight over a fixed number.
func freeLoopbackPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("reserve browser service port: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		return 0, err
	}
	return port, nil
}

func waitForListener(ctx context.Context, address string) error {
	deadline := time.Now().Add(startTimeout)
	for {
		connection, err := net.DialTimeout("tcp", address, probeInterval)
		if err == nil {
			return connection.Close()
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("browser service did not listen on %s", address)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(probeInterval):
		}
	}
}
