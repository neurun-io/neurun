// Package httpruntime contains server-owned atomic functions that perform
// policy-constrained HTTP work.
package httpruntime

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/textproto"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/dagflows/neurun-io/internal/function"
	"github.com/dagflows/neurun-io/internal/netpolicy"
)

const (
	FunctionName    = "http.fetch"
	FunctionVersion = "1"

	maxURLBytes                  = 8 << 10
	maxRequestHeaders            = 64
	maxRequestHeaderName         = 256
	maxRequestHeaderBytes        = 32 << 10
	maxRequestHeaderValue        = 8 << 10
	maxResponseHeaderBytes       = 32 << 10
	maxResponseHeaderValue       = 8 << 10
	maxResponseBodyBytes         = netpolicy.DefaultMaxResponseBytes
	defaultTimeoutMS       int64 = 30_000
	maximumTimeoutMS       int64 = 120_000
)

// doer is deliberately private. The exported constructor only accepts the
// policy-enforcing client; tests can still inject deterministic transports.
type doer interface {
	Do(*http.Request) (*http.Response, error)
}

type executor struct {
	client doer
}

type fetchInput struct {
	URL     string            `json:"url"`
	Method  string            `json:"method,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

type fetchOutput struct {
	StatusCode   int                 `json:"status_code"`
	Headers      map[string][]string `json:"headers"`
	Body         string              `json:"body"`
	BodyEncoding string              `json:"body_encoding"`
	FinalURL     string              `json:"final_url"`
	ContentType  string              `json:"content_type"`
	Bytes        int64               `json:"bytes"`
}

var safeResponseHeaders = []string{
	"accept-ranges",
	"age",
	"cache-control",
	"content-disposition",
	"content-encoding",
	"content-language",
	"content-length",
	"content-location",
	"content-range",
	"content-type",
	"date",
	"etag",
	"expires",
	"last-modified",
	"link",
	"location",
	"retry-after",
	"vary",
	"x-robots-tag",
}

var forbiddenRequestHeaders = map[string]struct{}{
	"connection":          {},
	"content-length":      {},
	"host":                {},
	"keep-alive":          {},
	"proxy-authenticate":  {},
	"proxy-authorization": {},
	"proxy-connection":    {},
	"te":                  {},
	"trailer":             {},
	"transfer-encoding":   {},
	"upgrade":             {},
}

// New constructs the immutable http.fetch@1 atomic function. Requiring
// *netpolicy.Client here prevents production callers from accidentally
// replacing the SSRF and DNS-rebinding boundary with an ordinary HTTP client.
func New(client *netpolicy.Client) (function.AtomicFunction, error) {
	if client == nil {
		return nil, errors.New("http.fetch: netpolicy client is required")
	}
	return newFetch(client)
}

func newFetch(client doer) (function.AtomicFunction, error) {
	if client == nil {
		return nil, errors.New("http.fetch: HTTP client is required")
	}
	runtime := &executor{client: client}
	return function.NewAtomicFunction(fetchManifest(), runtime.execute)
}

func fetchManifest() function.Manifest {
	return function.Manifest{
		Name:             FunctionName,
		Version:          FunctionVersion,
		Category:         "http",
		Description:      "Fetches one HTTP resource through the server-owned outbound network policy.",
		ExecutionContext: function.ExecutionContextHTTPAttempt,
		SideEffects:      function.SideEffectIdempotent,
		Timeout: function.TimeoutPolicy{
			DefaultMS: defaultTimeoutMS,
			MaximumMS: maximumTimeoutMS,
		},
		Capabilities: []string{"http"},
		Permissions:  []string{"network.egress"},
		InputSchema:  fetchInputSchema(),
		OutputSchema: fetchOutputSchema(),
		Resources: function.ResourcePolicy{
			MemoryBytes:     32 << 20,
			CPUMilliseconds: 2_000,
			NetworkBytes:    maxResponseBodyBytes,
		},
		Redaction: function.RedactionPolicy{
			// Header names are case-insensitive and user-selected. Redacting
			// the complete map avoids casing tricks that could persist secrets.
			SecretFields: []string{"headers"},
		},
		Retry: function.RetryPolicy{AllowedFailures: []function.FailureCategory{
			function.FailureAgentLost,
			function.FailureTimeout,
			function.FailureTransientNetwork,
		}},
		Telemetry: function.TelemetryPolicy{Dimensions: []string{
			"duration_ms",
			"network_bytes",
			"status_code",
		}},
	}
}

func fetchInputSchema() function.Schema {
	return function.Schema{
		Type:     function.TypeObject,
		Required: []string{"url"},
		Properties: map[string]function.Schema{
			"url": {
				Type:      function.TypeString,
				MinLength: function.Int(1),
				MaxLength: function.Int(maxURLBytes),
			},
			"method": {
				Type: function.TypeString,
				Enum: []any{http.MethodGet, http.MethodHead},
			},
			"headers": {
				Type:                 function.TypeObject,
				AdditionalProperties: function.Bool(true),
			},
		},
		AdditionalProperties: function.Bool(false),
	}
}

func fetchOutputSchema() function.Schema {
	return function.Schema{
		Type: function.TypeObject,
		Required: []string{
			"body",
			"body_encoding",
			"bytes",
			"content_type",
			"final_url",
			"headers",
			"status_code",
		},
		Properties: map[string]function.Schema{
			"status_code": {
				Type:    function.TypeInteger,
				Minimum: function.Number(100),
				Maximum: function.Number(599),
			},
			"headers": {
				Type:                 function.TypeObject,
				AdditionalProperties: function.Bool(true),
			},
			"body": {Type: function.TypeString},
			"body_encoding": {
				Type: function.TypeString,
				Enum: []any{"base64", "utf-8"},
			},
			"final_url":    {Type: function.TypeString, MinLength: function.Int(1)},
			"content_type": {Type: function.TypeString},
			"bytes":        {Type: function.TypeInteger, Minimum: function.Number(0)},
		},
		AdditionalProperties: function.Bool(false),
	}
}

func (e *executor) execute(
	ctx context.Context,
	_ *function.ExecutionContext,
	rawInput json.RawMessage,
) (function.FunctionResult, error) {
	input, err := decodeInput(rawInput)
	if err != nil {
		return function.FunctionResult{}, err
	}
	if ctx == nil {
		return function.FunctionResult{}, classified(
			function.FailureInternal,
			"nil_execution_context",
			"HTTP execution context is unavailable",
			false,
			errors.New("nil context"),
		)
	}

	method := input.Method
	if method == "" {
		method = http.MethodGet
	}
	if method != http.MethodGet && method != http.MethodHead {
		return function.FunctionResult{}, classified(
			function.FailureInvalidRequest,
			"method_not_allowed",
			"http.fetch only supports GET and HEAD",
			false,
			nil,
		)
	}

	target := strings.TrimSpace(input.URL)
	if target == "" || len(target) > maxURLBytes {
		return function.FunctionResult{}, classified(
			function.FailureInvalidRequest,
			"invalid_url",
			"url must be an absolute HTTP or HTTPS URL",
			false,
			nil,
		)
	}
	request, err := http.NewRequestWithContext(ctx, method, target, nil)
	if err != nil {
		return function.FunctionResult{}, classified(
			function.FailureInvalidRequest,
			"invalid_url",
			"url must be an absolute HTTP or HTTPS URL",
			false,
			err,
		)
	}
	if err := applyRequestHeaders(request, input.Headers); err != nil {
		return function.FunctionResult{}, err
	}

	response, err := e.client.Do(request)
	if err != nil {
		return function.FunctionResult{}, classifyOutboundError(err)
	}
	if response == nil {
		return function.FunctionResult{}, classified(
			function.FailureInternal,
			"invalid_http_response",
			"HTTP client returned an invalid response",
			false,
			errors.New("nil response"),
		)
	}
	if response.Body == nil {
		return function.FunctionResult{}, classified(
			function.FailureInternal,
			"invalid_http_response",
			"HTTP client returned an invalid response",
			false,
			errors.New("nil response body"),
		)
	}
	if response.StatusCode < 100 || response.StatusCode > 599 {
		_ = response.Body.Close()
		return function.FunctionResult{}, classified(
			function.FailureInternal,
			"invalid_http_status",
			"HTTP client returned an invalid status code",
			false,
			fmt.Errorf("status code %d", response.StatusCode),
		)
	}

	body, err := netpolicy.ReadBoundedAndClose(response.Body, maxResponseBodyBytes)
	if err != nil {
		return function.FunctionResult{}, classifyOutboundError(err)
	}

	headers := selectResponseHeaders(response.Header)
	bodyValue, bodyEncoding := encodeBody(body)
	output := fetchOutput{
		StatusCode:   response.StatusCode,
		Headers:      headers,
		Body:         bodyValue,
		BodyEncoding: bodyEncoding,
		FinalURL:     finalResponseURL(request, response),
		ContentType:  firstHeaderValue(headers, "content-type"),
		Bytes:        int64(len(body)),
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return function.FunctionResult{}, classified(
			function.FailureInternal,
			"encode_http_result_failed",
			"HTTP result could not be encoded",
			false,
			err,
		)
	}
	return function.FunctionResult{
		Output: encoded,
		Usage: function.Usage{
			NetworkBytes: int64(len(body)),
		},
	}, nil
}

func decodeInput(raw json.RawMessage) (fetchInput, error) {
	if err := fetchInputSchema().ValidateJSON(raw); err != nil {
		return fetchInput{}, classified(
			function.FailureInvalidRequest,
			"invalid_http_fetch_input",
			"http.fetch input does not match its contract",
			false,
			err,
		)
	}

	var input fetchInput
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return fetchInput{}, classified(
			function.FailureInvalidRequest,
			"invalid_http_fetch_input",
			"http.fetch input does not match its contract",
			false,
			err,
		)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		return fetchInput{}, classified(
			function.FailureInvalidRequest,
			"invalid_http_fetch_input",
			"http.fetch input does not match its contract",
			false,
			err,
		)
	}
	return input, nil
}

func applyRequestHeaders(request *http.Request, headers map[string]string) error {
	if len(headers) > maxRequestHeaders {
		return classified(
			function.FailureInvalidRequest,
			"too_many_request_headers",
			fmt.Sprintf("request headers exceed the limit of %d", maxRequestHeaders),
			false,
			nil,
		)
	}

	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)

	seen := make(map[string]struct{}, len(names))
	totalBytes := 0
	for _, name := range names {
		value := headers[name]
		if !validHeaderName(name) {
			return classified(
				function.FailureInvalidRequest,
				"invalid_request_header",
				fmt.Sprintf("request header %q has an invalid name", name),
				false,
				nil,
			)
		}
		lowerName := strings.ToLower(name)
		if _, forbidden := forbiddenRequestHeaders[lowerName]; forbidden {
			return classified(
				function.FailureInvalidRequest,
				"forbidden_request_header",
				fmt.Sprintf("request header %q cannot be set", name),
				false,
				nil,
			)
		}
		if _, duplicate := seen[lowerName]; duplicate {
			return classified(
				function.FailureInvalidRequest,
				"duplicate_request_header",
				fmt.Sprintf("request header %q is duplicated with different casing", name),
				false,
				nil,
			)
		}
		seen[lowerName] = struct{}{}

		if len(value) > maxRequestHeaderValue || !validHeaderValue(value) {
			return classified(
				function.FailureInvalidRequest,
				"invalid_request_header",
				fmt.Sprintf("request header %q has an invalid value", name),
				false,
				nil,
			)
		}
		totalBytes += len(name) + len(value)
		if totalBytes > maxRequestHeaderBytes {
			return classified(
				function.FailureInvalidRequest,
				"request_headers_too_large",
				"request headers exceed the byte limit",
				false,
				nil,
			)
		}
		request.Header.Set(textproto.CanonicalMIMEHeaderKey(name), value)
	}
	return nil
}

func validHeaderName(name string) bool {
	if name == "" || len(name) > maxRequestHeaderName {
		return false
	}
	for i := 0; i < len(name); i++ {
		character := name[i]
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') {
			continue
		}
		switch character {
		case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
			continue
		default:
			return false
		}
	}
	return true
}

func validHeaderValue(value string) bool {
	for i := 0; i < len(value); i++ {
		character := value[i]
		if character == '\t' || (character >= 0x20 && character != 0x7f) {
			continue
		}
		return false
	}
	return true
}

func selectResponseHeaders(headers http.Header) map[string][]string {
	normalized := make(map[string][]string)
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		lowerName := strings.ToLower(name)
		if !safeResponseHeader(lowerName) {
			continue
		}
		normalized[lowerName] = append(normalized[lowerName], headers[name]...)
	}

	selected := make(map[string][]string)
	totalBytes := 0
	for _, name := range safeResponseHeaders {
		values := normalized[name]
		for _, value := range values {
			value = strings.Trim(value, " \t")
			if len(value) > maxResponseHeaderValue ||
				!utf8.ValidString(value) ||
				!validHeaderValue(value) {
				continue
			}
			nextSize := totalBytes + len(name) + len(value)
			if nextSize > maxResponseHeaderBytes {
				continue
			}
			totalBytes = nextSize
			selected[name] = append(selected[name], value)
		}
	}
	return selected
}

func safeResponseHeader(name string) bool {
	for _, allowed := range safeResponseHeaders {
		if name == allowed {
			return true
		}
	}
	return false
}

func firstHeaderValue(headers map[string][]string, name string) string {
	values := headers[name]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func encodeBody(body []byte) (string, string) {
	if utf8.Valid(body) {
		return string(body), "utf-8"
	}
	return base64.StdEncoding.EncodeToString(body), "base64"
}

func finalResponseURL(original *http.Request, response *http.Response) string {
	target := original.URL
	if response.Request != nil && response.Request.URL != nil {
		target = response.Request.URL
	}
	if target == nil {
		// http.NewRequestWithContext always supplies this, but keep output
		// schema-valid if a test or future transport returns a partial request.
		return "about:blank"
	}
	safe := *target
	safe.User = nil
	safe.Fragment = ""
	safe.RawFragment = ""
	return safe.String()
}

func classifyOutboundError(err error) error {
	switch {
	case errors.Is(err, context.Canceled):
		return classified(
			function.FailureCanceled,
			"http_request_canceled",
			"HTTP request was canceled",
			false,
			err,
		)
	case errors.Is(err, context.DeadlineExceeded):
		return classified(
			function.FailureTimeout,
			"http_request_timeout",
			"HTTP request timed out",
			true,
			err,
		)
	case errors.Is(err, netpolicy.ErrInvalidURL):
		return classified(
			function.FailureInvalidRequest,
			"invalid_url",
			"url must be an absolute HTTP or HTTPS URL",
			false,
			err,
		)
	case errors.Is(err, netpolicy.ErrBlockedHost),
		errors.Is(err, netpolicy.ErrBlockedAddress),
		errors.Is(err, netpolicy.ErrPortNotAllowed):
		return classified(
			function.FailureInvalidRequest,
			"outbound_target_blocked",
			"outbound target is blocked by network policy",
			false,
			err,
		)
	case errors.Is(err, netpolicy.ErrTooManyRedirects):
		return classified(
			function.FailureInvalidRequest,
			"redirect_limit_exceeded",
			"HTTP redirect limit was exceeded",
			false,
			err,
		)
	case errors.Is(err, netpolicy.ErrResponseTooLarge):
		return classified(
			function.FailureResourceLimit,
			"response_too_large",
			"HTTP response exceeded the byte limit",
			false,
			err,
		)
	case errors.Is(err, netpolicy.ErrResolution):
		return classified(
			function.FailureTransientNetwork,
			"dns_resolution_failed",
			"outbound host could not be resolved",
			true,
			err,
		)
	case errors.Is(err, netpolicy.ErrInvalidPolicy):
		return classified(
			function.FailureInternal,
			"outbound_policy_invalid",
			"outbound network policy is unavailable",
			false,
			err,
		)
	}

	var unknownAuthority x509.UnknownAuthorityError
	var certificateInvalid x509.CertificateInvalidError
	var hostnameInvalid x509.HostnameError
	if errors.As(err, &unknownAuthority) ||
		errors.As(err, &certificateInvalid) ||
		errors.As(err, &hostnameInvalid) {
		return classified(
			function.FailureExecution,
			"tls_verification_failed",
			"outbound TLS certificate verification failed",
			false,
			err,
		)
	}

	var dnsError *net.DNSError
	if errors.As(err, &dnsError) {
		return classified(
			function.FailureTransientNetwork,
			"dns_resolution_failed",
			"outbound host could not be resolved",
			true,
			err,
		)
	}

	var networkError net.Error
	if errors.As(err, &networkError) {
		return classified(
			function.FailureTransientNetwork,
			"outbound_transport_failed",
			"outbound HTTP transport failed",
			true,
			err,
		)
	}
	return classified(
		function.FailureTransientNetwork,
		"outbound_request_failed",
		"outbound HTTP request failed",
		true,
		err,
	)
}

func classified(
	category function.FailureCategory,
	code string,
	message string,
	retryable bool,
	cause error,
) *function.ClassifiedError {
	result := function.NewClassifiedError(category, code, message, retryable)
	result.Cause = cause
	return result
}
