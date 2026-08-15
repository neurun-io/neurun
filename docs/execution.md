# Execution

One invocation of a built handler — the durable record of what was sent in,
which build answered, and how it ended.

It exists for three reasons:

1. **The handoff.** Creating one returns `202` immediately; a worker picks it
   up on its own clock. Something has to hold the request in between.
2. **The result.** The caller comes back later for output, logs or failure.
3. **Provenance.** `build_id` records exactly which build produced the answer.

## Pinned to a build

An execution is pinned to the build that was ready when it was created, and
never moves. Ten rebuilds later it still names the code that ran. That pinning
is the only reason a rerun means anything: `POST /v1/executions/{id}/rerun`
repeats the same input against the same build, and refuses if that build is no
longer ready.

## States

`queued` → `running` → `succeeded` or `failed`.

Claiming takes the oldest queued row with `FOR UPDATE SKIP LOCKED`, so several
workers can drain the queue without colliding. Finalizing is a compare-and-set:
only a running execution may go terminal, and nothing immutable about it may
have changed in between — a late write from a worker that lost its lease loses.

Executions a crashed worker left `running` are marked `failed` with
`worker_restarted` on the next start. They are never re-run.

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
