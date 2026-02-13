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
	"fmt"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// LLMLimitConfig LLM Provider 限流配置
type LLMLimitConfig struct {
	TokensPerMinute   int     `yaml:"tokens_per_minute"`   // 每分钟 token 配额
	RequestsPerMinute float64 `yaml:"requests_per_minute"` // 每分钟请求数
	MaxConcurrent     int     `yaml:"max_concurrent"`      // 最大并发请求数
}

// LLMRateLimiter LLM Provider 维度的限流器，支持 token budget + RPS + 并发控制
type LLMRateLimiter struct {
	mu       sync.RWMutex
	limiters map[string]*llmLimiter // provider -> limiter
	defaults *LLMLimitConfig
}

type llmLimiter struct {
	requestLimiter *rate.Limiter // RPS 限流器
	tokenLimiter   *rate.Limiter // Token 限流器
	semaphore      chan struct{} // 并发控制
	config         LLMLimitConfig

	// Token 统计
	mu               sync.Mutex
	tokensUsedMinute int
	minuteStart      time.Time
}

// NewLLMRateLimiter 创建 LLM 限流器
func NewLLMRateLimiter(configs map[string]LLMLimitConfig, defaults *LLMLimitConfig) *LLMRateLimiter {
	if defaults == nil {
		defaults = &LLMLimitConfig{
			TokensPerMinute:   90000, // 默认每分钟 90K tokens
			RequestsPerMinute: 3500,  // 默认每分钟 3500 次请求
			MaxConcurrent:     50,    // 默认最大并发 50
		}
	}

	limiter := &LLMRateLimiter{
		limiters: make(map[string]*llmLimiter),
		defaults: defaults,
	}

	// 初始化配置的 provider limiters
	for provider, config := range configs {
		limiter.addProviderLimiter(provider, config)
	}

	return limiter
}

// addProviderLimiter 添加 provider 限流器
func (l *LLMRateLimiter) addProviderLimiter(provider string, config LLMLimitConfig) {
	limiter := &llmLimiter{
		config:      config,
		minuteStart: time.Now(),
	}

	// RPS 限流器（转换为每秒）
	if config.RequestsPerMinute > 0 {
		rps := config.RequestsPerMinute / 60.0
		burst := int(config.RequestsPerMinute / 60.0 * 2) // burst = 2 秒的配额
		if burst < 1 {
			burst = 1
		}
		limiter.requestLimiter = rate.NewLimiter(rate.Limit(rps), burst)
	}

	// Token 限流器（转换为每秒）
	if config.TokensPerMinute > 0 {
		tps := float64(config.TokensPerMinute) / 60.0
		burst := config.TokensPerMinute / 60 * 2 // burst = 2 秒的配额
		if burst < 1 {
			burst = 1
		}
		limiter.tokenLimiter = rate.NewLimiter(rate.Limit(tps), burst)
	}

	// 并发控制
	if config.MaxConcurrent > 0 {
		limiter.semaphore = make(chan struct{}, config.MaxConcurrent)
	}

	l.mu.Lock()
	l.limiters[provider] = limiter
	l.mu.Unlock()
}

// Wait 等待获取执行许可（阻塞直到可以执行）
func (l *LLMRateLimiter) Wait(ctx context.Context, provider string, estimatedTokens int) error {
	l.mu.RLock()
	limiter, exists := l.limiters[provider]
	l.mu.RUnlock()

	if !exists {
		// 使用默认配置创建限流器
		l.addProviderLimiter(provider, *l.defaults)
		l.mu.RLock()
		limiter = l.limiters[provider]
		l.mu.RUnlock()
	}

	// Request 限流
	if limiter.requestLimiter != nil {
		if err := limiter.requestLimiter.Wait(ctx); err != nil {
			return fmt.Errorf("request rate limit wait failed: %w", err)
		}
	}

	// Token budget 限流（预扣 tokens）
	if limiter.tokenLimiter != nil && estimatedTokens > 0 {
		// 等待足够的 token 配额
		n := estimatedTokens
		if err := limiter.tokenLimiter.WaitN(ctx, n); err != nil {
			return fmt.Errorf("token budget wait failed: %w", err)
		}
	}

	// 并发限流
	if limiter.semaphore != nil {
		select {
		case limiter.semaphore <- struct{}{}:
			// 获取到 slot
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// 更新 token 统计
	limiter.mu.Lock()
	now := time.Now()
	if now.Sub(limiter.minuteStart) > time.Minute {
		// 新的一分钟，重置计数
		limiter.tokensUsedMinute = estimatedTokens
		limiter.minuteStart = now
	} else {
		limiter.tokensUsedMinute += estimatedTokens
	}
	limiter.mu.Unlock()

	return nil
}

// Release 释放并发 slot（在 LLM 调用完成后调用）
func (l *LLMRateLimiter) Release(provider string) {
	l.mu.RLock()
	limiter, exists := l.limiters[provider]
	l.mu.RUnlock()

	if exists && limiter.semaphore != nil {
		select {
		case <-limiter.semaphore:
			// 释放 slot
		default:
			// semaphore 已空，无需释放
		}
	}
}

// RecordTokenUsage 记录实际使用的 tokens（用于精确统计）
func (l *LLMRateLimiter) RecordTokenUsage(provider string, actualTokens int) {
	l.mu.RLock()
	limiter, exists := l.limiters[provider]
	l.mu.RUnlock()

	if !exists {
		return
	}

	limiter.mu.Lock()
	now := time.Now()
	if now.Sub(limiter.minuteStart) > time.Minute {
		// 新的一分钟，重置计数
		limiter.tokensUsedMinute = actualTokens
		limiter.minuteStart = now
	} else {
		limiter.tokensUsedMinute += actualTokens
	}
	limiter.mu.Unlock()
}

// GetStats 获取限流统计信息
func (l *LLMRateLimiter) GetStats(provider string) map[string]interface{} {
	l.mu.RLock()
	limiter, exists := l.limiters[provider]
	l.mu.RUnlock()

	if !exists {
		return nil
	}

	limiter.mu.Lock()
	tokensUsed := limiter.tokensUsedMinute
	limiter.mu.Unlock()

	stats := map[string]interface{}{
		"requests_per_minute": limiter.config.RequestsPerMinute,
		"tokens_per_minute":   limiter.config.TokensPerMinute,
		"tokens_used_minute":  tokensUsed,
		"max_concurrent":      limiter.config.MaxConcurrent,
	}

	if limiter.semaphore != nil {
		stats["current_concurrent"] = len(limiter.semaphore)
		stats["available_slots"] = cap(limiter.semaphore) - len(limiter.semaphore)
	}

	return stats
}

// Allow 检查是否允许执行（非阻塞）
func (l *LLMRateLimiter) Allow(provider string, estimatedTokens int) bool {
	l.mu.RLock()
	limiter, exists := l.limiters[provider]
	l.mu.RUnlock()

	if !exists {
		return true
	}

	// 检查 request 限流
	if limiter.requestLimiter != nil && !limiter.requestLimiter.Allow() {
		return false
	}

	// 检查 token budget
	if limiter.tokenLimiter != nil && estimatedTokens > 0 {
		if !limiter.tokenLimiter.AllowN(time.Now(), estimatedTokens) {
			return false
		}
	}

	// 检查并发限流
	if limiter.semaphore != nil {
		select {
		case limiter.semaphore <- struct{}{}:
			return true
		default:
			return false
		}
	}

	return true
}
