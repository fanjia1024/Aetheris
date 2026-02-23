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
	"time"

	"rag-platform/pkg/metrics"
)

// RateLimitedClient 包装任意 LLM Client，在真实调用前后执行限流控制。
// 用于 Planner、Generator 等直接持有 Client 的调用路径。
type RateLimitedClient struct {
	inner       Client
	rateLimiter *LLMRateLimiter
}

// NewRateLimitedClient 创建带限流的 LLM 客户端。rateLimiter 为 nil 时退化为直接调用。
func NewRateLimitedClient(inner Client, rateLimiter *LLMRateLimiter) *RateLimitedClient {
	return &RateLimitedClient{inner: inner, rateLimiter: rateLimiter}
}

// Generate 实现 Client.Generate。
func (c *RateLimitedClient) Generate(prompt string, options GenerateOptions) (string, error) {
	return c.GenerateWithContext(context.Background(), prompt, options)
}

// GenerateWithContext 实现 Client.GenerateWithContext，调用前后执行限流。
func (c *RateLimitedClient) GenerateWithContext(ctx context.Context, prompt string, options GenerateOptions) (string, error) {
	if c.rateLimiter != nil {
		provider := c.inner.Provider()
		estimatedTokens := estimateTokens(prompt, options.MaxTokens)
		start := time.Now()
		if err := c.rateLimiter.Wait(ctx, provider, estimatedTokens); err != nil {
			return "", err
		}
		waited := time.Since(start)
		if waited > 100*time.Millisecond {
			metrics.RateLimitWaitSeconds.WithLabelValues("llm", provider).Observe(waited.Seconds())
		}
		defer c.rateLimiter.Release(provider)
	}

	result, err := c.inner.GenerateWithContext(ctx, prompt, options)
	if err != nil {
		return "", err
	}
	if c.rateLimiter != nil {
		// 用 MaxTokens 近似记录实际用量（未来可从 response 中取 usage）
		c.rateLimiter.RecordTokenUsage(c.inner.Provider(), options.MaxTokens)
	}
	return result, nil
}

// Chat 实现 Client.Chat。
func (c *RateLimitedClient) Chat(messages []Message, options GenerateOptions) (string, error) {
	return c.ChatWithContext(context.Background(), messages, options)
}

// ChatWithContext 实现 Client.ChatWithContext，调用前后执行限流。
func (c *RateLimitedClient) ChatWithContext(ctx context.Context, messages []Message, options GenerateOptions) (string, error) {
	if c.rateLimiter != nil {
		provider := c.inner.Provider()
		promptText := messagesText(messages)
		estimatedTokens := estimateTokens(promptText, options.MaxTokens)
		start := time.Now()
		if err := c.rateLimiter.Wait(ctx, provider, estimatedTokens); err != nil {
			return "", err
		}
		waited := time.Since(start)
		if waited > 100*time.Millisecond {
			metrics.RateLimitWaitSeconds.WithLabelValues("llm", provider).Observe(waited.Seconds())
		}
		defer c.rateLimiter.Release(provider)
	}

	result, err := c.inner.ChatWithContext(ctx, messages, options)
	if err != nil {
		return "", err
	}
	if c.rateLimiter != nil {
		c.rateLimiter.RecordTokenUsage(c.inner.Provider(), options.MaxTokens)
	}
	return result, nil
}

// Model 返回底层 Client 的模型名称。
func (c *RateLimitedClient) Model() string { return c.inner.Model() }

// Provider 返回底层 Client 的提供商名称。
func (c *RateLimitedClient) Provider() string { return c.inner.Provider() }

// SetModel 代理到底层 Client。
func (c *RateLimitedClient) SetModel(model string) { c.inner.SetModel(model) }

// SetAPIKey 代理到底层 Client。
func (c *RateLimitedClient) SetAPIKey(apiKey string) { c.inner.SetAPIKey(apiKey) }

// estimateTokens 粗略估算请求的 token 数（4 字符 ≈ 1 token）。
func estimateTokens(text string, maxTokens int) int {
	estimated := len(text) / 4
	if maxTokens > 0 {
		estimated += maxTokens
	}
	if estimated < 1 {
		estimated = 1
	}
	return estimated
}

// messagesText 将消息列表合并为单一字符串，用于 token 估算。
func messagesText(msgs []Message) string {
	total := 0
	for _, m := range msgs {
		total += len(m.Content)
	}
	buf := make([]byte, 0, total)
	for _, m := range msgs {
		buf = append(buf, m.Content...)
	}
	return string(buf)
}
