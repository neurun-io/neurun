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
  "deployment_id": "dep_20260708_001",
  "workflow_id": "wf_branching_demo",
  "git_url": "https://github.com/Dagflows/sample-workflow.git",
  "git_branch": "main",
  "git_commit_hash": "e97b87e884588e0b6e2cfb0bd093279b5618c888"
}
```

`git_branch` and `git_commit_hash` are optional. Missing branch means `main`;
missing commit means the latest commit on that branch.

## Response

```json
{
  "deployment_id": "dep_20260708_001",
  "status": "SUCCESS",
  "error_message": "",
  "nodes": [
    {
      "key": "step_1",
      "type": "task",
      "language": "python",
      "entrypoint": "workflow.py:step_1",
      "artifact_id": "51efb0e8-7b9a-48d1-a071-6b6f7fbe1782",
      "deps_artifact_id": "f55e6ed1-b65f-4d64-90ac-97ddae0fe4f2",
      "config": {},
      "depends": [],
      "retry_count": 0,
      "timeout_seconds": 300,
      "memory_limit_mb": 512
    },
    {
      "key": "step_2",
      "type": "task",
      "language": "python",
      "entrypoint": "workflow.py:step_2",
      "artifact_id": "51efb0e8-7b9a-48d1-a071-6b6f7fbe1782",
      "deps_artifact_id": "f55e6ed1-b65f-4d64-90ac-97ddae0fe4f2",
      "config": {},
      "depends": ["step_1"],
      "retry_count": 0,
      "timeout_seconds": 30,
      "memory_limit_mb": 512
    }
  ]
}
```

Failed builds return the same deployment ID with an empty node list:

```json
{
  "deployment_id": "dep_20260708_001",
  "status": "FAILED",
  "error_message": "fetch code: clone repository: exit status 128",
  "nodes": []
}
```

## Notes

- Python projects are inspected with `python -m dagflows inspect`.
- The repo is built once; every node receives the same package artifacts.
- Artifact keys use `<repo-sequence>/<commit-hash>/<artifact-file>`.
