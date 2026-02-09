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

package eino

import (
	"context"

	"rag-platform/internal/pipeline/common"
)

// Node 通用 Node 封装：将 Pipeline 阶段适配为可挂到 eino Graph 的节点（设计：Pipeline 是节点集合，顺序由 eino 决定）
type Node struct {
	name  string
	stage common.PipelineStage
}

// NewNode 从 PipelineStage 创建 Node
func NewNode(name string, stage common.PipelineStage) *Node {
	return &Node{name: name, stage: stage}
}

// Name 返回节点名称
func (n *Node) Name() string {
	return n.name
}

// Stage 返回封装的 Pipeline 阶段
func (n *Node) Stage() common.PipelineStage {
	return n.stage
}

// Run 执行节点：在给定 Pipeline 上下文中执行阶段，便于在 Workflow 中调用
func (n *Node) Run(ctx context.Context, pipeCtx *common.PipelineContext, input interface{}) (interface{}, error) {
	if n.stage == nil {
		return input, nil
	}
	return n.stage.Execute(pipeCtx, input)
}
