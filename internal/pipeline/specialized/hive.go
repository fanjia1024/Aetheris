package specialized

import (
	"rag-platform/internal/pipeline/common"
)

// HIVEPipeline 处理 HIVE 数据源的专用 Pipeline（占位，便于后续与 eino 挂接）
type HIVEPipeline struct {
	name string
}

// NewHIVEPipeline 创建 HIVE Pipeline
func NewHIVEPipeline() *HIVEPipeline {
	return &HIVEPipeline{name: "hive"}
}

// Name 实现 Pipeline
func (p *HIVEPipeline) Name() string {
	return p.name
}

// Stages 实现 Pipeline
func (p *HIVEPipeline) Stages() []common.PipelineStage {
	return nil
}

// Execute 实现 Pipeline
func (p *HIVEPipeline) Execute(ctx *common.PipelineContext, input interface{}) (interface{}, error) {
	return p.ProcessSpecialized(input)
}

// AddStage 实现 Pipeline
func (p *HIVEPipeline) AddStage(stage common.PipelineStage) error {
	return nil
}

// RemoveStage 实现 Pipeline
func (p *HIVEPipeline) RemoveStage(name string) error {
	return nil
}

// ProcessSpecialized 实现 SpecializedPipeline
func (p *HIVEPipeline) ProcessSpecialized(input interface{}) (interface{}, error) {
	// 占位：后续实现 HIVE 查询与同步
	return input, nil
}
