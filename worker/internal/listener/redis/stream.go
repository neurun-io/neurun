package redis

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

type streamMessage struct {
	ID   string
	Data []byte
}

type streamConsumer struct {
	client        *goredis.Client
	stream        string
	group         string
	consumer      string
	minIdle       time.Duration
	blockDuration time.Duration
}

type streamPublisher struct {
	client *goredis.Client
	stream string
	maxLen int64
}

func newStreamConsumer(client *goredis.Client, stream, group, consumer string, minIdle, blockDuration time.Duration) (*streamConsumer, error) {
	if err := client.XGroupCreateMkStream(context.Background(), stream, group, "$").Err(); err != nil {
		if !strings.Contains(err.Error(), "BUSYGROUP") {
			return nil, fmt.Errorf("create redis stream group %q on %q: %w", group, stream, err)
		}
	}
	return &streamConsumer{
		client:        client,
		stream:        stream,
		group:         group,
		consumer:      consumer,
		minIdle:       minIdle,
		blockDuration: blockDuration,
	}, nil
}

func newStreamPublisher(client *goredis.Client, stream string, maxLen int64) *streamPublisher {
	return &streamPublisher{client: client, stream: stream, maxLen: maxLen}
}

func (s *streamConsumer) ReadOne(ctx context.Context) (streamMessage, error) {
	for {
		select {
		case <-ctx.Done():
			return streamMessage{}, ctx.Err()
		default:
		}

		claimed, _, err := s.client.XAutoClaim(ctx, &goredis.XAutoClaimArgs{
			Stream:   s.stream,
			Group:    s.group,
			Consumer: s.consumer,
			MinIdle:  s.minIdle,
			Start:    "0",
			Count:    1,
		}).Result()
		if err != nil && err != goredis.Nil {
			return streamMessage{}, fmt.Errorf("xautoclaim from %s: %w", s.stream, err)
		}
		if len(claimed) > 0 {
			return parseStreamEntry(claimed[0])
		}

		results, err := s.client.XReadGroup(ctx, &goredis.XReadGroupArgs{
			Group:    s.group,
			Consumer: s.consumer,
			Streams:  []string{s.stream, ">"},
			Count:    1,
			Block:    s.blockDuration,
		}).Result()
		if err == goredis.Nil {
			continue
		}
		if err != nil {
			return streamMessage{}, fmt.Errorf("xreadgroup from %s: %w", s.stream, err)
		}
		if len(results) == 0 || len(results[0].Messages) == 0 {
			continue
		}
		return parseStreamEntry(results[0].Messages[0])
	}
}

func (s *streamConsumer) Ack(ctx context.Context, id string) error {
	return s.client.XAck(ctx, s.stream, s.group, id).Err()
}

func (p *streamPublisher) Publish(ctx context.Context, data []byte) error {
	args := &goredis.XAddArgs{
		Stream: p.stream,
		Values: map[string]any{"data": string(data)},
	}
	if p.maxLen > 0 {
		args.MaxLen = p.maxLen
		args.Approx = true
	}
	return p.client.XAdd(ctx, args).Err()
}

func parseStreamEntry(msg goredis.XMessage) (streamMessage, error) {
	raw, ok := msg.Values["data"]
	if !ok {
		return streamMessage{}, fmt.Errorf("invalid stream entry %s: missing data field", msg.ID)
	}
	switch value := raw.(type) {
	case string:
		return streamMessage{ID: msg.ID, Data: []byte(value)}, nil
	case []byte:
		return streamMessage{ID: msg.ID, Data: value}, nil
	default:
		return streamMessage{}, fmt.Errorf("invalid stream entry %s: data field has type %T", msg.ID, raw)
	}
}

func consumerName() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "agent"
	}
	return fmt.Sprintf("%s-%d", host, os.Getpid())
}
