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

package executor

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/compose"

	"rag-platform/internal/agent/planner"
	"rag-platform/internal/agent/runtime"
)

type customNodeAdapterForTest struct{}

func (customNodeAdapterForTest) ToDAGNode(task *planner.TaskNode, _ *runtime.Agent) (*compose.Lambda, error) {
	return compose.InvokableLambda[*AgentDAGPayload, *AgentDAGPayload](func(ctx context.Context, p *AgentDAGPayload) (*AgentDAGPayload, error) {
		if p == nil {
			p = &AgentDAGPayload{}
		}
		return p, nil
	}), nil
}

func (customNodeAdapterForTest) ToNodeRunner(task *planner.TaskNode, _ *runtime.Agent) (NodeRunner, error) {
	return func(ctx context.Context, p *AgentDAGPayload) (*AgentDAGPayload, error) {
		if p == nil {
			p = &AgentDAGPayload{}
		}
		return p, nil
	}, nil
}

func TestCompiler_RegisterAndDiscoverCustomNode(t *testing.T) {
	c := NewCompiler(nil)
	c.Register("z_custom", customNodeAdapterForTest{})
	c.Register("a_custom", customNodeAdapterForTest{})

	types := c.RegisteredNodeTypes()
	if len(types) != 2 {
		t.Fatalf("registered types len = %d, want 2", len(types))
	}
	if types[0] != "a_custom" || types[1] != "z_custom" {
		t.Fatalf("registered types order = %v, want [a_custom z_custom]", types)
	}
}

func TestCompiler_CompileWithCustomNode(t *testing.T) {
	c := NewCompiler(map[string]NodeAdapter{"custom": customNodeAdapterForTest{}})
	g := &planner.TaskGraph{
		Nodes: []planner.TaskNode{{ID: "n1", Type: "custom"}},
		Edges: nil,
	}
	compiled, err := c.Compile(context.Background(), g, &runtime.Agent{ID: "a1"})
	if err != nil {
		t.Fatalf("Compile with custom node failed: %v", err)
	}
	if compiled == nil {
		t.Fatal("compiled graph is nil")
	}
}
