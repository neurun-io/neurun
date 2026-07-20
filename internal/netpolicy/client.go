package netpolicy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"
)

const (
	DefaultMaxResponseBytes int64 = 10 << 20
	DefaultRequestTimeout         = 30 * time.Second
	DefaultConnectTimeout         = 10 * time.Second
	DefaultHeaderTimeout          = 15 * time.Second
	DefaultMaxRedirects           = 5
)

var (
	ErrResponseTooLarge = errors.New("outbound response exceeds byte limit")
	ErrTooManyRedirects = errors.New("too many outbound redirects")
)

// ClientOptions configures a policy-enforcing HTTP client. Zero values select
// conservative defaults. Negative values are rejected.
type ClientOptions struct {
	Timeout               time.Duration
	ConnectTimeout        time.Duration
	ResponseHeaderTimeout time.Duration
	IdleConnTimeout       time.Duration
	MaxResponseBytes      int64
	MaxRedirects          int
}

// Client validates every initial URL and redirect, disables environment proxy
// discovery, and resolves then dials the validated IP itself. Its response body
// also enforces MaxResponseBytes even when the caller streams it manually.
type Client struct {
	policy           *Policy
	httpClient       *http.Client
	maxResponseBytes int64
}

// NewClient constructs a safe HTTP client. A Client owns its transport and
// should be reused; callers should call CloseIdleConnections when finished.
func NewClient(policy *Policy, options ClientOptions) (*Client, error) {
	if policy == nil {
		return nil, fmt.Errorf("%w: policy is required", ErrInvalidPolicy)
	}
	if options.Timeout < 0 ||
		options.ConnectTimeout < 0 ||
		options.ResponseHeaderTimeout < 0 ||
		options.IdleConnTimeout < 0 ||
		options.MaxResponseBytes < 0 ||
		options.MaxRedirects < 0 {
		return nil, fmt.Errorf("%w: client limits cannot be negative", ErrInvalidPolicy)
	}

	timeout := options.Timeout
	if timeout == 0 {
		timeout = DefaultRequestTimeout
	}
	connectTimeout := options.ConnectTimeout
	if connectTimeout == 0 {
		connectTimeout = DefaultConnectTimeout
	}
	headerTimeout := options.ResponseHeaderTimeout
	if headerTimeout == 0 {
		headerTimeout = DefaultHeaderTimeout
	}
	idleTimeout := options.IdleConnTimeout
	if idleTimeout == 0 {
		idleTimeout = 90 * time.Second
	}
	maxResponseBytes := options.MaxResponseBytes
	if maxResponseBytes == 0 {
		maxResponseBytes = DefaultMaxResponseBytes
	}
	maxRedirects := options.MaxRedirects
	if maxRedirects == 0 {
		maxRedirects = DefaultMaxRedirects
	}

	dialer := &net.Dialer{
		Timeout:   connectTimeout,
		KeepAlive: 30 * time.Second,
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// A proxy is another egress boundary with separate DNS semantics. Do not
	// silently trust HTTP_PROXY/HTTPS_PROXY in a security-sensitive client.
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		return policy.dialContext(ctx, dialer, network, address)
	}
	transport.ResponseHeaderTimeout = headerTimeout
	transport.IdleConnTimeout = idleTimeout

	client := &Client{
		policy:           policy,
		maxResponseBytes: maxResponseBytes,
	}
	client.httpClient = &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return ErrTooManyRedirects
			}
			if _, err := policy.Validate(request.Context(), request.URL); err != nil {
				return fmt.Errorf("redirect target rejected: %w", err)
			}
			return nil
		},
	}
	return client, nil
}

// Do validates req.URL immediately and again at dial time. On success, the
// returned body is bounded and must be closed by the caller.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	if c == nil || c.policy == nil || c.httpClient == nil {
		return nil, errors.New("netpolicy: client is nil or uninitialized")
	}
	if req == nil {
		return nil, errors.New("netpolicy: request is nil")
	}
	target, err := c.policy.Validate(req.Context(), req.URL)
	if err != nil {
		return nil, err
	}
	if req.Host != "" {
		host, explicitPort, err := parseAuthority(req.Host)
		if err != nil {
			return nil, err
		}
		host, err = canonicalHost(host)
		if err != nil {
			return nil, err
		}
		if host != target.Host || (explicitPort != 0 && explicitPort != target.Port) {
			return nil, fmt.Errorf("%w: request Host overrides are forbidden", ErrBlockedHost)
		}
	}

	// Use the normalized URL that was actually validated. Clone leaves the
	// caller's request untouched and preserves its context and body.
	outboundRequest := req.Clone(req.Context())
	normalizedURL := *target.URL
	outboundRequest.URL = &normalizedURL
	outboundRequest.Host = ""

	response, err := c.httpClient.Do(outboundRequest)
	if err != nil {
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		return nil, err
	}
	if response.Body == nil {
		response.Body = http.NoBody
	}

	if outboundRequest.Method != http.MethodHead &&
		response.ContentLength > c.maxResponseBytes {
		_ = response.Body.Close()
		return nil, fmt.Errorf("%w: declared content length %d, limit %d",
			ErrResponseTooLarge, response.ContentLength, c.maxResponseBytes)
	}
	response.Body = &boundedReadCloser{
		source:    response.Body,
		remaining: c.maxResponseBytes,
	}
	return response, nil
}

