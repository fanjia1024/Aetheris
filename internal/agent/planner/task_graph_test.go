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
	"testing"
)

func TestTaskGraph_Marshal_Unmarshal(t *testing.T) {
	g := &TaskGraph{
		Nodes: []TaskNode{
			{ID: "n1", Type: NodeLLM, Config: map[string]any{"goal": "g1"}},
			{ID: "n2", Type: NodeTool, ToolName: "search"},
		},
		Edges: []TaskEdge{{From: "n1", To: "n2"}},
	}
	data, err := g.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("Marshal returned empty")
	}
	var out TaskGraph
	if err := out.Unmarshal(data); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(out.Nodes) != 2 || len(out.Edges) != 1 {
		t.Errorf("Unmarshal: nodes=%d edges=%d", len(out.Nodes), len(out.Edges))
	}
	if out.Nodes[0].ID != "n1" || out.Nodes[0].Type != NodeLLM {
		t.Errorf("node0: %+v", out.Nodes[0])
	}
	if out.Edges[0].From != "n1" || out.Edges[0].To != "n2" {
		t.Errorf("edge: %+v", out.Edges[0])
	}
}

func TestTaskGraph_Unmarshal_Empty(t *testing.T) {
	var g TaskGraph
	if err := g.Unmarshal([]byte("{}")); err != nil {
		t.Fatalf("Unmarshal empty: %v", err)
	}
	if g.Nodes != nil || g.Edges != nil {
		t.Errorf("expected nil nodes/edges, got %+v", g)
	}
}
