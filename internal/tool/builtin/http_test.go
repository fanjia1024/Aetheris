// Copyright 2026 fanjia1024
// Tests for HTTP tool

package builtin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPTool_Name(t *testing.T) {
	tool := NewHTTPTool()
	assert.Equal(t, "http.request", tool.Name())
}

func TestHTTPTool_Description(t *testing.T) {
	tool := NewHTTPTool()
	assert.NotEmpty(t, tool.Description())
}

func TestHTTPTool_Schema(t *testing.T) {
	tool := NewHTTPTool()
	schema := tool.Schema()
	assert.Equal(t, "object", schema.Type)
	assert.Contains(t, schema.Properties, "method")
	assert.Contains(t, schema.Properties, "url")
}

func TestHTTPTool_MissingRequiredFields(t *testing.T) {
	tool := NewHTTPTool()

	tests := []struct {
		name   string
		input  map[string]any
		errMsg string
	}{
		{
			name:   "missing method and url",
			input:  map[string]any{},
			errMsg: "method and url are required",
		},
		{
			name:   "missing url",
			input:  map[string]any{"method": "GET"},
			errMsg: "method and url are required",
		},
		{
			name:   "missing method",
			input:  map[string]any{"url": "http://example.com"},
			errMsg: "method and url are required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tool.Execute(context.Background(), tt.input)
			require.NoError(t, err)
			assert.NotEmpty(t, result.Err)
			assert.Contains(t, result.Err, tt.errMsg)
		})
	}
}

func TestHTTPTool_InvalidMethod(t *testing.T) {
	tool := NewHTTPTool()
	result, err := tool.Execute(context.Background(), map[string]any{
		"method": "INVALID_METHOD",
		"url":    "http://example.com",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, result.Err)
	assert.Contains(t, result.Err, "unsupported HTTP method")
}

func TestHTTPTool_InvalidURLScheme(t *testing.T) {
	tool := NewHTTPTool()

	tests := []struct {
		name string
		url  string
	}{
		{
			name: "file scheme",
			url:  "file:///etc/passwd",
		},
		{
			name: "javascript scheme",
			url:  "javascript:alert(1)",
		},
		{
			name: "data scheme",
			url:  "data:text/html,<script>alert(1)</script>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tool.Execute(context.Background(), map[string]any{
				"method": "GET",
				"url":    tt.url,
			})
			require.NoError(t, err)
			assert.NotEmpty(t, result.Err)
			assert.Contains(t, result.Err, "unsupported URL scheme")
		})
	}
}

func TestHTTPTool_SuccessfulRequest(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message":"hello"}`))
	}))
	defer server.Close()

	tool := NewHTTPTool()
	result, err := tool.Execute(context.Background(), map[string]any{
		"method":  "GET",
		"url":     server.URL,
		"headers": map[string]interface{}{"Content-Type": "application/json"},
	})
	require.NoError(t, err)
	assert.Empty(t, result.Err)
	assert.NotEmpty(t, result.Content)

	// Parse the response
	var resp map[string]any
	err = json.Unmarshal([]byte(result.Content), &resp)
	require.NoError(t, err)
	assert.Equal(t, float64(http.StatusOK), resp["status_code"])
	assert.Contains(t, resp["body"], "hello")
}

func TestHTTPTool_PostWithBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		body := make([]byte, 1024)
		n, _ := r.Body.Read(body)
		assert.Contains(t, string(body[:n]), "test data")

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"created"}`))
	}))
	defer server.Close()

	tool := NewHTTPTool()
	result, err := tool.Execute(context.Background(), map[string]any{
		"method":  "POST",
		"url":     server.URL,
		"body":    `{"data":"test data"}`,
		"headers": map[string]interface{}{"Content-Type": "application/json"},
	})
	require.NoError(t, err)
	assert.Empty(t, result.Err)
}

func TestHTTPTool_RequestTimeout(t *testing.T) {
	// Create a slow server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Very long delay to trigger timeout
		select {}
	}))
	defer server.Close()

	// Create tool with very short timeout
	tool := NewHTTPTool(WithTimeout(100))
	result, err := tool.Execute(context.Background(), map[string]any{
		"method": "GET",
		"url":    server.URL,
	})
	require.NoError(t, err)
	// Should either timeout or fail
	assert.NotEmpty(t, result.Err)
}

func TestHTTPTool_WithOptions(t *testing.T) {
	tool := NewHTTPTool(
		WithTimeout(30),
		WithMaxBodySize(1024),
		WithAllowedSchemes([]string{"https"}),
	)

	// https should work
	result, _ := tool.Execute(context.Background(), map[string]any{
		"method": "GET",
		"url":    "https://example.com",
	})
	// May fail due to actual network, but should pass validation
	assert.NotContains(t, result.Err, "unsupported URL scheme")
}

func TestValidateMethod(t *testing.T) {
	validMethods := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS", "get", "post"}
	for _, m := range validMethods {
		err := validateMethod(m)
		assert.NoError(t, err, "method %s should be valid", m)
	}

	invalidMethods := []string{"TRACE", "CONNECT", "INVALID", ""}
	for _, m := range invalidMethods {
		err := validateMethod(m)
		assert.Error(t, err, "method %s should be invalid", m)
	}
}
