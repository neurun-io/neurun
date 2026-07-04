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

GIT_TEMP_DIR=

STORAGE_DRIVER=local
LOCAL_STORAGE_DIR=.dagflows-artifacts

R2_ACCOUNT_ID=<account-id>
R2_ENDPOINT=
R2_BUCKET=<bucket>
R2_ACCESS_KEY_ID=<access-key-id>
R2_SECRET_ACCESS_KEY=<secret-access-key>
R2_PREFIX=builds
```

`STORAGE_DRIVER` can be `local` or `r2`. Local storage writes artifacts under `LOCAL_STORAGE_DIR` and is the default. R2 config is only required when `STORAGE_DRIVER=r2`.

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

## Local Run

Run one deployment request without SQS:

```sh
go run ./cmd/builder-local \
  --deployment-id local-deployment \
  --workflow-id local-workflow \
  --git-url file:///D:/path/to/project
```

The local command uses the same build path as the SQS worker and prints the deployment response JSON to stdout. `--git-branch` is optional and defaults to `main`. `--git-commit-hash` is optional; when omitted, the builder uses the latest commit on the selected branch. `--git-url` can be a `file:///...` URL, but it must point to a Git repository.

## Messages

Request messages are read from `SQS_REQUEST_QUEUE_URL`:

```json
{
  "deployment_id": "uuid",
  "workflow_id": "uuid",
  "git_url": "https://github.com/..."
}
```

`git_branch` is optional and defaults to `main`. `git_commit_hash` is optional; when omitted, the builder uses the latest commit on the selected branch.

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
      "timeout_seconds": 300,
      "memory_limit_mb": 512
    }
  ]
}
```

For each request, the worker clones `git_url`, checks out `git_commit_hash` when provided, then runs the build against the fetched checkout. Without `git_commit_hash`, the checkout stays on the latest commit from the selected branch. Artifacts are stored by repository sequence and resolved commit hash, so rebuilding the same repo at the same commit overwrites the same artifact objects. For example, `https://github.com/Dagflows/python-sdk.git` stores under `github-com-dagflows-python-sdk/<commit-hash>/`. Python projects are inspected with `python -m dagflows inspect`, so the repo should expose a `Workflow` through the SDK CLI. The project is built once as a single package, and the resulting artifact IDs are attached to every node. Node and Go projects still use a single inferred `main` node from `package.json` or `go.mod`.
