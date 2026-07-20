package netpolicy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func TestClientDialsOnlyPolicyResolvedIP(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Host == "" {
			t.Error("request Host is empty")
		}
		_, _ = io.WriteString(writer, "pinned")
	}))
	t.Cleanup(server.Close)

	port := serverPort(t, server.URL)
	policy, err := NewPolicy(Options{
		Resolver: &fakeResolver{addresses: map[string][]netip.Addr{
			"target.test": {netip.MustParseAddr("127.0.0.1")},
		}},
		AllowPrivateNetworks:   true,
		AllowedHosts:           []string{"target.test"},
		AdditionalAllowedPorts: []int{port},
	})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(policy, ClientOptions{MaxResponseBytes: 64})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.CloseIdleConnections)

	response, err := client.Get(context.Background(), fmt.Sprintf("http://target.test:%d/", port))
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	body, err := ReadAllAndClose(response)
	if err != nil {
		t.Fatalf("ReadAllAndClose() error = %v", err)
	}
	if got, want := string(body), "pinned"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

func TestClientRejectsDNSRebindingBeforeDial(t *testing.T) {
	t.Parallel()

	resolver := &sequenceResolver{addresses: [][]netip.Addr{
		{netip.MustParseAddr("93.184.216.34")}, // Client.Do validation.
		{netip.MustParseAddr("127.0.0.1")},     // Transport dial validation.
	}}
	policy, err := NewPolicy(Options{Resolver: resolver})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(policy, ClientOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.CloseIdleConnections)

	response, err := client.Get(context.Background(), "http://rebind.test/")
	if response != nil {
		_ = response.Body.Close()
		t.Fatal("response was returned for rebound private address")
	}
	if !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("Get() error = %v, want ErrBlockedAddress", err)
	}
}

func TestClientRevalidatesRedirectTargets(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	var redirectURL string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.URL.Path == "/" {
			http.Redirect(writer, request, redirectURL, http.StatusFound)
			return
		}
		t.Error("blocked redirect target was requested")
	}))
	t.Cleanup(server.Close)

	port := serverPort(t, server.URL)
	redirectURL = fmt.Sprintf("http://blocked.test:%d/private", port)
	policy, err := NewPolicy(Options{
		Resolver: &fakeResolver{addresses: map[string][]netip.Addr{
			"entry.test":   {netip.MustParseAddr("127.0.0.1")},
			"blocked.test": {netip.MustParseAddr("127.0.0.1")},
		}},
		AllowPrivateNetworks:   true,
		DeniedHosts:            []string{"blocked.test"},
		AdditionalAllowedPorts: []int{port},
	})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(policy, ClientOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.CloseIdleConnections)

	response, err := client.Get(context.Background(), fmt.Sprintf("http://entry.test:%d/", port))
	if response != nil {
		_ = response.Body.Close()
		t.Fatal("response was returned for a rejected redirect")
	}
	if !errors.Is(err, ErrBlockedHost) {
		t.Fatalf("Get() error = %v, want ErrBlockedHost", err)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("server request count = %d, want 1", got)
	}
}

func TestClientRejectsHostAuthorityOverride(t *testing.T) {
	t.Parallel()

	policy, err := NewPolicy(Options{
		Resolver: &fakeResolver{addresses: map[string][]netip.Addr{
			"target.test": {netip.MustParseAddr("93.184.216.34")},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(policy, ClientOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.CloseIdleConnections)

	request, err := http.NewRequest(http.MethodGet, "http://target.test/", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Host = "internal.test"
	response, err := client.Do(request)
	if response != nil {
		_ = response.Body.Close()
	}
	if !errors.Is(err, ErrBlockedHost) {
		t.Fatalf("Do() error = %v, want ErrBlockedHost", err)
	}
}

func TestClientLimitsRedirectCount(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, request.URL.Path, http.StatusFound)
	}))
	t.Cleanup(server.Close)

	port := serverPort(t, server.URL)
	policy, err := NewPolicy(Options{
		Resolver: &fakeResolver{addresses: map[string][]netip.Addr{
			"loop.test": {netip.MustParseAddr("127.0.0.1")},
		}},
		AllowPrivateNetworks:   true,
		AdditionalAllowedPorts: []int{port},
	})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(policy, ClientOptions{MaxRedirects: 2})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.CloseIdleConnections)

	response, err := client.Get(context.Background(), fmt.Sprintf("http://loop.test:%d/again", port))
	if response != nil {
		_ = response.Body.Close()
		t.Fatal("response was returned for redirect loop")
	}
	if !errors.Is(err, ErrTooManyRedirects) {
		t.Fatalf("Get() error = %v, want ErrTooManyRedirects", err)
	}
}

