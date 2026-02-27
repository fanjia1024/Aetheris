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

package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/go-resty/resty/v2"
)

// OpenAIClient OpenAI 客户端
type OpenAIClient struct {
	provider string
	model    string
	apiKey   string
	baseURL  string
	client   *resty.Client
}

// NewOpenAIClient 创建新的 OpenAI 客户端（base 优先用 OPENAI_BASE_URL 环境变量）
func NewOpenAIClient(model, apiKey string) (*OpenAIClient, error) {
	return NewOpenAIClientWithBaseURL(model, apiKey, "")
}

// NewOpenAIClientWithBaseURL 创建 OpenAI 兼容客户端；baseURL 为空时用默认或 OPENAI_BASE_URL
func NewOpenAIClientWithBaseURL(model, apiKey, baseURL string) (*OpenAIClient, error) {
	if model == "" {
		model = "gpt-3.5-turbo"
	}
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
		if envURL := os.Getenv("OPENAI_BASE_URL"); envURL != "" {
			baseURL = envURL
		}
	}

	client := resty.New()
	client.SetTimeout(30 * time.Second)
	client.SetRetryCount(3)
	client.SetRetryWaitTime(1 * time.Second)
	client.SetRetryMaxWaitTime(5 * time.Second)

	return &OpenAIClient{
		provider: "openai",
		model:    model,
		apiKey:   apiKey,
		baseURL:  baseURL,
		client:   client,
	}, nil
}

// Generate 生成文本
func (c *OpenAIClient) Generate(prompt string, options GenerateOptions) (string, error) {
	return c.GenerateWithContext(context.Background(), prompt, options)
}

// GenerateWithContext 使用上下文生成文本
func (c *OpenAIClient) GenerateWithContext(ctx context.Context, prompt string, options GenerateOptions) (string, error) {
	// 构建请求
	request := map[string]interface{}{
		"model":       c.model,
		"messages":    []map[string]string{{"role": "user", "content": prompt}},
		"temperature": options.Temperature,
		"max_tokens":  options.MaxTokens,
		"top_p":       options.TopP,
		"stop":        options.Stop,
	}

	// 发送请求
	response, err := c.client.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", "Bearer "+c.apiKey).
		SetBody(request).
		Post(c.baseURL + "/chat/completions")

	if err != nil {
		return "", fmt.Errorf("调用 OpenAI API failed: %w", err)
	}

	// 检查响应状态
	if response.StatusCode() != http.StatusOK {
		return "", fmt.Errorf("OpenAI API 返回错误: %s", response.String())
	}

	// 解析响应
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(response.Body(), &result); err != nil {
		return "", fmt.Errorf("解析 OpenAI 响应failed: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("OpenAI API 没有返回结果")
	}

	return result.Choices[0].Message.Content, nil
}

// Chat 聊天
func (c *OpenAIClient) Chat(messages []Message, options GenerateOptions) (string, error) {
	return c.ChatWithContext(context.Background(), messages, options)
}

// ChatWithContext 使用上下文聊天
func (c *OpenAIClient) ChatWithContext(ctx context.Context, messages []Message, options GenerateOptions) (string, error) {
	// 转换消息格式
	openAIMessages := make([]map[string]string, len(messages))
	for i, msg := range messages {
		openAIMessages[i] = map[string]string{
			"role":    msg.Role,
			"content": msg.Content,
		}
	}

	// 构建请求
	request := map[string]interface{}{
		"model":       c.model,
		"messages":    openAIMessages,
		"temperature": options.Temperature,
		"max_tokens":  options.MaxTokens,
		"top_p":       options.TopP,
		"stop":        options.Stop,
	}

	// 发送请求
	response, err := c.client.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", "Bearer "+c.apiKey).
		SetBody(request).
		Post(c.baseURL + "/chat/completions")

	if err != nil {
		return "", fmt.Errorf("调用 OpenAI API failed: %w", err)
	}

	// 检查响应状态
	if response.StatusCode() != http.StatusOK {
		return "", fmt.Errorf("OpenAI API 返回错误: %s", response.String())
	}

	// 解析响应
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(response.Body(), &result); err != nil {
		return "", fmt.Errorf("解析 OpenAI 响应failed: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("OpenAI API 没有返回结果")
	}

	return result.Choices[0].Message.Content, nil
}

// Model 返回模型名称
func (c *OpenAIClient) Model() string {
	return c.model
}

// Provider 返回提供商名称
func (c *OpenAIClient) Provider() string {
	return c.provider
}

// SetModel 设置模型
func (c *OpenAIClient) SetModel(model string) {
	c.model = model
}

// SetAPIKey 设置 API Key
func (c *OpenAIClient) SetAPIKey(apiKey string) {
	c.apiKey = apiKey
}
