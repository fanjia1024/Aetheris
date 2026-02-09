package planner

import (
	"context"
	"testing"

	"rag-platform/internal/agent/memory"
)

func TestRulePlanner_PlanGoal_DefaultSingleNode(t *testing.T) {
	p := NewRulePlanner()
	ctx := context.Background()
	mem := memory.NewCompositeMemory()
	g, err := p.PlanGoal(ctx, "my goal", mem)
	if err != nil {
		t.Fatalf("PlanGoal: %v", err)
	}
	if g == nil || len(g.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %+v", g)
	}
	if g.Nodes[0].ID != "n1" || g.Nodes[0].Type != NodeLLM {
		t.Errorf("node: %+v", g.Nodes[0])
	}
	if g.Nodes[0].Config["goal"] != "my goal" {
		t.Errorf("config goal: %v", g.Nodes[0].Config["goal"])
	}
	if len(g.Edges) != 0 {
		t.Errorf("expected no edges, got %d", len(g.Edges))
	}
}

func TestRulePlanner_PlanGoal_WithDefaultGraph(t *testing.T) {
	defaultGraph := &TaskGraph{
		Nodes: []TaskNode{
			{ID: "a", Type: NodeLLM, Config: map[string]any{"key": "v"}},
		},
		Edges: []TaskEdge{},
	}
	p := &RulePlanner{DefaultGraph: defaultGraph}
	ctx := context.Background()
	mem := memory.NewCompositeMemory()
	g, err := p.PlanGoal(ctx, "goal2", mem)
	if err != nil {
		t.Fatalf("PlanGoal: %v", err)
	}
	if len(g.Nodes) != 1 || g.Nodes[0].ID != "a" {
		t.Errorf("nodes: %+v", g.Nodes)
	}
	if g.Nodes[0].Config["goal"] != "goal2" {
		t.Errorf("goal not set: %v", g.Nodes[0].Config["goal"])
	}
	if g.Nodes[0].Config["key"] != "v" {
		t.Errorf("existing config overwritten: %v", g.Nodes[0].Config["key"])
	}
}

func TestRulePlanner_Plan(t *testing.T) {
	p := NewRulePlanner()
	ctx := context.Background()
	res, err := p.Plan(ctx, "q", nil, nil)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if res == nil || res.Next != "finish" {
		t.Errorf("Plan: %+v", res)
	}
}
