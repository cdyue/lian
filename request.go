package lian

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/http/httputil"
	"net/url"
	"reflect"
	"time"

	"github.com/klauspost/compress/zstd"
)

// Logger is the interface for logging
type Logger interface {
	Info(msg string, args ...interface{})
	Debug(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
	Error(msg string, args ...interface{})
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

	// Header mapping configuration
	headerMapping HeaderMapping // Custom core header names and extraction methods

	// Configuration flags
	timeout                 time.Duration
	dumpRequest             bool
	dumpResponse            bool
	enableHTTPTrace         bool // Enable HTTP trace logging to console
	disableCompression      bool
	compressRequest         bool
	disableTrace            bool        // Disable OpenTelemetry tracing for this request
	disableTracePropagation bool        // Disable trace header propagation
	disableForwardedFor     bool        // Disable X-Forwarded-For header injection
	disableAutoUnmarshal    bool        // Disable automatic response unmarshaling, return raw body directly
	markAsAsync             bool        // Mark as async request, span kind will be Producer
	result                  interface{} // Automatically unmarshal success response to this object
	errorResult             interface{} // Automatically unmarshal error response to this object
	errorStatusCodes        []int       // Custom list of error status codes
}

// slogLogger wraps slog as default logger implementation
type slogLogger struct {
	logger *slog.Logger
}

func (l *slogLogger) Info(msg string, args ...interface{}) {
	l.logger.Info(msg, args...)
}

func (l *slogLogger) Debug(msg string, args ...interface{}) {
	l.logger.Debug(msg, args...)
}

func (l *slogLogger) Warn(msg string, args ...interface{}) {
	l.logger.Warn(msg, args...)
}

func (l *slogLogger) Error(msg string, args ...interface{}) {
	l.logger.Error(msg, args...)
}

// Global default logger
var defaultLogger Logger = &slogLogger{logger: slog.Default()}

// NewRequest creates a new HTTP request with default settings
func NewRequest() *Request {
	return &Request{
		client:        defaultClient,
		header:        make(http.Header),
		queryParams:   make(url.Values),
		cookies:       make([]*http.Cookie, 0),
		logger:        defaultLogger,
		headerMapping: defaultHeaderMapping, // Use global default header configuration
	}
}

// SetHeaderMapping sets custom header mapping configuration for current request
// Completely overrides the default configuration
func (r *Request) SetHeaderMapping(mapping HeaderMapping) *Request {
	r.headerMapping = mapping
	return r
}

// SetAuthHeaderName sets custom authentication header name for current request
func (r *Request) SetAuthHeaderName(headerName string) *Request {
	r.headerMapping.AuthHeader = headerName
	return r
}

// SetTenantIDHeaderName sets custom tenant ID header name for current request
func (r *Request) SetTenantIDHeaderName(headerName string) *Request {
	r.headerMapping.TenantIDHeader = headerName
	return r
}

// SetUserIDHeaderName sets custom user ID header name for current request
func (r *Request) SetUserIDHeaderName(headerName string) *Request {
	r.headerMapping.UserIDHeader = headerName
	return r
}

// SetTargetTenantHeaderName sets custom target tenant header name for current request
func (r *Request) SetTargetTenantHeaderName(headerName string) *Request {
	r.headerMapping.TargetTenantHeader = headerName
	return r
}

// SetEntryPathHeaderName sets custom entry path header name for current request
func (r *Request) SetEntryPathHeaderName(headerName string) *Request {
	r.headerMapping.EntryPathHeader = headerName
	return r
}

// SetAuthExtractor sets custom authentication extractor function for current request
func (r *Request) SetAuthExtractor(extractor HeaderExtractor) *Request {
	r.headerMapping.AuthExtractor = extractor
	return r
}

// SetTenantIDExtractor sets custom tenant ID extractor function for current request
func (r *Request) SetTenantIDExtractor(extractor HeaderExtractor) *Request {
	r.headerMapping.TenantIDExtractor = extractor
	return r
}

// SetUserIDExtractor sets custom user ID extractor function for current request
func (r *Request) SetUserIDExtractor(extractor HeaderExtractor) *Request {
	r.headerMapping.UserIDExtractor = extractor
	return r
}

// SetTargetTenantExtractor sets custom target tenant extractor function for current request
func (r *Request) SetTargetTenantExtractor(extractor HeaderExtractor) *Request {
	r.headerMapping.TargetTenantExtractor = extractor
	return r
}

// SetEntryPathExtractor sets custom entry path extractor function for current request
func (r *Request) SetEntryPathExtractor(extractor HeaderExtractor) *Request {
	r.headerMapping.EntryPathExtractor = extractor
	return r
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

// HeaderExtractor is a function type for custom value extraction from context
type HeaderExtractor func(ctx context.Context) string

// HeaderMapping configures custom names and extraction methods for the 5 core headers
type HeaderMapping struct {
	// Custom header names
	AuthHeader         string // Authentication header, default "Authorization"
	TenantIDHeader     string // Tenant ID header, default "X-Tenant-Id"
	UserIDHeader       string // User ID header, default "X-User-Id"
	TargetTenantHeader string // Target tenant header, default "X-Target-Tenant"
	EntryPathHeader    string // Entry path header, default "X-Entry-Path"

	// Custom extractor functions, take precedence over header name configuration
	AuthExtractor         HeaderExtractor // Custom authentication info extraction
	TenantIDExtractor     HeaderExtractor // Custom tenant ID extraction
	UserIDExtractor       HeaderExtractor // Custom user ID extraction
	TargetTenantExtractor HeaderExtractor // Custom target tenant extraction
	EntryPathExtractor    HeaderExtractor // Custom entry path extraction
}

// Default header configuration
var defaultHeaderMapping = HeaderMapping{
	AuthHeader:         "Authorization",
	TenantIDHeader:     "X-Tenant-Id",
	UserIDHeader:       "X-User-Id",
	TargetTenantHeader: "X-Target-Tenant",
	EntryPathHeader:    "X-Entry-Path",
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

// FromContext extracts core headers from standard context.Context according to header mapping configuration
// All headers are optional, only non-empty values will be set
func (r *Request) FromContext(ctx context.Context) *Request {
	m := r.headerMapping

	// Extract authentication info
	var auth string
	if m.AuthExtractor != nil {
		auth = m.AuthExtractor(ctx)
	} else if m.AuthHeader != "" {
		auth = GetHeaderFromContext(ctx, m.AuthHeader)
	}
	if auth != "" {
		r.SetHeader(m.AuthHeader, auth)
	}

	// Extract tenant ID
	var tenantID string
	if m.TenantIDExtractor != nil {
		tenantID = m.TenantIDExtractor(ctx)
	} else if m.TenantIDHeader != "" {
		tenantID = GetHeaderFromContext(ctx, m.TenantIDHeader)
	}
	if tenantID != "" {
		r.SetHeader(m.TenantIDHeader, tenantID)
	}

	// Extract user ID
	var userID string
	if m.UserIDExtractor != nil {
		userID = m.UserIDExtractor(ctx)
	} else if m.UserIDHeader != "" {
		userID = GetHeaderFromContext(ctx, m.UserIDHeader)
	}
	if userID != "" {
		r.SetHeader(m.UserIDHeader, userID)
	}

	// Extract target tenant
	var targetTenant string
	if m.TargetTenantExtractor != nil {
		targetTenant = m.TargetTenantExtractor(ctx)
	} else if m.TargetTenantHeader != "" {
		targetTenant = GetHeaderFromContext(ctx, m.TargetTenantHeader)
	}
	if targetTenant != "" {
		r.SetHeader(m.TargetTenantHeader, targetTenant)
	}

	// Extract entry path
	var entryPath string
	if m.EntryPathExtractor != nil {
		entryPath = m.EntryPathExtractor(ctx)
	} else if m.EntryPathHeader != "" {
		entryPath = GetHeaderFromContext(ctx, m.EntryPathHeader)
	}
	if entryPath != "" {
		r.SetHeader(m.EntryPathHeader, entryPath)
	}

	return r
}

// SetTenant sets X-Tenant-Id header
func (r *Request) SetTenant(tenant string) *Request {
	if tenant == "" {
		r.header.Del("X-Tenant-Id")
		return r
	}
	r.header.Set("X-Tenant-Id", tenant)
	return r
}

// SetOperator sets X-User-Id header
func (r *Request) SetOperator(operator string) *Request {
	if operator == "" {
		r.header.Del("X-User-Id")
		return r
	}
	r.header.Set("X-User-Id", operator)
	return r
}

// SetTenantUser sets both tenant and operator headers
func (r *Request) SetTenantUser(tenant, operator string) *Request {
	return r.SetTenant(tenant).SetOperator(operator)
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

// DisableTracePropagation disables trace header injection and propagation
func (r *Request) DisableTracePropagation() *Request {
	r.disableTracePropagation = true
	return r
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
func (r *Request) SetResult(v interface{}) *Request {
	r.result = v
	return r
}

// SetErrorResult sets the error result object for automatic error response parsing
func (r *Request) SetErrorResult(v interface{}) *Request {
	r.errorResult = v
	return r
}

// SetErrorStatusCodes sets custom error status codes
func (r *Request) SetErrorStatusCodes(codes ...int) *Request {
	r.errorStatusCodes = codes
	return r
}

// SetJSONBody sets the request body as JSON
func (r *Request) SetJSONBody(v interface{}) *Request {
	r.body = jsonBodyProvider{payload: v}
	r.SetContentType(r.body.ContentType())
	return r
}

// SetFormBody sets the request body as form encoded
func (r *Request) SetFormBody(v interface{}) *Request {
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

// Send executes the request
func (r *Request) Send(ctx context.Context) *Response {
	if r.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.timeout)
		defer cancel()
	}

	// Validate that result and errorResult must be pointers
	if r.result != nil && !isPointer(r.result) {
		return NewResponse(nil, fmt.Errorf("result must be a pointer"))
	}
	if r.errorResult != nil && !isPointer(r.errorResult) {
		return NewResponse(nil, fmt.Errorf("error result must be a pointer"))
	}

	req, err := r.buildRequest(ctx)
	if err != nil {
		return NewResponse(nil, err)
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
	}

	// Inject trace headers
	if !r.disableTracePropagation {
		InjectTraceHeaders(ctx, req.Header)
	}

	// Set span request attributes
	SetSpanRequestAttributes(span, r.method, req.URL.String(), req.URL.Host, req.URL.Path)

	if r.dumpRequest {
		dump, err := httputil.DumpRequestOut(req, true)
		if err == nil {
			r.logger.Debug("HTTP Request", "dump", string(dump))
		} else {
			r.logger.Warn("Failed to dump request", "error", err)
		}
	}

	if r.enableHTTPTrace {
		// Create OTel trace (if enabled) and console trace
		otelTrace := CreateClientTrace(span)

		// Console trace logging
		logger := r.logger
		consoleTrace := &httptrace.ClientTrace{
			GetConn: func(hostPort string) {
				logger.Debug("HTTP Trace: Connecting", "host_port", hostPort)
			},
			GotConn: func(info httptrace.GotConnInfo) {
				logger.Debug("HTTP Trace: Connected",
					"remote_addr", info.Conn.RemoteAddr().String(),
					"reused", info.Reused,
				)
			},
			DNSStart: func(info httptrace.DNSStartInfo) {
				logger.Debug("HTTP Trace: Resolving DNS", "host", info.Host)
			},
			DNSDone: func(info httptrace.DNSDoneInfo) {
				if info.Err != nil {
					logger.Warn("HTTP Trace: DNS resolution failed", "error", info.Err)
				} else {
					addrs := make([]string, len(info.Addrs))
					for i, addr := range info.Addrs {
						addrs[i] = addr.String()
					}
					logger.Debug("HTTP Trace: DNS resolved", "addresses", addrs)
				}
			},
			ConnectStart: func(network, addr string) {
				logger.Debug("HTTP Trace: Dialing", "network", network, "address", addr)
			},
			ConnectDone: func(network, addr string, err error) {
				if err != nil {
					logger.Warn("HTTP Trace: Dial failed", "error", err)
				} else {
					logger.Debug("HTTP Trace: Connected", "network", network, "address", addr)
				}
			},
			TLSHandshakeStart: func() {
				logger.Debug("HTTP Trace: Starting TLS handshake")
			},
			TLSHandshakeDone: func(state tls.ConnectionState, err error) {
				if err != nil {
					logger.Warn("HTTP Trace: TLS handshake failed", "error", err)
				} else {
					logger.Debug("HTTP Trace: TLS handshake completed",
						"tls_version", fmt.Sprintf("%x", state.Version),
						"cipher_suite", tls.CipherSuiteName(state.CipherSuite),
					)
				}
			},
			WroteHeaders: func() {
				logger.Debug("HTTP Trace: Wrote request headers")
			},
			WroteRequest: func(info httptrace.WroteRequestInfo) {
				if info.Err != nil {
					logger.Warn("HTTP Trace: Failed to write request", "error", info.Err)
				} else {
					logger.Debug("HTTP Trace: Wrote full request")
				}
			},
			GotFirstResponseByte: func() {
				logger.Debug("HTTP Trace: Received first response byte")
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
		return NewResponse(nil, err)
	}

	// Set span response attributes
	SetSpanResponseAttributes(span, resp.StatusCode)

	if r.dumpResponse {
		dump, err := httputil.DumpResponse(resp, true)
		if err == nil {
			r.logger.Debug("HTTP Response", "dump", string(dump))
		} else {
			r.logger.Warn("Failed to dump response", "error", err)
		}
	}

	response := NewResponse(resp, nil)

	// Auto unmarshal response
	if !r.disableAutoUnmarshal {
		if resp.StatusCode >= 200 && resp.StatusCode < 300 && r.result != nil {
			if err := response.JSON(r.result); err != nil {
				RecordSpanError(span, err)
				return NewResponse(resp, fmt.Errorf("failed to unmarshal success response: %w", err))
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
					return NewResponse(resp, fmt.Errorf("failed to unmarshal error response: %w", err))
				}
			}
		}
	}

	return response
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
			encoder, err := zstd.NewWriter(&compressed)
			if err != nil {
				return nil, fmt.Errorf("failed to create zstd encoder: %w", err)
			}
			if _, err := encoder.Write(bodyBytes); err != nil {
				encoder.Close()
				return nil, fmt.Errorf("failed to compress body: %w", err)
			}
			encoder.Close()
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

	for _, t := range traces {
		if t == nil {
			continue
		}

		// GetConn
		if t.GetConn != nil {
			prev := merged.GetConn
			merged.GetConn = func(hostPort string) {
				if prev != nil {
					prev(hostPort)
				}
				t.GetConn(hostPort)
			}
		}

		// GotConn
		if t.GotConn != nil {
			prev := merged.GotConn
			merged.GotConn = func(info httptrace.GotConnInfo) {
				if prev != nil {
					prev(info)
				}
				t.GotConn(info)
			}
		}

		// DNSStart
		if t.DNSStart != nil {
			prev := merged.DNSStart
			merged.DNSStart = func(info httptrace.DNSStartInfo) {
				if prev != nil {
					prev(info)
				}
				t.DNSStart(info)
			}
		}

		// DNSDone
		if t.DNSDone != nil {
			prev := merged.DNSDone
			merged.DNSDone = func(info httptrace.DNSDoneInfo) {
				if prev != nil {
					prev(info)
				}
				t.DNSDone(info)
			}
		}

		// ConnectStart
		if t.ConnectStart != nil {
			prev := merged.ConnectStart
			merged.ConnectStart = func(network, addr string) {
				if prev != nil {
					prev(network, addr)
				}
				t.ConnectStart(network, addr)
			}
		}

		// ConnectDone
		if t.ConnectDone != nil {
			prev := merged.ConnectDone
			merged.ConnectDone = func(network, addr string, err error) {
				if prev != nil {
					prev(network, addr, err)
				}
				t.ConnectDone(network, addr, err)
			}
		}

		// TLSHandshakeStart
		if t.TLSHandshakeStart != nil {
			prev := merged.TLSHandshakeStart
			merged.TLSHandshakeStart = func() {
				if prev != nil {
					prev()
				}
				t.TLSHandshakeStart()
			}
		}

		// TLSHandshakeDone
		if t.TLSHandshakeDone != nil {
			prev := merged.TLSHandshakeDone
			merged.TLSHandshakeDone = func(state tls.ConnectionState, err error) {
				if prev != nil {
					prev(state, err)
				}
				t.TLSHandshakeDone(state, err)
			}
		}

		// WroteHeaders
		if t.WroteHeaders != nil {
			prev := merged.WroteHeaders
			merged.WroteHeaders = func() {
				if prev != nil {
					prev()
				}
				t.WroteHeaders()
			}
		}

		// WroteRequest
		if t.WroteRequest != nil {
			prev := merged.WroteRequest
			merged.WroteRequest = func(info httptrace.WroteRequestInfo) {
				if prev != nil {
					prev(info)
				}
				t.WroteRequest(info)
			}
		}

		// GotFirstResponseByte
		if t.GotFirstResponseByte != nil {
			prev := merged.GotFirstResponseByte
			merged.GotFirstResponseByte = func() {
				if prev != nil {
					prev()
				}
				t.GotFirstResponseByte()
			}
		}
	}

	return merged
}

// isPointer checks if a value is a pointer
func isPointer(v interface{}) bool {
	return reflect.ValueOf(v).Kind() == reflect.Ptr
}