// Get creates and executes a GET request with ctx.
func (c *Client) Get(ctx context.Context, rawURL string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: malformed URL", ErrInvalidURL)
	}
	return c.Do(request)
}

// CloseIdleConnections closes connections held by this client's transport.
func (c *Client) CloseIdleConnections() {
	if c != nil && c.httpClient != nil {
		c.httpClient.CloseIdleConnections()
	}
}

// ReadAllAndClose drains a bounded response body and always closes it. Bodies
// returned by Client surface ErrResponseTooLarge instead of silent truncation.
func ReadAllAndClose(response *http.Response) ([]byte, error) {
	if response == nil || response.Body == nil {
		return nil, errors.New("netpolicy: response body is nil")
	}
	defer response.Body.Close()
	return io.ReadAll(response.Body)
}

// ReadBoundedAndClose reads at most maxBytes from body. It probes one byte past
// the boundary so oversized bodies fail instead of appearing truncated.
func ReadBoundedAndClose(body io.ReadCloser, maxBytes int64) ([]byte, error) {
	if body == nil {
		return nil, errors.New("netpolicy: body is nil")
	}
	if maxBytes < 0 {
		_ = body.Close()
		return nil, errors.New("netpolicy: byte limit cannot be negative")
	}
	wrapped := &boundedReadCloser{source: body, remaining: maxBytes}
	defer wrapped.Close()
	return io.ReadAll(wrapped)
}

func (p *Policy) dialContext(
	ctx context.Context,
	dialer *net.Dialer,
	network string,
	endpoint string,
) (net.Conn, error) {
	host, rawPort, err := net.SplitHostPort(endpoint)
	if err != nil {
		return nil, fmt.Errorf("%w: malformed dial endpoint", ErrInvalidURL)
	}
	host, err = canonicalHost(host)
	if err != nil {
		return nil, err
	}
	portValue, err := strconv.ParseUint(rawPort, 10, 16)
	if err != nil || portValue == 0 {
		return nil, fmt.Errorf("%w: malformed dial port", ErrInvalidURL)
	}
	port := uint16(portValue)
	if !p.portAllowedForDial(port) {
		return nil, fmt.Errorf("%w: port %d", ErrPortNotAllowed, port)
	}
	if err := p.validateHostRules(host); err != nil {
		return nil, err
	}

	addresses, err := p.resolveHost(ctx, host)
	if err != nil {
		return nil, err
	}

	var dialErrors []error
	for _, address := range addresses {
		target := net.JoinHostPort(address.String(), rawPort)
		connection, dialErr := dialer.DialContext(ctx, network, target)
		if dialErr == nil {
			return connection, nil
		}
		dialErrors = append(dialErrors, dialErr)
		if ctx.Err() != nil {
			break
		}
	}
	return nil, fmt.Errorf("dial validated target: %w", errors.Join(dialErrors...))
}

type boundedReadCloser struct {
	source    io.ReadCloser
	remaining int64
	exceeded  bool
}

func (reader *boundedReadCloser) Read(buffer []byte) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}
	if reader.exceeded {
		return 0, ErrResponseTooLarge
	}
	if reader.remaining == 0 {
		var probe [1]byte
		count, err := reader.source.Read(probe[:])
		if count > 0 {
			reader.exceeded = true
			return 0, ErrResponseTooLarge
		}
		return 0, err
	}
	if int64(len(buffer)) > reader.remaining {
		buffer = buffer[:reader.remaining]
	}
	count, err := reader.source.Read(buffer)
	reader.remaining -= int64(count)
	return count, err
}

func (reader *boundedReadCloser) Close() error {
	return reader.source.Close()
}
