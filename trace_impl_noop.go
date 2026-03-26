//go:build !otel
// +build !otel

package lian

import (
	"context"
	"net/http"
	"net/http/httptrace"
)

type noopSpan struct{}

func startSpan(ctx context.Context, method, host string, isAsync bool) (context.Context, TraceContext) {
	return ctx, noopSpan{}
}

func endSpan(span TraceContext) {}

func injectTraceHeaders(ctx context.Context, header http.Header) {}

func setSpanRequestAttributes(span TraceContext, method, url, host, path string) {}

func setSpanResponseAttributes(span TraceContext, statusCode int) {}

func recordSpanError(span TraceContext, err error) {}

func addSpanEvent(span TraceContext, name string, attributes ...interface{}) {}

func createClientTrace(span TraceContext) *httptrace.ClientTrace {
	return nil
}
