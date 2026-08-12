# Unit: Profile State

Cookies and DOM storage — what a profile remembers between runs. Two storage
models with different rules, one write semantic that surprises people, and one
footgun that silently destroys a profile.

## Cookies

Read browser-wide, restored browser-wide. A capture is the **entire jar**, which
is what makes the write semantic work.

Stored fields: `name`, `value`, `domain`, `path`, `expires` (Unix seconds, absent
for a session cookie), `secure`, `http_only`, `same_site`.

Redaction on read: `GET /v1/browser-profiles` and `GET .../{id}` return a cookie
as its name, domain, path, flags and **`value_size`** — enough to see what a
profile is logged into, and to decide to delete it, without reading the
credential. Only `GET .../{id}/state` returns values, and it takes the write
scope.

## DOM storage is per-origin, and asymmetric

This is the constraint that shapes everything else.

**Capture.** DOM storage is partitioned by origin, and CDP will not hand over an
area the page never opened. A close therefore captures the storage of the origins
that were **actually visited**. An origin you did not open is not in the capture,
and after a whole-state replace it is not in the profile either.

**Restore.** You cannot set an origin's `localStorage` while sitting on a
different origin. Restoring therefore **visits each origin in turn** before the
caller is handed the session — the same wall the older Python `DOMStorageManager`
hit and documented.

Practical consequences:

- Visit the origins whose state you care about, or their storage is not captured.
- Restoring N origins costs N navigations before your first real page. A profile
  that accumulates origins gets slower to open.
- `sessionStorage` dies with the tab by definition. Treat it as best-effort.
- The profile's `storage_origins` list on a read response names what it
  remembers without disclosing any values — a cheap way to see whether the origin
  you need is in there.

## `PUT` replaces, it does not merge

The browser hands back its whole cookie jar, so **a cookie missing from the body
was deleted**. Merging would resurrect a login the site had already ended — a
session cookie the server invalidated would come back, and the next run would
present a credential the target knows is dead.

So:

- Send the complete capture, never a filtered subset.
- Never hand-assemble a partial body "to update one cookie".
- Absent collections mean empty, not unchanged.

## The Firefox footgun

`CloseSession` on Firefox returns an **empty state**. Not because the browser had
none — because rustenium exposes no BiDi storage API to move cookies with, and
rustenium-identity drives Chrome over CDP only.

An empty body is indistinguishable from a browser that genuinely holds no
cookies. Combined with replace semantics, **PUTting a Firefox capture erases the
profile**.

The server cannot catch this: both cases are the same request. Until Firefox
carries state, the SDK must not write back after a Firefox session. A Firefox
profile is usable — it launches, it runs — it just cannot accumulate a past.

## Concurrency

One profile, one session at a time. Two concurrent sessions both capture, and the
last `PUT` replaces wholesale, so one run's logins vanish. Worse, the target sees
two overlapping sessions from one "machine", which is a continuity signal in its
own right.

Profiles are organization-scoped, so this is an organization-wide rule.

## Deletion

Deleting a profile takes its cookies and stored logins with it; anything signed
in through it has to sign in again. The dashboard gates the delete behind typing
the profile's name, which is an interface property — a programmatic client
confirms nothing, because calling `DELETE` already said what it wanted.

## What state means for detection

Everything above is mechanism. The reason it matters to a detector — fingerprint
stability, a plausible history, state that agrees with the claimed locale — is a
stealth concern. → `stealth` skill, `references/continuity.md`.

## Checklist

1. Are the origins you need actually visited in the run?
2. Is the capture written back **whole**?
3. Is the browser Chrome? If Firefox, skip the write.
4. Is anything else touching this profile right now?
5. Did the run leave the profile in a state you want to keep — or is abandoning
   it the better outcome?
