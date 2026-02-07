package builtin

import (
	"rag-platform/internal/runtime/eino"
	"rag-platform/internal/tool"
	"rag-platform/internal/tool/registry"
)

// RegisterBuiltin 将内置工具注册到 ToolRegistry（需传入已装配的 engine 与 generator）
func RegisterBuiltin(reg *registry.Registry, engine *eino.Engine, generator eino.Generator) {
	if reg == nil {
		return
	}
	if engine != nil {
		reg.Register(NewRAGSearchTool(engine))
		reg.Register(NewIngestTool(engine))
		reg.Register(NewWorkflowTool(engine))
	}
	if generator != nil {
		reg.Register(NewLLMGenerateTool(generator))
	}
	reg.Register(NewHTTPTool())
}

// RegisterBuiltinWithTools 仅注册不依赖 engine 的通用工具（用于测试或最小装配）
func RegisterBuiltinWithTools(reg *registry.Registry, tools ...tool.Tool) {
	if reg == nil {
		return
	}
	for _, t := range tools {
		reg.Register(t)
	}
}
