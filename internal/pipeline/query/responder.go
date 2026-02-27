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

package query

import (
	"fmt"
	"time"

	"rag-platform/internal/pipeline/common"
)

// Responder 响应器
type Responder struct {
	name string
}

// NewResponder 创建新的响应器
func NewResponder() *Responder {
	return &Responder{
		name: "responder",
	}
}

// Name 返回组件名称
func (r *Responder) Name() string {
	return r.name
}

// Execute 执行响应操作
func (r *Responder) Execute(ctx *common.PipelineContext, input interface{}) (interface{}, error) {
	// 验证输入
	if err := r.Validate(input); err != nil {
		return nil, common.NewPipelineError(r.name, "input validation failed", err)
	}

	// 生成响应
	generationResult, ok := input.(*common.GenerationResult)
	if !ok {
		return nil, common.NewPipelineError(r.name, "input type error", fmt.Errorf("expected *common.GenerationResult, got %T", input))
	}

	// 处理响应
	response, err := r.buildResponse(generationResult)
	if err != nil {
		return nil, common.NewPipelineError(r.name, "build response failed", err)
	}

	return response, nil
}

// Validate 验证输入
func (r *Responder) Validate(input interface{}) error {
	if input == nil {
		return common.ErrInvalidInput
	}

	if _, ok := input.(*common.GenerationResult); !ok {
		return fmt.Errorf("unsupported input type输入类型: %T", input)
	}

	return nil
}

// ProcessQuery 处理查询
func (r *Responder) ProcessQuery(query *common.Query) (*common.Query, error) {
	// 这里可以添加查询后处理逻辑
	return query, nil
}

// Response 响应结构体
type Response struct {
	Answer      string                 `json:"answer"`
	References  []string               `json:"references"`
	Metadata    map[string]interface{} `json:"metadata"`
	ProcessTime time.Duration          `json:"process_time"`
	Timestamp   time.Time              `json:"timestamp"`
}

// buildResponse 构建响应
func (r *Responder) buildResponse(result *common.GenerationResult) (*Response, error) {
	if result == nil {
		return nil, fmt.Errorf("生成结果为空")
	}

	// 构建元数据
	metadata := map[string]interface{}{
		"reference_count": len(result.References),
		"generated_at":    time.Now(),
		"responder":       r.name,
	}

	// 创建响应
	response := &Response{
		Answer:      result.Answer,
		References:  result.References,
		Metadata:    metadata,
		ProcessTime: result.ProcessTime,
		Timestamp:   time.Now(),
	}

	return response, nil
}
