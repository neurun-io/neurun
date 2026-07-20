package queue

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/dagflows/neurun-io/internal/job"
)

func TestMemoryBrokerDeduplicatesAndAcknowledges(t *testing.T) {
	t.Parallel()

	broker := NewMemoryBroker(time.Second)
	message := job.Message{ID: "msg_1", Topic: "jobs", Payload: []byte(`{"job_id":"job_1"}`)}
	if err := broker.Publish(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	if err := broker.Publish(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	if broker.Pending() != 1 {
		t.Fatalf("pending = %d", broker.Pending())
	}

	delivery, err := broker.Next(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := delivery.Ack(); err != nil {
		t.Fatal(err)
	}
	if broker.Pending() != 0 {
		t.Fatalf("pending after ack = %d", broker.Pending())
	}
	if err := delivery.Ack(); !errors.Is(err, ErrDeliverySettled) {
		t.Fatalf("second ack error = %v", err)
	}
}

func TestMemoryBrokerNackRedelivers(t *testing.T) {
	t.Parallel()

	broker := NewMemoryBroker(time.Second)
	message := job.Message{ID: "msg_1", Topic: "jobs", Payload: []byte(`{}`)}
	if err := broker.Publish(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	first, err := broker.Next(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Nack(); err != nil {
		t.Fatal(err)
	}
	second, err := broker.Next(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if second.Message().ID != first.Message().ID {
		t.Fatalf("redelivered ID = %q", second.Message().ID)
	}
	if err := second.Ack(); err != nil {
		t.Fatal(err)
	}
}

func TestMemoryBrokerNextHonorsContext(t *testing.T) {
	t.Parallel()

	broker := NewMemoryBroker(time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := broker.Next(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Next error = %v", err)
	}
}

func TestMemoryBrokerCanceledNextDoesNotClaimAvailableMessage(t *testing.T) {
	t.Parallel()

	broker := NewMemoryBroker(time.Second)
	message := job.Message{ID: "msg_1", Topic: "jobs", Payload: []byte(`{}`)}
	if err := broker.Publish(context.Background(), message); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := broker.Next(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Next error = %v", err)
	}

	delivery, err := broker.Next(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := delivery.Ack(); err != nil {
		t.Fatal(err)
	}
}

func TestMemoryBrokerRejectsSettlementAtVisibilityDeadline(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
	broker := NewMemoryBroker(time.Second)
	broker.now = func() time.Time { return now }
	if err := broker.Publish(context.Background(), job.Message{
		ID: "msg_1", Topic: "jobs", Payload: []byte(`{}`),
	}); err != nil {
		t.Fatal(err)
	}

	expired, err := broker.Next(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Second)
	if err := expired.Ack(); !errors.Is(err, ErrDeliveryLost) {
		t.Fatalf("expired Ack error = %v", err)
	}

	redelivered, err := broker.Next(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := redelivered.Ack(); err != nil {
		t.Fatal(err)
	}
}

func TestMemoryBrokerConcurrentPublishDeduplicates(t *testing.T) {
	t.Parallel()

	broker := NewMemoryBroker(time.Second)
	message := job.Message{
		ID: "msg_1", Topic: "jobs", Payload: []byte(`{"job_id":"job_1"}`),
	}
	const publishers = 32
	errs := make(chan error, publishers)
	var wait sync.WaitGroup
	for range publishers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			errs <- broker.Publish(context.Background(), message)
		}()
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if pending := broker.Pending(); pending != 1 {
		t.Fatalf("pending = %d, want 1", pending)
	}
}

func TestMemoryBrokerRejectsMessageIDContentConflict(t *testing.T) {
	t.Parallel()

	broker := NewMemoryBroker(time.Second)
	if err := broker.Publish(context.Background(), job.Message{
		ID: "msg_1", Topic: "jobs", Payload: []byte(`{"attempt":1}`),
	}); err != nil {
		t.Fatal(err)
	}
	if err := broker.Publish(context.Background(), job.Message{
		ID: "msg_1", Topic: "jobs", Payload: []byte(`{"attempt":2}`),
	}); err == nil {
		t.Fatal("message ID content reuse should fail")
	}
}
