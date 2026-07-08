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
	"github.com/dagflows/worker/internal/service"
)

type Listener struct {
	client      *goredis.Client
	requests    *streamConsumer
	responses   *streamPublisher
	nodeService *service.NodeRunService
	capacity    hostCapacityGate
	active      activeTracker
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

	consumer := cfg.Worker.ID
	if consumer == "" {
		consumer = consumerName()
	}

	requests, err := newStreamConsumer(client, cfg.Streams.RequestStream, cfg.Streams.RequestGroup, consumer, cfg.Streams.MinIdle, cfg.Streams.BlockDuration)
	if err != nil {
		_ = client.Close()
		return nil, err
	}

	return &Listener{
		client:      client,
		requests:    requests,
		responses:   newStreamPublisher(client, cfg.Streams.ResponseStream, cfg.Streams.MaxLen),
		nodeService: nodeService,
		capacity:    newHostCapacityGate(cfg.Worker.MinFreeMemoryMB),
	}, nil
}

func (l *Listener) Listen(ctx context.Context) error {
	defer l.client.Close()
	log.Printf("worker listening stream=%s group=%s runtime=firecracker concurrency=capacity-gated", l.requests.stream, l.requests.group)

	var wg sync.WaitGroup
	for {
		if err := ctx.Err(); err != nil {
			break
		}
		if err := l.capacity.Wait(ctx); err != nil {
			break
		}

		msg, err := l.requests.ReadOne(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				break
			}
			log.Printf("read request failed: %v", err)
			if err := sleep(ctx, 2*time.Second); err != nil {
				break
			}
			continue
		}

		if !l.active.Mark(msg.ID) {
			log.Printf("request id=%s is already active locally; skipping duplicate delivery", msg.ID)
			continue
		}

		wg.Add(1)
		go func(msg streamMessage) {
			defer wg.Done()
			defer l.active.Unmark(msg.ID)
			if err := l.processMessage(ctx, msg); err != nil {
				log.Printf("process request id=%s failed: %v", msg.ID, err)
			}
		}(msg)
	}

	wg.Wait()
	return nil
}

func (l *Listener) processMessage(ctx context.Context, msg streamMessage) error {
	var req domain.WorkflowNodeRunRequest
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		log.Printf("request id=%s decode failed: %v", msg.ID, err)
		return l.requests.Ack(ctx, msg.ID)
	}

	log.Printf("node run received id=%s run=%s node=%s token=%d", msg.ID, req.WorkflowRunID, req.NodeKey, req.ExecutionToken)
	resp := l.nodeService.Execute(ctx, req)
	body, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("marshal response: %w", err)
	}

	if err := l.responses.Publish(ctx, body); err != nil {
		return fmt.Errorf("publish response: %w", err)
	}
	if err := l.requests.Ack(ctx, msg.ID); err != nil {
		return fmt.Errorf("ack request: %w", err)
	}

	log.Printf("node run completed id=%s run=%s node=%s status=%s duration_ms=%d", msg.ID, resp.WorkflowRunID, resp.NodeKey, resp.Status, resp.DurationMs)
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
