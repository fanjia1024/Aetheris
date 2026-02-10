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

package worker

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"time"

	"rag-platform/internal/agent/job"
	agentexec "rag-platform/internal/agent/runtime/executor"
	"rag-platform/internal/runtime/jobstore"
	"rag-platform/pkg/log"
	"rag-platform/pkg/metrics"
)

// AgentJobRunner 从事件存储 Claim Job，从元数据存储取 Job，执行 Runner 并写回事件与状态；支持并发上限（Backpressure）与按能力派发
type AgentJobRunner struct {
	workerID        string
	jobEventStore   jobstore.JobStore
	jobStore        job.JobStore
	runJob          func(ctx context.Context, j *job.Job) error
	capabilities    []string // Worker 能力列表；非空时按能力从 jobStore 选 Job 再在 eventStore 占租约
	pollInterval    time.Duration
	leaseDuration   time.Duration
	heartbeatTicker time.Duration
	maxConcurrency  int
	limiter         chan struct{} // 信号量，限制同时执行的 Job 数，避免 goroutine/LLM 爆炸
	logger          *log.Logger
	stopCh          chan struct{}
	wg              sync.WaitGroup
}

// NewAgentJobRunner 创建 Agent Job 拉取执行器；runJob 由外部注入；maxConcurrency 为同时执行 Job 上限，<=0 时默认 2；capabilities 非空时按能力派发（仅认领 RequiredCapabilities 满足的 Job）
func NewAgentJobRunner(
	workerID string,
	jobEventStore jobstore.JobStore,
	jobStore job.JobStore,
	runJob func(ctx context.Context, j *job.Job) error,
	pollInterval, leaseDuration time.Duration,
	maxConcurrency int,
	capabilities []string,
	logger *log.Logger,
) *AgentJobRunner {
	heartbeat := leaseDuration / 2
	if heartbeat <= 0 {
		heartbeat = 15 * time.Second
	}
	if maxConcurrency <= 0 {
		maxConcurrency = 2
	}
	return &AgentJobRunner{
		workerID:        workerID,
		jobEventStore:   jobEventStore,
		jobStore:        jobStore,
		runJob:          runJob,
		capabilities:    capabilities,
		pollInterval:    pollInterval,
		leaseDuration:   leaseDuration,
		heartbeatTicker: heartbeat,
		maxConcurrency:  maxConcurrency,
		limiter:         make(chan struct{}, maxConcurrency),
		logger:          logger,
		stopCh:          make(chan struct{}),
	}
}

// Start 启动 Claim 循环；先占并发槽位再 Claim，执行后释放槽位（Backpressure）；capabilities 非空时按能力从 jobStore 选 Job 再在 eventStore 占租约
func (r *AgentJobRunner) Start(ctx context.Context) {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		for {
			select {
			case <-r.stopCh:
				return
			case <-ctx.Done():
				return
			case r.limiter <- struct{}{}:
				// 孤儿回收（design/runtime-contract.md §2）：以 event store 租约过期为准，且不回收 Blocked(JobWaiting) 的 Job
				if reclaimed, err := job.ReclaimOrphanedFromEventStore(ctx, r.jobStore, r.jobEventStore); err == nil && reclaimed > 0 {
					r.logger.Info("回收孤儿 Job", "reclaimed", reclaimed)
				}
				var jobID string
				if len(r.capabilities) > 0 {
					// 按能力派发：先从 metadata store 认领能力匹配的 Job，再在 event store 占租约
					j, errClaim := r.jobStore.ClaimNextPendingForWorker(ctx, "", r.capabilities)
					if errClaim != nil || j == nil {
						<-r.limiter
						select {
						case <-r.stopCh:
							return
						case <-ctx.Done():
							return
						case <-time.After(r.pollInterval):
						}
						continue
					}
					_, attemptID, errEvent := r.jobEventStore.ClaimJob(ctx, r.workerID, j.ID)
					if errEvent != nil {
						_ = r.jobStore.Requeue(ctx, j)
						<-r.limiter
						if errEvent != jobstore.ErrNoJob && errEvent != jobstore.ErrClaimNotFound {
							r.logger.Error("ClaimJob 失败", "job_id", j.ID, "error", errEvent)
						}
						time.Sleep(r.pollInterval)
						continue
					}
					jobID = j.ID
					r.wg.Add(1)
					go func(claimedJobID, aid string) {
						defer r.wg.Done()
						defer func() { <-r.limiter }()
						r.executeJob(ctx, claimedJobID, aid)
					}(jobID, attemptID)
					continue
				}
				{
					claimedJobID, _, attemptID, err := r.jobEventStore.Claim(ctx, r.workerID)
					if err != nil {
						<-r.limiter
						if err == jobstore.ErrNoJob {
							select {
							case <-r.stopCh:
								return
							case <-ctx.Done():
								return
							case <-time.After(r.pollInterval):
								continue
							}
						}
						r.logger.Error("Claim 失败", "error", err)
						time.Sleep(r.pollInterval)
						continue
					}
					r.wg.Add(1)
					go func(claimedJobID, aid string) {
						defer r.wg.Done()
						defer func() { <-r.limiter }()
						r.executeJob(ctx, claimedJobID, aid)
					}(claimedJobID, attemptID)
				}
			}
		}
	}()
}

