package llm

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-resty/resty/v2"
)

// GeminiClient Gemini 客户端
type GeminiClient struct {
	provider string
	model    string
	apiKey   string
	baseURL  string
	client   *resty.Client
}

// NewGeminiClient 创建新的 Gemini 客户端
func NewGeminiClient(model, apiKey string) (*GeminiClient, error) {
	if model == "" {
		model = "gemini-1.5-flash"
	}

	baseURL := "https://generativelanguage.googleapis.com/v1"
	if envURL := getEnv("GEMINI_BASE_URL"); envURL != "" {
		baseURL = envURL
	}

	client := resty.New()
	client.SetTimeout(30 * time.Second)
	client.SetRetryCount(3)
	client.SetRetryWaitTime(1 * time.Second)
	client.SetRetryMaxWaitTime(5 * time.Second)

	return &GeminiClient{
		provider: "gemini",
		model:    model,
		apiKey:   apiKey,
		baseURL:  baseURL,
		client:   client,
	}, nil
}

// Generate 生成文本
func (c *GeminiClient) Generate(prompt string, options GenerateOptions) (string, error) {
	return c.GenerateWithContext(context.Background(), prompt, options)
}

// GenerateWithContext 使用上下文生成文本
func (c *GeminiClient) GenerateWithContext(ctx context.Context, prompt string, options GenerateOptions) (string, error) {
	// 构建请求
	request := map[string]interface{}{
		"contents": []map[string]interface{}{{
			"parts": []map[string]interface{}{{
				"text": prompt,
			}},
		}},
		"temperature": options.Temperature,
		"max_output_tokens": options.MaxTokens,
		"top_p": options.TopP,
		"stop_sequences": options.Stop,
	}

	// 发送请求
	response, err := c.client.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(request).
		Post(c.baseURL + "/models/" + c.model + ":generateContent?key=" + c.apiKey)

	if err != nil {
		return "", fmt.Errorf("调用 Gemini API 失败: %w", err)
	}

	// 检查响应状态
	if response.StatusCode() != http.StatusOK {
		return "", fmt.Errorf("Gemini API 返回错误: %s", response.String())
	}

	// 解析响应
	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}

	if err := response.Unmarshal(&result); err != nil {
		return "", fmt.Errorf("解析 Gemini 响应失败: %w", err)
	}

	if len(result.Candidates) == 0 {
		return "", fmt.Errorf("Gemini API 没有返回结果")
	}

	if len(result.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("Gemini API 没有返回文本")
	}

	return result.Candidates[0].Content.Parts[0].Text, nil
}

// Chat 聊天
func (c *GeminiClient) Chat(messages []Message, options GenerateOptions) (string, error) {
	return c.ChatWithContext(context.Background(), messages, options)
}

// ChatWithContext 使用上下文聊天
func (c *GeminiClient) ChatWithContext(ctx context.Context, messages []Message, options GenerateOptions) (string, error) {
	// 转换消息格式
	contents := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		contents[i] = map[string]interface{}{
			"role": msg.Role,
			"parts": []map[string]interface{}{{
				"text": msg.Content,
			}},
		}
	}

	// 构建请求
	request := map[string]interface{}{
		"contents": contents,
		"temperature": options.Temperature,
		"max_output_tokens": options.MaxTokens,
		"top_p": options.TopP,
		"stop_sequences": options.Stop,
	}

	// 发送请求
	response, err := c.client.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(request).
		Post(c.baseURL + "/models/" + c.model + ":generateContent?key=" + c.apiKey)

	if err != nil {
		return "", fmt.Errorf("调用 Gemini API 失败: %w", err)
	}

	// 检查响应状态
	if response.StatusCode() != http.StatusOK {
		return "", fmt.Errorf("Gemini API 返回错误: %s", response.String())
	}

	// 解析响应
	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}

	if err := response.Unmarshal(&result); err != nil {
		return "", fmt.Errorf("解析 Gemini 响应失败: %w", err)
	}

	if len(result.Candidates) == 0 {
		return "", fmt.Errorf("Gemini API 没有返回结果")
	}

	if len(result.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("Gemini API 没有返回文本")
	}

	return result.Candidates[0].Content.Parts[0].Text, nil
}

// Model 返回模型名称
func (c *GeminiClient) Model() string {
	return c.model
}

// Provider 返回提供商名称
func (c *GeminiClient) Provider() string {
	return c.provider
}

// SetModel 设置模型
func (c *GeminiClient) SetModel(model string) {
	c.model = model
}

// SetAPIKey 设置 API Key
func (c *GeminiClient) SetAPIKey(apiKey string) {
	c.apiKey = apiKey
}