func TestClientEnforcesDeclaredAndStreamingResponseLimits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		handler http.HandlerFunc
		read    bool
	}{
		{
			name: "declared content length",
			handler: func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Length", "6")
				_, _ = io.WriteString(writer, "123456")
			},
		},
		{
			name: "chunked body",
			handler: func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(http.StatusOK)
				writer.(http.Flusher).Flush()
				_, _ = io.WriteString(writer, "123456")
			},
			read: true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(test.handler)
			t.Cleanup(server.Close)
			port := serverPort(t, server.URL)

			policy, err := NewPolicy(Options{
				AllowPrivateNetworks:   true,
				AdditionalAllowedPorts: []int{port},
			})
			if err != nil {
				t.Fatal(err)
			}
			client, err := NewClient(policy, ClientOptions{MaxResponseBytes: 5})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(client.CloseIdleConnections)

			response, requestErr := client.Get(context.Background(), server.URL)
			if test.read && requestErr == nil {
				_, requestErr = ReadAllAndClose(response)
				response = nil
			}
			if response != nil {
				_ = response.Body.Close()
			}
			if !errors.Is(requestErr, ErrResponseTooLarge) {
				t.Fatalf("error = %v, want ErrResponseTooLarge", requestErr)
			}
		})
	}
}

func TestClientAllowsResponseExactlyAtLimit(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
		writer.(http.Flusher).Flush()
		_, _ = io.WriteString(writer, "12345")
	}))
	t.Cleanup(server.Close)
	port := serverPort(t, server.URL)

	policy, err := NewPolicy(Options{
		AllowPrivateNetworks:   true,
		AdditionalAllowedPorts: []int{port},
	})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(policy, ClientOptions{MaxResponseBytes: 5})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.CloseIdleConnections)

	response, err := client.Get(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	body, err := ReadAllAndClose(response)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "12345" {
		t.Fatalf("body = %q", body)
	}
}

func TestReadBoundedAndCloseClosesBody(t *testing.T) {
	t.Parallel()

	body := &trackingReadCloser{Reader: io.NopCloser(
		&oneByteReader{remaining: []byte("too long")},
	)}
	data, err := ReadBoundedAndClose(body, 3)
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("error = %v, want ErrResponseTooLarge", err)
	}
	if string(data) != "too" {
		t.Fatalf("partial data = %q, want %q", data, "too")
	}
	if !body.closed.Load() {
		t.Fatal("body was not closed")
	}
}

func TestClientTotalTimeout(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_, _ = io.WriteString(writer, "late")
	}))
	t.Cleanup(server.Close)
	port := serverPort(t, server.URL)

	policy, err := NewPolicy(Options{
		AllowPrivateNetworks:   true,
		AdditionalAllowedPorts: []int{port},
	})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(policy, ClientOptions{
		Timeout:               20 * time.Millisecond,
		ResponseHeaderTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.CloseIdleConnections)

	start := time.Now()
	response, err := client.Get(context.Background(), server.URL)
	if response != nil {
		_ = response.Body.Close()
	}
	if err == nil {
		t.Fatal("request unexpectedly succeeded")
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("timeout took too long: %s", elapsed)
	}
}

func serverPort(t *testing.T, rawURL string) int {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	_, rawPort, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil {
		t.Fatal(err)
	}
	return port
}

type trackingReadCloser struct {
	io.Reader
	closed atomic.Bool
}

func (body *trackingReadCloser) Close() error {
	body.closed.Store(true)
	if closer, ok := body.Reader.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

type oneByteReader struct {
	remaining []byte
}

func (reader *oneByteReader) Read(buffer []byte) (int, error) {
	if len(reader.remaining) == 0 {
		return 0, io.EOF
	}
	buffer[0] = reader.remaining[0]
	reader.remaining = reader.remaining[1:]
	return 1, nil
}
