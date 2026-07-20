package httpruntime

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/dagflows/neurun-io/internal/function"
	"github.com/dagflows/neurun-io/internal/netpolicy"
)

type doerFunc func(*http.Request) (*http.Response, error)

func (do doerFunc) Do(request *http.Request) (*http.Response, error) {
	return do(request)
}

func TestNewRequiresPolicyClient(t *testing.T) {
	t.Parallel()

	if _, err := New(nil); err == nil {
		t.Fatal("New(nil) error = nil, want client requirement")
	}
}

func TestFetchUsesNetworkPolicyToRejectPrivateTargets(t *testing.T) {
	t.Parallel()

	policy, err := netpolicy.NewPolicy(netpolicy.Options{})
	if err != nil {
		t.Fatalf("netpolicy.NewPolicy() error = %v", err)
	}
	client, err := netpolicy.NewClient(policy, netpolicy.ClientOptions{})
	if err != nil {
		t.Fatalf("netpolicy.NewClient() error = %v", err)
	}
	t.Cleanup(client.CloseIdleConnections)
	fetch, err := New(client)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = fetch.Execute(
		context.Background(),
		&function.ExecutionContext{},
		json.RawMessage(`{"url":"http://127.0.0.1/private"}`),
	)
	assertClassified(
		t,
		err,
		function.FailureInvalidRequest,
		"outbound_target_blocked",
		false,
	)
}

func TestManifestPublishesImmutableIdempotentContract(t *testing.T) {
	t.Parallel()

	fetch := mustFetch(t, func(*http.Request) (*http.Response, error) {
		return nil, errors.New("unused")
	})
	manifest := fetch.Manifest()

	if err := manifest.Validate(); err != nil {
		t.Fatalf("manifest.Validate() error = %v", err)
	}
	if manifest.Name != FunctionName || manifest.Version != FunctionVersion {
		t.Fatalf("function ref = %s@%s, want %s@%s",
			manifest.Name, manifest.Version, FunctionName, FunctionVersion)
	}
	if manifest.ExecutionContext != function.ExecutionContextHTTPAttempt {
		t.Fatalf("execution context = %q, want http_attempt", manifest.ExecutionContext)
	}
	if manifest.SideEffects != function.SideEffectIdempotent {
		t.Fatalf("side effects = %q, want idempotent", manifest.SideEffects)
	}
	if len(manifest.Capabilities) != 1 || manifest.Capabilities[0] != "http" {
		t.Fatalf("capabilities = %#v, want [http]", manifest.Capabilities)
	}
	if !manifest.RetryAllowed(function.FailureTransientNetwork) {
		t.Fatal("transient network failures should be retryable")
	}
	if err := manifest.InputSchema.ValidateJSON(json.RawMessage(
		`{"url":"https://example.com/resource","method":"GET","headers":{"Accept":"text/plain"}}`,
	)); err != nil {
		t.Fatalf("valid input rejected: %v", err)
	}
	if err := manifest.InputSchema.ValidateJSON(json.RawMessage(
		`{"url":"https://example.com/resource","method":"POST"}`,
	)); err == nil {
		t.Fatal("POST input accepted, want GET/HEAD-only contract")
	}
}

