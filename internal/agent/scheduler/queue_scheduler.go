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
	"sync"
	"time"

	"rag-platform/internal/agent/job"
)

// FairnessPolicy 公平性策略配置
type FairnessPolicy struct {
	// 各优先级队列的时间片分配权重（百分比）
	RealtimeWeight   int `yaml:"realtime_weight"`   // 实时队列权重
	DefaultWeight    int `yaml:"default_weight"`    // 默认队列权重
	BackgroundWeight int `yaml:"background_weight"` // 后台队列权重
	HeavyWeight      int `yaml:"heavy_weight"`      // 重任务队列权重

	// Starvation 防护：低优先级任务等待超过此时长，临时提升优先级
	StarvationThreshold time.Duration `yaml:"starvation_threshold"`
}

// DefaultFairnessPolicy 默认公平性策略
func DefaultFairnessPolicy() FairnessPolicy {
	return FairnessPolicy{
		RealtimeWeight:      70, // 高优先级 70%
		DefaultWeight:       20, // 中优先级 20%
		BackgroundWeight:    8,  // 低优先级 8%
		HeavyWeight:         2,  // 重任务 2%
		StarvationThreshold: 5 * time.Minute,
	}
}

// FairQueueScheduler 公平队列调度器，防止低优先级任务饿死
type FairQueueScheduler struct {
	mu     sync.RWMutex
	policy FairnessPolicy

	// 队列状态追踪
	queueWeights map[string]int // queue class -> weight
	currentRound int            // 当前轮次
	queueTickets map[string]int // queue class -> remaining tickets in current round

	// Job 等待时间追踪（用于 starvation 防护）
	jobWaitStart map[string]time.Time // job_id -> 开始等待时间
}

// NewFairQueueScheduler 创建公平队列调度器
func NewFairQueueScheduler(policy FairnessPolicy) *FairQueueScheduler {
	scheduler := &FairQueueScheduler{
		policy:       policy,
		queueWeights: make(map[string]int),
		queueTickets: make(map[string]int),
		jobWaitStart: make(map[string]time.Time),
	}

	// 初始化队列权重
	scheduler.queueWeights[job.QueueRealtime] = policy.RealtimeWeight
	scheduler.queueWeights[job.QueueDefault] = policy.DefaultWeight
	scheduler.queueWeights[job.QueueBackground] = policy.BackgroundWeight
	scheduler.queueWeights[job.QueueHeavy] = policy.HeavyWeight

	scheduler.resetRound()
	return scheduler
}

// resetRound 重置轮次，分配新的 tickets
func (s *FairQueueScheduler) resetRound() {
	s.queueTickets = make(map[string]int)
	for queueClass, weight := range s.queueWeights {
		s.queueTickets[queueClass] = weight
	}
	s.currentRound++
}

// SelectQueue 选择下一个应该执行的队列（weighted round-robin）
func (s *FairQueueScheduler) SelectQueue(ctx context.Context, availableQueues []string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 检查当前轮次是否还有 tickets
	totalTickets := 0
	for _, tickets := range s.queueTickets {
		totalTickets += tickets
	}

	// 如果当前轮次所有 tickets 用完，开始新轮次
	if totalTickets == 0 {
		s.resetRound()
	}

	// 按权重选择队列（tickets 多的队列更有可能被选中）
	maxTickets := -1
	selectedQueue := ""
	for _, queueClass := range availableQueues {
		tickets := s.queueTickets[queueClass]
		if tickets > maxTickets {
			maxTickets = tickets
			selectedQueue = queueClass
		}
	}

	// 消耗一个 ticket
	if selectedQueue != "" && s.queueTickets[selectedQueue] > 0 {
		s.queueTickets[selectedQueue]--
	}

	return selectedQueue
}

// TrackJobStart 记录 job 开始等待的时间
func (s *FairQueueScheduler) TrackJobStart(jobID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.jobWaitStart[jobID]; !exists {
		s.jobWaitStart[jobID] = time.Now()
	}
}

// CheckStarvation 检查 job 是否等待过久（需要临时提升优先级）
func (s *FairQueueScheduler) CheckStarvation(jobID string) bool {
	s.mu.RLock()
	startTime, exists := s.jobWaitStart[jobID]
	s.mu.RUnlock()

	if !exists {
		return false
	}

	waitDuration := time.Since(startTime)
	return waitDuration > s.policy.StarvationThreshold
}

// CompleteJob 记录 job 完成，清理追踪状态
func (s *FairQueueScheduler) CompleteJob(jobID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.jobWaitStart, jobID)
}

// GetStats 获取调度统计信息
func (s *FairQueueScheduler) GetStats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := map[string]interface{}{
		"current_round": s.currentRound,
		"queue_tickets": make(map[string]int),
		"queue_weights": make(map[string]int),
		"waiting_jobs":  len(s.jobWaitStart),
	}

	for queueClass, tickets := range s.queueTickets {
		stats["queue_tickets"].(map[string]int)[queueClass] = tickets
	}

	for queueClass, weight := range s.queueWeights {
		stats["queue_weights"].(map[string]int)[queueClass] = weight
	}

	// 统计饿死风险的 jobs
	starvedCount := 0
	now := time.Now()
	for _, startTime := range s.jobWaitStart {
		if now.Sub(startTime) > s.policy.StarvationThreshold {
			starvedCount++
		}
	}
	stats["starved_jobs"] = starvedCount

	return stats
}

// AdjustWeights 动态调整队列权重（用于负载均衡）
func (s *FairQueueScheduler) AdjustWeights(queueClass string, newWeight int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if newWeight < 0 {
		newWeight = 0
	}
	if newWeight > 100 {
		newWeight = 100
	}

	s.queueWeights[queueClass] = newWeight
}
