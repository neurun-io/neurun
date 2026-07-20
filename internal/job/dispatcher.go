package job

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// DispatchReport summarizes one bounded transactional-outbox pass.
type DispatchReport struct {
	Claimed   int
	Published int
	Failed    int
}

// Dispatcher publishes claimed rows and records the result. If publication
// succeeds but acknowledgement fails, the claim expires and the same
// deterministic Message.ID is published again. JetStream can therefore dedupe
// the ambiguous publish.
type Dispatcher struct {
	Outbox    OutboxRepository
	Publisher Publisher
	Owner     string
	BatchSize int
	ClaimTTL  time.Duration
}

func (dispatcher Dispatcher) DispatchOnce(ctx context.Context) (DispatchReport, error) {
	if dispatcher.Outbox == nil {
		return DispatchReport{}, fmt.Errorf("%w: outbox repository is required", ErrInvalid)
	}
	if dispatcher.Publisher == nil {
		return DispatchReport{}, fmt.Errorf("%w: publisher is required", ErrInvalid)
	}
	if dispatcher.Owner == "" {
		return DispatchReport{}, fmt.Errorf("%w: dispatcher owner is required", ErrInvalid)
	}
	batchSize := dispatcher.BatchSize
	if batchSize == 0 {
		batchSize = 100
	}
	claimTTL := dispatcher.ClaimTTL
	if claimTTL == 0 {
		claimTTL = 30 * time.Second
	}
	claims, err := dispatcher.Outbox.ClaimOutbox(ctx, ClaimOutboxCommand{
		Owner: dispatcher.Owner,
		Limit: batchSize,
		TTL:   claimTTL,
	})
	if err != nil {
		return DispatchReport{}, err
	}

	report := DispatchReport{Claimed: len(claims)}
	var dispatchErrors []error
	for _, claim := range claims {
		message := Message{
			ID:      claim.Outbox.MessageID,
			Topic:   claim.Outbox.Topic,
			Payload: cloneBytes(claim.Outbox.Payload),
		}
		if err := dispatcher.Publisher.Publish(ctx, message); err != nil {
			report.Failed++
			if _, markErr := dispatcher.Outbox.MarkOutboxFailed(ctx, claim.Outbox.ID, claim.Token, err.Error()); markErr != nil {
				dispatchErrors = append(dispatchErrors, fmt.Errorf("record publish failure for %s: %w", claim.Outbox.ID, markErr))
			}
			dispatchErrors = append(dispatchErrors, fmt.Errorf("publish %s: %w", claim.Outbox.ID, err))
			continue
		}
		if _, err := dispatcher.Outbox.MarkOutboxPublished(ctx, claim.Outbox.ID, claim.Token); err != nil {
			report.Failed++
			dispatchErrors = append(dispatchErrors, fmt.Errorf("acknowledge publish %s: %w", claim.Outbox.ID, err))
			continue
		}
		report.Published++
	}
	return report, errors.Join(dispatchErrors...)
}

type publishedRecord struct {
	message Message
}

// MemoryPublisher is a race-safe development queue adapter with message-ID
// deduplication. It models the JetStream behavior on which outbox retries rely.
type MemoryPublisher struct {
	mu    sync.RWMutex
	byID  map[string]publishedRecord
	order []string
}

func NewMemoryPublisher() *MemoryPublisher {
	return &MemoryPublisher{byID: make(map[string]publishedRecord)}
}

func (publisher *MemoryPublisher) Publish(ctx context.Context, message Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if message.ID == "" || message.Topic == "" {
		return fmt.Errorf("%w: message id and topic are required", ErrInvalid)
	}
	publisher.mu.Lock()
	defer publisher.mu.Unlock()
	if existing, ok := publisher.byID[message.ID]; ok {
		if existing.message.Topic != message.Topic ||
			!bytes.Equal(existing.message.Payload, message.Payload) {
			return ErrMessageIDConflict
		}
		return nil
	}
	stored := Message{
		ID:      message.ID,
		Topic:   message.Topic,
		Payload: cloneBytes(message.Payload),
	}
	publisher.byID[message.ID] = publishedRecord{message: stored}
	publisher.order = append(publisher.order, message.ID)
	return nil
}

// Messages returns published messages in first-publication order.
func (publisher *MemoryPublisher) Messages() []Message {
	publisher.mu.RLock()
	defer publisher.mu.RUnlock()
	result := make([]Message, 0, len(publisher.order))
	for _, messageID := range publisher.order {
		message := publisher.byID[messageID].message
		message.Payload = cloneBytes(message.Payload)
		result = append(result, message)
	}
	return result
}

var _ Publisher = (*MemoryPublisher)(nil)
