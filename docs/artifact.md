# Artifact

One stored ZIP: a layer a build produced.

Source is not an artifact. A deployment names the commit it built, and GitHub
still has those bytes, so nothing here stores them — the archive a build reads
is a temporary file, deleted when the build ends.

## Names

`Name` is what the layer is to the runtime, and it is the directory a runner
unpacks it into: `code`, `install`. It is unique within its build, so a build
carries as many layers as a toolchain has reason to produce.

## Addressing

The key is `<build_id>/<artifact_id>.zip` — addressed by what owns it, so
everything one build made sits together and a build that is deleted takes its
layers with it.

Keys are immutable. An object at a key can never change, which is what lets the
read-through cache in front of S3 serve a hit with no expiry, no validation and
no revalidation round trip. The store hashes what it writes, so the recorded
digest describes what actually landed rather than what was staged, and every
read verifies size and digest before the bytes are used.

## The storage handle

`StorageKey` names internal storage topology and must not reach a client. The
API never serves the domain type directly: responses are projected through
`internal/dto`, which drops the handle. A test asserts a rendered response
contains neither the key nor the string `storage_key`.

## Fields

`id`, `name`, `size_bytes`, `sha256`, `created_at` — plus `StorageKey`,
internal only.
