package sqs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/dagflows/builder/internal/domain"
)

const (
	defaultMaxMessages       = 1
	defaultWaitSeconds       = 20
	defaultVisibilitySeconds = 900
)

type DeploymentBuilder interface {
	BuildDeployment(ctx context.Context, req domain.DeploymentRequest) (domain.DeploymentResponse, error)
}

type Listener struct {
	client        *awssqs.Client
	builder       DeploymentBuilder
	requestQueue  string
	responseQueue string
	options       receiveOptions
}

type receiveOptions struct {
	maxMessages              int32
	waitTimeSeconds          int32
	visibilityTimeoutSeconds int32
}

func NewListenerFromEnv(builder DeploymentBuilder) (*Listener, error) {
	requestQueue := firstEnv("SQS_REQUEST_QUEUE_URL", "SQS_QUEUE_URL")
	responseQueue := firstEnv("SQS_RESPONSE_QUEUE_URL", "SQS_RESULT_QUEUE_URL")
	if requestQueue == "" {
		return nil, fmt.Errorf("SQS_REQUEST_QUEUE_URL or SQS_QUEUE_URL is required")
	}
	if responseQueue == "" {
		return nil, fmt.Errorf("SQS_RESPONSE_QUEUE_URL or SQS_RESULT_QUEUE_URL is required")
	}

	cfg, err := loadAWSConfig(context.Background())
	if err != nil {
		return nil, err
	}

	return &Listener{
		client:        awssqs.NewFromConfig(cfg),
		builder:       builder,
		requestQueue:  requestQueue,
		responseQueue: responseQueue,
		options: receiveOptions{
			maxMessages:              envInt32Or("SQS_MAX_MESSAGES", defaultMaxMessages),
			waitTimeSeconds:          envInt32Or("SQS_WAIT_TIME_SECONDS", defaultWaitSeconds),
			visibilityTimeoutSeconds: envInt32Or("SQS_VISIBILITY_TIMEOUT_SECONDS", defaultVisibilitySeconds),
		},
	}, nil
}

func (l *Listener) Listen(ctx context.Context) error {
	log.Printf("builder listening on SQS queue %s", l.requestQueue)
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}

		output, err := l.client.ReceiveMessage(ctx, &awssqs.ReceiveMessageInput{
			QueueUrl:              aws.String(l.requestQueue),
			MaxNumberOfMessages:   l.options.maxMessages,
			WaitTimeSeconds:       l.options.waitTimeSeconds,
			VisibilityTimeout:     l.options.visibilityTimeoutSeconds,
			AttributeNames:        []types.QueueAttributeName{types.QueueAttributeNameAll},
			MessageAttributeNames: []string{"All"},
		})
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			log.Printf("receive SQS messages: %v", err)
			if err := sleep(ctx, 5*time.Second); err != nil {
				return nil
			}
			continue
		}

		for _, message := range output.Messages {
			if err := l.processReceivedMessage(ctx, message); err != nil {
				log.Printf("process SQS message %s: %v", aws.ToString(message.MessageId), err)
			}
		}
	}
}

func (l *Listener) processReceivedMessage(ctx context.Context, message types.Message) error {
	response := l.processMessage(ctx, aws.ToString(message.Body))
	responseBody, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("encode response: %w", err)
	}

	if _, err := l.client.SendMessage(ctx, &awssqs.SendMessageInput{
		QueueUrl:    aws.String(l.responseQueue),
		MessageBody: aws.String(string(responseBody)),
	}); err != nil {
		return fmt.Errorf("send response: %w", err)
	}
	if _, err := l.client.DeleteMessage(ctx, &awssqs.DeleteMessageInput{
		QueueUrl:      aws.String(l.requestQueue),
		ReceiptHandle: message.ReceiptHandle,
	}); err != nil {
		return fmt.Errorf("delete request: %w", err)
	}

	log.Printf("deployment %s finished with status %s", response.DeploymentID, response.Status)
	return nil
}

func (l *Listener) processMessage(ctx context.Context, body string) domain.DeploymentResponse {
	var req domain.DeploymentRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		return failedResponse(extractDeploymentID(body), fmt.Errorf("decode request: %w", err))
	}

	response, err := l.builder.BuildDeployment(ctx, req)
	if err != nil {
		return failedResponse(req.DeploymentID, err)
	}
	return response
}

func failedResponse(deploymentID string, err error) domain.DeploymentResponse {
	return domain.DeploymentResponse{
		DeploymentID: deploymentID,
		Status:       domain.DeploymentStatusFailed,
		ErrorMessage: err.Error(),
		Nodes:        []domain.WorkflowNode{},
	}
}

func extractDeploymentID(body string) string {
	var payload struct {
		DeploymentID string `json:"deployment_id"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return ""
	}
	return payload.DeploymentID
}

func sleep(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func firstEnv(names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ""
}

func loadAWSConfig(ctx context.Context) (aws.Config, error) {
	region := firstEnv("SQS_REGION", "AWS_REGION", "AWS_DEFAULT_REGION")
	if region == "" {
		return config.LoadDefaultConfig(ctx)
	}
	return config.LoadDefaultConfig(ctx, config.WithRegion(region))
}

func envInt32Or(name string, fallback int32) int32 {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	if name == "SQS_MAX_MESSAGES" && parsed > 10 {
		return 10
	}
	return int32(parsed)
}