// Stop 停止 Claim 循环并等待当前执行中的 Job 结束
func (r *AgentJobRunner) Stop() {
	close(r.stopCh)
	r.wg.Wait()
}

const cancelPollInterval = 500 * time.Millisecond

func (r *AgentJobRunner) executeJob(ctx context.Context, jobID string, attemptID string) {
	j, err := r.jobStore.Get(ctx, jobID)
	if err != nil || j == nil {
		r.logger.Warn("Get Job 失败或不存在，跳过", "job_id", jobID, "error", err)
		return
	}
	metrics.WorkerBusy.WithLabelValues(r.workerID).Inc()
	defer metrics.WorkerBusy.WithLabelValues(r.workerID).Dec()
	start := time.Now()
	defer func() {
		dur := time.Since(start).Seconds()
		metrics.JobDuration.WithLabelValues(j.AgentID).Observe(dur)
	}()
	// 元数据与事件一致：Claim 成功后标记 Running，便于查询与运维
	_ = r.jobStore.UpdateStatus(ctx, jobID, job.StatusRunning)
	r.logger.Info("开始执行 Job", "job_id", jobID, "agent_id", j.AgentID, "goal", j.Goal)
	runCtx, cancel := context.WithCancel(ctx)
	runCtx = jobstore.WithAttemptID(runCtx, attemptID)
	defer cancel()
	// 后台 Heartbeat
	heartbeatDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(r.heartbeatTicker)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				close(heartbeatDone)
				return
			case <-ticker.C:
				if err := r.jobEventStore.Heartbeat(runCtx, r.workerID, jobID); err != nil {
					r.logger.Warn("Heartbeat 失败", "job_id", jobID, "error", err)
				}
			}
		}
	}()
	// 轮询取消请求：API 调用 RequestCancel 后 Worker 取消 runCtx，使 LLM/tool 中断
	go func() {
		ticker := time.NewTicker(cancelPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				j2, _ := r.jobStore.Get(ctx, jobID)
				if j2 != nil && !j2.CancelRequestedAt.IsZero() {
					cancel()
					return
				}
			}
		}
	}()
	err = r.runJob(runCtx, j)
	<-heartbeatDone
	if runCtx.Err() == context.Canceled {
		r.logger.Info("Job 已取消", "job_id", jobID)
		metrics.JobTotal.WithLabelValues("cancelled").Inc()
		metrics.JobFailTotal.WithLabelValues("cancelled").Inc()
		_, ver, _ := r.jobEventStore.ListEvents(ctx, jobID)
		payload, _ := json.Marshal(map[string]interface{}{"goal": j.Goal})
		_, _ = r.jobEventStore.Append(runCtx, jobID, ver, jobstore.JobEvent{JobID: jobID, Type: jobstore.JobCancelled, Payload: payload})
		_ = r.jobStore.UpdateStatus(ctx, jobID, job.StatusCancelled)
		return
	}
	if err != nil {
		r.logger.Info("Job 执行失败", "job_id", jobID, "error", err)
		metrics.JobTotal.WithLabelValues("failed").Inc()
		metrics.JobFailTotal.WithLabelValues("failed").Inc()
		// Append job_failed so event stream has terminal event; include result_type when available
		if r.jobEventStore != nil {
			_, ver, _ := r.jobEventStore.ListEvents(ctx, jobID)
			pl := map[string]interface{}{"goal": j.Goal, "error": err.Error()}
			var sf *agentexec.StepFailure
			if errors.As(err, &sf) {
				pl["result_type"] = string(sf.Type)
				pl["node_id"] = sf.FailedNodeID()
				pl["reason"] = err.Error()
			}
			payload, _ := json.Marshal(pl)
			_, _ = r.jobEventStore.Append(runCtx, jobID, ver, jobstore.JobEvent{JobID: jobID, Type: jobstore.JobFailed, Payload: payload})
		}
		return
	}
	metrics.JobTotal.WithLabelValues("completed").Inc()
	// 事件与状态已在 runJob 内写回（由注入的 runJob 负责 Append job_completed/job_failed 与 UpdateStatus）
}

// DefaultWorkerID 返回默认 Worker 标识（hostname 或 env）
func DefaultWorkerID() string {
	if id := os.Getenv("WORKER_ID"); id != "" {
		return id
	}
	host, _ := os.Hostname()
	if host != "" {
		return host
	}
	return "worker-unknown"
}

// jobStoreForRunnerAdapter 将 job.JobStore 适配为 executor.JobStoreForRunner
type jobStoreForRunnerAdapter struct {
	job.JobStore
}

func (a *jobStoreForRunnerAdapter) UpdateStatus(ctx context.Context, jobID string, status int) error {
	return a.JobStore.UpdateStatus(ctx, jobID, job.JobStatus(status))
}
