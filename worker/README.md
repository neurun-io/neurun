# Agent

Linux-only service that consumes node-run requests from Redis, fetches code and
dependency artifacts from R2, runs them in Firecracker, and publishes results.

## Setup

```sh
make setup
```

Setup builds:

```txt
.local/bin/agent
.local/bin/firecracker
.local/vm/vmlinux
.local/vm/rootfs.ext4
```

It also builds `cmd/guest` as `/usr/local/bin/agent` inside the VM. Downloads and extraction use
OS temporary directories; only final assets stay in `.local`.

Run from the repository root:

```sh
.local/bin/agent
```

Use `make kvm-check` if `/dev/kvm` is inaccessible. It prints group and ACL
options without changing host permissions.

## Execution

The host agent and VM agent share only `internal/protocol`.

1. Code and dependency artifacts are expanded under OS temp.
2. Each becomes a read-only ext4 drive.
3. The guest mounts both and combines them with an ephemeral OverlayFS.
4. Request and result JSON are exchanged over vsock.
5. The Firecracker process and temporary files are removed after the request.

The scheduler normally sends R2 keys:

```json
{
  "artifact_key": "builds/repo/commit/code-layer.zip",
  "deps_artifact_key": "builds/repo/commit/install-layer.zip"
}
```

## Resources

`WORKER_MAX_CONCURRENCY` defaults to `1`. Before work starts, the agent reserves
`memory_limit_mb` plus 128 MB against Linux `MemAvailable`. Missing limits
default to 512 MB. Insufficient memory returns a retryable failure without
starting a VM.

Redis requests are acknowledged only after their response is published.
