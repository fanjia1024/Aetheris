package specialized

import (
	"rag-platform/internal/pipeline/common"
)

// LongTextPipeline 长文本专用 Pipeline（占位，便于后续与 eino 挂接）
type LongTextPipeline struct {
	name string
}

// NewLongTextPipeline 创建长文本 Pipeline
func NewLongTextPipeline() *LongTextPipeline {
	return &LongTextPipeline{name: "longtext"}
}

// Name 实现 Pipeline
func (p *LongTextPipeline) Name() string {
	return p.name
}

// Stages 实现 Pipeline
func (p *LongTextPipeline) Stages() []common.PipelineStage {
	return nil
}

// Execute 实现 Pipeline
func (p *LongTextPipeline) Execute(ctx *common.PipelineContext, input interface{}) (interface{}, error) {
	return p.ProcessSpecialized(input)
}

// AddStage 实现 Pipeline
func (p *LongTextPipeline) AddStage(stage common.PipelineStage) error {
	return nil
}

// RemoveStage 实现 Pipeline
func (p *LongTextPipeline) RemoveStage(name string) error {
	return nil
}

// ProcessSpecialized 实现 SpecializedPipeline
func (p *LongTextPipeline) ProcessSpecialized(input interface{}) (interface{}, error) {
	// 占位：后续实现长文本分段与摘要
	return input, nil
}
