# Artifact

An immutable blob: an uploaded source ZIP, or a layer a build produced.

## Kinds

- `deployment_source` — the ZIP as uploaded.
- `code_layer` — compiled source, packaged.
- `install_layer` — dependencies from `requirements.txt`, when there are any.

A build carries at most one of each layer kind, and a `ready` build must have a
code layer.

## Content addressing

Blobs are stored under `objects/sha256/<first two>/<digest>`, so identical
content is stored once regardless of who uploaded it. Writing an existing key
is not an error — the store verifies the existing blob's digest matches before
reusing it, so a corrupted blob can never be silently adopted.

Every read verifies size and digest against the metadata before the bytes are
used.

## The storage handle

`StorageKey` names internal storage topology and must not reach a client. The
API never serves the domain type directly: responses are projected through
`internal/dto`, which drops the handle. A test asserts a rendered response
contains neither the key nor the string `storage_key`.

## Fields

`id`, `kind`, `name`, `media_type`, `size_bytes`, `sha256`, `created_at` —
plus `StorageKey`, internal only.
