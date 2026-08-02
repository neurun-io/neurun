# Deployment

One upload of source code, plus every build made from it.

A deployment is the whole path from raw code to something runnable: you upload
a ZIP, it is stored immutably, and it is built into artifacts a worker can
execute. The upload never changes — rebuilding produces a new build, never new
source.

## Why builds are nested underneath

A deployment can have several builds. The first may fail on a missing
dependency, the second succeed after the toolchain is fixed. Both belong to the
same uploaded source, so they live under one deployment rather than becoming
unrelated records.

The deployment's own `status` always mirrors its newest build — `uploaded`
before any build exists, then `building`, `ready` or `failed`. That invariant
is enforced on every write.

## Creating one

`POST /v1/deployments` as `multipart/form-data` with `app_id`, `runtime`, an
optional `entrypoint`, and a `source` ZIP. The app must already exist. The
entrypoint defaults to `main.py:handler` and is normalized before storage.

Creation is synchronous: the request builds the source and returns the finished
deployment, ready or failed.

## Fields

`id`, `project_id`, `app_id`, `runtime` (`python`), `entrypoint`, `status`,
`source` (an [artifact](artifact.md)), `builds`, `created_at`, `updated_at`.
