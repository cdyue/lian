package lian

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/http/httputil"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/klauspost/compress/zstd"
	"go.opentelemetry.io/contrib/propagators/b3"
	"go.opentelemetry.io/otel/propagation"
)

// Logger is the interface for logging
type Logger interface {
	Info(msg string, args ...any)
	Debug(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

// Request represents an HTTP request
type Request struct {
	client      *http.Client
	method      string
	url         string
	header      http.Header
	queryParams url.Values
	body        BodyProvider
	cookies     []*http.Cookie
	logger      Logger // Logger instance


	// Configuration flags
	timeout                 time.Duration
	dumpRequest             bool
	dumpResponse            bool
	enableHTTPTrace         bool // Enable HTTP trace logging to console
	disableCompression      bool
	compressRequest         bool
	disableTrace            bool  // Disable OpenTelemetry tracing for this request
	disableTracePropagation bool  // Disable trace header propagation
	disableForwardedFor     bool  // Disable X-Forwarded-For header injection
	disableAutoUnmarshal    bool  // Disable automatic response unmarshaling, return raw body directly
	markAsAsync             bool  // Mark as async request, span kind will be Producer
	result                  any   // Automatically unmarshal success response to this object
	errorResult             any   // Automatically unmarshal error response to this object
	errorStatusCodes        []int // Custom list of error status codes

	// Retry configuration
	maxRetries         int           // Maximum number of retries, 0 means no retries
	retryInterval      time.Duration // Base retry interval
	retryBackoffFactor float64       // Exponential backoff factor
	retryJitter        float64       // Jitter factor (0-1)
	retryableStatuses  []int         // HTTP status codes that should trigger a retry

	// Zstd configuration
	zstdCompressionLevel int    // Zstd compression level
	zstdDictionary       []byte // Pre-trained zstd dictionary
	zstdEnablePooling    bool   // Enable zstd encoder/decoder pooling

	// Trace propagation configuration (per-request override)
	propagators propagation.TextMapPropagator // Custom trace propagators for this request
}

// slogLogger wraps slog as default logger implementation
type slogLogger struct {
	logger *slog.Logger
}

func (l *slogLogger) Info(msg string, args ...any) {
	l.logger.Info(msg, args...)
}

func (l *slogLogger) Debug(msg string, args ...any) {
	l.logger.Debug(msg, args...)
}

func (l *slogLogger) Warn(msg string, args ...any) {
	l.logger.Warn(msg, args...)
}

func (l *slogLogger) Error(msg string, args ...any) {
	l.logger.Error(msg, args...)
}

// Global default logger
var defaultLogger Logger = &slogLogger{logger: slog.Default()}

// Global zstd encoder pool
var zstdEncoderPool = &sync.Pool{
	New: func() any {
		encoder, _ := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
		return encoder
	},
}

// GetZstdEncoder gets an encoder from the pool or creates a new one
func GetZstdEncoder(w io.Writer, level int, dict []byte) (*zstd.Encoder, error) {
	// If dictionary is provided, we can't use the pool
	if dict != nil {
		return zstd.NewWriter(w, zstd.WithEncoderLevel(zstd.EncoderLevel(level)), zstd.WithEncoderDict(dict))
	}

	encoder := zstdEncoderPool.Get().(*zstd.Encoder)
	encoder.Reset(w)
	return encoder, nil
}

// PutZstdEncoder returns an encoder to the pool
func PutZstdEncoder(encoder *zstd.Encoder) {
	// Don't pool encoders with custom dictionaries
	if encoder != nil {
		encoder.Close()
		zstdEncoderPool.Put(encoder)
	}
}

// NewRequest creates a new HTTP request with default settings
// RequestOption is a functional option for configuring Request
type RequestOption func(*Request)

// NewRequest creates a new Request instance with default configuration
func NewRequest(opts ...RequestOption) *Request {
	r := &Request{
		client:        defaultClient,
		header:        make(http.Header),
		queryParams:   make(url.Values),
		cookies:       make([]*http.Cookie, 0),
		logger:        defaultLogger,
		// Retry defaults (same as client defaults
		maxRetries:         0, // Disable by default for backward compatibility
		retryInterval:      100 * time.Millisecond,
		retryBackoffFactor: 2.0,
		retryJitter:        0.2,
		retryableStatuses:  []int{429, 500, 502, 503, 504},
		// Zstd defaults
		zstdCompressionLevel: int(zstd.SpeedDefault),
		zstdEnablePooling:    true, // Enable pooling by default for better performance
	}

	// Apply functional options
	for _, opt := range opts {
		opt(r)
	}

	return r
}

// WithTracePropagationFormats sets trace propagation formats for the request
func WithTracePropagationFormats(formats ...string) RequestOption {
	return func(r *Request) {
		r.SetTracePropagationFormats(formats...)
	}
}

// WithB3TracePropagation enables B3 single header propagation format
func WithB3TracePropagation() RequestOption {
	return func(r *Request) {
		r.EnableB3TracePropagation()
	}
}

// WithCompositeTracePropagation enables both W3C and B3 propagation formats
func WithCompositeTracePropagation() RequestOption {
	return func(r *Request) {
		r.EnableCompositeTracePropagation()
	}
}



// SetClient sets a custom http.Client
func (r *Request) SetClient(client *http.Client) *Request {
	r.client = client
	return r
}

// SetTimeout sets the request timeout
func (r *Request) SetTimeout(timeout time.Duration) *Request {
	r.timeout = timeout
	return r
}

// SetMethod sets the HTTP method
func (r *Request) SetMethod(method string) *Request {
	r.method = method
	return r
}

// SetURL sets the request URL
func (r *Request) SetURL(u string) *Request {
	r.url = u
	return r
}

// SetHeader sets a single header
func (r *Request) SetHeader(key, value string) *Request {
	r.header.Set(key, value)
	return r
}

// AddHeader adds a header (appends if already exists)
func (r *Request) AddHeader(key, value string) *Request {
	r.header.Add(key, value)
	return r
}

// SetHeaders sets multiple headers from a map
func (r *Request) SetHeaders(headers map[string]string) *Request {
	for k, v := range headers {
		r.header.Set(k, v)
	}
	return r
}

// RemoveHeader deletes a header
func (r *Request) RemoveHeader(key string) *Request {
	r.header.Del(key)
	return r
}

// SetQueryParam sets a single query parameter
func (r *Request) SetQueryParam(key, value string) *Request {
	r.queryParams.Set(key, value)
	return r
}

// AddQueryParam adds a query parameter (appends if already exists)
func (r *Request) AddQueryParam(key, value string) *Request {
	r.queryParams.Add(key, value)
	return r
}

// SetQueryParams sets multiple query parameters from a map
func (r *Request) SetQueryParams(params map[string]string) *Request {
	for k, v := range params {
		r.queryParams.Set(k, v)
	}
	return r
}

// SetQueryValues sets multiple query parameters from url.Values
func (r *Request) SetQueryValues(values url.Values) *Request {
	for k, v := range values {
		for _, val := range v {
			r.queryParams.Add(k, val)
		}
	}
	return r
}

// SetCookie adds a cookie to the request
func (r *Request) SetCookie(cookie *http.Cookie) *Request {
	r.cookies = append(r.cookies, cookie)
	return r
}

// SetCookies sets multiple cookies
func (r *Request) SetCookies(cookies []*http.Cookie) *Request {
	r.cookies = append(r.cookies, cookies...)
	return r
}

// SetBasicAuth sets basic authentication
func (r *Request) SetBasicAuth(username, password string) *Request {
	r.header.Set("Authorization", basicAuth(username, password))
	return r
}

// SetBearerToken sets bearer token authentication
func (r *Request) SetBearerToken(token string) *Request {
	r.header.Set("Authorization", "Bearer "+token)
	return r
}


// Standard context key definition
type contextKey string

const (
	// ContextHeaderPrefix is the prefix for HTTP headers stored in Context
	ContextHeaderPrefix contextKey = "http.header."
)

// GetHeaderFromContext retrieves the value of the specified HTTP header from Context
func GetHeaderFromContext(ctx context.Context, headerName string) string {
	if val := ctx.Value(ContextHeaderPrefix + contextKey(headerName)); val != nil {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// SetHeaderToContext stores the HTTP header value into Context
func SetHeaderToContext(ctx context.Context, headerName, value string) context.Context {
	return context.WithValue(ctx, ContextHeaderPrefix+contextKey(headerName), value)
}

// SetUserAgent sets the User-Agent header
func (r *Request) SetUserAgent(ua string) *Request {
	r.header.Set("User-Agent", ua)
	return r
}



// SetContentType sets the Content-Type header
func (r *Request) SetContentType(ct string) *Request {
	r.header.Set("Content-Type", ct)
	return r
}

// SetAccept sets the Accept header
func (r *Request) SetAccept(accept string) *Request {
	r.header.Set("Accept", accept)
	return r
}

// EnableDumpRequest enables request dumping to logs
func (r *Request) EnableDumpRequest() *Request {
	r.dumpRequest = true
	return r
}

// EnableDumpResponse enables response dumping to logs
func (r *Request) EnableDumpResponse() *Request {
	r.dumpResponse = true
	return r
}

// EnableHTTPTrace enables HTTP trace logging to console
func (r *Request) EnableHTTPTrace() *Request {
	r.enableHTTPTrace = true
	return r
}

// DisableOtelTrace disables OpenTelemetry tracing for current request
func (r *Request) DisableOtelTrace() *Request {
	r.disableTrace = true
	return r
}

// EnableOtelTraceForRequest enables OpenTelemetry tracing for current request (overrides global setting)
func (r *Request) EnableOtelTraceForRequest() *Request {
	r.disableTrace = false
	return r
}

// DisableCompression disables response compression
func (r *Request) DisableCompression() *Request {
	r.disableCompression = true
	r.header.Del("Accept-Encoding")
	return r
}

// EnableZstdCompression enables zstd compression for request body
func (r *Request) EnableZstdCompression() *Request {
	r.compressRequest = true
	r.header.Set("Content-Encoding", "zstd")
	return r
}

// EnableZstdCompressionWithLevel enables zstd compression with custom level
func (r *Request) EnableZstdCompressionWithLevel(level int) *Request {
	r.compressRequest = true
	r.zstdCompressionLevel = level
	r.header.Set("Content-Encoding", "zstd")
	return r
}

// SetZstdCompressionLevel sets the zstd compression level for this request
func (r *Request) SetZstdCompressionLevel(level int) *Request {
	r.zstdCompressionLevel = level
	return r
}

// SetZstdDictionary sets the pre-trained zstd dictionary for this request
func (r *Request) SetZstdDictionary(dict []byte) *Request {
	r.zstdDictionary = dict
	return r
}

// SetRetry enables retry for this request with specified max retries
func (r *Request) SetRetry(maxRetries int) *Request {
	r.maxRetries = maxRetries
	return r
}

// SetRetryConfig sets full retry configuration for this request
func (r *Request) SetRetryConfig(maxRetries int, interval time.Duration, backoffFactor float64, jitter float64) *Request {
	r.maxRetries = maxRetries
	r.retryInterval = interval
	r.retryBackoffFactor = backoffFactor
	r.retryJitter = jitter
	return r
}

// SetRetryableStatuses sets custom retryable status codes for this request
func (r *Request) SetRetryableStatuses(statuses ...int) *Request {
	r.retryableStatuses = statuses
	return r
}

// DisableTracePropagation disables trace header injection and propagation
func (r *Request) DisableTracePropagation() *Request {
	r.disableTracePropagation = true
	return r
}

// SetTracePropagationFormats sets trace propagation formats for this request (overrides global setting)
// Supported formats: "w3c" (default), "b3", "b3multi"
func (r *Request) SetTracePropagationFormats(formats ...string) *Request {
	var propagators []propagation.TextMapPropagator

	for _, format := range formats {
		switch format {
		case TracePropagationW3C:
			propagators = append(propagators, propagation.TraceContext{})
		case TracePropagationB3:
			propagators = append(propagators, b3.New(b3.WithInjectEncoding(b3.B3SingleHeader)))
		case TracePropagationB3Multi:
			propagators = append(propagators, b3.New(b3.WithInjectEncoding(b3.B3MultipleHeader)))
		}
	}

	if len(propagators) == 0 {
		// Default to W3C if no valid formats provided
		r.propagators = propagation.TraceContext{}
	} else if len(propagators) == 1 {
		r.propagators = propagators[0]
	} else {
		r.propagators = propagation.NewCompositeTextMapPropagator(propagators...)
	}
	return r
}

// EnableB3TracePropagation enables B3 single header propagation format for this request
func (r *Request) EnableB3TracePropagation() *Request {
	return r.SetTracePropagationFormats(TracePropagationB3)
}

// EnableCompositeTracePropagation enables both W3C and B3 propagation formats for this request
func (r *Request) EnableCompositeTracePropagation() *Request {
	return r.SetTracePropagationFormats(TracePropagationW3C, TracePropagationB3)
}

// DisableForwardedFor disables X-Forwarded-For header injection
func (r *Request) DisableForwardedFor() *Request {
	r.disableForwardedFor = true
	return r
}

// DisableAutoUnmarshal disables automatic response unmarshaling
func (r *Request) DisableAutoUnmarshal() *Request {
	r.disableAutoUnmarshal = true
	return r
}

// MarkAsAsync marks request as async, span kind will be Producer
func (r *Request) MarkAsAsync() *Request {
	r.markAsAsync = true
	return r
}

// SetResult sets the result object for automatic response parsing
func (r *Request) SetResult(v any) *Request {
	r.result = v
	return r
}

// SetErrorResult sets the error result object for automatic error response parsing
func (r *Request) SetErrorResult(v any) *Request {
	r.errorResult = v
	return r
}

// SetErrorStatusCodes sets custom error status codes
func (r *Request) SetErrorStatusCodes(codes ...int) *Request {
	r.errorStatusCodes = codes
	return r
}

// SetJSONBody sets the request body as JSON
func (r *Request) SetJSONBody(v any) *Request {
	r.body = jsonBodyProvider{payload: v}
	r.SetContentType(r.body.ContentType())
	return r
}

// SetFormBody sets the request body as form encoded
func (r *Request) SetFormBody(v any) *Request {
	r.body = formBodyProvider{payload: v}
	r.SetContentType(r.body.ContentType())
	return r
}

// SetFormValuesBody sets the request body from url.Values
func (r *Request) SetFormValuesBody(values url.Values) *Request {
	r.body = urlValuesBodyProvider{values: values}
	r.SetContentType(r.body.ContentType())
	return r
}

// SetRawBody sets a raw io.Reader as body
func (r *Request) SetRawBody(body io.Reader, contentType string) *Request {
	r.body = rawBodyProvider{body: body, contentType: contentType}
	r.SetContentType(contentType)
	return r
}

// Get makes a GET request
func (r *Request) Get(url string) *Response {
	return r.SetMethod(http.MethodGet).SetURL(url).Send(context.Background())
}

// Post makes a POST request
func (r *Request) Post(url string) *Response {
	return r.SetMethod(http.MethodPost).SetURL(url).Send(context.Background())
}

// Put makes a PUT request
func (r *Request) Put(url string) *Response {
	return r.SetMethod(http.MethodPut).SetURL(url).Send(context.Background())
}

// Delete makes a DELETE request
func (r *Request) Delete(url string) *Response {
	return r.SetMethod(http.MethodDelete).SetURL(url).Send(context.Background())
}

// Patch makes a PATCH request
func (r *Request) Patch(url string) *Response {
	return r.SetMethod(http.MethodPatch).SetURL(url).Send(context.Background())
}

// Head makes a HEAD request
func (r *Request) Head(url string) *Response {
	return r.SetMethod(http.MethodHead).SetURL(url).Send(context.Background())
}

// Options makes an OPTIONS request
func (r *Request) Options(url string) *Response {
	return r.SetMethod(http.MethodOptions).SetURL(url).Send(context.Background())
}

// GetWithContext makes a GET request with custom context
func (r *Request) GetWithContext(ctx context.Context, url string) *Response {
	return r.SetMethod(http.MethodGet).SetURL(url).Send(ctx)
}

// PostWithContext makes a POST request with custom context
func (r *Request) PostWithContext(ctx context.Context, url string) *Response {
	return r.SetMethod(http.MethodPost).SetURL(url).Send(ctx)
}

// PutWithContext makes a PUT request with custom context
func (r *Request) PutWithContext(ctx context.Context, url string) *Response {
	return r.SetMethod(http.MethodPut).SetURL(url).Send(ctx)
}

// DeleteWithContext makes a DELETE request with custom context
func (r *Request) DeleteWithContext(ctx context.Context, url string) *Response {
	return r.SetMethod(http.MethodDelete).SetURL(url).Send(ctx)
}

// PatchWithContext makes a PATCH request with custom context
func (r *Request) PatchWithContext(ctx context.Context, url string) *Response {
	return r.SetMethod(http.MethodPatch).SetURL(url).Send(ctx)
}

// HeadWithContext makes a HEAD request with custom context
func (r *Request) HeadWithContext(ctx context.Context, url string) *Response {
	return r.SetMethod(http.MethodHead).SetURL(url).Send(ctx)
}

// OptionsWithContext makes an OPTIONS request with custom context
func (r *Request) OptionsWithContext(ctx context.Context, url string) *Response {
	return r.SetMethod(http.MethodOptions).SetURL(url).Send(ctx)
}

// sendOnce executes the request once, used by the retry loop
func (r *Request) sendOnce(ctx context.Context, attempt int) (*Response, error) {
	// Validate that result and errorResult must be pointers
	if r.result != nil && !isPointer(r.result) {
		return nil, fmt.Errorf("result must be a pointer")
	}
	if r.errorResult != nil && !isPointer(r.errorResult) {
		return nil, fmt.Errorf("error result must be a pointer")
	}

	req, err := r.buildRequest(ctx)
	if err != nil {
		return nil, err
	}

	// Inject X-Forwarded-For header
	if !r.disableForwardedFor {
		if ip, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
			req.Header.Add("X-Forwarded-For", ip)
		}
	}

	// Distributed tracing (optional)
	var span TraceContext
	if !r.disableTrace {
		ctx, span = StartSpan(ctx, r.method, req.URL.Host, r.markAsAsync)
		defer EndSpan(span)

		// Add retry attributes to span
		if attempt > 0 {
			SetSpanAttribute(span, "retry.attempt", attempt)
			SetSpanAttribute(span, "retry.max_attempts", r.maxRetries)
		}
	}

	// Inject trace headers
	if !r.disableTracePropagation {
		if r.propagators != nil {
			// Use per-request propagators if set
			r.propagators.Inject(ctx, propagation.HeaderCarrier(req.Header))
		} else {
			// Use global propagators
			InjectTraceHeaders(ctx, req.Header)
		}
	}

	// Set span request attributes
	SetSpanRequestAttributes(span, r.method, req.URL.String(), req.URL.Host, req.URL.Path)

	if r.dumpRequest {
		dump, err := httputil.DumpRequestOut(req, true)
		if err == nil {
			r.logger.Debug("HTTP Request", "dump", string(dump), "attempt", attempt)
		} else {
			r.logger.Warn("Failed to dump request", "error", err, "attempt", attempt)
		}
	}

	if r.enableHTTPTrace {
		// Create OTel trace (if enabled) and console trace
		otelTrace := CreateClientTrace(span)

		// Console trace logging
		logger := r.logger
		consoleTrace := &httptrace.ClientTrace{
			GetConn: func(hostPort string) {
				logger.Debug("HTTP Trace: Connecting", "host_port", hostPort, "attempt", attempt)
			},
			GotConn: func(info httptrace.GotConnInfo) {
				logger.Debug("HTTP Trace: Connected",
					"remote_addr", info.Conn.RemoteAddr().String(),
					"reused", info.Reused,
					"attempt", attempt,
				)
			},
			DNSStart: func(info httptrace.DNSStartInfo) {
				logger.Debug("HTTP Trace: Resolving DNS", "host", info.Host, "attempt", attempt)
			},
			DNSDone: func(info httptrace.DNSDoneInfo) {
				if info.Err != nil {
					logger.Warn("HTTP Trace: DNS resolution failed", "error", info.Err, "attempt", attempt)
				} else {
					addrs := make([]string, len(info.Addrs))
					for i, addr := range info.Addrs {
						addrs[i] = addr.String()
					}
					logger.Debug("HTTP Trace: DNS resolved", "addresses", addrs, "attempt", attempt)
				}
			},
			ConnectStart: func(network, addr string) {
				logger.Debug("HTTP Trace: Dialing", "network", network, "address", addr, "attempt", attempt)
			},
			ConnectDone: func(network, addr string, err error) {
				if err != nil {
					logger.Warn("HTTP Trace: Dial failed", "error", err, "attempt", attempt)
				} else {
					logger.Debug("HTTP Trace: Connected", "network", network, "address", addr, "attempt", attempt)
				}
			},
			TLSHandshakeStart: func() {
				logger.Debug("HTTP Trace: Starting TLS handshake", "attempt", attempt)
			},
			TLSHandshakeDone: func(state tls.ConnectionState, err error) {
				if err != nil {
					logger.Warn("HTTP Trace: TLS handshake failed", "error", err, "attempt", attempt)
				} else {
					logger.Debug("HTTP Trace: TLS handshake completed",
						"tls_version", fmt.Sprintf("%x", state.Version),
						"cipher_suite", tls.CipherSuiteName(state.CipherSuite),
						"attempt", attempt,
					)
				}
			},
			WroteHeaders: func() {
				logger.Debug("HTTP Trace: Wrote request headers", "attempt", attempt)
			},
			WroteRequest: func(info httptrace.WroteRequestInfo) {
				if info.Err != nil {
					logger.Warn("HTTP Trace: Failed to write request", "error", info.Err, "attempt", attempt)
				} else {
					logger.Debug("HTTP Trace: Wrote full request", "attempt", attempt)
				}
			},
			GotFirstResponseByte: func() {
				logger.Debug("HTTP Trace: Received first response byte", "attempt", attempt)
			},
		}

		// Merge traces
		var finalTrace *httptrace.ClientTrace
		if otelTrace != nil {
			finalTrace = mergeClientTraces(consoleTrace, otelTrace)
		} else {
			finalTrace = consoleTrace
		}

		ctx = httptrace.WithClientTrace(ctx, finalTrace)
		req = req.WithContext(ctx)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		RecordSpanError(span, err)
		return nil, err
	}

	// Set span response attributes
	SetSpanResponseAttributes(span, resp.StatusCode)

	if r.dumpResponse {
		dump, err := httputil.DumpResponse(resp, true)
		if err == nil {
			r.logger.Debug("HTTP Response", "dump", string(dump), "attempt", attempt)
		} else {
			r.logger.Warn("Failed to dump response", "error", err, "attempt", attempt)
		}
	}

	response := NewResponseWithZstdDict(resp, nil, r.zstdDictionary)

	// Auto unmarshal response
	if !r.disableAutoUnmarshal {
		if resp.StatusCode >= 200 && resp.StatusCode < 300 && r.result != nil {
			if err := response.JSON(r.result); err != nil {
				RecordSpanError(span, err)
				return nil, fmt.Errorf("failed to unmarshal success response: %w", err)
			}
		} else {
			// Check if it's a custom error status code
			isError := false
			if len(r.errorStatusCodes) > 0 {
				for _, code := range r.errorStatusCodes {
					if resp.StatusCode == code {
						isError = true
						break
					}
				}
			} else if resp.StatusCode >= 400 {
				isError = true
			}

			if isError && r.errorResult != nil {
				if err := response.JSON(r.errorResult); err != nil {
					RecordSpanError(span, err)
					return nil, fmt.Errorf("failed to unmarshal error response: %w", err)
				}
			}
		}
	}

	return response, nil
}

// Send executes the request with optional retry
func (r *Request) Send(ctx context.Context) *Response {
	if r.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.timeout)
		defer cancel()
	}

	// If no retries configured, just send once
	if r.maxRetries <= 0 {
		resp, err := r.sendOnce(ctx, 0)
		if err != nil {
			return NewResponse(nil, err)
		}
		return resp
	}

	// Retry loop
	var lastErr error
	var lastResp *Response
	for attempt := 0; attempt <= r.maxRetries; attempt++ {
		resp, err := r.sendOnce(ctx, attempt)
		if err == nil {
			// Check if status code is retryable
			statusCode := resp.StatusCode()
			if !isRetryableStatusCode(statusCode, r.retryableStatuses) {
				return resp
			}
			// Status code is retryable, continue to retry
			lastResp = resp
			lastErr = fmt.Errorf("retryable status code: %d", statusCode)
		} else {
			// Check if error is retryable
			if !isRetryableError(err) {
				return NewResponse(nil, err)
			}
			lastErr = err
		}

		// If this was the last attempt, break
		if attempt >= r.maxRetries {
			break
		}

		// Calculate backoff duration
		backoff := calculateBackoff(attempt, r.retryInterval, r.retryBackoffFactor, r.retryJitter)

		// Log retry
		r.logger.Info("Retrying request",
			"attempt", attempt+1,
			"max_attempts", r.maxRetries,
			"backoff", backoff.String(),
			"error", lastErr.Error(),
		)

		// Wait for backoff or context cancellation
		select {
		case <-ctx.Done():
			return NewResponse(nil, ctx.Err())
		case <-time.After(backoff):
			// Continue to next attempt
		}
	}

	// All retries failed
	if lastResp != nil {
		return NewResponseWithZstdDict(lastResp.Response, lastErr, r.zstdDictionary)
	}
	return NewResponse(nil, lastErr)
}

// buildRequest constructs the http.Request
func (r *Request) buildRequest(ctx context.Context) (*http.Request, error) {
	parsedURL, err := url.Parse(r.url)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	// Merge query parameters
	if len(r.queryParams) > 0 {
		q := parsedURL.Query()
		for k, v := range r.queryParams {
			for _, val := range v {
				q.Add(k, val)
			}
		}
		parsedURL.RawQuery = q.Encode()
	}

	// Handle request body
	var bodyReader io.Reader
	if r.body != nil {
		if r.compressRequest {
			bodyBytes, err := r.body.Bytes()
			if err != nil {
				return nil, fmt.Errorf("failed to get body bytes: %w", err)
			}

			var compressed bytes.Buffer
			encoder, err := GetZstdEncoder(&compressed, r.zstdCompressionLevel, r.zstdDictionary)
			if err != nil {
				return nil, fmt.Errorf("failed to create zstd encoder: %w", err)
			}
			if _, err := encoder.Write(bodyBytes); err != nil {
				if r.zstdDictionary == nil {
					PutZstdEncoder(encoder)
				} else {
					encoder.Close()
				}
				return nil, fmt.Errorf("failed to compress body: %w", err)
			}
			encoder.Close()
			if r.zstdDictionary == nil {
				PutZstdEncoder(encoder)
			}
			bodyReader = &compressed
		} else {
			bodyReader, err = r.body.Body()
			if err != nil {
				return nil, fmt.Errorf("failed to get body: %w", err)
			}
		}
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, r.method, parsedURL.String(), bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header = r.header.Clone()

	// Set cookies
	for _, cookie := range r.cookies {
		req.AddCookie(cookie)
	}

	// Set default user agent if not set
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "lian-http-client/1.0")
	}

	// Set accept encoding if not disabled
	if !r.disableCompression && req.Header.Get("Accept-Encoding") == "" {
		req.Header.Set("Accept-Encoding", "zstd, br, gzip, deflate")
	}

	return req, nil
}

