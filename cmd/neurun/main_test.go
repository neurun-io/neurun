package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dagflows/neurun-io/internal/config"
	"github.com/dagflows/neurun-io/internal/job"
)

func TestDoctorUsesConfiguredPublicURL(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(
		w http.ResponseWriter,
		request *http.Request,
	) {
		if request.URL.Path != "/base/healthz" {
			t.Errorf("path = %q", request.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	if err := doctor(config.Config{PublicURL: server.URL + "/base"}); err != nil {
		t.Fatal(err)
	}
}

func TestMaintainPublishesTransactionalOutbox(t *testing.T) {
	t.Parallel()
	repository := job.NewMemoryRepository()
	publisher := job.NewMemoryPublisher()
	request, err := job.NewRequest(
		"prj_test",
		job.FunctionRef{
			Name:    "system.echo",
			Version: "1",
			Digest:  "sha256:test",
		},
		json.RawMessage(`{"ok":true}`),
		job.RequestOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := repository.Accept(context.Background(), job.AcceptCommand{
		Request:        request,
		IdempotencyKey: "test-maintenance",
	})
	if err != nil {
		t.Fatal(err)
	}

	maintain(
		context.Background(),
		repository,
		job.Dispatcher{
			Outbox:    repository,
			Publisher: publisher,
			Owner:     "dsp_test",
		},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	if len(publisher.Messages()) != 1 {
		t.Fatalf("published messages = %d, want 1", len(publisher.Messages()))
	}
	snapshot, err := repository.Get(context.Background(), "prj_test", accepted.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.State != job.StateQueued {
		t.Fatalf("job state = %s, want %s", snapshot.State, job.StateQueued)
	}
}

func TestBoundedFinalizeTimeout(t *testing.T) {
	t.Parallel()
	tests := []struct {
		shutdown time.Duration
		want     time.Duration
	}{
		{shutdown: 20 * time.Second, want: 5 * time.Second},
		{shutdown: 6 * time.Second, want: 3 * time.Second},
		{shutdown: time.Second, want: 500 * time.Millisecond},
		{shutdown: 0, want: 5 * time.Second},
	}
	for _, test := range tests {
		if got := boundedFinalizeTimeout(test.shutdown); got != test.want {
			t.Errorf("boundedFinalizeTimeout(%s) = %s, want %s",
				test.shutdown, got, test.want)
		}
	}
}
