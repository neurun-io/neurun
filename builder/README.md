# Dagflows Builder

SQS worker for packaging workflow code into deployable artifacts.

## Runtimes

- Python: installs `requirements.txt` into `install-layer.zip`, compiles source, uploads `code-layer.zip`.
- Node: runs `npm ci` or `npm install`, runs `npm run build` when present, prunes dev deps, uploads `install-layer.zip` and `code-layer.zip`.
- Go: builds a binary and uploads it directly as a deployable.

## Config

```sh
AWS_REGION=us-east-1
AWS_ACCESS_KEY_ID=<access-key-id>
AWS_SECRET_ACCESS_KEY=<secret-access-key>
AWS_SESSION_TOKEN=

SQS_REQUEST_QUEUE_URL=https://sqs.us-east-1.amazonaws.com/<account-id>/<request-queue>
SQS_RESPONSE_QUEUE_URL=https://sqs.us-east-1.amazonaws.com/<account-id>/<response-queue>
SQS_MAX_MESSAGES=1
SQS_WAIT_TIME_SECONDS=20
SQS_VISIBILITY_TIMEOUT_SECONDS=900

GIT_DEFAULT_BRANCH=main
GIT_TEMP_DIR=

R2_ACCOUNT_ID=<account-id>
R2_ENDPOINT=
R2_BUCKET=<bucket>
R2_ACCESS_KEY_ID=<access-key-id>
R2_SECRET_ACCESS_KEY=<secret-access-key>
R2_PREFIX=builds
```

`.env` is loaded into `internal/config.Config` on startup. Defaults are applied before env overrides. If `R2_ENDPOINT` is blank, the service uses Cloudflare's documented S3 endpoint format: `https://<R2_ACCOUNT_ID>.r2.cloudflarestorage.com`. `R2_BUCKET` has no provider default and must be set.

Host tools must be installed: `git`, `python`, `npm`, and `go`.

## Layout

- SQS listener: `internal/listener/sqs`
- Config: `internal/config`
- Domain: `internal/domain`
- Services, including GitHub checkout and build: `internal/service`
- Shared helpers: `pkg`
- Storage: `internal/storage`

## Run

```sh
go run ./cmd/builder
```

## Messages

Request messages are read from `SQS_REQUEST_QUEUE_URL`:

```json
{
  "deployment_id": "uuid",
  "workflow_id": "uuid",
  "organization_id": "uuid",
  "git_url": "https://github.com/...",
  "git_branch": "main",
  "git_commit_hash": "abc1234"
}
```

Response messages are sent to `SQS_RESPONSE_QUEUE_URL`:

```json
{
  "deployment_id": "uuid",
  "status": "SUCCESS",
  "error_message": "",
  "nodes": [
    {
      "key": "main",
      "type": "task",
      "language": "python",
      "entrypoint": "main.py:handler",
      "artifact_id": "uuid",
      "deps_artifact_id": "uuid",
      "config": {},
      "depends": [],
      "retry_count": 3,
      "timeout_seconds": 300
    }
  ]
}
```

For each request, the worker clones `git_url`, checks out `git_commit_hash` when provided, then runs the build against the fetched checkout. It uses `nodes` from the request when present. Otherwise it checks for `dagflows.json`, `.dagflows.json`, `dagflows.workflow.json`, or `workflow.json`. If no manifest exists, it infers one `main` node from `package.json`, `go.mod`, `requirements.txt`, or Python source files.
