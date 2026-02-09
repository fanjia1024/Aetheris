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
