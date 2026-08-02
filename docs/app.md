# App

The unit you deploy to. An app belongs to one project and owns its deployments.

## It must exist first

This is the rule worth knowing: **an SDK cannot create an app by deploying to
it.** `POST /v1/deployments` takes an `app_id`, looks it up, and fails with
`app not found` when it is missing. Nothing auto-creates one.

That is deliberate. Auto-creation means a typo in a client silently produces a
second app that looks fine and receives none of your traffic. Create apps
explicitly (`POST /v1/apps`), then deploy to them.

The app is also what decides a deployment's project — the caller never supplies
a project when deploying.

## Lifecycle

Created with a project and a name; renamed with `PATCH`; deleted with `DELETE`,
which cascades to its deployments, builds and executions and requires the app's
exact name in a `confirm` query parameter.

## Fields

`id`, `project_id`, `name` (1–120 characters, unique within the project),
`created_at`, `updated_at`.
