package job

import (
	"context"
	"sync"
	"time"

	"rag-platform/internal/agent/runtime"
	agentexec "rag-platform/internal/agent/runtime/executor"
)

// JobRunner 后台拉取 Pending Job 并调用 executor.Runner 执行
type JobRunner struct {
	store   JobStore
	manager *runtime.Manager
	runner  *agentexec.Runner

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewJobRunner 创建 JobRunner
func NewJobRunner(store JobStore, manager *runtime.Manager, runner *agentexec.Runner) *JobRunner {
	return &JobRunner{
		store:   store,
		manager: manager,
		runner:  runner,
		stopCh:  make(chan struct{}),
	}
}

// Start 启动后台循环：拉取 Pending Job，执行，更新状态；ctx 用于执行时传递，不用于停止
func (r *JobRunner) Start(ctx context.Context) {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		for {
			select {
			case <-r.stopCh:
				return
			default:
			}
			j, _ := r.store.ClaimNextPending(ctx)
			if j == nil {
				time.Sleep(200 * time.Millisecond)
				continue
			}
			runCtx := context.Background()
			agent, _ := r.manager.Get(runCtx, j.AgentID)
			if agent == nil {
				_ = r.store.UpdateStatus(runCtx, j.ID, StatusFailed)
				continue
			}
			err := r.runner.RunForJob(runCtx, agent, &agentexec.JobForRunner{
				ID: j.ID, AgentID: j.AgentID, Goal: j.Goal, Cursor: j.Cursor,
			})
			if err != nil {
				_ = r.store.UpdateStatus(runCtx, j.ID, StatusFailed)
			} else {
				_ = r.store.UpdateStatus(runCtx, j.ID, StatusCompleted)
			}
		}
	}()
}

// Stop 优雅退出：关闭 stopCh，等待后台 goroutine 结束
func (r *JobRunner) Stop() {
	close(r.stopCh)
	r.wg.Wait()
}
