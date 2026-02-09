package worker

import (
	"context"
	"os"
	"sync"
	"time"

	"rag-platform/internal/agent/job"
	"rag-platform/internal/runtime/jobstore"
	"rag-platform/pkg/log"
)

// AgentJobRunner 从事件存储 Claim Job，从元数据存储取 Job，执行 Runner 并写回事件与状态
type AgentJobRunner struct {
	workerID        string
	jobEventStore   jobstore.JobStore
	jobStore        job.JobStore
	runJob          func(ctx context.Context, j *job.Job) error
	pollInterval    time.Duration
	leaseDuration   time.Duration
	heartbeatTicker time.Duration
	logger          *log.Logger
	stopCh          chan struct{}
	wg              sync.WaitGroup
}

// NewAgentJobRunner 创建 Agent Job 拉取执行器；runJob 由外部注入（含 DAG Runner 与事件写回）
func NewAgentJobRunner(
	workerID string,
	jobEventStore jobstore.JobStore,
	jobStore job.JobStore,
	runJob func(ctx context.Context, j *job.Job) error,
	pollInterval, leaseDuration time.Duration,
	logger *log.Logger,
) *AgentJobRunner {
	heartbeat := leaseDuration / 2
	if heartbeat <= 0 {
		heartbeat = 15 * time.Second
	}
	return &AgentJobRunner{
		workerID:        workerID,
		jobEventStore:   jobEventStore,
		jobStore:        jobStore,
		runJob:          runJob,
		pollInterval:    pollInterval,
		leaseDuration:   leaseDuration,
		heartbeatTicker: heartbeat,
		logger:          logger,
		stopCh:          make(chan struct{}),
	}
}

// Start 启动 Claim 循环；在 goroutine 中拉取并执行 Job
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
			default:
				jobID, _, err := r.jobEventStore.Claim(ctx, r.workerID)
				if err != nil {
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
				// 在 goroutine 中执行，避免阻塞 Claim 循环
				r.wg.Add(1)
				go func(claimedJobID string) {
					defer r.wg.Done()
					r.executeJob(ctx, claimedJobID)
				}(jobID)
			}
		}
	}()
}

// Stop 停止 Claim 循环并等待当前执行中的 Job 结束
func (r *AgentJobRunner) Stop() {
	close(r.stopCh)
	r.wg.Wait()
}

func (r *AgentJobRunner) executeJob(ctx context.Context, jobID string) {
	j, err := r.jobStore.Get(ctx, jobID)
	if err != nil || j == nil {
		r.logger.Warn("Get Job 失败或不存在，跳过", "job_id", jobID, "error", err)
		return
	}
	r.logger.Info("开始执行 Job", "job_id", jobID, "agent_id", j.AgentID, "goal", j.Goal)
	runCtx, cancel := context.WithCancel(ctx)
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
	err = r.runJob(runCtx, j)
	<-heartbeatDone
	if err != nil {
		r.logger.Info("Job 执行失败", "job_id", jobID, "error", err)
	}
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
