package llm

import (
	"context"
)

// Client LLM 客户端接口
type Client interface {
	// Generate 生成文本
	Generate(prompt string, options GenerateOptions) (string, error)
	// GenerateWithContext 使用上下文生成文本
	GenerateWithContext(ctx context.Context, prompt string, options GenerateOptions) (string, error)
	// Chat 聊天
	Chat(messages []Message, options GenerateOptions) (string, error)
	// ChatWithContext 使用上下文聊天
	ChatWithContext(ctx context.Context, messages []Message, options GenerateOptions) (string, error)
	// Model 返回模型名称
	Model() string
	// Provider 返回提供商名称
	Provider() string
	// SetModel 设置模型
	SetModel(model string)
	// SetAPIKey 设置 API Key
	SetAPIKey(apiKey string)
}

// GenerateOptions 生成选项
type GenerateOptions struct {
	Temperature      float64 `json:"temperature"`
	MaxTokens        int     `json:"max_tokens"`
	TopP             float64 `json:"top_p"`
	FrequencyPenalty float64 `json:"frequency_penalty"`
	PresencePenalty  float64 `json:"presence_penalty"`
	Stop             []string `json:"stop"`
}

// Message 聊天消息
type Message struct {
	Role    string `json:"role"`    // system, user, assistant
	Content string `json:"content"`
}

// NewClient 创建新的 LLM 客户端；baseURL 用于 OpenAI 兼容端点（如 Qwen/DashScope），空则用默认或环境变量
func NewClient(provider, model, apiKey string, baseURL string) (Client, error) {
	switch provider {
	case "openai":
		return NewOpenAIClientWithBaseURL(model, apiKey, baseURL)
	case "qwen":
		return NewOpenAIClientWithBaseURL(model, apiKey, baseURL)
	case "claude":
		return NewClaudeClient(model, apiKey)
	case "gemini":
		return NewGeminiClient(model, apiKey)
	default:
		return NewOpenAIClientWithBaseURL(model, apiKey, baseURL)
	}
}
