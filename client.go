package lian

import (
	"crypto/tls"
	"net"
	"net/http"
	"runtime"
	"time"
)

var defaultClient = &http.Client{}

func init() {
	ts := createTransport()
	defaultClient.Transport = ts
}

// Option represents client configuration options
type Option struct {
	disableKeepAlive   bool
	insecureSkipVerify bool
	disableCompression bool
	maxIdleConns       int
	idleConnTimeout    time.Duration
	timeout            time.Duration
}

// OpFunc is a function type for setting options
type OpFunc func(*Option)

// WithDisableKeepAlive disables keep alive connections
func WithDisableKeepAlive() OpFunc {
	return func(op *Option) {
		op.disableKeepAlive = true
	}
}

// WithInsecure disables TLS certificate verification
func WithInsecure() OpFunc {
	return func(op *Option) {
		op.insecureSkipVerify = true
	}
}

// WithDisableCompression disables response compression
func WithDisableCompression() OpFunc {
	return func(op *Option) {
		op.disableCompression = true
	}
}

// WithMaxIdleConns sets maximum number of idle connections
func WithMaxIdleConns(n int) OpFunc {
	return func(op *Option) {
		op.maxIdleConns = n
	}
}

// WithIdleConnTimeout sets idle connection timeout
func WithIdleConnTimeout(d time.Duration) OpFunc {
	return func(op *Option) {
		op.idleConnTimeout = d
	}
}

// WithTimeout sets overall request timeout
func WithTimeout(d time.Duration) OpFunc {
	return func(op *Option) {
		op.timeout = d
	}
}

// NewClient creates a new http.Client with custom options
func NewClient(opts ...OpFunc) *http.Client {
	op := &Option{
		maxIdleConns:    100,
		idleConnTimeout: 90 * time.Second,
		timeout:         30 * time.Second,
	}
	for _, v := range opts {
		v(op)
	}

	ts := createTransport()
	if op.disableKeepAlive {
		ts.DisableKeepAlives = true
	}
	if op.insecureSkipVerify {
		ts.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	if op.disableCompression {
		ts.DisableCompression = true
	}
	if op.maxIdleConns > 0 {
		ts.MaxIdleConns = op.maxIdleConns
	}
	if op.idleConnTimeout > 0 {
		ts.IdleConnTimeout = op.idleConnTimeout
	}
	ts.MaxIdleConnsPerHost = runtime.GOMAXPROCS(0) + 1

	return &http.Client{
		Transport: ts,
		Timeout:   op.timeout,
	}
}

// createTransport creates a default http.Transport with optimized settings
func createTransport() *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableKeepAlives:     false,
	}
}
