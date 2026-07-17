#!/bin/sh

set -eu

endpoint="${AWS_ENDPOINT_URL:-http://ministack:4566}"
region="${AWS_DEFAULT_REGION:-us-east-1}"
request_queue="${SQS_REQUEST_QUEUE_NAME:-builder-requests}"
response_queue="${SQS_RESPONSE_QUEUE_NAME:-builder-responses}"
bucket="${R2_BUCKET:-dagflows-builds}"

if [ "$request_queue" = "$response_queue" ]; then
    echo "SQS request and response queue names must be different" >&2
    exit 1
fi

aws_local() {
    aws --endpoint-url "$endpoint" --region "$region" "$@"
}

echo "Creating SQS queue: $request_queue"
aws_local sqs create-queue --queue-name "$request_queue"

echo "Creating SQS queue: $response_queue"
aws_local sqs create-queue --queue-name "$response_queue"

if aws_local s3api head-bucket --bucket "$bucket" >/dev/null 2>&1; then
    echo "S3-compatible R2 bucket already exists: $bucket"
else
    echo "Creating S3-compatible R2 bucket: $bucket"
    aws_local s3api create-bucket --bucket "$bucket"
fi
