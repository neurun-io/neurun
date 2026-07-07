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

The `aws` CLI is used for local SQS/S3 setup against Ministack.

## Environment

Copy the example env file:

```powershell
Copy-Item .env.example .env
```

The default `.env.example` is local-first and points both SQS and S3-compatible
storage at Ministack on `http://localhost:4566`.

## Ministack Resources

Start Ministack, then configure dummy AWS credentials for the current shell.
The AWS CLI needs these even when talking to a local endpoint:

```powershell
$env:AWS_ACCESS_KEY_ID = "test"
$env:AWS_SECRET_ACCESS_KEY = "test"
$env:AWS_DEFAULT_REGION = "us-east-1"
```

Create the request queue, response queue, and artifact bucket:

```powershell
aws --endpoint-url http://localhost:4566 sqs create-queue --queue-name builder-requests
aws --endpoint-url http://localhost:4566 sqs create-queue --queue-name builder-responses
aws --endpoint-url http://localhost:4566 s3 mb s3://dagflows-builds
```

The default local queue URLs are:

```txt
http://localhost:4566/000000000000/builder-requests
http://localhost:4566/000000000000/builder-responses
```

## Run Worker

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
  --message-body '{"deployment_id":"local-deployment","workflow_id":"local-workflow","git_url":"D:\\Documents\\m-workspace\\dagflows\\python-sdk"}'
```

Use a cloneable Git repo path or URL for `git_url`.

## Read Responses

```powershell
aws --endpoint-url http://localhost:4566 sqs receive-message `
  --queue-url http://localhost:4566/000000000000/builder-responses `
  --wait-time-seconds 5
```

## Cloudflare R2

For Cloudflare R2 instead of Ministack S3:

```sh
R2_ACCOUNT_ID=
R2_ENDPOINT=http://localhost:4566
R2_REGION=us-east-1
R2_BUCKET=dagflows-builds
R2_ACCESS_KEY_ID=test
R2_SECRET_ACCESS_KEY=test
R2_PREFIX=builds
```

Keep `R2_ENDPOINT` empty for Cloudflare R2. The builder derives the endpoint as
`https://<cloudflare-account-id>.r2.cloudflarestorage.com`.
