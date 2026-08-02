# API key

A credential for a program. It carries scopes, and nothing else.

## Not bound to a project

A key is **not** attached to a project. Scopes are the entire authorization
model, so pinning a key to one project would both duplicate that decision and
cap the key for no reason.

The same follows for callers generally: `auth.Principal` has no project. A
signed-in user is unrestricted; a key reaches exactly as far as its scopes
allow. Projects scope *resources*, not callers.

## Scopes

`deployments:read|write`, `executions:read|write`, `builds:read`,
`apps:read|write`, `projects:read|write`, `users:read|write`,
`api_keys:read|write`, and `*` for everything including scopes added later.

A key can never be granted a scope its creator does not already hold, so a
limited key cannot mint an unlimited one.

## The secret

`POST /v1/api-keys` returns the secret exactly once. Only a SHA-256 digest is
stored; the plaintext is unrecoverable. The form is
`neu_<environment>_<prefix>.<secret>` — the part before the dot is the indexed
lookup prefix, and the comparison against the stored digest is constant time.

## Revocation

`DELETE /v1/api-keys/{id}` sets `revoked_at` and is idempotent — revoking twice
keeps the first timestamp. Nothing restores a revoked key: no configuration
re-asserts it at boot.

Deleting the user who created a key leaves the key working, with its
attribution cleared.

## Fields

`id`, `user_id` (optional), `name`, `prefix`, `scopes`, `created_at`,
`revoked_at`.
