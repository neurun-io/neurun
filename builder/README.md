# Dagflows Builder

Builds a workflow repo into deployable artifacts and returns the discovered
workflow nodes.

The worker listens for deployment requests on SQS, fetches the requested repo,
inspects the workflow, builds one package for the deployment, uploads artifacts
to S3-compatible storage, and posts a response message.

## Run

```sh
go run ./cmd/builder
```

Copy `.env.example` to `.env` first. See [setup.md](setup.md) for Ministack,
SQS, storage, and local test-message setup.

## Request

```json
{
  "deployment_id": "uuid",
  "workflow_id": "uuid",
  "git_url": "https://github.com/owner/repo.git",
  "git_branch": "main",
  "git_commit_hash": "abc1234"
}
```

`git_branch` and `git_commit_hash` are optional.

## Response

```json
{
  "deployment_id": "uuid",
  "status": "SUCCESS",
  "error_message": "",
  "nodes": []
}
```

## Notes

- Python projects are inspected with `python -m dagflows inspect`.
- The repo is built once; every node receives the same package artifacts.
- Artifact keys use `<repo-sequence>/<commit-hash>/<artifact-file>`.
