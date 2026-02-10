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

package planner

import (
	"context"
	"strings"
	"testing"

	"rag-platform/internal/agent/memory"
	"rag-platform/internal/model/llm"
)

// mockLLMClient 记录 ChatWithContext 收到的第一条 system 消息，并返回预定义 TaskGraph JSON
type mockLLMClient struct {
	lastSystemPrompt string
	reply            string
}

func (m *mockLLMClient) ChatWithContext(ctx context.Context, messages []llm.Message, opts llm.GenerateOptions) (string, error) {
	for _, msg := range messages {
		if msg.Role == "system" {
			m.lastSystemPrompt = msg.Content
			break
		}
	}
	if m.reply != "" {
		return m.reply, nil
	}
	return `{"nodes":[{"id":"n1","type":"tool","tool_name":"knowledge.search"},{"id":"n2","type":"llm","config":{"goal":"summarize"}}],"edges":[{"from":"n1","to":"n2"}]}`, nil
}

func (m *mockLLMClient) Generate(prompt string, opts llm.GenerateOptions) (string, error) {
	return "", nil
}
func (m *mockLLMClient) GenerateWithContext(ctx context.Context, prompt string, opts llm.GenerateOptions) (string, error) {
	return "", nil
}
func (m *mockLLMClient) Chat(messages []llm.Message, opts llm.GenerateOptions) (string, error) {
	return "", nil
}
func (m *mockLLMClient) Model() string           { return "mock" }
func (m *mockLLMClient) Provider() string        { return "mock" }
func (m *mockLLMClient) SetModel(model string)   {}
func (m *mockLLMClient) SetAPIKey(apiKey string) {}

func TestLLMPlanner_PlanGoal_NilClient(t *testing.T) {
	p := NewLLMPlanner(nil)
	ctx := context.Background()
	mem := memory.NewCompositeMemory()
	g, err := p.PlanGoal(ctx, "my goal", mem)
	if err != nil {
		t.Fatalf("PlanGoal: %v", err)
	}
	if g == nil || len(g.Nodes) != 1 {
		t.Fatalf("expected 1 node (fallback), got %+v", g)
	}
	if g.Nodes[0].Type != NodeLLM {
		t.Errorf("fallback node type: %s", g.Nodes[0].Type)
	}
}

func TestLLMPlanner_PlanGoal_WithToolsSchema_InjectsToolListInPrompt(t *testing.T) {
	mock := &mockLLMClient{}
	p := NewLLMPlanner(mock)
	p.SetToolsSchemaForGoal([]byte(`[{"name":"knowledge.search","description":"在知识库中检索与问题相关的文档片段"},{"name":"llm_generate","description":"LLM 生成"}]`))
	ctx := context.Background()
	mem := memory.NewCompositeMemory()

	g, err := p.PlanGoal(ctx, "总结文档要点", mem)
	if err != nil {
		t.Fatalf("PlanGoal: %v", err)
	}
	if g == nil {
		t.Fatal("expected non-nil graph")
	}

	if mock.lastSystemPrompt == "" {
		t.Fatal("expected system prompt to be set by PlanGoal")
	}
	if !strings.Contains(mock.lastSystemPrompt, "knowledge.search") {
		t.Errorf("expected system prompt to contain tool name knowledge.search; got: %s", mock.lastSystemPrompt[:min(200, len(mock.lastSystemPrompt))])
	}
	if !strings.Contains(mock.lastSystemPrompt, "可用工具") {
		t.Errorf("expected system prompt to contain 可用工具; got: %s", mock.lastSystemPrompt[:min(200, len(mock.lastSystemPrompt))])
	}

	if len(g.Nodes) >= 1 && g.Nodes[0].ToolName == "knowledge.search" {
		t.Logf("PlanGoal returned graph with tool node: %+v", g.Nodes)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