// basicAuth creates a basic auth header value
func basicAuth(username, password string) string {
	auth := username + ":" + password
	return "Basic " + b64encode(auth)
}

// b64encode encodes a string to base64
func b64encode(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

// mergeClientTraces merges multiple httptrace.ClientTrace instances
func mergeClientTraces(traces ...*httptrace.ClientTrace) *httptrace.ClientTrace {
	merged := &httptrace.ClientTrace{}

	// Helper to merge hook functions
	mergeHook := func(existing, new any) any {
		if new == nil {
			return existing
		}
		if existing == nil {
			return new
		}

		switch e := existing.(type) {
		case func(string):
			n := new.(func(string))
			return func(s string) { e(s); n(s) }
		case func(httptrace.GotConnInfo):
			n := new.(func(httptrace.GotConnInfo))
			return func(info httptrace.GotConnInfo) { e(info); n(info) }
		case func(httptrace.DNSStartInfo):
			n := new.(func(httptrace.DNSStartInfo))
			return func(info httptrace.DNSStartInfo) { e(info); n(info) }
		case func(httptrace.DNSDoneInfo):
			n := new.(func(httptrace.DNSDoneInfo))
			return func(info httptrace.DNSDoneInfo) { e(info); n(info) }
		case func(string, string):
			n := new.(func(string, string))
			return func(network, addr string) { e(network, addr); n(network, addr) }
		case func(string, string, error):
			n := new.(func(string, string, error))
			return func(network, addr string, err error) { e(network, addr, err); n(network, addr, err) }
		case func():
			n := new.(func())
			return func() { e(); n() }
		case func(tls.ConnectionState, error):
			n := new.(func(tls.ConnectionState, error))
			return func(state tls.ConnectionState, err error) { e(state, err); n(state, err) }
		case func(httptrace.WroteRequestInfo):
			n := new.(func(httptrace.WroteRequestInfo))
			return func(info httptrace.WroteRequestInfo) { e(info); n(info) }
		default:
			return new
		}
	}

	for _, t := range traces {
		if t == nil {
			continue
		}

		merged.GetConn = mergeHook(merged.GetConn, t.GetConn).(func(string))
		merged.GotConn = mergeHook(merged.GotConn, t.GotConn).(func(httptrace.GotConnInfo))
		merged.DNSStart = mergeHook(merged.DNSStart, t.DNSStart).(func(httptrace.DNSStartInfo))
		merged.DNSDone = mergeHook(merged.DNSDone, t.DNSDone).(func(httptrace.DNSDoneInfo))
		merged.ConnectStart = mergeHook(merged.ConnectStart, t.ConnectStart).(func(string, string))
		merged.ConnectDone = mergeHook(merged.ConnectDone, t.ConnectDone).(func(string, string, error))
		merged.TLSHandshakeStart = mergeHook(merged.TLSHandshakeStart, t.TLSHandshakeStart).(func())
		merged.TLSHandshakeDone = mergeHook(merged.TLSHandshakeDone, t.TLSHandshakeDone).(func(tls.ConnectionState, error))
		merged.WroteHeaders = mergeHook(merged.WroteHeaders, t.WroteHeaders).(func())
		merged.WroteRequest = mergeHook(merged.WroteRequest, t.WroteRequest).(func(httptrace.WroteRequestInfo))
		merged.GotFirstResponseByte = mergeHook(merged.GotFirstResponseByte, t.GotFirstResponseByte).(func())
	}

	return merged
}

// isPointer checks if a value is a pointer
func isPointer(v any) bool {
	return reflect.ValueOf(v).Kind() == reflect.Ptr
}

// isRetryableError checks if an error should trigger a retry
func isRetryableError(err error) bool {
	// Check for timeout errors, connection errors, etc.
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	// Check for other retryable error types
	if errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, io.ErrUnexpectedEOF) ||
		strings.Contains(err.Error(), "connection reset by peer") ||
		strings.Contains(err.Error(), "connection refused") ||
		strings.Contains(err.Error(), "EOF") {
		return true
	}
	return false
}

// isRetryableStatusCode checks if a status code should trigger a retry
func isRetryableStatusCode(code int, retryableStatuses []int) bool {
	for _, s := range retryableStatuses {
		if code == s {
			return true
		}
	}
	return false
}

// calculateBackoff calculates the backoff duration with jitter
func calculateBackoff(attempt int, baseInterval time.Duration, backoffFactor float64, jitter float64) time.Duration {
	// Exponential backoff: baseInterval * (backoffFactor ^ attempt)
	backoff := float64(baseInterval) * math.Pow(backoffFactor, float64(attempt))

	// Add jitter: random value between backoff*(1-jitter) and backoff*(1+jitter)
	if jitter > 0 {
		jitterAmount := backoff * jitter
		backoff = backoff - jitterAmount + (rand.Float64() * 2 * jitterAmount)
	}

	return time.Duration(backoff)
}
