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

package eino

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/adk"
)

// ContextManager 上下文管理器
type ContextManager struct {
	runners map[string]*adk.Runner
}

// NewContextManager 创建新的上下文管理器
func NewContextManager() *ContextManager {
	return &ContextManager{
		runners: make(map[string]*adk.Runner),
	}
}

// RegisterRunner 注册 Runner
func (cm *ContextManager) RegisterRunner(name string, runner *adk.Runner) {
	cm.runners[name] = runner
}

// GetRunner 获取 Runner
func (cm *ContextManager) GetRunner(name string) (*adk.Runner, error) {
	runner, exists := cm.runners[name]
	if !exists {
		return nil, fmt.Errorf("Runner %s 不存在", name)
	}
	return runner, nil
}

// ExecuteQuery 执行查询
func (cm *ContextManager) ExecuteQuery(ctx context.Context, runnerName, query string) (chan *adk.AgentEvent, error) {
	r, err := cm.GetRunner(runnerName)
	if err != nil {
		return nil, err
	}

	iter := r.Query(ctx, query)
	eventCh := make(chan *adk.AgentEvent)

	go func() {
		defer close(eventCh)
		for {
			event, ok := iter.Next()
			if !ok {
				break
			}
			eventCh <- event
		}
	}()

	return eventCh, nil
}

// ExecuteTool 执行工具
func (cm *ContextManager) ExecuteTool(ctx context.Context, runnerName, toolName, input string) (string, error) {
	_, err := cm.GetRunner(runnerName)
	if err != nil {
		return "", err
	}
	// 这里可以通过 Agent 执行工具；暂时返回模拟结果
	return fmt.Sprintf("工具 %s 执行结果: %s", toolName, input), nil
}
