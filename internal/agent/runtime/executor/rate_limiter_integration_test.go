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
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestToolRateLimiter_QPSEnforcement 验证 ToolRateLimiter 在并发场景下 QPS 上限有效。
// 配置 QPS=5，并发发出 20 个请求，实际通过数应 ≤ QPS*测量窗口 + burst。
func TestToolRateLimiter_QPSEnforcement(t *testing.T) {
	const (
		qps           = 5.0
		concurrency   = 20
		measureWindow = 300 * time.Millisecond
	)

	limiter := NewToolRateLimiter(map[string]ToolLimitConfig{
		"test_tool": {QPS: qps, MaxConcurrent: concurrency, Burst: 1},
	}, nil)

	ctx, cancel := context.WithTimeout(context.Background(), measureWindow)
	defer cancel()

	var passed int64
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := limiter.Wait(ctx, "test_tool"); err == nil {
				atomic.AddInt64(&passed, 1)
				limiter.Release("test_tool")
			}
		}()
	}
	wg.Wait()

	// 300ms 内按 QPS=5 最多通过 qps*0.3s + burst = 1.5+1 ≈ 2~3，加上 burst 最多 4。
	// 保守上限设为 qps*窗口(秒)*2 = 3，实际可根据实现微调。
	maxExpected := int64(qps*float64(measureWindow.Seconds())*2) + 2
	if passed > maxExpected {
		t.Errorf("rate limiter QPS 超限：passed=%d > maxExpected=%d (qps=%.1f, window=%s)",
			passed, maxExpected, qps, measureWindow)
	}
	t.Logf("rate limit QPS test: passed=%d (qps=%.1f, window=%s)", passed, qps, measureWindow)
}

// TestToolRateLimiter_ConcurrencyEnforcement 验证并发限制正确工作。
func TestToolRateLimiter_ConcurrencyEnforcement(t *testing.T) {
	const maxConcurrent = 3

	limiter := NewToolRateLimiter(map[string]ToolLimitConfig{
		"test_tool": {QPS: 1000, MaxConcurrent: maxConcurrent, Burst: 1000},
	}, nil)

	ctx := context.Background()
	var inflight int64
	var maxObserved int64
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := limiter.Wait(ctx, "test_tool"); err != nil {
				return
			}
			curr := atomic.AddInt64(&inflight, 1)
			// 记录峰值
			for {
				obs := atomic.LoadInt64(&maxObserved)
				if curr <= obs || atomic.CompareAndSwapInt64(&maxObserved, obs, curr) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
			atomic.AddInt64(&inflight, -1)
			limiter.Release("test_tool")
		}()
	}
	wg.Wait()

	if maxObserved > maxConcurrent {
		t.Errorf("并发超限：maxObserved=%d > maxConcurrent=%d", maxObserved, maxConcurrent)
	}
	t.Logf("concurrency test: maxObserved=%d (limit=%d)", maxObserved, maxConcurrent)
}

// TestToolRateLimiter_DefaultConfig 验证未配置的 tool 使用默认配置。
func TestToolRateLimiter_DefaultConfig(t *testing.T) {
	defaults := &ToolLimitConfig{QPS: 100, MaxConcurrent: 10}
	limiter := NewToolRateLimiter(nil, defaults)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	if err := limiter.Wait(ctx, "unknown_tool"); err != nil {
		t.Errorf("unexpected error for unknown tool with default config: %v", err)
	}
	limiter.Release("unknown_tool")
}
