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
	"testing"

	"rag-platform/internal/agent/planner"
)

func TestTopoOrder_NilOrEmpty(t *testing.T) {
	order, err := TopoOrder(nil)
	if err != nil || order != nil {
		t.Errorf("TopoOrder(nil): order=%v err=%v", order, err)
	}
	order, err = TopoOrder(&planner.TaskGraph{Nodes: []planner.TaskNode{}, Edges: nil})
	if err != nil || order != nil {
		t.Errorf("TopoOrder(empty): order=%v err=%v", order, err)
	}
}

func TestTopoOrder_SingleNode(t *testing.T) {
	g := &planner.TaskGraph{
		Nodes: []planner.TaskNode{{ID: "a", Type: "llm"}},
		Edges: nil,
	}
	order, err := TopoOrder(g)
	if err != nil {
		t.Fatalf("TopoOrder: %v", err)
	}
	if len(order) != 1 || order[0] != "a" {
		t.Errorf("expected [a], got %v", order)
	}
}

func TestTopoOrder_Linear(t *testing.T) {
	g := &planner.TaskGraph{
		Nodes: []planner.TaskNode{
			{ID: "a", Type: "llm"},
			{ID: "b", Type: "llm"},
			{ID: "c", Type: "llm"},
		},
		Edges: []planner.TaskEdge{
			{From: "a", To: "b"},
			{From: "b", To: "c"},
		},
	}
	order, err := TopoOrder(g)
	if err != nil {
		t.Fatalf("TopoOrder: %v", err)
	}
	if len(order) != 3 {
		t.Fatalf("expected 3 nodes, got %v", order)
	}
	if order[0] != "a" || order[1] != "b" || order[2] != "c" {
		t.Errorf("expected [a,b,c], got %v", order)
	}
}

func TestTopoOrder_Diamond(t *testing.T) {
	//   a
	//  / \
	// b   c
	//  \ /
	//   d
	g := &planner.TaskGraph{
		Nodes: []planner.TaskNode{
			{ID: "a", Type: "llm"},
			{ID: "b", Type: "llm"},
			{ID: "c", Type: "llm"},
			{ID: "d", Type: "llm"},
		},
		Edges: []planner.TaskEdge{
			{From: "a", To: "b"},
			{From: "a", To: "c"},
			{From: "b", To: "d"},
			{From: "c", To: "d"},
		},
	}
	order, err := TopoOrder(g)
	if err != nil {
		t.Fatalf("TopoOrder: %v", err)
	}
	if len(order) != 4 {
		t.Fatalf("expected 4 nodes, got %v", order)
	}
	pos := make(map[string]int)
	for i, id := range order {
		pos[id] = i
	}
	if pos["a"] != 0 {
		t.Errorf("a should be first, order=%v", order)
	}
	if pos["d"] != 3 {
		t.Errorf("d should be last, order=%v", order)
	}
	if pos["b"] >= pos["d"] || pos["c"] >= pos["d"] {
		t.Errorf("b and c must be before d, order=%v", order)
	}
}

func TestTopoOrder_Cycle(t *testing.T) {
	g := &planner.TaskGraph{
		Nodes: []planner.TaskNode{
			{ID: "a", Type: "llm"},
			{ID: "b", Type: "llm"},
			{ID: "c", Type: "llm"},
		},
		Edges: []planner.TaskEdge{
			{From: "a", To: "b"},
			{From: "b", To: "c"},
			{From: "c", To: "a"},
		},
	}
	order, err := TopoOrder(g)
	if err == nil {
		t.Errorf("expected error for cycle, got order=%v", order)
	}
	if order != nil {
		t.Errorf("expected nil order on cycle, got %v", order)
	}
}

func TestLevelGroups_Diamond(t *testing.T) {
	//   a (level 0)
	//  / \
	// b   c (level 1)
	//  \ /
	//   d (level 2)
	g := &planner.TaskGraph{
		Nodes: []planner.TaskNode{
			{ID: "a", Type: "llm"},
			{ID: "b", Type: "llm"},
			{ID: "c", Type: "llm"},
			{ID: "d", Type: "llm"},
		},
		Edges: []planner.TaskEdge{
			{From: "a", To: "b"},
			{From: "a", To: "c"},
			{From: "b", To: "d"},
			{From: "c", To: "d"},
		},
	}
	groups, err := LevelGroups(g)
	if err != nil {
		t.Fatalf("LevelGroups: %v", err)
	}
	if len(groups) != 3 {
		t.Fatalf("expected 3 levels, got %d: %v", len(groups), groups)
	}
	if len(groups[0]) != 1 || groups[0][0] != "a" {
		t.Errorf("level 0 want [a], got %v", groups[0])
	}
	if len(groups[1]) != 2 {
		t.Errorf("level 1 want 2 nodes (b,c), got %v", groups[1])
	}
	if len(groups[2]) != 1 || groups[2][0] != "d" {
		t.Errorf("level 2 want [d], got %v", groups[2])
	}
}
