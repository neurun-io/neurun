package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/dagflows/worker/internal/config"
	"github.com/dagflows/worker/internal/domain"
	"github.com/dagflows/worker/internal/dto"
	"github.com/dagflows/worker/internal/service"
)

type Listener struct {
	client      *goredis.Client
	requests    *streamConsumer
	responses   *streamPublisher
	nodeService *service.NodeRunService
	resources   memoryGate
	slots       chan struct{}
}

func NewListener(cfg config.Config, nodeService *service.NodeRunService) (*Listener, error) {
	if cfg.Streams.RequestStream == "" {
		return nil, fmt.Errorf("WORKER_REQUEST_STREAM is required")
	}
	if cfg.Streams.RequestGroup == "" {
		return nil, fmt.Errorf("WORKER_REQUEST_GROUP is required")
	}
	if cfg.Streams.ResponseStream == "" {
		return nil, fmt.Errorf("WORKER_RESPONSE_STREAM is required")
	}

	client := goredis.NewClient(&goredis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	if err := client.Ping(context.Background()).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("connect redis: %w", err)
	}

	requests, err := newStreamConsumer(client, cfg.Streams.RequestStream, cfg.Streams.RequestGroup, consumerName(), cfg.Streams.BlockDuration)
	if err != nil {
		_ = client.Close()
		return nil, err
	}

	return &Listener{
		client:      client,
		requests:    requests,
		responses:   newStreamPublisher(client, cfg.Streams.ResponseStream, cfg.Streams.MaxLen),
		nodeService: nodeService,
		slots:       make(chan struct{}, max(1, cfg.Worker.MaxConcurrency)),
	}, nil
}

func (l *Listener) Listen(ctx context.Context) error {
	defer l.client.Close()
	log.Printf("worker listening stream=%s group=%s runtime=firecracker max_concurrency=%d", l.requests.stream, l.requests.group, cap(l.slots))

	var wg sync.WaitGroup

listen:
	for {
		if err := ctx.Err(); err != nil {
			break
		}
		select {
		case l.slots <- struct{}{}:
		case <-ctx.Done():
			break listen
		}
		msg, err := l.requests.ReadOne(ctx)
		if err != nil {
			<-l.slots
			if errors.Is(err, context.Canceled) {
				break
			}
			log.Printf("read request failed: %v", err)
			if err := sleep(ctx, 2*time.Second); err != nil {
				break
			}
			continue
		}

		wg.Add(1)
		go func(msg streamMessage) {
			defer wg.Done()
			defer func() { <-l.slots }()
			if err := l.processMessage(ctx, msg); err != nil {
				log.Printf("process request id=%s failed: %v", msg.ID, err)
			}
		}(msg)
	}

	wg.Wait()
	return nil
}

func (l *Listener) processMessage(ctx context.Context, msg streamMessage) error {
	var req dto.WorkflowNodeRunRequest
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		log.Printf("request id=%s decode failed: %v", msg.ID, err)
		return l.requests.Ack(ctx, msg.ID)
	}

	log.Printf("node run received id=%s run=%s node=%s", msg.ID, req.WorkflowRunID, req.NodeKey)
	release, err := l.resources.Reserve(req.RequiredMemoryMB())
	if err != nil {
		log.Printf("node run blocked id=%s run=%s node=%s: %v", msg.ID, req.WorkflowRunID, req.NodeKey, err)
		return l.publishResponse(ctx, msg.ID, dto.WorkflowNodeRunResponse{
			WorkflowRunID: req.WorkflowRunID,
			NodeKey:       req.NodeKey,
			Status:        domain.WorkflowNodeRunAttemptStatusFailed,
			ErrorMessage:  err.Error(),
			ErrorCategory: "infrastructure",
			Retryable:     true,
		})
	}
	defer release()

	resp := l.nodeService.Execute(ctx, req)
	if err := l.publishResponse(ctx, msg.ID, resp); err != nil {
		return err
	}

	log.Printf("node run completed id=%s run=%s node=%s status=%s duration_ms=%d", msg.ID, resp.WorkflowRunID, resp.NodeKey, resp.Status, resp.DurationMs)
	return nil
}

func (l *Listener) publishResponse(ctx context.Context, id string, resp dto.WorkflowNodeRunResponse) error {
	body, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("marshal response: %w", err)
	}

	if err := l.responses.Publish(ctx, body); err != nil {
		return fmt.Errorf("publish response: %w", err)
	}
	if err := l.requests.Ack(ctx, id); err != nil {
		return fmt.Errorf("ack request: %w", err)
	}
	return nil
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
