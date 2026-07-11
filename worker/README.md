# Worker

Linux-only service that consumes Python node-run requests from Redis, fetches
builder artifacts from R2, runs them in Firecracker, and publishes results.

## Setup

```sh
make setup
```

Setup builds:

```txt
.local/bin/firecracker
.local/vm/vmlinux
.local/vm/rootfs.ext4
```

It builds `internal/agent` as `/usr/local/bin/agent` inside the VM. Downloads and extraction use
OS temporary directories; only final assets stay in `.local`.

Run `make vm-assets` after changing `internal/agent`; the VM executable is
embedded in `rootfs.ext4`.

Run from the repository root:

```sh
go run ./cmd
```

The service loads `.env` when present. Existing process environment variables
take precedence.

Use `make kvm-check` if `/dev/kvm` is inaccessible. It prints group and ACL
options without changing host permissions.

## Execution

The worker service and VM agent share only `internal/protocol`.

1. Code and dependency artifacts are expanded under OS temp.
2. Each becomes a read-only ext4 drive.
3. The guest mounts both and combines them with an ephemeral OverlayFS.
4. Request and result JSON are exchanged over vsock.
5. The Firecracker process and temporary files are removed after the request.

The scheduler sends the builder's code-layer key:

```json
{
  "artifact_key": "builds/repo/commit/code-layer.zip"
}
```

The dependency key is inferred as `builds/repo/commit/install-layer.zip`.

## Resources

`WORKER_MAX_CONCURRENCY` defaults to `1`. Before work starts, the worker reserves
`memory_limit_mb` plus 128 MB against Linux `MemAvailable`. Missing limits
default to 512 MB. Insufficient memory returns a retryable failure without
starting a VM.

Redis requests are acknowledged only after their response is published.
