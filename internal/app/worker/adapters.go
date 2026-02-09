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
