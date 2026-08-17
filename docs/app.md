# App

The unit you deploy to. An app belongs to one project, connects to one GitHub
repository, and owns the deployments, builds and executions beneath it.

An app is **executed, not hosted**. It has no resident process and no endpoint
of its own: calls create [executions](execution.md), each pinned to a build, and
the compute they consume is what gets billed. A resident app behind an endpoint
is a [server](server.md), which is a different object on a different meter, and
is not built.

## It must exist first

This is the rule worth knowing: **an SDK cannot create an app by deploying to
it.** Deploying takes an `app_id`, looks it up, and fails with `app not found`
when it is missing. Nothing auto-creates one.

That is deliberate. Auto-creation means a typo in a client silently produces a
second app that looks fine and receives none of your traffic. Create apps
explicitly (`POST /v1/apps`), then deploy to them.

The app is also what decides a deployment's project — the caller never supplies
a project when deploying.

## The build it runs

An app runs one [build](build.md) at a time, and `active_build_id` is what says
which. Empty means the newest build it has, so an app that is never pinned
follows its deployments.

`PUT /v1/apps/{app_id}/active-build` sets it, with an empty `build_id` to
release it. The build has to be one this app already produced; anything else is
refused rather than left to fail at the next run.

Pinning is what a rollback is: point the app at an older build and every
execution created afterwards runs that, immediately, with no rebuild. It also
means later deployments build but do not go live — that is the point of a pin,
and the reason releasing it is one call.

An execution names only an app. Which build answers is this field's business,
and the answer is recorded on the execution so it is still known afterwards.

## Connecting a repository

`PUT /v1/apps/{app_id}/repository` points the app at `owner/name` and, optionally,
the ref whose pushes deploy. An empty repository disconnects it. The
installation has to be able to read it: the ref is resolved before anything is
stored, so a misconfigured connection fails there rather than on the first
deploy.

An app that names no production ref follows the repository's default branch.

## Lifecycle

Created with a project, a name and a repository; renamed with `PATCH`; deleted
with `DELETE`, which cascades to its deployments, builds and executions and
requires the app's exact name in a `confirm` query parameter.

## Fields

`id`, `project_id`, `name` (1–120 characters, unique within the project),
`repository`, `production_ref`, `active_build_id`, `created_at`, `updated_at`.
