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

package sdk

import (
	"context"
	"fmt"
	"time"
)

// Agent 高层 Agent 门面：通过 AgentRuntime 提交任务并等待结果，可选注册工具
type Agent struct {
	runtime AgentRuntime
	agentID string
	config  AgentConfig
}

// NewAgent 创建 SDK Agent；runtime 由应用层注入（封装 JobStore + Runner）
func NewAgent(runtime AgentRuntime, agentID string, opts ...Option) *Agent {
	cfg := AgentConfig{WaitTimeout: 5 * time.Minute}
	for _, o := range opts {
		o(&cfg)
	}
	return &Agent{runtime: runtime, agentID: agentID, config: cfg}
}

// RegisterTool 注册工具；若 runtime 实现 ToolRegistrar 则转发，否则忽略
func (a *Agent) RegisterTool(name string, fn ToolFunc) {
	if r, ok := a.runtime.(ToolRegistrar); ok {
		r.RegisterTool(a.agentID, name, fn)
	}
}

// Run 提交 query 为 goal、等待完成并返回最终回答
func (a *Agent) Run(ctx context.Context, query string) (answer string, err error) {
	jobID, err := a.runtime.Submit(ctx, a.agentID, query, "")
	if err != nil {
		return "", fmt.Errorf("sdk: submit: %w", err)
	}
	waitCtx := ctx
	if a.config.WaitTimeout > 0 {
		var cancel context.CancelFunc
		waitCtx, cancel = context.WithTimeout(ctx, a.config.WaitTimeout)
		defer cancel()
	}
	status, answer, err := a.runtime.WaitCompleted(waitCtx, jobID)
	if err != nil {
		return "", fmt.Errorf("sdk: wait %s: %w", jobID, err)
	}
	if status != "completed" && status != "2" {
		return answer, fmt.Errorf("sdk: job %s status %s", jobID, status)
	}
	return answer, nil
}

// RunWithSession 提交 query 并指定 sessionID（用于多轮对话）
func (a *Agent) RunWithSession(ctx context.Context, sessionID, query string) (answer string, err error) {
	jobID, err := a.runtime.Submit(ctx, a.agentID, query, sessionID)
	if err != nil {
		return "", fmt.Errorf("sdk: submit: %w", err)
	}
	waitCtx := ctx
	if a.config.WaitTimeout > 0 {
		var cancel context.CancelFunc
		waitCtx, cancel = context.WithTimeout(ctx, a.config.WaitTimeout)
		defer cancel()
	}
	status, answer, err := a.runtime.WaitCompleted(waitCtx, jobID)
	if err != nil {
		return "", fmt.Errorf("sdk: wait %s: %w", jobID, err)
	}
	if status != "completed" && status != "2" {
		return answer, fmt.Errorf("sdk: job %s status %s", jobID, status)
	}
	return answer, nil
}
