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
	"io"
	"net/http"
	"strings"

	"rag-platform/internal/tool"
)

// HTTPTool 实现 http.request（占位/简单实现）
type HTTPTool struct {
	client *http.Client
}

// NewHTTPTool 创建 http.request 工具
func NewHTTPTool() *HTTPTool {
	return &HTTPTool{client: http.DefaultClient}
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
	if method == "" || urlStr == "" {
		return tool.ToolResult{Err: "method 和 url 不能为空"}, nil
	}
	if method == "" {
		method = http.MethodGet
	}
	var body io.Reader
	if b, ok := input["body"].(string); ok && b != "" {
		body = strings.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, urlStr, body)
	if err != nil {
		return tool.ToolResult{Err: err.Error()}, nil
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
		return tool.ToolResult{Err: err.Error()}, nil
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return tool.ToolResult{Err: err.Error()}, nil
	}
	out := map[string]interface{}{
		"status_code": resp.StatusCode,
		"body":        string(data),
	}
	raw, _ := json.Marshal(out)
	return tool.ToolResult{Content: string(raw)}, nil
}
