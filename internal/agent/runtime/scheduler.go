package runtime

import (
	"context"
	"sync"
)

// RunFunc 执行指定 Agent 的回调（由应用层注入，避免 runtime 依赖执行细节）
type RunFunc func(ctx context.Context, agentID string)

// Scheduler 调度器：Wake / Suspend / Resume，与 Manager 协作驱动 Agent 执行
type Scheduler struct {
	manager *Manager
	run     RunFunc
	mu      sync.Mutex
}

// NewScheduler 创建调度器；run 可为 nil，则 WakeAgent/Resume 仅改状态不触发执行
func NewScheduler(manager *Manager, run RunFunc) *Scheduler {
	return &Scheduler{manager: manager, run: run}
}

// SetRunFunc 设置执行回调（可在启动后注入）
func (s *Scheduler) SetRunFunc(run RunFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.run = run
}

// WakeAgent 唤醒 Agent：若已 Idle/Suspended 则触发执行
func (s *Scheduler) WakeAgent(ctx context.Context, agentID string) error {
	agent, err := s.manager.Get(ctx, agentID)
	if err != nil || agent == nil {
		return nil
	}
	status := agent.GetStatus()
	if status == StatusRunning || status == StatusWaitingTool {
		return nil
	}
	agent.SetStatus(StatusRunning)
	s.mu.Lock()
	fn := s.run
	s.mu.Unlock()
	if fn != nil {
		fn(ctx, agentID)
	}
	return nil
}

// Suspend 挂起 Agent
func (s *Scheduler) Suspend(ctx context.Context, agentID string) error {
	agent, err := s.manager.Get(ctx, agentID)
	if err != nil || agent == nil {
		return nil
	}
	agent.SetStatus(StatusSuspended)
	return nil
}

// Resume 恢复 Agent：置为 Idle 并触发执行
func (s *Scheduler) Resume(ctx context.Context, agentID string) error {
	agent, err := s.manager.Get(ctx, agentID)
	if err != nil || agent == nil {
		return nil
	}
	agent.SetStatus(StatusIdle)
	agent.SetStatus(StatusRunning)
	s.mu.Lock()
	fn := s.run
	s.mu.Unlock()
	if fn != nil {
		fn(ctx, agentID)
	}
	return nil
}

// Stop 停止 Agent（置为 Idle，不触发执行）
func (s *Scheduler) Stop(ctx context.Context, agentID string) error {
	agent, err := s.manager.Get(ctx, agentID)
	if err != nil || agent == nil {
		return nil
	}
	agent.SetStatus(StatusIdle)
	return nil
}
