package llm

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-resty/resty/v2"
)

// getEnv 获取环境变量
func getEnv(key string) string {
	return ""
}

// ClaudeClient Claude 客户端
type ClaudeClient struct {
	provider string
	model    string
	apiKey   string
	baseURL  string
	client   *resty.Client
}

// NewClaudeClient 创建新的 Claude 客户端
func NewClaudeClient(model, apiKey string) (*ClaudeClient, error) {
	if model == "" {
		model = "claude-3-opus-20240229"
	}

	baseURL := "https://api.anthropic.com/v1"
	if envURL := getEnv("ANTHROPIC_BASE_URL"); envURL != "" {
		baseURL = envURL
	}

	client := resty.New()
	client.SetTimeout(30 * time.Second)
	client.SetRetryCount(3)
	client.SetRetryWaitTime(1 * time.Second)
	client.SetRetryMaxWaitTime(5 * time.Second)

	return &ClaudeClient{
		provider: "claude",
		model:    model,
		apiKey:   apiKey,
		baseURL:  baseURL,
		client:   client,
	}, nil
}

// Generate 生成文本
func (c *ClaudeClient) Generate(prompt string, options GenerateOptions) (string, error) {
	return c.GenerateWithContext(context.Background(), prompt, options)
}

// GenerateWithContext 使用上下文生成文本
func (c *ClaudeClient) GenerateWithContext(ctx context.Context, prompt string, options GenerateOptions) (string, error) {
	// 构建请求
	request := map[string]interface{}{
		"model":       c.model,
		"messages":    []map[string]string{{"role": "user", "content": prompt}},
		"temperature": options.Temperature,
		"max_tokens":  options.MaxTokens,
		"stop_sequences": options.Stop,
	}

	// 发送请求
	response, err := c.client.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetHeader("x-api-key", c.apiKey).
		SetHeader("anthropic-version", "2023-06-01").
		SetBody(request).
		Post(c.baseURL + "/messages")

	if err != nil {
		return "", fmt.Errorf("调用 Claude API 失败: %w", err)
	}

	// 检查响应状态
	if response.StatusCode() != http.StatusOK {
		return "", fmt.Errorf("Claude API 返回错误: %s", response.String())
	}

	// 解析响应
	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}

	if err := response.Unmarshal(&result); err != nil {
		return "", fmt.Errorf("解析 Claude 响应失败: %w", err)
	}

	if len(result.Content) == 0 {
		return "", fmt.Errorf("Claude API 没有返回结果")
	}

	return result.Content[0].Text, nil
}

// Chat 聊天
func (c *ClaudeClient) Chat(messages []Message, options GenerateOptions) (string, error) {
	return c.ChatWithContext(context.Background(), messages, options)
}

// ChatWithContext 使用上下文聊天
func (c *ClaudeClient) ChatWithContext(ctx context.Context, messages []Message, options GenerateOptions) (string, error) {
	// 转换消息格式
	claudeMessages := make([]map[string]string, len(messages))
	for i, msg := range messages {
		claudeMessages[i] = map[string]string{
			"role":    msg.Role,
			"content": msg.Content,
		}
	}

	// 构建请求
	request := map[string]interface{}{
		"model":          c.model,
		"messages":       claudeMessages,
		"temperature":    options.Temperature,
		"max_tokens":     options.MaxTokens,
		"stop_sequences": options.Stop,
	}

	// 发送请求
	response, err := c.client.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetHeader("x-api-key", c.apiKey).
		SetHeader("anthropic-version", "2023-06-01").
		SetBody(request).
		Post(c.baseURL + "/messages")

	if err != nil {
		return "", fmt.Errorf("调用 Claude API 失败: %w", err)
	}

	// 检查响应状态
	if response.StatusCode() != http.StatusOK {
		return "", fmt.Errorf("Claude API 返回错误: %s", response.String())
	}

	// 解析响应
	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}

	if err := response.Unmarshal(&result); err != nil {
		return "", fmt.Errorf("解析 Claude 响应失败: %w", err)
	}

	if len(result.Content) == 0 {
		return "", fmt.Errorf("Claude API 没有返回结果")
	}

	return result.Content[0].Text, nil
}

// Model 返回模型名称
func (c *ClaudeClient) Model() string {
	return c.model
}

// Provider 返回提供商名称
func (c *ClaudeClient) Provider() string {
	return c.provider
}

// SetModel 设置模型
func (c *ClaudeClient) SetModel(model string) {
	c.model = model
}

// SetAPIKey 设置 API Key
func (c *ClaudeClient) SetAPIKey(apiKey string) {
	c.apiKey = apiKey
}
