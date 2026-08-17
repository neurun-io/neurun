# Deployment

The act of turning one commit into a build: how far it got, and what the
toolchain printed on the way.

What it produced is the [build](build.md) that names it — at most one, and none
at all when it failed before the toolchain ran. The source it built is not
kept: `commit_sha` is where those bytes come from, and GitHub still has them.

## Creating one

Source is never uploaded. A deployment is created by a push to the production
ref of an app's connected repository, or by `POST /v1/github/deployments` with
an `app_id` and an optional `ref`. The app must already exist and be connected.

Creation is a queue write, not a build. The request records the commit and
returns `202`; a builder claims it afterwards. That is what makes a deployment
survive the process that accepted it — the row is the queue, so nothing is lost
to a restart between accepting and building.

The runtime is settled at creation, because a push carries none and the
repository is the only thing that knows: a `Cargo.toml` is a Rust crate whoever
pushed it.

## States

`queued` → `building` → `publishing` → `ready` or `failed`.

Queueing and publishing are not folded into building because they are different
things to be waiting on: one waits for a builder, the other for an upload. Logs
arrive while it runs, written every couple of seconds rather than on every
burst.

## Claiming

A builder takes the oldest queued row with `FOR UPDATE SKIP LOCKED` and moves it
to `building` inside that transaction, the same way a worker claims an
[execution](execution.md). The row is the claim: a deployment nobody has claimed
is one nobody is building, so several builders never collide.

## Retrying

`POST /v1/deployments/{id}/retry` builds the same commit again as a new
deployment. The ref is not resolved a second time — a ref moves, and a retry
that silently built something else would not be a retry. The original keeps its
logs, its build and its failure.

## Interrupted builds

If the process dies mid-build the row is left `building`. On the next start,
recovery marks it `failed` with `build_interrupted`. It is never retried
automatically — whatever the first attempt did to a toolchain cache already
happened.

## Fields

`id`, `project_id`, `app_id`, `runtime`, `status`, `commit_sha`, `git_ref`,
`build`, `failure`, `logs`, `started_at`, `finished_at`, `created_at`,
`updated_at`.
