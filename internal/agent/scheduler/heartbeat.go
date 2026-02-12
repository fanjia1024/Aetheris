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

package scheduler

import (
	"context"
	"time"
)

// HeartbeatRunner 在独立循环中定期续租，与 Worker 执行循环解耦；runLoop 卡住时仍能在一段时间后因「未 Heartbeat」导致租约过期，便于 Reclaim。
type HeartbeatRunner struct {
	manager  LeaseManager
	interval time.Duration
	onError  func(jobID string, err error)
}

// HeartbeatRunnerConfig 心跳运行器配置
type HeartbeatRunnerConfig struct {
	// Interval 心跳间隔；应小于租约 TTL（如 TTL/2）
	Interval time.Duration
	// OnError 心跳失败时的回调（如打日志）；可选
	OnError func(jobID string, err error)
}

// NewHeartbeatRunner 创建心跳运行器
func NewHeartbeatRunner(manager LeaseManager, cfg HeartbeatRunnerConfig) *HeartbeatRunner {
	interval := cfg.Interval
	if interval <= 0 {
		interval = 10 * time.Second
	}
	return &HeartbeatRunner{
		manager:  manager,
		interval: interval,
		onError:  cfg.OnError,
	}
}

// Run 在 goroutine 中周期调用 Heartbeat(workerID, jobID)，直到 ctx 取消或 stopCh 关闭
func (h *HeartbeatRunner) Run(ctx context.Context, workerID, jobID string, stopCh <-chan struct{}) {
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-stopCh:
			return
		case <-ticker.C:
			if err := h.manager.Heartbeat(ctx, workerID, jobID); err != nil && h.onError != nil {
				h.onError(jobID, err)
			}
		}
	}
}
