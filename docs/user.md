# User

A person who can sign in. Global to the installation — a user is not attached
to a project.

## Roles

- `admin` — every scope, including ones added in later releases.
- `operator` — read, plus deploy and execute.
- `viewer` — read only.

## How an account comes into being

Registration, and nothing else. The server creates nothing on boot and there is
no CLI to run on the host:

```sh
curl -sS -X POST $NEURUN_URL/v1/auth/register \
  -d '{"username":"ada","password":"a-long-password"}'
```

`POST /v1/auth/register` is public and unauthenticated — it is how a caller
obtains the credential everything under `/v1` requires. It creates the account
as an `admin` and signs the account in by setting the same session cookie sign-in
issues.

Sign-up is open, so **per-IP limiting belongs at the edge**, alongside the
throttle that used to sit in front of sign-in. The server deliberately holds no
rate limiter of its own.

The password is a minimum of 12 characters and is stored as a bcrypt hash. There
are no credentials in configuration — nothing in `.env` creates or restores an
account, so a password changed later is never silently reverted by a restart.

Later accounts are invited rather than registered: `POST /v1/users` takes a role
and needs `users:write`.

## Sessions

Signing in exchanges username and password for an opaque token in an HttpOnly,
SameSite=Strict cookie. Only the token's SHA-256 digest is stored, so a dump of
the session store cannot be replayed.

The role is re-read from the database on **every** request, so disabling or
demoting someone takes effect on their next call rather than when their cookie
happens to expire. Sessions live in an in-process cache: they do not survive a
restart, and do not work behind more than one replica.

Unknown username, wrong password and disabled account all return one
indistinguishable error, and all three spend the same bcrypt time, so the
endpoint cannot be used to enumerate accounts.

## Deleting one

Removes the person and nothing else. Keys they created keep working with their
attribution cleared; every project resource stands.

## Fields

`id`, `username` (unique, lowercase), `display_name`, `role`, `disabled`,
`created_at`, `updated_at`.
