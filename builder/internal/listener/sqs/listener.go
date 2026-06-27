package sqs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/dagflows/builder/internal/config"
	"github.com/dagflows/builder/internal/domain"
	"github.com/dagflows/builder/internal/service"
)

type Listener struct {
	client            *awssqs.Client
	deploymentService *service.DeploymentService
	requestQueue      string
	responseQueue     string
	options           receiveOptions
}

type receiveOptions struct {
	maxMessages              int32
	waitTimeSeconds          int32
	visibilityTimeoutSeconds int32
}

func NewListener(cfg config.Config, deploymentService *service.DeploymentService) (*Listener, error) {
	if cfg.SQS.RequestQueueURL == "" {
		return nil, fmt.Errorf("SQS_REQUEST_QUEUE_URL or SQS_QUEUE_URL is required")
	}
	if cfg.SQS.ResponseQueueURL == "" {
		return nil, fmt.Errorf("SQS_RESPONSE_QUEUE_URL or SQS_RESULT_QUEUE_URL is required")
	}

	awsCfg, err := loadAWSConfig(context.Background(), cfg)
	if err != nil {
		return nil, err
	}

	return &Listener{
		client:            awssqs.NewFromConfig(awsCfg),
		deploymentService: deploymentService,
		requestQueue:      cfg.SQS.RequestQueueURL,
		responseQueue:     cfg.SQS.ResponseQueueURL,
		options: receiveOptions{
			maxMessages:              cfg.SQS.MaxMessages,
			waitTimeSeconds:          cfg.SQS.WaitTimeSeconds,
			visibilityTimeoutSeconds: cfg.SQS.VisibilityTimeoutSeconds,
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

	response, err := l.deploymentService.BuildDeployment(ctx, req)
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

func loadAWSConfig(ctx context.Context, cfg config.Config) (aws.Config, error) {
	var options []func(*awsconfig.LoadOptions) error
	if cfg.AWS.Region != "" {
		options = append(options, awsconfig.WithRegion(cfg.AWS.Region))
	}
	if cfg.AWS.AccessKeyID != "" || cfg.AWS.SecretAccessKey != "" || cfg.AWS.SessionToken != "" {
		if cfg.AWS.AccessKeyID == "" || cfg.AWS.SecretAccessKey == "" {
			return aws.Config{}, fmt.Errorf("AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY are required when AWS credentials are configured")
		}
		options = append(options, awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AWS.AccessKeyID,
			cfg.AWS.SecretAccessKey,
			cfg.AWS.SessionToken,
		)))
	}
	return awsconfig.LoadDefaultConfig(ctx, options...)
}
