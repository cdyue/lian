package lian

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Global default request instance
var defaultRequest = NewRequest()

// Global trace enable flag
var enableTraceGlobal = true

// SetTimeout sets the default timeout for all requests
func SetTimeout(timeout time.Duration) {
	defaultRequest.SetTimeout(timeout)
}

// SetClient sets the default http.Client for all requests
func SetClient(client *http.Client) {
	defaultRequest.SetClient(client)
}

// SetHeader sets a default header for all requests
func SetHeader(key, value string) {
	defaultRequest.SetHeader(key, value)
}

// SetUserAgent sets the default User-Agent for all requests
func SetUserAgent(ua string) {
	defaultRequest.SetUserAgent(ua)
}

// SetBearerToken sets the default bearer token for all requests
func SetBearerToken(token string) {
	defaultRequest.SetBearerToken(token)
}

// SetBasicAuth sets the default basic auth for all requests
func SetBasicAuth(username, password string) {
	defaultRequest.SetBasicAuth(username, password)
}

// SetLogger sets the global logger for all requests
func SetLogger(logger Logger) {
	if logger != nil {
		defaultLogger = logger
		// Also update the logger of the global default request
		defaultRequest.logger = logger
	}
}

// SetDefaultHeaderMapping sets the global default header mapping configuration
// All newly created Request instances will inherit this configuration
func SetDefaultHeaderMapping(mapping HeaderMapping) {
	defaultHeaderMapping = mapping
	defaultRequest.headerMapping = mapping
}

// SetDefaultAuthHeaderName sets the global default authentication header name
func SetDefaultAuthHeaderName(headerName string) {
	defaultHeaderMapping.AuthHeader = headerName
	defaultRequest.headerMapping.AuthHeader = headerName
}

// SetDefaultTenantIDHeaderName sets the global default tenant ID header name
func SetDefaultTenantIDHeaderName(headerName string) {
	defaultHeaderMapping.TenantIDHeader = headerName
	defaultRequest.headerMapping.TenantIDHeader = headerName
}

// SetDefaultUserIDHeaderName sets the global default user ID header name
func SetDefaultUserIDHeaderName(headerName string) {
	defaultHeaderMapping.UserIDHeader = headerName
	defaultRequest.headerMapping.UserIDHeader = headerName
}

// SetDefaultTargetTenantHeaderName sets the global default target tenant header name
func SetDefaultTargetTenantHeaderName(headerName string) {
	defaultHeaderMapping.TargetTenantHeader = headerName
	defaultRequest.headerMapping.TargetTenantHeader = headerName
}

// SetDefaultEntryPathHeaderName sets the global default entry path header name
func SetDefaultEntryPathHeaderName(headerName string) {
	defaultHeaderMapping.EntryPathHeader = headerName
	defaultRequest.headerMapping.EntryPathHeader = headerName
}

// SetDefaultAuthExtractor sets the global default authentication extractor function
func SetDefaultAuthExtractor(extractor HeaderExtractor) {
	defaultHeaderMapping.AuthExtractor = extractor
	defaultRequest.headerMapping.AuthExtractor = extractor
}

// SetDefaultTenantIDExtractor sets the global default tenant ID extractor function
func SetDefaultTenantIDExtractor(extractor HeaderExtractor) {
	defaultHeaderMapping.TenantIDExtractor = extractor
	defaultRequest.headerMapping.TenantIDExtractor = extractor
}

// SetDefaultUserIDExtractor sets the global default user ID extractor function
func SetDefaultUserIDExtractor(extractor HeaderExtractor) {
	defaultHeaderMapping.UserIDExtractor = extractor
	defaultRequest.headerMapping.UserIDExtractor = extractor
}

// SetDefaultTargetTenantExtractor sets the global default target tenant extractor function
func SetDefaultTargetTenantExtractor(extractor HeaderExtractor) {
	defaultHeaderMapping.TargetTenantExtractor = extractor
	defaultRequest.headerMapping.TargetTenantExtractor = extractor
}

// SetDefaultEntryPathExtractor sets the global default entry path extractor function
func SetDefaultEntryPathExtractor(extractor HeaderExtractor) {
	defaultHeaderMapping.EntryPathExtractor = extractor
	defaultRequest.headerMapping.EntryPathExtractor = extractor
}

// EnableDumpRequest enables request dumping for all requests
func EnableDumpRequest() {
	defaultRequest.EnableDumpRequest()
}

// EnableDumpResponse enables response dumping for all requests
func EnableDumpResponse() {
	defaultRequest.EnableDumpResponse()
}

// EnableOtelTrace enables OpenTelemetry tracing globally for all requests
func EnableOtelTrace() {
	enableTraceGlobal = true
}

// DisableOtelTrace disables OpenTelemetry tracing globally for all requests
func DisableOtelTrace() {
	enableTraceGlobal = false
}

// Get makes a GET request using the default client
func Get(url string) *Response {
	return defaultRequest.Get(url)
}

// Post makes a POST request using the default client
func Post(url string) *Response {
	return defaultRequest.Post(url)
}

// Put makes a PUT request using the default client
func Put(url string) *Response {
	return defaultRequest.Put(url)
}

// Delete makes a DELETE request using the default client
func Delete(url string) *Response {
	return defaultRequest.Delete(url)
}

// Patch makes a PATCH request using the default client
func Patch(url string) *Response {
	return defaultRequest.Patch(url)
}

// Head makes a HEAD request using the default client
func Head(url string) *Response {
	return defaultRequest.Head(url)
}

// Options makes an OPTIONS request using the default client
func Options(url string) *Response {
	return defaultRequest.Options(url)
}

// GetWithContext makes a GET request with custom context using the default client
func GetWithContext(ctx context.Context, url string) *Response {
	return defaultRequest.GetWithContext(ctx, url)
}

// PostWithContext makes a POST request with custom context using the default client
func PostWithContext(ctx context.Context, url string) *Response {
	return defaultRequest.PostWithContext(ctx, url)
}

// PutWithContext makes a PUT request with custom context using the default client
func PutWithContext(ctx context.Context, url string) *Response {
	return defaultRequest.PutWithContext(ctx, url)
}

// DeleteWithContext makes a DELETE request with custom context using the default client
func DeleteWithContext(ctx context.Context, url string) *Response {
	return defaultRequest.DeleteWithContext(ctx, url)
}

// PatchWithContext makes a PATCH request with custom context using the default client
func PatchWithContext(ctx context.Context, url string) *Response {
	return defaultRequest.PatchWithContext(ctx, url)
}

// HeadWithContext makes a HEAD request with custom context using the default client
func HeadWithContext(ctx context.Context, url string) *Response {
	return defaultRequest.HeadWithContext(ctx, url)
}

// OptionsWithContext makes an OPTIONS request with custom context using the default client
func OptionsWithContext(ctx context.Context, url string) *Response {
	return defaultRequest.OptionsWithContext(ctx, url)
}

// DoWithContext executes a custom request with context
func DoWithContext(ctx context.Context, method, urlStr string, body interface{}, headers map[string]string) *Response {
	req := NewRequest().SetMethod(method).SetURL(urlStr).SetHeaders(headers)

	if body != nil {
		switch v := body.(type) {
		case io.Reader:
			req.SetRawBody(v, "application/octet-stream")
		case url.Values:
			req.SetFormValuesBody(v)
		default:
			req.SetJSONBody(body)
		}
	}

	return req.Send(ctx)
}

// Do executes a custom request
func Do(method, urlStr string, body interface{}, headers map[string]string) *Response {
	return DoWithContext(context.Background(), method, urlStr, body, headers)
}