func TestFetchReturnsNormalizedUTF8Response(t *testing.T) {
	t.Parallel()

	finalURL := mustURL(t, "https://user:secret@final.example/path?q=1#fragment")
	var observed *http.Request
	fetch := mustFetch(t, func(request *http.Request) (*http.Response, error) {
		observed = request.Clone(request.Context())
		return &http.Response{
			StatusCode: http.StatusPartialContent,
			Header: http.Header{
				"Cache-Control":  []string{"max-age=60"},
				"Content-Type":   []string{"text/plain; charset=utf-8"},
				"Set-Cookie":     []string{"session=secret"},
				"X-Robots-Tag":   []string{"noindex"},
				"X-Unsafe-Value": []string{"must-not-escape"},
			},
			Body:    io.NopCloser(strings.NewReader("hello, 世界")),
			Request: &http.Request{URL: finalURL},
		}, nil
	})

	result, err := fetch.Execute(
		context.Background(),
		&function.ExecutionContext{},
		json.RawMessage(`{
			"url":"https://example.com/start",
			"headers":{"Accept":"text/plain","Authorization":"Bearer secret"}
		}`),
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if observed == nil {
		t.Fatal("HTTP client did not receive a request")
	}
	if observed.Method != http.MethodGet {
		t.Fatalf("request method = %q, want GET", observed.Method)
	}
	if got := observed.Header.Get("Accept"); got != "text/plain" {
		t.Fatalf("Accept = %q, want text/plain", got)
	}
	if got := observed.Header.Get("Authorization"); got != "Bearer secret" {
		t.Fatalf("Authorization = %q, want supplied value", got)
	}

	output := decodeOutput(t, result.Output)
	if output.StatusCode != http.StatusPartialContent {
		t.Fatalf("status_code = %d, want %d", output.StatusCode, http.StatusPartialContent)
	}
	if output.Body != "hello, 世界" || output.BodyEncoding != "utf-8" {
		t.Fatalf("body = %q (%s), want UTF-8 payload", output.Body, output.BodyEncoding)
	}
	if output.Bytes != int64(len("hello, 世界")) {
		t.Fatalf("bytes = %d, want %d", output.Bytes, len("hello, 世界"))
	}
	if result.Usage.NetworkBytes != output.Bytes {
		t.Fatalf("network usage = %d, want %d", result.Usage.NetworkBytes, output.Bytes)
	}
	if output.ContentType != "text/plain; charset=utf-8" {
		t.Fatalf("content_type = %q", output.ContentType)
	}
	if output.FinalURL != "https://final.example/path?q=1" {
		t.Fatalf("final_url = %q, want credentials and fragment removed", output.FinalURL)
	}
	if got := output.Headers["cache-control"]; len(got) != 1 || got[0] != "max-age=60" {
		t.Fatalf("cache-control = %#v", got)
	}
	if got := output.Headers["x-robots-tag"]; len(got) != 1 || got[0] != "noindex" {
		t.Fatalf("x-robots-tag = %#v", got)
	}
	if _, leaked := output.Headers["set-cookie"]; leaked {
		t.Fatal("Set-Cookie leaked into normalized response headers")
	}
	if _, leaked := output.Headers["x-unsafe-value"]; leaked {
		t.Fatal("non-allowlisted response header leaked")
	}

	if err := fetch.Manifest().OutputSchema.ValidateJSON(result.Output); err != nil {
		t.Fatalf("result violates published output schema: %v", err)
	}
}

func TestFetchBase64EncodesBinaryBody(t *testing.T) {
	t.Parallel()

	payload := []byte{0x00, 0xff, 0x80, 0x41}
	fetch := mustFetch(t, func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/octet-stream"}},
			Body:       io.NopCloser(strings.NewReader(string(payload))),
			Request:    request,
		}, nil
	})

	result, err := fetch.Execute(
		context.Background(),
		&function.ExecutionContext{},
		json.RawMessage(`{"url":"https://example.com/file"}`),
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	output := decodeOutput(t, result.Output)
	if output.BodyEncoding != "base64" {
		t.Fatalf("body_encoding = %q, want base64", output.BodyEncoding)
	}
	if output.Body != base64.StdEncoding.EncodeToString(payload) {
		t.Fatalf("body = %q, want encoded binary payload", output.Body)
	}
	if output.Bytes != int64(len(payload)) {
		t.Fatalf("bytes = %d, want %d", output.Bytes, len(payload))
	}
}

