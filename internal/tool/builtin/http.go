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

package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"rag-platform/internal/tool"
)

// DefaultTimeout 是默认 HTTP 请求超时
const DefaultTimeout = 30 * time.Second

// DefaultMaxBodySize 是默认最大响应体大小 (10MB)
const DefaultMaxBodySize = 10 * 1024 * 1024

// HTTPTool 实现 http.request（占位/简单实现）
type HTTPTool struct {
	client         *http.Client
	maxBodySize    int64
	allowedSchemes []string
}

// HTTPToolOption 配置选项
type HTTPToolOption func(*HTTPTool)

// WithTimeout 设置请求超时
func WithTimeout(timeout time.Duration) HTTPToolOption {
	return func(t *HTTPTool) {
		t.client.Timeout = timeout
	}
}

// WithMaxBodySize 设置最大响应体大小
func WithMaxBodySize(size int64) HTTPToolOption {
	return func(t *HTTPTool) {
		t.maxBodySize = size
	}
}

// WithAllowedSchemes 设置允许的 URL scheme
func WithAllowedSchemes(schemes []string) HTTPToolOption {
	return func(t *HTTPTool) {
		t.allowedSchemes = schemes
	}
}

// NewHTTPTool 创建 http.request 工具
func NewHTTPTool(opts ...HTTPToolOption) *HTTPTool {
	t := &HTTPTool{
		client: &http.Client{
			Timeout: DefaultTimeout,
		},
		maxBodySize:    DefaultMaxBodySize,
		allowedSchemes: []string{"http", "https"},
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// validateURL 验证 URL 是否安全
func (t *HTTPTool) validateURL(urlStr string) error {
	u, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme == "" {
		return errors.New("URL scheme is required")
	}
	for _, s := range t.allowedSchemes {
		if u.Scheme == s {
			return nil
		}
	}
	return fmt.Errorf("unsupported URL scheme: %s (allowed: %v)", u.Scheme, t.allowedSchemes)
}

// validateMethod 验证 HTTP 方法是否合法
func validateMethod(method string) error {
	switch strings.ToUpper(method) {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete,
		http.MethodPatch, http.MethodHead, http.MethodOptions:
		return nil
	default:
		return fmt.Errorf("unsupported HTTP method: %s", method)
	}
}

// Name 实现 tool.Tool
func (t *HTTPTool) Name() string { return "http.request" }

// Description 实现 tool.Tool
func (t *HTTPTool) Description() string {
	return "发送 HTTP 请求。传入 method、url，可选 body、headers。"
}

// Schema 实现 tool.Tool
func (t *HTTPTool) Schema() tool.Schema {
	return tool.Schema{
		Type:        "object",
		Description: "HTTP 请求参数",
		Properties: map[string]tool.SchemaProperty{
			"method":  {Type: "string", Description: "GET, POST, PUT, DELETE 等"},
			"url":     {Type: "string", Description: "请求 URL"},
			"body":    {Type: "string", Description: "请求体（可选）"},
			"headers": {Type: "object", Description: "请求头（可选）"},
		},
		Required: []string{"method", "url"},
	}
}

// Execute 实现 tool.Tool
func (t *HTTPTool) Execute(ctx context.Context, input map[string]any) (tool.ToolResult, error) {
	method, _ := input["method"].(string)
	urlStr, _ := input["url"].(string)

	// Validate required fields
	if method == "" || urlStr == "" {
		return tool.ToolResult{Err: "method and url are required"}, nil
	}

	// Validate HTTP method
	if err := validateMethod(method); err != nil {
		return tool.ToolResult{Err: err.Error()}, nil
	}

	// Validate URL scheme (prevent SSRF)
	if err := t.validateURL(urlStr); err != nil {
		return tool.ToolResult{Err: err.Error()}, nil
	}

	var body io.Reader
	if b, ok := input["body"].(string); ok && b != "" {
		body = strings.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, urlStr, body)
	if err != nil {
		return tool.ToolResult{Err: fmt.Sprintf("failed to create request: %s", err.Error())}, nil
	}

	if h, ok := input["headers"].(map[string]interface{}); ok {
		for k, v := range h {
			if s, ok := v.(string); ok {
				req.Header.Set(k, s)
			}
		}
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return tool.ToolResult{Err: fmt.Sprintf("request failed: %s", err.Error())}, nil
	}
	defer resp.Body.Close()

	// Limit body read to prevent memory issues
	data, err := io.ReadAll(io.LimitReader(resp.Body, t.maxBodySize+1))
	if err != nil {
		return tool.ToolResult{Err: fmt.Sprintf("failed to read response: %s", err.Error())}, nil
	}
	if int64(len(data)) > t.maxBodySize {
		return tool.ToolResult{Err: fmt.Sprintf("response body exceeds max size of %d bytes", t.maxBodySize)}, nil
	}

	out := map[string]interface{}{
		"status_code": resp.StatusCode,
		"body":        string(data),
	}
	raw, _ := json.Marshal(out)
	return tool.ToolResult{Content: string(raw)}, nil
}
