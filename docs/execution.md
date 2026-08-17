# Execution

One invocation of an app — the durable record of what was sent in, which build
answered, and how it ended.

It exists for three reasons:

1. **The handoff.** Creating one returns `202` immediately; a worker picks it
   up on its own clock. Something has to hold the request in between.
2. **The result.** The caller comes back later for output, logs or failure.
3. **Provenance.** `build_id` records exactly which build produced the answer.

## Naming an app, running a build

`POST /v1/executions` takes an `app_id` and an input, and nothing else. Which
build runs is the [app](app.md)'s own answer: the build it is active on, or the
newest it has. A caller does not choose, because a caller choosing is how two
clients end up running different code against the same app without either
noticing.

What was chosen is written to the row, so a finished execution still names the
code that ran.

## Pinned to a build

An execution is pinned to the build resolved when it was created, and never
moves. Ten deployments later it still names the code that ran. That pinning is
the only reason a rerun means anything: `POST /v1/executions/{id}/rerun` repeats
the same input against the same build, whatever the app is active on now, and
refuses only if that build no longer exists.

## States

`queued` → `running` → `succeeded` or `failed`.

Claiming takes the oldest queued row with `FOR UPDATE SKIP LOCKED`, so several
workers can drain the queue without colliding. Finalizing is a compare-and-set:
only a running execution may go terminal, and nothing immutable about it may
have changed in between — a late write from a worker that lost its lease loses.

Executions a crashed worker left `running` are marked `failed` with
`worker_restarted` on the next start. They are never re-run.

## Output and logs

Two different things, on two different paths.

**Logs** are whatever the app printed on stdout and stderr, capped at 256 KiB
with the head kept and the loss marked. They are written to the row while the
app is still running — every couple of seconds rather than on every burst — so
following an execution shows it happening rather than showing nothing and then
everything.

**Output** is the value the app returned: JSON, written once when the execution
goes terminal. It is kept on the row while it is small enough to belong there;
past that an execution fails rather than silently losing the tail of it.

## The billable unit

An app is executed, not hosted, so the execution is what gets metered: memory
reserved multiplied by wall time, from the moment a worker claims the row to the
moment it goes terminal. Queued time is not billed — a queued execution holds no
worker. Builds are not billed. A rerun costs whatever it consumes, like any
other execution.

Nothing in this repository charges anybody; the meter is the wall time between
`started_at` and `finished_at`, which is why both are recorded to the row rather
than derived from logs. [Servers](server.md) would be metered by resident time
instead, and are unbuilt.

## Fields

`id`, `project_id`, `app_id`, `build_id`, `status`, `input`, `output`,
`failure`, `logs`, `created_at`, `started_at`, `finished_at`,
`rerun_of_execution_id`.
