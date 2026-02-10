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

import "context"

// AgentRuntime 供 SDK Agent 使用的运行时抽象：提交 Job、等待完成并取回结果
type AgentRuntime interface {
	// Submit 提交一条 Agent 任务，返回 jobID
	Submit(ctx context.Context, agentID, goal, sessionID string) (jobID string, err error)
	// WaitCompleted 阻塞直到 Job 完成或 ctx 取消，返回状态与最终回答（从 Session 或 Job 结果取）
	WaitCompleted(ctx context.Context, jobID string) (status string, answer string, err error)
}

// ToolRegistrar 可选：支持在运行时注册工具（由实现方将工具注入 Executor/Planner）
type ToolRegistrar interface {
	RegisterTool(agentID, name string, fn ToolFunc)
}
