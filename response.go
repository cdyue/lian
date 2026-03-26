package lian

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
)

// Response wraps the standard http.Response with additional functionality
type Response struct {
	*http.Response
	body []byte
	err  error
}

// NewResponse creates a new Response wrapper
func NewResponse(resp *http.Response, err error) *Response {
	return &Response{
		Response: resp,
		err:      err,
	}
}

// Error returns any error that occurred during the request
func (r *Response) Error() error {
	return r.err
}

// StatusCode returns the HTTP status code
func (r *Response) StatusCode() int {
	if r.Response == nil {
		return 0
	}
	return r.Response.StatusCode
}

// IsSuccess returns true if status code is 2xx
func (r *Response) IsSuccess() bool {
	code := r.StatusCode()
	return code >= 200 && code < 300
}

// IsError returns true if status code is 4xx or 5xx
func (r *Response) IsError() bool {
	code := r.StatusCode()
	return code >= 400 && code < 600
}

// Bytes returns the response body as bytes, automatically decompressing if needed
func (r *Response) Bytes() ([]byte, error) {
	if r.err != nil {
		return nil, r.err
	}
	if r.body != nil {
		return r.body, nil
	}
	if r.Response == nil || r.Response.Body == nil {
		return nil, nil
	}
	defer r.Response.Body.Close()

	var reader io.Reader = r.Response.Body

	// Handle compression
	switch r.Header.Get("Content-Encoding") {
	case "br":
		reader = brotli.NewReader(reader)
	case "gzip":
		gr, err := gzip.NewReader(reader)
		if err != nil {
			return nil, err
		}
		defer gr.Close()
		reader = gr
	case "zstd":
		zr, err := zstd.NewReader(reader)
		if err != nil {
			return nil, err
		}
		defer zr.Close()
		reader = zr
	case "deflate":
		reader = flate.NewReader(reader)
		defer reader.(io.Closer).Close()
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	r.body = body
	return body, nil
}

// String returns the response body as string
func (r *Response) String() (string, error) {
	bytes, err := r.Bytes()
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// JSON unmarshals the response body into the provided value
func (r *Response) JSON(v interface{}) error {
	bytes, err := r.Bytes()
	if err != nil {
		return err
	}
	return json.Unmarshal(bytes, v)
}

// Save writes the response body to a writer
func (r *Response) Save(w io.Writer) (int64, error) {
	bodyBytes, err := r.Bytes()
	if err != nil {
		return 0, err
	}
	return io.Copy(w, bytes.NewReader(bodyBytes))
}
