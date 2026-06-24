# Dagflows Builder

Small gRPC build service for packaging apps into deployable artifacts.

## Runtimes

- Python: installs `requirements.txt` into `install-layer.zip`, compiles source, uploads `code-layer.zip`.
- Node: runs `npm ci` or `npm install`, runs `npm run build` when present, prunes dev deps, uploads `install-layer.zip` and `code-layer.zip`.
- Go: builds a binary and uploads it directly as a deployable.

## Config

```sh
GRPC_PORT=50051

R2_ACCOUNT_ID=<account-id>
R2_ENDPOINT=
R2_BUCKET=<bucket>
R2_ACCESS_KEY_ID=<access-key-id>
R2_SECRET_ACCESS_KEY=<secret-access-key>
R2_PREFIX=builds
```

`.env` is loaded on startup. If `R2_ENDPOINT` is blank, the service uses Cloudflare's documented S3 endpoint format: `https://<R2_ACCOUNT_ID>.r2.cloudflarestorage.com`. `R2_BUCKET` has no provider default and must be set.

Host tools must be installed: `python`, `npm`, and `go`.

## Layout

- DTOs: `proto/builder/v1`
- Domain: `internal/domain`
- Handlers: `internal/handler/grpc`
- Build service: `internal/service`
- Shared helpers: `pkg`
- Storage: `internal/storage`

## Run

```sh
go run ./cmd/builderd
```

## API

See `proto/builder/v1/builder.proto`.

`BuildRequest.source_path` is a directory visible to the builder host. The response returns uploaded R2 bucket/key pairs plus SHA-256 and size for each artifact.
