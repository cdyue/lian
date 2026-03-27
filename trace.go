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
	if !enableTraceGlobal {
		return ctx, nil
	}
	return startSpan(ctx, method, host, isAsync)
}

// EndSpan ends the span
func EndSpan(span TraceContext) {
	if span == nil {
		return
	}
	endSpan(span)
}

// InjectTraceHeaders injects trace headers into request
func InjectTraceHeaders(ctx context.Context, header http.Header) {
	if !enableTraceGlobal {
		return
	}
	injectTraceHeaders(ctx, header)
}

// SetSpanRequestAttributes sets request attributes to span
func SetSpanRequestAttributes(span TraceContext, method, url, host, path string) {
	if span == nil {
		return
	}
	setSpanRequestAttributes(span, method, url, host, path)
}

// SetSpanResponseAttributes sets response attributes to span
func SetSpanResponseAttributes(span TraceContext, statusCode int) {
	if span == nil {
		return
	}
	setSpanResponseAttributes(span, statusCode)
}

// RecordSpanError records error to span
func RecordSpanError(span TraceContext, err error) {
	if span == nil {
		return
	}
	recordSpanError(span, err)
}

// AddSpanEvent adds event to span
func AddSpanEvent(span TraceContext, name string, attributes ...interface{}) {
	if span == nil {
		return
	}
	addSpanEvent(span, name, attributes...)
}

// CreateClientTrace creates an httptrace.ClientTrace that reports to trace system
func CreateClientTrace(span TraceContext) *httptrace.ClientTrace {
	if span == nil {
		return nil
	}
	return createClientTrace(span)
}
