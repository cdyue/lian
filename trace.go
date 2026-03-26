package lian

import (
	"context"
	"net/http"
	"net/http/httptrace"
)

// TraceContext is an abstraction for trace context
type TraceContext interface{}

// StartSpan starts a new span, returns context and span
func StartSpan(ctx context.Context, method, host string, isAsync bool) (context.Context, TraceContext) {
	return startSpan(ctx, method, host, isAsync)
}

// EndSpan ends the span
func EndSpan(span TraceContext) {
	endSpan(span)
}

// InjectTraceHeaders injects trace headers into request
func InjectTraceHeaders(ctx context.Context, header http.Header) {
	injectTraceHeaders(ctx, header)
}

// SetSpanRequestAttributes sets request attributes to span
func SetSpanRequestAttributes(span TraceContext, method, url, host, path string) {
	setSpanRequestAttributes(span, method, url, host, path)
}

// SetSpanResponseAttributes sets response attributes to span
func SetSpanResponseAttributes(span TraceContext, statusCode int) {
	setSpanResponseAttributes(span, statusCode)
}

// RecordSpanError records error to span
func RecordSpanError(span TraceContext, err error) {
	recordSpanError(span, err)
}

// AddSpanEvent adds event to span
func AddSpanEvent(span TraceContext, name string, attributes ...interface{}) {
	addSpanEvent(span, name, attributes...)
}

// CreateClientTrace creates an httptrace.ClientTrace that reports to trace system
func CreateClientTrace(span TraceContext) *httptrace.ClientTrace {
	return createClientTrace(span)
}
