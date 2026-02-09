package worker

import (
	"context"

	"rag-platform/internal/agent/memory"
	"rag-platform/internal/agent/planner"
	"rag-platform/internal/agent/runtime"
	"rag-platform/internal/agent/tools"
)

// planGoalProvider 与 api.planGoaler 一致：供 runtime.PlannerProvider 适配
type planGoalProvider interface {
	PlanGoal(ctx context.Context, goal string, mem memory.Memory) (*planner.TaskGraph, error)
}

type plannerProviderAdapter struct {
	p planGoalProvider
}

func newPlannerProviderAdapter(p planGoalProvider) runtime.PlannerProvider {
	return &plannerProviderAdapter{p: p}
}

func (a *plannerProviderAdapter) Plan(ctx interface{}, goal string, mem interface{}) (interface{}, error) {
	c, ok := ctx.(context.Context)
	if !ok {
		c = context.Background()
	}
	memImpl, _ := mem.(memory.Memory)
	if memImpl == nil {
		memImpl = memory.NewCompositeMemory()
	}
	return a.p.PlanGoal(c, goal, memImpl)
}

type toolsProviderAdapter struct {
	r *tools.Registry
}

func newToolsProviderAdapter(r *tools.Registry) runtime.ToolsProvider {
	return &toolsProviderAdapter{r: r}
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
