# Dagflows Builder

Builds a workflow repo into artifacts and returns the discovered nodes.

Python projects are inspected through the SDK:

```sh
python -m dagflows inspect
```

The repo is built once. Every node gets the same package artifacts.

Artifacts are stored as:

```txt
<repo-sequence>/<commit-hash>/<artifact-file>
```

Example:

```txt
github-com-dagflows-python-sdk/e97b87e.../code-layer.zip
```

## Run

Copy `.env.example` to `.env` and edit what you need. Local storage is the default, so R2 is optional.

```sh
go run ./cmd/builder
```

Run one build without SQS:

```sh
go run ./cmd/builder-local \
  --deployment-id local-deployment \
  --workflow-id local-workflow \
  --git-url file:///D:/path/to/project
```

Optional:

```sh
--git-branch main
--git-commit-hash abc1234
```

## Storage

```sh
STORAGE_DRIVER=local
LOCAL_STORAGE_DIR=.dagflows-artifacts
```

Use R2 later with:

```sh
STORAGE_DRIVER=r2
```

## SQS

Request:

```json
{
  "deployment_id": "uuid",
  "workflow_id": "uuid",
  "git_url": "https://github.com/...",
  "git_branch": "main",
  "git_commit_hash": "abc1234"
}
```

`git_branch` and `git_commit_hash` are optional. Missing branch means `main`; missing commit means latest commit on that branch.

Response:

```json
{
  "deployment_id": "uuid",
  "status": "SUCCESS",
  "error_message": "",
  "nodes": []
}
```

## Requirements

Host tools: `git`, `python`, `npm`, and `go`.
