package github

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

var secret = []byte("a-webhook-secret")

func delivery(t *testing.T, event, body string, sign []byte) *http.Request {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/v1/github/webhook", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Github-Event", event)
	request.Header.Set("X-Github-Delivery", "d-1")
	if sign != nil {
		mac := hmac.New(sha256.New, sign)
		mac.Write([]byte(body))
		request.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	return request
}

func testClient(t *testing.T) *Client {
	t.Helper()
	client, err := New(Options{
		AppID: 1, PrivateKey: []byte("key"), WebhookSecret: secret,
	})
	if err != nil {
		t.Fatalf("construct client: %v", err)
	}
	return client
}

const pushBody = `{
  "ref": "refs/heads/main",
  "after": "9fceb02f1e4d3c5a6b7d8e9f0a1b2c3d4e5f6a7b",
  "deleted": false,
  "repository": {"full_name": "acme/scraper", "default_branch": "main"},
  "installation": {"id": 42}
}`

func TestParsePushAcceptsASignedPush(t *testing.T) {
	t.Parallel()
	push, deployable, err := testClient(t).ParsePush(
		delivery(t, "push", pushBody, secret),
	)
	if err != nil {
		t.Fatalf("parse push: %v", err)
	}
	if !deployable {
		t.Fatal("a push to a branch should be deployable")
	}
	if push.InstallationID != 42 || push.Repository != "acme/scraper" ||
		push.Ref != "refs/heads/main" || push.DefaultBranch != "main" ||
		push.Commit != "9fceb02f1e4d3c5a6b7d8e9f0a1b2c3d4e5f6a7b" ||
		push.Delivery != "d-1" {
		t.Fatalf("push = %#v", push)
	}
}

func TestParsePushRejectsAnUnsignedOrTamperedDelivery(t *testing.T) {
	t.Parallel()
	client := testClient(t)

	for name, request := range map[string]*http.Request{
		"unsigned":     delivery(t, "push", pushBody, nil),
		"wrong secret": delivery(t, "push", pushBody, []byte("not-the-secret")),
	} {
		if _, _, err := client.ParsePush(request); !errors.Is(err, ErrSignature) {
			t.Fatalf("%s: err = %v, want ErrSignature", name, err)
		}
	}

	// A body altered after signing must not verify against its own signature.
	signed := delivery(t, "push", pushBody, secret)
	tampered := delivery(t, "push", strings.Replace(pushBody, "acme", "evil", 1), nil)
	tampered.Header.Set("X-Hub-Signature-256", signed.Header.Get("X-Hub-Signature-256"))
	if _, _, err := client.ParsePush(tampered); !errors.Is(err, ErrSignature) {
		t.Fatalf("tampered: err = %v, want ErrSignature", err)
	}
}

// A signed delivery the server has no work for is not an error: it is answered,
// not retried.
func TestParsePushIgnoresDeliveriesWithNothingToBuild(t *testing.T) {
	t.Parallel()
	client := testClient(t)

	bodies := map[string]string{
		"other event":    pushBody,
		"branch deleted": strings.Replace(pushBody, `"deleted": false`, `"deleted": true`, 1),
		"deleted ref sha": strings.Replace(
			pushBody, "9fceb02f1e4d3c5a6b7d8e9f0a1b2c3d4e5f6a7b", zeroCommit, 1,
		),
		"no installation": strings.Replace(pushBody, `"id": 42`, `"id": 0`, 1),
	}
	for name, body := range bodies {
		event := "push"
		if name == "other event" {
			event = "installation"
		}
		_, deployable, err := client.ParsePush(delivery(t, event, body, secret))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if deployable {
			t.Fatalf("%s: should not be deployable", name)
		}
	}
}

func TestParsePushRefusesWithoutAWebhookSecret(t *testing.T) {
	t.Parallel()
	client, err := New(Options{AppID: 1, PrivateKey: []byte("key")})
	if err != nil {
		t.Fatalf("construct client: %v", err)
	}
	if _, _, err := client.ParsePush(
		delivery(t, "push", pushBody, secret),
	); !errors.Is(err, ErrNoWebhookSecret) {
		t.Fatalf("err = %v, want ErrNoWebhookSecret", err)
	}
}
