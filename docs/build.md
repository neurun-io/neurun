# Build

What one deployment produced: the layers it made, and enough about them to run
them.

A build carries no status and no failure. How it went belongs to the
[deployment](deployment.md) that ran the toolchain; a deployment that failed
before the toolchain produced anything has no build at all. Nothing rewrites a
build once it is written, which is why it has no version column.

## Whose it is

A build names its app and the deployment that made it. Both are columns, not
joins: an [execution](execution.md) reaches its build directly, and asking how a
build came to exist is a separate question from running it.

One deployment produces at most one build — `builds.deployment_id` is unique.

## Layers

A code layer always, and an install layer for a runtime that resolves
dependencies separately: Python with a non-empty `requirements.txt`, Ruby with a
Gemfile. Compiled runtimes link their dependencies in and ship one binary, so
they never carry an install layer; Node bundles for the same reason.

Each layer is an [artifact](artifact.md), named for what it is to the runtime —
that name is the directory a runner unpacks it into.

## The runnable file

Every runtime agrees with its runner on a fixed name rather than a manifest:
`app` for a compiled binary (`app.exe` on Windows), `app.js` for a Node bundle.
The builder packages under that name and the runner execs it, and neither
package imports the other.

## Fields

`id`, `app_id`, `deployment_id`, `runtime`, `source_sha256`, `artifacts`,
`created_at`.
