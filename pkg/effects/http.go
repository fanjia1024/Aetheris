// Copyright 2026 fanjia1024
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package effects

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

// HTTPRequest represents an HTTP effect request.
type HTTPRequest struct {
	Method   string            `json:"method"`
	URL      string            `json:"url"`
	Headers  map[string]string `json:"headers,omitempty"`
	Body     []byte            `json:"body,omitempty"`
	Timeout  *time.Duration    `json:"timeout,omitempty"`
	Metadata map[string]any    `json:"metadata,omitempty"`
}

// HTTPResponse represents an HTTP effect response.
type HTTPResponse struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
	Body       []byte            `json:"body"`
	Duration   time.Duration    `json:"duration"`
}

// HTTPCaller is the function type for actual HTTP calls.
type HTTPCaller func(ctx context.Context, req HTTPRequest) (HTTPResponse, error)

// ExecuteHTTP executes an HTTP effect.
// This is the primary entry point for all HTTP requests.
func ExecuteHTTP(ctx context.Context, sys System, req HTTPRequest, caller HTTPCaller) (HTTPResponse, error) {
	key := computeHTTPIdempotencyKey(req)

	effect := NewEffect(KindHTTP, req).
		WithIdempotencyKey(key).
		WithDescription("http." + req.Method + " " + req.URL)

	result, err := sys.Execute(ctx, effect)
	if err != nil {
		return HTTPResponse{}, err
	}

	if result.Cached {
		if result.Data != nil {
			return result.Data.(HTTPResponse), nil
		}
		return HTTPResponse{}, nil
	}

	// Real execution
	response, err := caller(ctx, req)
	if err != nil {
		return HTTPResponse{}, err
	}

	// Store the result data using Complete
	_ = sys.Complete(result.ID, response)

	return response, nil
}

// computeHTTPIdempotencyKey creates a deterministic key from HTTP request.
func computeHTTPIdempotencyKey(req HTTPRequest) string {
	keyData := struct {
		Method string
		URL    string
		Headers map[string]string
		Body   []byte
	}{
		Method:  req.Method,
		URL:     req.URL,
		Headers: req.Headers,
		Body:     req.Body,
	}

	data, _ := json.Marshal(keyData)
	hash := sha256.Sum256(data)
	return "http:" + hex.EncodeToString(hash[:8])
}

// NewHTTPRequest creates a new HTTP request.
func NewHTTPRequest(method, url string) HTTPRequest {
	return HTTPRequest{
		Method:  method,
		URL:     url,
		Headers: make(map[string]string),
		Body:    []byte{},
	}
}

// WithHeader adds a header to the request.
func (r HTTPRequest) WithHeader(key, value string) HTTPRequest {
	r.Headers[key] = value
	return r
}

// WithBody sets the request body.
func (r HTTPRequest) WithBody(body []byte) HTTPRequest {
	r.Body = body
	return r
}

// WithJSONBody sets the request body to JSON.
func (r HTTPRequest) WithJSONBody(v any) (HTTPRequest, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return r, err
	}
	r.Body = data
	if r.Headers == nil {
		r.Headers = make(map[string]string)
	}
	r.Headers["Content-Type"] = "application/json"
	return r, nil
}

// WithTimeout sets the timeout for the request.
func (r HTTPRequest) WithTimeout(timeout time.Duration) HTTPRequest {
	r.Timeout = &timeout
	return r
}

// HTTPEffect creates an effect for HTTP execution.
func HTTPEffect(req HTTPRequest) Effect {
	return NewEffect(KindHTTP, req).
		WithIdempotencyKey(computeHTTPIdempotencyKey(req)).
		WithDescription("http." + req.Method + " " + req.URL)
}

// RecordHTTPToRecorder records an HTTP effect using the EventRecorder.
func RecordHTTPToRecorder(ctx context.Context, recorder EventRecorder, effectID string, idempotencyKey string, req HTTPRequest, resp HTTPResponse, duration time.Duration) error {
	if recorder == nil {
		return ErrNoRecorder
	}
	result := SuccessResult(effectID, KindHTTP, resp, duration)
	return recorder.RecordHTTP(ctx, effectID, idempotencyKey, req, result)
}

// DefaultHTTPCaller provides a default HTTP implementation.
func DefaultHTTPCaller(ctx context.Context, req HTTPRequest) (HTTPResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, req.URL, bytes.NewReader(req.Body))
	if err != nil {
		return HTTPResponse{}, err
	}

	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	if req.Timeout != nil {
		client.Timeout = *req.Timeout
	}

	start := time.Now()
	resp, err := client.Do(httpReq)
	duration := time.Since(start)

	if err != nil {
		return HTTPResponse{}, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	response := HTTPResponse{
		StatusCode: resp.StatusCode,
		Headers:    make(map[string]string),
		Body:       body,
		Duration:   duration,
	}

	for k := range resp.Header {
		response.Headers[k] = resp.Header.Get(k)
	}

	return response, nil
}