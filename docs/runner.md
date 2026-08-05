# Runner

A server that holds one app resident and exposes an endpoint.

**Not built.** There is no `/v1/runners`, no runner in the dashboard, and no
plan that includes one. This note records the shape being designed so the
difference from an [execution](execution.md) stays clear while it is unbuilt.

## The inversion

An app is executed, not hosted. A [deployment](deployment.md) produces a
[build](build.md), an execution invokes that build once, and the process goes
away. A runner inverts it: it pins one app to one ready build, keeps it resident,
and exposes an endpoint callers reach directly — no execution record per call,
no cold start per call.

| | Execution | Runner |
| --- | --- | --- |
| Unit | one invocation | one resident process |
| Record | one row, pinned to a build | a lifecycle, not a row per call |
| Metered | compute consumed while running | time the runner is up |

## When it is the right answer

When startup cost dwarfs the work: a crawler holding a warm connection pool, a
handler loading a large model once, anything fronting a latency budget a queue
cannot meet. A handler that starts in milliseconds and runs on a schedule is
cheaper as an execution, and leaves a better record.

## Why it cannot share the execution meter

Resident time and per-execution compute cannot be added together without
double-counting the same second. A runner is therefore its own meter and its own
invoice line. Nothing about execution pricing changes when runners ship.

## What it still needs

- create / list / detail / delete, pinned to one app and one ready build;
- a lifecycle the server owns — starting, ready, draining, stopped — carrying
  the reason a runner left `ready`;
- the exposed endpoint and its authentication, scoped to the runner rather than
  to a project key;
- resident-time metering, separate from per-execution compute;
- health and logs, since a resident process has no execution record to attach
  either to.

The dashboard's `/runners` route names this same list rather than rendering a
runner it cannot start, stop or bill.
