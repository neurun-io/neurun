# Deployment

One commit of an app's repository, plus every build made from it.

A deployment is the whole path from raw code to something runnable: the commit
is fetched from GitHub, stored immutably, and built into artifacts a worker can
execute. The source never changes — rebuilding produces a new build, never new
source.

## Why builds are nested underneath

A deployment can have several builds. The first may fail on a missing
dependency, the second succeed after the toolchain is fixed. Both belong to the
same fetched source, so they live under one deployment rather than becoming
unrelated records.

The deployment's own `status` always mirrors its newest build — `uploaded`
before any build exists, then `building`, `ready` or `failed`. That invariant
is enforced on every write.

## Creating one

Source is never uploaded. A deployment is created by a push to the production
ref of an app's connected repository, or by `POST /v1/github/deployments` with
an `app_id` and an optional `ref`. The app must already exist and be connected.
The entrypoint defaults to `main.py:handler` and is normalized before storage.

Creation is synchronous: the request builds the source and returns the finished
deployment, ready or failed.

## Fields

`id`, `project_id`, `app_id`, `runtime` (`python`), `entrypoint`, `status`,
`source` (an [artifact](artifact.md)), `builds`, `created_at`, `updated_at`.
