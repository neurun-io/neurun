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

Copy `.env.example` to `.env` and edit what you need.

```sh
go run ./cmd/builder
```

For local development with Ministack, keep `AWS_ENDPOINT_URL_SQS` and both
`SQS_*_QUEUE_URL` values pointed at the Ministack SQS endpoint. The example uses
`http://localhost:4566`, account `000000000000`, and queues `builder-requests`
and `builder-responses`.

## Storage

Artifacts are uploaded to an S3-compatible object store. For local development,
the example points at Ministack S3 on `http://localhost:4566` with bucket
`dagflows-builds`.

```sh
R2_ACCOUNT_ID=
R2_ENDPOINT=http://localhost:4566
R2_REGION=us-east-1
R2_BUCKET=dagflows-builds
R2_ACCESS_KEY_ID=test
R2_SECRET_ACCESS_KEY=test
R2_PREFIX=builds
```

For Cloudflare R2, leave `R2_ENDPOINT` empty, set `R2_ACCOUNT_ID`, and use
`R2_REGION=auto`.

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
