# Dagflows to Neurun migration boundary

This repository preserves the complete Builder and Worker commit lineage while
changing the product boundary.

## Retained as lineage

The source trees remain under:

- `legacy/dagflows/builder`
- `legacy/dagflows/worker`

They are nested Go modules and are excluded from the active server build. They
exist to keep the imported code reviewable and to make the migration
provenance explicit.

## Refactored concepts

| Dagflows concept | Neurun destination |
| --- | --- |
| Artifact SHA-256 and size metadata | immutable artifact records |
| S3/R2 object abstraction | local filesystem artifact adapter behind a generic blob-store boundary |
| Worker bounded slots and memory gate | agent admission control |
| Publish result before acknowledging work | transactional finalization before queue acknowledgement |
| Firecracker process-group cleanup | Chromium process-group and cgroup cleanup |
| Structured execution errors | public failure categories and retry policy |
| Signal-aware service shutdown | every Neurun command |

## Deliberately retired

- cloning and compiling arbitrary user repositories;
- `python -m dagflows inspect` and dynamic workflow-node discovery;
- SQS deployment messages;
- Redis Streams as work ownership;
- Python artifact layers and the guest invocation protocol;
- Firecracker as the primary MVP runtime;
- runtime-selected native plugins.

Neurun ships built-in immutable atomic functions. PostgreSQL owns jobs, leases,
attempts, and final state; NATS JetStream transports references. Chromium and
HTTP are the only initial execution runtimes.

## Historical safety

The only historically tracked `.env` contained empty object-store credential
values. No local ignored `.env`, Go cache, VM image, Firecracker binary, or
generated runtime asset was imported from the working copies.
