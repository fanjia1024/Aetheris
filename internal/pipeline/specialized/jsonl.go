package specialized

import (
	"rag-platform/internal/pipeline/common"
)

// JSONLPipeline 处理 JSONL 格式的专用 Pipeline（占位，便于后续与 eino 挂接）
type JSONLPipeline struct {
	name string
}

// NewJSONLPipeline 创建 JSONL Pipeline
func NewJSONLPipeline() *JSONLPipeline {
	return &JSONLPipeline{name: "jsonl"}
}

// Name 实现 Pipeline
func (p *JSONLPipeline) Name() string {
	return p.name
}

// Stages 实现 Pipeline
func (p *JSONLPipeline) Stages() []common.PipelineStage {
	return nil
}

// Execute 实现 Pipeline
func (p *JSONLPipeline) Execute(ctx *common.PipelineContext, input interface{}) (interface{}, error) {
	return p.ProcessSpecialized(input)
}

// AddStage 实现 Pipeline
func (p *JSONLPipeline) AddStage(stage common.PipelineStage) error {
	return nil
}

// RemoveStage 实现 Pipeline
func (p *JSONLPipeline) RemoveStage(name string) error {
	return nil
}

// ProcessSpecialized 实现 SpecializedPipeline
func (p *JSONLPipeline) ProcessSpecialized(input interface{}) (interface{}, error) {
	// 占位：后续实现 JSONL 解析与入库
	return input, nil
}
