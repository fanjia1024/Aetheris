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

package executor

import (
	"context"
	"fmt"
	"sync"

	"golang.org/x/time/rate"
)

// ToolLimitConfig Tool 限流配置
type ToolLimitConfig struct {
	QPS           float64 `yaml:"qps"`            // 每秒请求数限制
	MaxConcurrent int     `yaml:"max_concurrent"` // 最大并发数
	Burst         int     `yaml:"burst"`          // 令牌桶容量（可选，默认为 QPS）
}

// ToolRateLimiter Tool 维度的限流器，支持 QPS + 并发控制
type ToolRateLimiter struct {
	mu       sync.RWMutex
	limiters map[string]*toolLimiter // toolName -> limiter
	defaults *ToolLimitConfig        // 默认配置
}

type toolLimiter struct {
	rateLimiter *rate.Limiter // QPS 限流器
	semaphore   chan struct{} // 并发控制
	config      ToolLimitConfig
}

// NewToolRateLimiter 创建 Tool 限流器
func NewToolRateLimiter(configs map[string]ToolLimitConfig, defaults *ToolLimitConfig) *ToolRateLimiter {
	if defaults == nil {
		defaults = &ToolLimitConfig{
			QPS:           100, // 默认每秒 100 次
			MaxConcurrent: 10,  // 默认最大并发 10
			Burst:         100,
		}
	}

	limiter := &ToolRateLimiter{
		limiters: make(map[string]*toolLimiter),
		defaults: defaults,
	}

	// 初始化配置的 tool limiters
	for toolName, config := range configs {
		limiter.addToolLimiter(toolName, config)
	}

	return limiter
}

// addToolLimiter 添加 tool 限流器
func (t *ToolRateLimiter) addToolLimiter(toolName string, config ToolLimitConfig) {
	if config.Burst == 0 {
		config.Burst = int(config.QPS)
	}

	limiter := &toolLimiter{
		config: config,
	}

	if config.QPS > 0 {
		limiter.rateLimiter = rate.NewLimiter(rate.Limit(config.QPS), config.Burst)
	}

	if config.MaxConcurrent > 0 {
		limiter.semaphore = make(chan struct{}, config.MaxConcurrent)
	}

	t.mu.Lock()
	t.limiters[toolName] = limiter
	t.mu.Unlock()
}

// Wait 等待获取执行许可（阻塞直到可以执行）
func (t *ToolRateLimiter) Wait(ctx context.Context, toolName string) error {
	t.mu.RLock()
	limiter, exists := t.limiters[toolName]
	t.mu.RUnlock()

	if !exists {
		// 使用默认配置创建限流器
		t.addToolLimiter(toolName, *t.defaults)
		t.mu.RLock()
		limiter = t.limiters[toolName]
		t.mu.RUnlock()
	}

	// QPS 限流
	if limiter.rateLimiter != nil {
		if err := limiter.rateLimiter.Wait(ctx); err != nil {
			return fmt.Errorf("rate limit wait failed: %w", err)
		}
	}

	// 并发限流（acquire semaphore）
	if limiter.semaphore != nil {
		select {
		case limiter.semaphore <- struct{}{}:
			// 获取到 slot
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return nil
}

// Release 释放并发 slot（在 tool 执行完成后调用）
func (t *ToolRateLimiter) Release(toolName string) {
	t.mu.RLock()
	limiter, exists := t.limiters[toolName]
	t.mu.RUnlock()

	if exists && limiter.semaphore != nil {
		select {
		case <-limiter.semaphore:
			// 释放 slot
		default:
			// semaphore 已空，无需释放
		}
	}
}

// Allow 检查是否允许执行（非阻塞，立即返回）
func (t *ToolRateLimiter) Allow(toolName string) bool {
	t.mu.RLock()
	limiter, exists := t.limiters[toolName]
	t.mu.RUnlock()

	if !exists {
		// not configured限流，允许执行
		return true
	}

	// 检查 QPS 限流
	if limiter.rateLimiter != nil && !limiter.rateLimiter.Allow() {
		return false
	}

	// 检查并发限流
	if limiter.semaphore != nil {
		select {
		case limiter.semaphore <- struct{}{}:
			// 获取到 slot
			return true
		default:
			// 并发已满
			return false
		}
	}

	return true
}

// GetStats 获取限流统计信息
func (t *ToolRateLimiter) GetStats(toolName string) map[string]interface{} {
	t.mu.RLock()
	limiter, exists := t.limiters[toolName]
	t.mu.RUnlock()

	if !exists {
		return nil
	}

	stats := map[string]interface{}{
		"qps":            limiter.config.QPS,
		"max_concurrent": limiter.config.MaxConcurrent,
	}

	if limiter.semaphore != nil {
		stats["current_concurrent"] = len(limiter.semaphore)
		stats["available_slots"] = cap(limiter.semaphore) - len(limiter.semaphore)
	}

	return stats
}
