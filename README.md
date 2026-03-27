# Lian - Modern Go HTTP Client Library

Lian is a high-performance, production-ready HTTP client library for Go, designed with best practices in mind, offering a clean and intuitive API while providing powerful features for modern microservices architectures.

## ✨ Key Features

- **Fluent Chainable API**: Clean and readable method chaining for request configuration
- **Automatic Compression**: Built-in support for gzip, brotli, zstd, and deflate decompression
- **Request Compression**: Optional zstd compression for request bodies
- **Structured Logging**: Uses standard library `slog` by default, supports custom logger implementations
- **OpenTelemetry Support**: Optional built-in distributed tracing integration
- **Context-Aware**: Full support for standard `context.Context` propagation
- **Customizable Header Mapping**: Flexible configuration for authentication and tenant headers
- **Automatic Response Unmarshaling**: Directly unmarshal JSON responses into structs
- **Debugging Tools**: Request/response dumping and HTTP trace capabilities
- **Connection Pooling**: Optimized HTTP client with configurable connection pooling
- **No Framework Lock-In**: Works with any Go web framework (Gin, Echo, Fiber, standard library, etc.)

## 📦 Installation

```bash
go get github.com/cdyue/lian
```

## 🚀 Quick Start

### Simple GET Request

```go
package main

import (
	"context"
	"fmt"
	"github.com/cdyue/lian"
)

func main() {
	// Simple GET request
	resp := lian.Get("https://jsonplaceholder.typicode.com/posts/1")
	if resp.IsError() {
		panic(resp.Error())
	}

	// Parse JSON response
	var post map[string]interface{}
	if err := resp.JSON(&post); err != nil {
		panic(err)
	}

	fmt.Printf("Post Title: %s\n", post["title"])
}
```

### POST Request with JSON Body

```go
user := map[string]interface{}{
	"name":  "John Doe",
	"email": "john@example.com",
}

resp := lian.NewRequest().
	SetJSONBody(user).
	SetBearerToken("your-auth-token").
	Post("https://api.example.com/users")

if resp.IsSuccess() {
	fmt.Println("User created successfully!")
}
```

## ⚙️ Advanced Configuration

### Custom HTTP Client

```go
import "time"

// Create a custom HTTP client with specific settings
client := lian.NewClient(
	lian.WithTimeout(10 * time.Second),
	lian.WithInsecure(), // Skip TLS certificate verification (use carefully!)
	lian.WithMaxIdleConns(200),
)

// Use the custom client for requests
req := lian.NewRequest().SetClient(client)
resp := req.Get("https://api.example.com/data")
```

### Custom Header Mapping

Lian provides flexible configuration for core headers like authentication, tenant ID, etc.

#### Global Configuration (applies to all requests)
```go
// Customize header names globally
lian.SetDefaultAuthHeaderName("X-Access-Token")
lian.SetDefaultTenantIDHeaderName("X-Organization-ID")
lian.SetDefaultUserIDHeaderName("X-Current-User-ID")

// Custom value extractor functions (for integrating with your existing context)
lian.SetDefaultTenantIDExtractor(func(ctx context.Context) string {
	// Example: Extract tenant ID from Gin context
	if ginCtx, ok := ctx.Value("gin.Context").(*gin.Context); ok {
		return ginCtx.GetString("tenant_id")
	}
	return ""
})
```

#### Per-Request Configuration
```go
// Customize headers for a specific request
customMapping := lian.HeaderMapping{
	AuthHeader:     "X-API-Key",
	TenantIDHeader: "X-Company-ID",
	// Leave other fields empty to disable extraction
}

req := lian.NewRequest().
	SetHeaderMapping(customMapping).
	FromContext(ctx).
	Get("https://api.example.com/data")
```

### Context Header Propagation

```go
// In your middleware, store headers in context
ctx := context.Background()
ctx = lian.SetHeaderToContext(ctx, "Authorization", "Bearer token123")
ctx = lian.SetHeaderToContext(ctx, "X-Tenant-Id", "acme-corp")

// Later when making requests, headers are automatically extracted
resp := lian.NewRequest().
	FromContext(ctx).
	Get("https://api.example.com/protected")
```

### Logging Configuration

```go
import (
	"log/slog"
	"os"
)

// Use a custom slog instance
logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
	Level: slog.LevelDebug,
}))
lian.SetLogger(logger)

// Or implement the Logger interface for full customization
type CustomLogger struct {
	// Your logger implementation
}

func (l *CustomLogger) Info(msg string, args ...interface{})  { /* ... */ }
func (l *CustomLogger) Debug(msg string, args ...interface{}) { /* ... */ }
func (l *CustomLogger) Warn(msg string, args ...interface{})  { /* ... */ }
func (l *CustomLogger) Error(msg string, args ...interface{}) { /* ... */ }

lian.SetLogger(&CustomLogger{})
```

### Debugging

```go
// Enable request/response dumping
req := lian.NewRequest().
	EnableDumpRequest().  // Log full request details
	EnableDumpResponse(). // Log full response details
	EnableHTTPTrace().    // Log HTTP connection details (DNS, TCP, TLS, etc.)
	Get("https://api.example.com/debug")
```

### OpenTelemetry Tracing

Lian has built-in OpenTelemetry distributed tracing support (enabled by default, no additional build tags required).

#### Global Control
```go
// Disable OpenTelemetry tracing globally for all requests
lian.DisableOtelTrace()

// Re-enable OpenTelemetry tracing globally
lian.EnableOtelTrace()
```

#### Per-Request Control
```go
// Disable tracing for a specific request
req := lian.NewRequest().
	DisableOtelTrace().
	Get("https://api.example.com/debug")

// Force enable tracing for a specific request even when globally disabled
req := lian.NewRequest().
	EnableOtelTraceForRequest().
	Get("https://api.example.com/debug")
```

Lian automatically creates client spans, propagates trace headers, and records all HTTP events to your OpenTelemetry collector when tracing is enabled.

## 🔧 Compilation Notes

### Using Standard JSON (stable)
To use the stable standard library `encoding/json`, modify the imports in:
- `response.go`: Change `encoding/json/v2` to `encoding/json`
- `body.go`: Change `encoding/json/v2` to `encoding/json`

Then build normally:
```bash
go build ./...
```

### Using JSON v2 (experimental, higher performance)
Build with the JSON v2 experiment enabled:
```bash
GOEXPERIMENT=jsonv2 go build ./...
```

## 📖 API Reference

### Core Methods
- `lian.Get(url string) *Response`
- `lian.Post(url string) *Response`
- `lian.Put(url string) *Response`
- `lian.Delete(url string) *Response`
- `lian.Patch(url string) *Response`
- `lian.Do(method, url string, body interface{}, headers map[string]string) *Response`

### Request Configuration
- `SetHeader(key, value string) *Request`
- `SetQueryParam(key, value string) *Request`
- `SetJSONBody(v interface{}) *Request`
- `SetFormBody(v interface{}) *Request`
- `SetRawBody(body io.Reader, contentType string) *Request`
- `SetBearerToken(token string) *Request`
- `SetBasicAuth(username, password string) *Request`
- `FromContext(ctx context.Context) *Request`

### Response Methods
- `resp.StatusCode() int`
- `resp.IsSuccess() bool`
- `resp.IsError() bool`
- `resp.Bytes() ([]byte, error)`
- `resp.String() (string, error)`
- `resp.JSON(v interface{}) error`
- `resp.Save(w io.Writer) (int64, error)`

## 📄 License

MIT License
