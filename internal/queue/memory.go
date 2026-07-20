package queue

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/dagflows/neurun-io/internal/job"
)

var (
	ErrDeliverySettled = errors.New("queue delivery is already settled")
	ErrDeliveryLost    = errors.New("queue delivery ownership is lost")
)

type record struct {
	message       job.Message
	acknowledged  bool
	delivery      uint64
	leasedUntil   time.Time
	lastDelivered time.Time
}

type Delivery interface {
	Message() job.Message
	Ack() error
	Nack() error
	Extend(time.Duration) error
}

type Consumer interface {
	Next(context.Context) (Delivery, error)
}

// MemoryBroker is a deterministic, at-least-once development broker. It keeps
// acknowledged message IDs for deduplication and redelivers abandoned
// deliveries after the visibility timeout.
type MemoryBroker struct {
	mu                sync.Mutex
	records           map[string]*record
	order             []string
	visibilityTimeout time.Duration
	notify            chan struct{}
	now               func() time.Time
}

func NewMemoryBroker(visibilityTimeout time.Duration) *MemoryBroker {
	if visibilityTimeout <= 0 {
		visibilityTimeout = 30 * time.Second
	}
	return &MemoryBroker{
		records:           make(map[string]*record),
		visibilityTimeout: visibilityTimeout,
		notify:            make(chan struct{}, 1),
		now:               time.Now,
	}
}

func (b *MemoryBroker) Publish(ctx context.Context, message job.Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if message.ID == "" || message.Topic == "" {
		return errors.New("queue message ID and topic are required")
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if existing := b.records[message.ID]; existing != nil {
		if existing.message.Topic != message.Topic ||
			!bytes.Equal(existing.message.Payload, message.Payload) {
			return fmt.Errorf("queue message ID %s was reused with different content", message.ID)
		}
		return nil
	}
	b.records[message.ID] = &record{
		message: job.Message{
			ID:      message.ID,
			Topic:   message.Topic,
			Payload: append([]byte(nil), message.Payload...),
		},
	}
	b.order = append(b.order, message.ID)
	b.signal()
	return nil
}

func (b *MemoryBroker) Next(ctx context.Context) (Delivery, error) {
	for {
		b.mu.Lock()
		if err := ctx.Err(); err != nil {
			b.mu.Unlock()
			return nil, err
		}
		now := b.now().UTC()
		for _, id := range b.order {
			current := b.records[id]
			if current == nil || current.acknowledged || current.leasedUntil.After(now) {
				continue
			}
			current.delivery++
			current.lastDelivered = now
			current.leasedUntil = now.Add(b.visibilityTimeout)
			delivery := &memoryDelivery{
				broker:     b,
				messageID:  id,
				generation: current.delivery,
				message: job.Message{
					ID:      current.message.ID,
					Topic:   current.message.Topic,
					Payload: append([]byte(nil), current.message.Payload...),
				},
			}
			b.mu.Unlock()
			return delivery, nil
		}
		b.mu.Unlock()

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-b.notify:
		case <-time.After(min(b.visibilityTimeout, 100*time.Millisecond)):
		}
	}
}

func (b *MemoryBroker) Pending() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	count := 0
	for _, current := range b.records {
		if !current.acknowledged {
			count++
		}
	}
	return count
}

func (b *MemoryBroker) signal() {
	select {
	case b.notify <- struct{}{}:
	default:
	}
}

type memoryDelivery struct {
	mu         sync.Mutex
	broker     *MemoryBroker
	messageID  string
	generation uint64
	settled    bool
	message    job.Message
}

func (d *memoryDelivery) Message() job.Message {
	return job.Message{
		ID:      d.message.ID,
		Topic:   d.message.Topic,
		Payload: append([]byte(nil), d.message.Payload...),
	}
}

func (d *memoryDelivery) Ack() error {
	return d.settle(true)
}

func (d *memoryDelivery) Nack() error {
	return d.settle(false)
}

func (d *memoryDelivery) Extend(duration time.Duration) error {
	if duration <= 0 {
		return errors.New("visibility extension must be positive")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.settled {
		return ErrDeliverySettled
	}

	d.broker.mu.Lock()
	defer d.broker.mu.Unlock()
	current := d.broker.records[d.messageID]
	if current == nil || current.acknowledged || current.delivery != d.generation {
		return ErrDeliveryLost
	}
	now := d.broker.now().UTC()
	if !current.leasedUntil.After(now) {
		return ErrDeliveryLost
	}
	current.leasedUntil = now.Add(duration)
	return nil
}

func (d *memoryDelivery) settle(acknowledge bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.settled {
		return ErrDeliverySettled
	}

	d.broker.mu.Lock()
	defer d.broker.mu.Unlock()
	current := d.broker.records[d.messageID]
	if current == nil || current.acknowledged || current.delivery != d.generation {
		d.settled = true
		return ErrDeliveryLost
	}
	if !current.leasedUntil.After(d.broker.now().UTC()) {
		d.settled = true
		return ErrDeliveryLost
	}
	if acknowledge {
		current.acknowledged = true
	} else {
		current.leasedUntil = time.Time{}
		d.broker.signal()
	}
	d.settled = true
	return nil
}

var _ job.Publisher = (*MemoryBroker)(nil)
var _ Consumer = (*MemoryBroker)(nil)
