# Build

One attempt at turning a deployment's source into runnable artifacts.

## States

- `building` — running. No finish time, no failure.
- `ready` — produced artifacts, including a code layer. Executions may pin to it.
- `failed` — carries a failure code and message.

A build only ever moves out of `building`, and only once. Finishing a finished
build is refused, because an execution may already be pinned to it.

## Numbering

Builds are numbered from 1 within their deployment and must stay contiguous —
`StartBuild` assigns the next number rather than trusting a caller.

## What it produces

A code layer always, and an install layer when the source has a non-empty
`requirements.txt`. Both are [artifacts](artifact.md).

## Interrupted builds

If the process dies mid-build the row is left `building`. On the next start,
recovery marks it `failed` with `build_interrupted`. It is never retried
automatically — the side effects of the first attempt already happened.

## Fields

`id`, `app_id`, `deployment_id`, `runtime`, `source_sha256`, `artifacts`,
`created_at`.
