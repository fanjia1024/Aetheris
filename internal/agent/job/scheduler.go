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

package job

import (
	"context"
	"errors"
	"sync"
	"time"

	agentexec "rag-platform/internal/agent/runtime/executor"
	"rag-platform/pkg/metrics"
)

// RunJobFunc 执行单条 Job 的回调（由应用层注入，如 Runner.RunForJob）
type RunJobFunc func(ctx context.Context, j *Job) error

// CompensateFunc 在 CompensatableFailure 时调用（jobID、失败节点 nodeID）；Week 1 可为 stub，Phase B 接真实回滚
type CompensateFunc func(ctx context.Context, jobID, nodeID string) error

// SchedulerConfig 调度器配置：并发上限、重试、backoff、队列优先级与能力派发
type SchedulerConfig struct {
	MaxConcurrency int           // 最大并发执行数，<=0 表示 1
	RetryMax       int           // 最大重试次数（不含首次）
	Backoff        time.Duration // 重试前等待时间
	// Queues 按优先级轮询的队列列表（如 realtime, default, background）；空则使用 ClaimNextPending 不区分队列
	Queues []string
	// Capabilities 调度器（Worker）能力列表；非空时仅认领 Job.RequiredCapabilities 满足的 Job
	Capabilities []string
}

// Scheduler 在 JobStore 之上提供排队、并发限制与重试；形态为 API→Job Queue→Scheduler→Worker→Executor
type Scheduler struct {
	store      JobStore
	runJob     RunJobFunc
	config     SchedulerConfig
	compensate CompensateFunc // optional; called on CompensatableFailure before marking job failed
	stopCh     chan struct{}
	wg         sync.WaitGroup
	limiter    chan struct{} // 信号量，限制并发
}

// NewScheduler 创建调度器；config 为并发与重试策略
func NewScheduler(store JobStore, runJob RunJobFunc, config SchedulerConfig) *Scheduler {
	max := config.MaxConcurrency
	if max <= 0 {
		max = 1
	}
	return &Scheduler{
		store:   store,
		runJob:  runJob,
		config:  config,
		stopCh:  make(chan struct{}),
		limiter: make(chan struct{}, max),
	}
}

// SetCompensate 设置 CompensatableFailure 时的补偿回调（可选）
func (s *Scheduler) SetCompensate(fn CompensateFunc) {
	s.compensate = fn
}

// Start 启动调度循环：最多 MaxConcurrency 个 worker 拉取 Pending、执行、成功则 UpdateStatus(Completed)，失败则按 RetryMax/Backoff 重试或 UpdateStatus(Failed)
func (s *Scheduler) Start(ctx context.Context) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			select {
			case <-s.stopCh:
				return
			case <-ctx.Done():
				return
			case s.limiter <- struct{}{}:
				tickStart := time.Now()
				// 占一个槽位后拉取；若配置了 Queues 则按队列优先级依次尝试；Capabilities 非空时按能力派发
				var j *Job
				if len(s.config.Queues) > 0 {
					for _, q := range s.config.Queues {
						if len(s.config.Capabilities) > 0 {
							j, _ = s.store.ClaimNextPendingForWorker(ctx, q, s.config.Capabilities, "")
						} else {
							j, _ = s.store.ClaimNextPendingFromQueue(ctx, q)
						}
						if j != nil {
							break
						}
					}
				} else {
					if len(s.config.Capabilities) > 0 {
						j, _ = s.store.ClaimNextPendingForWorker(ctx, "", s.config.Capabilities, "")
					} else {
						j, _ = s.store.ClaimNextPending(ctx)
					}
				}
				metrics.SchedulerTickDurationSeconds.Observe(time.Since(tickStart).Seconds())
				if j == nil {
					metrics.LeaseAcquireTotal.WithLabelValues("default", "false").Inc()
					<-s.limiter
					time.Sleep(200 * time.Millisecond)
					continue
				}
				tenant := j.TenantID
				if tenant == "" {
					tenant = "default"
				}
				metrics.LeaseAcquireTotal.WithLabelValues(tenant, "true").Inc()
				go func(job *Job) {
					defer func() { <-s.limiter }()
					runCtx := context.Background()
					err := s.runJob(runCtx, job)
					if err != nil {
						var sf *agentexec.StepFailure
						if errors.As(err, &sf) {
							switch sf.Type {
							case agentexec.StepResultRetryableFailure:
								if job.RetryCount < s.config.RetryMax {
									time.Sleep(s.config.Backoff)
									_ = s.store.Requeue(runCtx, job)
								} else {
									_ = s.store.UpdateStatus(runCtx, job.ID, StatusFailed)
								}
							case agentexec.StepResultPermanentFailure:
								_ = s.store.UpdateStatus(runCtx, job.ID, StatusFailed)
							case agentexec.StepResultCompensatableFailure:
								if s.compensate != nil {
									_ = s.compensate(runCtx, job.ID, sf.FailedNodeID())
								}
								_ = s.store.UpdateStatus(runCtx, job.ID, StatusFailed)
							case agentexec.StepResultSideEffectCommitted, agentexec.StepResultCompensated:
								// 不应以错误返回；若出现则不再重试，直接失败
								_ = s.store.UpdateStatus(runCtx, job.ID, StatusFailed)
							default:
								_ = s.store.UpdateStatus(runCtx, job.ID, StatusFailed)
							}
						} else {
							// No step outcome: backward compat, retry up to RetryMax
							if job.RetryCount < s.config.RetryMax {
								time.Sleep(s.config.Backoff)
								_ = s.store.Requeue(runCtx, job)
							} else {
								_ = s.store.UpdateStatus(runCtx, job.ID, StatusFailed)
							}
						}
					} else {
						_ = s.store.UpdateStatus(runCtx, job.ID, StatusCompleted)
					}
				}(j)
			}
		}
	}()
}

// Stop 优雅退出：关闭 stopCh，等待当前循环结束（不等待已在执行的 job 完成）
func (s *Scheduler) Stop() {
	close(s.stopCh)
	s.wg.Wait()
}
