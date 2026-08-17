# Project

A namespace. It owns apps, and through them every deployment, build and
execution beneath.

A project is only a container. It grants nothing and authenticates nobody —
callers are not bound to a project, so a project is a property of the
*resources* you address, never of who is asking.

## Lifecycle

Created explicitly (`POST /v1/projects`). Nothing creates one implicitly: the
server seeds no project at boot.

Deleting one cascades to its apps, deployments, builds and executions. It does
**not** touch users or API keys — those belong to the install. Deletion is
irreversible, so the API requires the project's exact name echoed in a
`confirm` query parameter before it will proceed.

Blob payloads in the artifact store are left behind: the rows that named them
are gone, so nothing reaches them, and reclaiming them is a separate sweep.

## Fields

`id`, `name` (1–120 characters, unique), `created_at`, `updated_at`.
