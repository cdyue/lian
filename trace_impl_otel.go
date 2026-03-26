//go:build otel
// +build otel

package lian

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httptrace"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type otelSpan struct {
	span trace.Span
}

func startSpan(ctx context.Context, method, host string, isAsync bool) (context.Context, TraceContext) {
	spanKind := trace.SpanKindClient
	if isAsync {
		spanKind = trace.SpanKindProducer
	}
	tracer := otel.Tracer("lian-http-client")
	spanName := fmt.Sprintf("HTTP %s %s", method, host)
	ctx, span := tracer.Start(ctx, spanName, trace.WithSpanKind(spanKind))
	return ctx, &otelSpan{span: span}
}

func endSpan(spanCtx TraceContext) {
	if s, ok := spanCtx.(*otelSpan); ok {
		s.span.End()
	}
}

func injectTraceHeaders(ctx context.Context, header http.Header) {
	propagator := otel.GetTextMapPropagator()
	propagator.Inject(ctx, propagation.HeaderCarrier(header))
}

func setSpanRequestAttributes(spanCtx TraceContext, method, url, host, path string) {
	if s, ok := spanCtx.(*otelSpan); ok {
		s.span.SetAttributes(
			attribute.String("http.method", method),
			attribute.String("http.url", url),
			attribute.String("http.host", host),
			attribute.String("http.path", path),
		)
	}
}

func setSpanResponseAttributes(spanCtx TraceContext, statusCode int) {
	if s, ok := spanCtx.(*otelSpan); ok {
		s.span.SetAttributes(attribute.Int("http.status_code", statusCode))
		if statusCode >= 400 {
			s.span.SetStatus(codes.Error, http.StatusText(statusCode))
		}
	}
}

func recordSpanError(spanCtx TraceContext, err error) {
	if s, ok := spanCtx.(*otelSpan); ok && err != nil {
		s.span.SetStatus(codes.Error, err.Error())
		s.span.RecordError(err)
	}
}

func addSpanEvent(spanCtx TraceContext, name string, attributes ...interface{}) {}

func createClientTrace(spanCtx TraceContext) *httptrace.ClientTrace {
	if s, ok := spanCtx.(*otelSpan); ok {
		span := s.span
		return &httptrace.ClientTrace{
			GetConn: func(hostPort string) {
				span.AddEvent("GetConn", trace.WithAttributes(attribute.String("host_port", hostPort)))
			},
			GotConn: func(info httptrace.GotConnInfo) {
				span.AddEvent("GotConn", trace.WithAttributes(
					attribute.String("remote_addr", info.Conn.RemoteAddr().String()),
					attribute.Bool("reused", info.Reused),
				))
			},
			DNSStart: func(info httptrace.DNSStartInfo) {
				span.AddEvent("DNSStart", trace.WithAttributes(attribute.String("host", info.Host)))
			},
			DNSDone: func(info httptrace.DNSDoneInfo) {
				if info.Err != nil {
					span.RecordError(info.Err)
				} else {
					addrs := make([]string, len(info.Addrs))
					for i, addr := range info.Addrs {
						addrs[i] = addr.String()
					}
					span.AddEvent("DNSDone", trace.WithAttributes(attribute.StringSlice("addrs", addrs)))
				}
			},
			ConnectStart: func(network, addr string) {
				span.AddEvent("ConnectStart", trace.WithAttributes(
					attribute.String("network", network),
					attribute.String("addr", addr),
				))
			},
			ConnectDone: func(network, addr string, err error) {
				if err != nil {
					span.RecordError(err)
				} else {
					span.AddEvent("ConnectDone", trace.WithAttributes(
						attribute.String("network", network),
						attribute.String("addr", addr),
					))
				}
			},
			TLSHandshakeStart: func() {
				span.AddEvent("TLSHandshakeStart")
			},
			TLSHandshakeDone: func(state tls.ConnectionState, err error) {
				if err != nil {
					span.RecordError(err)
				} else {
					span.AddEvent("TLSHandshakeDone", trace.WithAttributes(
						attribute.Int("tls_version", int(state.Version)),
						attribute.String("cipher_suite", tls.CipherSuiteName(state.CipherSuite)),
					))
				}
			},
			WroteHeaders: func() {
				span.AddEvent("WroteHeaders")
			},
			WroteRequest: func(info httptrace.WroteRequestInfo) {
				if info.Err != nil {
					span.RecordError(info.Err)
				} else {
					span.AddEvent("WroteRequest")
				}
			},
			GotFirstResponseByte: func() {
				span.AddEvent("GotFirstResponseByte")
			},
		}
	}
	return nil
}
