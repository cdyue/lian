package lian

import (
	"bytes"
	"encoding/json"
	"io"
	"net/url"
	"strings"

	goquery "github.com/google/go-querystring/query"
)

// BodyProvider provides Body content for http.Request attachment.
type BodyProvider interface {
	// ContentType returns the Content-Type of the body.
	ContentType() string
	// Body returns the io.Reader body.
	Body() (io.Reader, error)
	// Bytes returns the body as bytes
	Bytes() ([]byte, error)
}

// jsonBodyProvider encodes a JSON tagged struct value as a Body for requests.
type jsonBodyProvider struct {
	payload interface{}
}

func (p jsonBodyProvider) ContentType() string {
	return "application/json; charset=utf-8"
}

func (p jsonBodyProvider) Body() (io.Reader, error) {
	buf := &bytes.Buffer{}
	encoder := json.NewEncoder(buf)
	err := encoder.Encode(p.payload)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

func (p jsonBodyProvider) Bytes() ([]byte, error) {
	return json.Marshal(p.payload)
}

// formBodyProvider encodes a url tagged struct value as Body for requests.
type formBodyProvider struct {
	payload interface{}
}

func (p formBodyProvider) ContentType() string {
	return "application/x-www-form-urlencoded; charset=utf-8"
}

func (p formBodyProvider) Body() (io.Reader, error) {
	values, err := goquery.Values(p.payload)
	if err != nil {
		return nil, err
	}
	return strings.NewReader(values.Encode()), nil
}

func (p formBodyProvider) Bytes() ([]byte, error) {
	values, err := goquery.Values(p.payload)
	if err != nil {
		return nil, err
	}
	return []byte(values.Encode()), nil
}

// rawBodyProvider wraps an existing io.Reader as body
type rawBodyProvider struct {
	body        io.Reader
	contentType string
}

func (p rawBodyProvider) ContentType() string {
	return p.contentType
}

func (p rawBodyProvider) Body() (io.Reader, error) {
	return p.body, nil
}

func (p rawBodyProvider) Bytes() ([]byte, error) {
	var buffer bytes.Buffer
	_, err := io.Copy(&buffer, p.body)
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// urlValuesBodyProvider wraps url.Values as form body
type urlValuesBodyProvider struct {
	values url.Values
}

func (p urlValuesBodyProvider) ContentType() string {
	return "application/x-www-form-urlencoded; charset=utf-8"
}

func (p urlValuesBodyProvider) Body() (io.Reader, error) {
	return strings.NewReader(p.values.Encode()), nil
}

func (p urlValuesBodyProvider) Bytes() ([]byte, error) {
	return []byte(p.values.Encode()), nil
}
