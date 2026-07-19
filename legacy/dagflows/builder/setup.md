# Setup

## Requirements

Install these host tools:

```sh
git
go
aws
docker
```

On Windows, install Docker Desktop with:

```powershell
winget install --exact --id Docker.DockerDesktop
```

The `aws` CLI is only needed for sending and receiving local test messages when
the stack is run with Docker Compose.

## Environment

Copy the example env file:

```powershell
Copy-Item .env.example .env
```

The default `.env.example` is local-first and points both SQS and S3-compatible
storage at MiniStack on `http://localhost:4566`.

## Docker Compose

Start the builder and its local dependencies:

```powershell
docker compose up --build
```

Compose waits for MiniStack, runs the `provision` service, and creates these
resources before it starts the builder:

- `builder-requests` SQS request queue
- `builder-responses` SQS response queue
- `dagflows-builds` S3-compatible bucket used as local R2

The worker executes build commands from the repository in each request. Use
this local stack only with repository URLs you trust.

Resource creation is idempotent. Run it again without restarting the worker
with:

```powershell
docker compose run --rm provision
```

Names can be overridden with `SQS_REQUEST_QUEUE_NAME`,
`SQS_RESPONSE_QUEUE_NAME`, and `R2_BUCKET`. The exposed MiniStack port can be
changed with `MINISTACK_PORT` and binds to `127.0.0.1` by default;
container-to-container traffic always uses `http://ministack:4566`.

## Run on the Host

Start only MiniStack and provision its resources:

```powershell
docker compose up -d ministack
docker compose run --rm provision
```

Then configure dummy AWS credentials for the current shell. The AWS CLI needs
these even when talking to a local endpoint:

```powershell
$env:AWS_ACCESS_KEY_ID = "test"
$env:AWS_SECRET_ACCESS_KEY = "test"
$env:AWS_DEFAULT_REGION = "us-east-1"
```

The default local queue URLs are:

```txt
http://localhost:4566/000000000000/builder-requests
http://localhost:4566/000000000000/builder-responses
```

```powershell
go run ./cmd/builder
```

The worker logs message receipt, decode, fetch, node resolution, package build,
artifact upload, response send, request delete, final status, and failure
reasons.

## Send Test Message

PowerShell:

```powershell
aws --endpoint-url http://localhost:4566 sqs send-message `
  --queue-url http://localhost:4566/000000000000/builder-requests `
  --message-body '{"deployment_id":"local-deployment","workflow_id":"local-workflow","git_url":"https://github.com/Dagflows/python-example.git"}'
```

Use a Git URL that is reachable from the builder container. A local filesystem
path only works when the worker itself is running on the host.

## Read Responses

```powershell
aws --endpoint-url http://localhost:4566 sqs receive-message `
  --queue-url http://localhost:4566/000000000000/builder-responses `
  --wait-time-seconds 5
```

## AWS SQS and Cloudflare R2

For a deployed worker, create both queues in AWS and retain the returned queue
URLs:

```sh
aws sqs create-queue --queue-name builder-requests
aws sqs create-queue --queue-name builder-responses
```

Create the real R2 bucket with Wrangler:

```sh
npx wrangler login
npx wrangler r2 bucket create dagflows-builds
```

Alternatively, an R2 Admin Read & Write API token can create the bucket through
the S3-compatible API:

```sh
AWS_ACCESS_KEY_ID="$R2_ACCESS_KEY_ID" \
AWS_SECRET_ACCESS_KEY="$R2_SECRET_ACCESS_KEY" \
aws --endpoint-url "https://${R2_ACCOUNT_ID}.r2.cloudflarestorage.com" \
  --region auto s3api create-bucket --bucket "$R2_BUCKET"
```

Use a separate bucket-scoped Object Read & Write token for the running builder.
Set the returned AWS queue URLs and the R2 values in its environment, and do
not set `AWS_ENDPOINT_URL_SQS` outside local development.

For Cloudflare R2 instead of local MiniStack storage, configure:

```sh
R2_ACCOUNT_ID=
R2_ENDPOINT=
R2_REGION=auto
R2_BUCKET=dagflows-builds
R2_ACCESS_KEY_ID=
R2_SECRET_ACCESS_KEY=
R2_PREFIX=builds
```

Keep `R2_ENDPOINT` empty for Cloudflare R2. The builder derives the endpoint as
`https://<cloudflare-account-id>.r2.cloudflarestorage.com`.