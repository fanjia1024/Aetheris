package api

import (
	"context"

	"rag-platform/internal/agent/memory"
	"rag-platform/internal/agent/planner"
	"rag-platform/internal/agent/runtime"
	"rag-platform/internal/agent/tools"
)

// planGoaler 仅需 PlanGoal，供 Agent 执行 DAG 使用；*planner.LLMPlanner 与 *planner.RulePlanner 均实现此接口
type planGoaler interface {
	PlanGoal(ctx context.Context, goal string, mem memory.Memory) (*planner.TaskGraph, error)
}

// memoryProviderAdapter 将 memory.Memory 适配为 runtime.MemoryProvider
type memoryProviderAdapter struct {
	m memory.Memory
}

func (a *memoryProviderAdapter) Recall(ctx interface{}, query string) (interface{}, error) {
	c, ok := ctx.(context.Context)
	if !ok {
		c = context.Background()
	}
	return a.m.Recall(c, query)
}

func (a *memoryProviderAdapter) Store(ctx interface{}, item interface{}) error {
	c, ok := ctx.(context.Context)
	if !ok {
		c = context.Background()
	}
	mi, ok := item.(memory.MemoryItem)
	if !ok {
		return nil
	}
	return a.m.Store(c, mi)
}

// plannerProviderAdapter 将 planGoaler 适配为 runtime.PlannerProvider
type plannerProviderAdapter struct {
	p planGoaler
}

func (a *plannerProviderAdapter) Plan(ctx interface{}, goal string, mem interface{}) (interface{}, error) {
	c, ok := ctx.(context.Context)
	if !ok {
		c = context.Background()
	}
	memImpl, _ := mem.(memory.Memory)
	if memImpl == nil {
		return a.p.PlanGoal(c, goal, memory.NewCompositeMemory())
	}
	return a.p.PlanGoal(c, goal, memImpl)
}

// toolsProviderAdapter 将 *tools.Registry 适配为 runtime.ToolsProvider
type toolsProviderAdapter struct {
	r *tools.Registry
}

func (a *toolsProviderAdapter) Get(name string) (interface{}, bool) {
	return a.r.Get(name)
}

func (a *toolsProviderAdapter) List() []interface{} {
	list := a.r.List()
	out := make([]interface{}, len(list))
	for i := range list {
		out[i] = list[i]
	}
	return out
}

// agentCreatorImpl 实现 http.AgentCreator，用于 POST /api/agents
type agentCreatorImpl struct {
	manager *runtime.Manager
	planner planGoaler
	tools   *tools.Registry
}

// NewAgentCreator 创建 v1 Agent 的工厂（由 app 注入 Manager、Planner、Tools；planner 可为 *planner.LLMPlanner 或 *planner.RulePlanner）
func NewAgentCreator(manager *runtime.Manager, plannerAgent planGoaler, toolsReg *tools.Registry) *agentCreatorImpl {
	return &agentCreatorImpl{manager: manager, planner: plannerAgent, tools: toolsReg}
}

// Create 实现 http.AgentCreator
func (c *agentCreatorImpl) Create(ctx context.Context, name string) (*runtime.Agent, error) {
	sess := runtime.NewSession("", "")
	working := memory.NewWorkingSession(sess)
	episodic := memory.NewEpisodic(1000)
	composite := memory.NewCompositeMemory(working, episodic)
	memProvider := &memoryProviderAdapter{m: composite}
	plannerProvider := &plannerProviderAdapter{p: c.planner}
	toolsProvider := &toolsProviderAdapter{r: c.tools}
	return c.manager.Create(ctx, name, sess, memProvider, plannerProvider, toolsProvider)
}
