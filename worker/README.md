# Dagflows Worker

Executes workflow node-run requests from Redis Streams and publishes node-run
responses for the scheduler.

The service mirrors the builder layout: `cmd/worker` wires config, listener, and
service; `internal/listener/redis` owns stream consumption; `internal/service`
executes a node; `internal/artifact` prepares code/dependency layers; and
`internal/vm` isolates the runtime backend.

## Layer Model

The builder already emits two layers for Python/Node and one deployable for Go:

- `deps_artifact_url` is unpacked into `deps/`.
- `artifact_url` is unpacked into `code/`.
- The worker writes `input.json` and `manifest.json` beside those directories.

Inside the VM, mount or copy them as:

- `/srv/dagflows/deps`
- `/srv/dagflows/code`
- `/srv/dagflows/input.json`
- `/srv/dagflows/manifest.json`

Do not permanently merge dependency and code layers in storage. Keep deps and
code as separate artifacts so dependency layers can be cached by hash and reused
across code-only deploys. At execution time, combine them with runtime search
paths:

- Python: `PYTHONPATH=/srv/dagflows/code:/srv/dagflows/deps`
- Node: `NODE_PATH=/srv/dagflows/deps/node_modules`
- Go: execute the deployable from `/srv/dagflows/code/deployable`

## Firecracker

Set `WORKER_RUNTIME_MODE=firecracker` and provide
`FIRECRACKER_RUNNER_COMMAND`. The command is expected to launch Firecracker,
attach the prepared workload to the guest, wait for the guest runner to finish,
and write JSON to the path passed by `--output`.

The command receives:

```txt
--work-dir <dir>
--manifest <dir>/manifest.json
--input <dir>/input.json
--code-dir <dir>/code
--deps-dir <dir>/deps
--output <dir>/output.json
```

For local development, `WORKER_RUNTIME_MODE=host` currently supports Python via:

```sh
python -m dagflows invoke --node <node>
```

## Redis Defaults

```txt
WORKER_REQUEST_STREAM=goflow:node_run_requests
WORKER_REQUEST_GROUP=goflow:node_run_requests:group
WORKER_RESPONSE_STREAM=goflow:node_run_responses
WORKER_MAX_CONCURRENCY=2
```

ACK happens only after the response is published. If the process crashes before
ACK, Redis pending-entry reclaim redelivers the request.
