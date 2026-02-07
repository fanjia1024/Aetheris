package tools

import (
	"rag-platform/internal/runtime/eino"
	"rag-platform/internal/tool/builtin"
)

// RegisterBuiltin 将 tool/builtin 包装为 Session 感知工具并注册到 agent/tools.Registry
func RegisterBuiltin(reg *Registry, engine *eino.Engine, generator eino.Generator) {
	if reg == nil {
		return
	}
	if engine != nil {
		reg.Register(Wrap(builtin.NewRAGSearchTool(engine)))
		reg.Register(Wrap(builtin.NewIngestTool(engine)))
		reg.Register(Wrap(builtin.NewWorkflowTool(engine)))
	}
	if generator != nil {
		reg.Register(Wrap(builtin.NewLLMGenerateTool(generator)))
	}
	reg.Register(Wrap(builtin.NewHTTPTool()))
}
