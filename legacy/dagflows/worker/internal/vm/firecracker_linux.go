//go:build linux

package vm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/dagflows/worker/internal/artifact"
	"github.com/dagflows/worker/internal/protocol"
)

const (
	localFirecracker = ".local/bin/firecracker"
	localKernel      = ".local/vm/vmlinux"
	localRootFS      = ".local/vm/rootfs.ext4"
)

type FirecrackerRunner struct {
	binary  string
	kernel  string
	rootFS  string
	nextCID atomic.Uint32
}

func NewFirecrackerRunner() (*FirecrackerRunner, error) {
	paths := make([]string, 3)
	var err error
	for i, path := range []string{localFirecracker, localKernel, localRootFS} {
		paths[i], err = requiredAsset(path, i == 0)
		if err != nil {
			return nil, err
		}
	}
	if err := unix.Access("/dev/kvm", unix.R_OK|unix.W_OK); err != nil {
		return nil, fmt.Errorf("/dev/kvm is not readable/writable: %w", err)
	}
	for _, name := range []string{"cp", "mkfs.ext4"} {
		if _, err := exec.LookPath(name); err != nil {
			return nil, fmt.Errorf("%s is required", name)
		}
	}
	runner := &FirecrackerRunner{binary: paths[0], kernel: paths[1], rootFS: paths[2]}
	runner.nextCID.Store(100)
	return runner, nil
}

func (r *FirecrackerRunner) Run(ctx context.Context, workload *artifact.PreparedWorkload) (result Result, runErr error) {
	ctx, cancel := context.WithTimeout(ctx, timeoutFor(workload.Request.TimeoutSeconds))
	defer cancel()

	runDir, err := os.MkdirTemp("/tmp", "dagflows-vm-")
	if err != nil {
		return Result{}, infrastructure("create VM directory", err)
	}
	defer os.RemoveAll(runDir)

	disks, err := r.prepareDisks(ctx, workload, runDir)
	if err != nil {
		return Result{}, infrastructure("prepare VM disks", err)
	}

	vsockBase := filepath.Join(runDir, "vsock")
	listener, err := net.Listen("unix", fmt.Sprintf("%s_%d", vsockBase, protocol.Port))
	if err != nil {
		return Result{}, infrastructure("listen for guest", err)
	}
	defer listener.Close()

	configPath := filepath.Join(runDir, "firecracker.json")
	if err := writeConfig(configPath, r.kernel, disks, vsockBase, workload.Request.RequiredMemoryMB(), r.nextCID.Add(1)); err != nil {
		return Result{}, infrastructure("write Firecracker config", err)
	}

	var logs bytes.Buffer
	cmd := exec.CommandContext(ctx, r.binary, "--id", filepath.Base(runDir), "--no-api", "--config-file", configPath)
	cmd.Stdout, cmd.Stderr = &logs, &logs
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return Result{}, infrastructure("start Firecracker", err)
	}
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
		close(done)
	}()
	defer func() {
		_ = unix.Kill(-cmd.Process.Pid, unix.SIGKILL)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			if runErr == nil {
				runErr = infrastructure("stop Firecracker", errors.New("process did not exit"))
			}
		}
	}()

	conn, err := waitForGuest(ctx, listener, done)
	if err != nil {
		return Result{}, firecrackerError(err, &logs)
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	output, duration, err := transact(conn, workload)
	if err != nil {
		return Result{Duration: duration}, err
	}
	return Result{Output: output, Duration: duration}, nil
}

func requiredAsset(path string, executable bool) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a file", path)
	}
	if executable && unix.Access(path, unix.X_OK) != nil {
		return "", fmt.Errorf("%s is not executable", path)
	}
	return path, nil
}

type vmDisks struct {
	root string
	code string
	deps string
}