func TestFetchSupportsHEADWithoutRequestBody(t *testing.T) {
	t.Parallel()

	fetch := mustFetch(t, func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodHead {
			t.Fatalf("method = %q, want HEAD", request.Method)
		}
		if request.Body != nil {
			t.Fatal("HEAD request unexpectedly has a body")
		}
		return &http.Response{
			StatusCode: http.StatusNoContent,
			Header:     http.Header{"ETag": []string{`"v1"`}},
			Body:       http.NoBody,
			Request:    request,
		}, nil
	})

	result, err := fetch.Execute(
		context.Background(),
		&function.ExecutionContext{},
		json.RawMessage(`{"url":"https://example.com/resource","method":"HEAD"}`),
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	output := decodeOutput(t, result.Output)
	if output.Body != "" || output.BodyEncoding != "utf-8" || output.Bytes != 0 {
		t.Fatalf("HEAD body = %q (%s), %d bytes", output.Body, output.BodyEncoding, output.Bytes)
	}
	if got := output.Headers["etag"]; len(got) != 1 || got[0] != `"v1"` {
		t.Fatalf("etag = %#v", got)
	}
}

func TestFetchRejectsUnsupportedMethodsAndMalformedHeaders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		wantCode string
	}{
		{
			name:     "post",
			input:    `{"url":"https://example.com","method":"POST"}`,
			wantCode: "invalid_http_fetch_input",
		},
		{
			name:     "empty URL after normalization",
			input:    `{"url":"   "}`,
			wantCode: "invalid_url",
		},
		{
			name:     "host override",
			input:    `{"url":"https://example.com","headers":{"Host":"internal.example"}}`,
			wantCode: "forbidden_request_header",
		},
		{
			name:     "hop by hop",
			input:    `{"url":"https://example.com","headers":{"Transfer-Encoding":"chunked"}}`,
			wantCode: "forbidden_request_header",
		},
		{
			name:     "newline",
			input:    "{\"url\":\"https://example.com\",\"headers\":{\"X-Test\":\"one\\r\\ntwo\"}}",
			wantCode: "invalid_request_header",
		},
		{
			name:     "duplicate casing",
			input:    `{"url":"https://example.com","headers":{"Accept":"one","accept":"two"}}`,
			wantCode: "duplicate_request_header",
		},
		{
			name:     "non-string value",
			input:    `{"url":"https://example.com","headers":{"X-Number":7}}`,
			wantCode: "invalid_http_fetch_input",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fetch := mustFetch(t, func(*http.Request) (*http.Response, error) {
				t.Fatal("HTTP client called for invalid request")
				return nil, nil
			})

			_, err := fetch.Execute(
				context.Background(),
				&function.ExecutionContext{},
				json.RawMessage(test.input),
			)
			assertClassified(t, err, function.FailureInvalidRequest, test.wantCode, false)
		})
	}
}

func TestFetchClassifiesPolicyAndTransportErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		err           error
		wantCategory  function.FailureCategory
		wantCode      string
		wantRetryable bool
	}{
		{
			name:         "invalid URL",
			err:          fmt.Errorf("wrapped: %w", netpolicy.ErrInvalidURL),
			wantCategory: function.FailureInvalidRequest,
			wantCode:     "invalid_url",
		},
		{
			name:         "blocked host",
			err:          fmt.Errorf("wrapped: %w", netpolicy.ErrBlockedHost),
			wantCategory: function.FailureInvalidRequest,
			wantCode:     "outbound_target_blocked",
		},
		{
			name:         "blocked address",
			err:          fmt.Errorf("wrapped: %w", netpolicy.ErrBlockedAddress),
			wantCategory: function.FailureInvalidRequest,
			wantCode:     "outbound_target_blocked",
		},
		{
			name:         "port blocked",
			err:          fmt.Errorf("wrapped: %w", netpolicy.ErrPortNotAllowed),
			wantCategory: function.FailureInvalidRequest,
			wantCode:     "outbound_target_blocked",
		},
		{
			name:         "redirect limit",
			err:          fmt.Errorf("wrapped: %w", netpolicy.ErrTooManyRedirects),
			wantCategory: function.FailureInvalidRequest,
			wantCode:     "redirect_limit_exceeded",
		},
		{
			name:         "response limit",
			err:          fmt.Errorf("wrapped: %w", netpolicy.ErrResponseTooLarge),
			wantCategory: function.FailureResourceLimit,
			wantCode:     "response_too_large",
		},
		{
			name:          "DNS",
			err:           fmt.Errorf("wrapped: %w", netpolicy.ErrResolution),
			wantCategory:  function.FailureTransientNetwork,
			wantCode:      "dns_resolution_failed",
			wantRetryable: true,
		},
		{
			name:         "invalid policy",
			err:          fmt.Errorf("wrapped: %w", netpolicy.ErrInvalidPolicy),
			wantCategory: function.FailureInternal,
			wantCode:     "outbound_policy_invalid",
		},
		{
			name:         "canceled",
			err:          context.Canceled,
			wantCategory: function.FailureCanceled,
			wantCode:     "http_request_canceled",
		},
		{
			name:          "deadline",
			err:           context.DeadlineExceeded,
			wantCategory:  function.FailureTimeout,
			wantCode:      "http_request_timeout",
			wantRetryable: true,
		},
		{
			name:         "TLS verification",
			err:          x509.UnknownAuthorityError{},
			wantCategory: function.FailureExecution,
			wantCode:     "tls_verification_failed",
		},
		{
			name:          "generic transport",
			err:           errors.New("connection disappeared"),
			wantCategory:  function.FailureTransientNetwork,
			wantCode:      "outbound_request_failed",
			wantRetryable: true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fetch := mustFetch(t, func(*http.Request) (*http.Response, error) {
				return nil, test.err
			})
			_, err := fetch.Execute(
				context.Background(),
				&function.ExecutionContext{},
				json.RawMessage(`{"url":"https://example.com"}`),
			)
			classified := assertClassified(
				t,
				err,
				test.wantCategory,
				test.wantCode,
				test.wantRetryable,
			)
			if !errors.Is(classified, test.err) {
				t.Fatalf("classified error does not retain cause %v", test.err)
			}
		})
	}
}

func TestFetchEnforcesBodyLimitForInjectedClients(t *testing.T) {
	t.Parallel()

	fetch := mustFetch(t, func(request *http.Request) (*http.Response, error) {
		body := io.NopCloser(io.LimitReader(repeatingReader{}, maxResponseBodyBytes+1))
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       body,
			Request:    request,
		}, nil
	})

	_, err := fetch.Execute(
		context.Background(),
		&function.ExecutionContext{},
		json.RawMessage(`{"url":"https://example.com/oversized"}`),
	)
	assertClassified(t, err, function.FailureResourceLimit, "response_too_large", false)
}

type repeatingReader struct{}

func (repeatingReader) Read(buffer []byte) (int, error) {
	for i := range buffer {
		buffer[i] = 'x'
	}
	return len(buffer), nil
}

func mustFetch(t *testing.T, do doerFunc) function.AtomicFunction {
	t.Helper()
	fetch, err := newFetch(do)
	if err != nil {
		t.Fatalf("newFetch() error = %v", err)
	}
	return fetch
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("url.Parse(%q) error = %v", raw, err)
	}
	return parsed
}

func decodeOutput(t *testing.T, raw json.RawMessage) fetchOutput {
	t.Helper()
	var output fetchOutput
	if err := json.Unmarshal(raw, &output); err != nil {
		t.Fatalf("json.Unmarshal(output) error = %v\noutput: %s", err, raw)
	}
	return output
}

func assertClassified(
	t *testing.T,
	err error,
	category function.FailureCategory,
	code string,
	retryable bool,
) *function.ClassifiedError {
	t.Helper()
	if err == nil {
		t.Fatal("error = nil, want classified error")
	}
	var classified *function.ClassifiedError
	if !errors.As(err, &classified) {
		t.Fatalf("error type = %T, want *function.ClassifiedError: %v", err, err)
	}
	if classified.Category != category ||
		classified.Code != code ||
		classified.Retryable != retryable {
		t.Fatalf(
			"classified error = (%q, %q, retryable=%t), want (%q, %q, retryable=%t)",
			classified.Category,
			classified.Code,
			classified.Retryable,
			category,
			code,
			retryable,
		)
	}
	return classified
}
