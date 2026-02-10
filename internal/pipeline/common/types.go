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

package common

import (
	"context"
	"time"
)

// PipelineContext Pipeline 执行上下文
type PipelineContext struct {
	Context   context.Context
	ID        string
	Metadata  map[string]interface{}
	StartTime time.Time
	EndTime   time.Time
	Status    string
	Error     error
}

// NewPipelineContext 创建新的 Pipeline 上下文
func NewPipelineContext(ctx context.Context, id string) *PipelineContext {
	return &PipelineContext{
		Context:   ctx,
		ID:        id,
		Metadata:  make(map[string]interface{}),
		StartTime: time.Now(),
		Status:    "running",
	}
}

// Document 文档结构体
type Document struct {
	ID        string                 `json:"id"`
	Content   string                 `json:"content"`
	Metadata  map[string]interface{} `json:"metadata"`
	Embedding []float64              `json:"embedding,omitempty"`
	Chunks    []Chunk                `json:"chunks,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}

// Chunk 文档切片
type Chunk struct {
	ID         string                 `json:"id"`
	Content    string                 `json:"content"`
	Metadata   map[string]interface{} `json:"metadata"`
	Embedding  []float64              `json:"embedding,omitempty"`
	DocumentID string                 `json:"document_id"`
	Index      int                    `json:"index"`
	TokenCount int                    `json:"token_count"`
}

// Query 查询结构体
type Query struct {
	ID        string                 `json:"id"`
	Text      string                 `json:"text"`
	Metadata  map[string]interface{} `json:"metadata"`
	Embedding []float64              `json:"embedding,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
}

// RetrievalResult 检索结果
type RetrievalResult struct {
	Chunks      []Chunk       `json:"chunks"`
	Scores      []float64     `json:"scores"`
	TotalCount  int           `json:"total_count"`
	ProcessTime time.Duration `json:"process_time"`
}

// GenerationResult 生成结果
type GenerationResult struct {
	Answer      string        `json:"answer"`
	References  []string      `json:"references"`
	ProcessTime time.Duration `json:"process_time"`
}

// PipelineStage Pipeline 阶段
type PipelineStage interface {
	Name() string
	Execute(ctx *PipelineContext, input interface{}) (interface{}, error)
	Validate(input interface{}) error
}

// IngestStage Ingest Pipeline 阶段
type IngestStage interface {
	PipelineStage
	ProcessDocument(doc *Document) (*Document, error)
}

// QueryStage Query Pipeline 阶段
type QueryStage interface {
	PipelineStage
	ProcessQuery(query *Query) (*Query, error)
}

// Pipeline Pipeline 接口
type Pipeline interface {
	Name() string
	Stages() []PipelineStage
	Execute(ctx *PipelineContext, input interface{}) (interface{}, error)
	AddStage(stage PipelineStage) error
	RemoveStage(name string) error
}

// IngestPipeline Ingest Pipeline 接口
type IngestPipeline interface {
	Pipeline
	ProcessDocument(doc *Document) (*Document, error)
}

// QueryPipeline Query Pipeline 接口
type QueryPipeline interface {
	Pipeline
	ProcessQuery(query *Query) (*GenerationResult, error)
}

// SpecializedPipeline 特殊 Pipeline 接口
type SpecializedPipeline interface {
	Pipeline
	ProcessSpecialized(input interface{}) (interface{}, error)
}