func (r *FirecrackerRunner) prepareDisks(ctx context.Context, workload *artifact.PreparedWorkload, dir string) (vmDisks, error) {
	disks := vmDisks{
		root: filepath.Join(dir, "rootfs.ext4"),
		code: filepath.Join(dir, "code.ext4"),
		deps: filepath.Join(dir, "deps.ext4"),
	}
	if output, err := exec.CommandContext(ctx, "cp", "--reflink=auto", "--sparse=always", r.rootFS, disks.root).CombinedOutput(); err != nil {
		return vmDisks{}, fmt.Errorf("clone rootfs: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if err := makeLayer(ctx, workload.CodeDir, disks.code); err != nil {
		return vmDisks{}, err
	}
	if err := makeLayer(ctx, workload.DepsDir, disks.deps); err != nil {
		return vmDisks{}, err
	}
	return disks, nil
}

func makeLayer(ctx context.Context, source, target string) error {
	size, err := layerSize(source)
	if err != nil {
		return err
	}
	if err := os.WriteFile(target, nil, 0o600); err != nil {
		return err
	}
	if err := os.Truncate(target, size); err != nil {
		return err
	}
	output, err := exec.CommandContext(ctx, "mkfs.ext4", "-q", "-F", "-d", source, target).CombinedOutput()
	if err != nil {
		return fmt.Errorf("build layer: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func layerSize(root string) (int64, error) {
	var size int64 = 32 << 20
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			size += info.Size()
		}
		return nil
	})
	if size < 64<<20 {
		size = 64 << 20
	}
	return size + size/4, err
}

func writeConfig(path, kernel string, disks vmDisks, vsockPath string, memoryMB int64, cid uint32) error {
	if memoryMB < 128 {
		memoryMB = 128
	}
	config := map[string]any{
		"boot-source": map[string]any{
			"kernel_image_path": kernel,
			"boot_args":         "console=ttyS0 reboot=k panic=1 pci=off root=/dev/vda rw",
		},
		"drives": []map[string]any{
			drive("rootfs", disks.root, true, false),
			drive("code", disks.code, false, true),
			drive("deps", disks.deps, false, true),
		},
		"machine-config": map[string]any{"vcpu_count": 1, "mem_size_mib": memoryMB, "smt": false},
		"vsock":          map[string]any{"guest_cid": cid, "uds_path": vsockPath},
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	return json.NewEncoder(file).Encode(config)
}

func drive(id, path string, root, readOnly bool) map[string]any {
	return map[string]any{"drive_id": id, "path_on_host": path, "is_root_device": root, "is_read_only": readOnly}
}

func waitForGuest(ctx context.Context, listener net.Listener, done <-chan error) (net.Conn, error) {
	type accepted struct {
		conn net.Conn
		err  error
	}
	result := make(chan accepted, 1)
	go func() {
		conn, err := listener.Accept()
		result <- accepted{conn, err}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-done:
		return nil, fmt.Errorf("Firecracker exited: %v", err)
	case result := <-result:
		return result.conn, result.err
	}
}

func transact(conn net.Conn, workload *artifact.PreparedWorkload) ([]byte, time.Duration, error) {
	if err := json.NewEncoder(conn).Encode(protocol.RunRequest{
		NodeKey:        workload.Request.NodeKey,
		TimeoutSeconds: workload.Request.TimeoutSeconds,
		Input:          workload.Input,
	}); err != nil {
		return nil, 0, err
	}
	var result protocol.RunResult
	if err := json.NewDecoder(conn).Decode(&result); err != nil {
		return nil, 0, err
	}
	duration := time.Duration(result.DurationMS) * time.Millisecond
	if result.Error != "" {
		return nil, duration, RunError{Message: result.Error, Category: result.Category, Retryable: result.Retryable}
	}
	return result.Output, duration, nil
}

func infrastructure(action string, err error) RunError {
	return RunError{Message: fmt.Sprintf("%s: %v", action, err), Category: "infrastructure", Retryable: true}
}

func firecrackerError(err error, logs *bytes.Buffer) RunError {
	message := err.Error()
	if detail := strings.TrimSpace(logs.String()); detail != "" {
		message += ": " + detail
	}
	return infrastructure("Firecracker failed", errors.New(message))
}
