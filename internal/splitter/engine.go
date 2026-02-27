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

package splitter

import (
	"fmt"

	"rag-platform/internal/pipeline/common"
)

// Engine 切片引擎
type Engine struct {
	name      string
	splitters map[string]Splitter
	embedder  TextEmbedder
}

// Splitter 切片器接口
type Splitter interface {
	Split(content string, options map[string]interface{}) ([]common.Chunk, error)
	Name() string
}

// NewEngine 创建新的切片引擎；embedder 可选，传入时 semantic 切片器将使用语义相似度断块
func NewEngine(embedder TextEmbedder) *Engine {
	engine := &Engine{
		name:      "splitter_engine",
		splitters: make(map[string]Splitter),
		embedder:  embedder,
	}

	// 注册内置切片器
	engine.registerSplitters()

	return engine
}

// Name 返回引擎名称
func (e *Engine) Name() string {
	return e.name
}

// registerSplitters 注册切片器
func (e *Engine) registerSplitters() {
	// 注册结构切片器
	e.splitters["structural"] = NewStructuralSplitter()

	// 注册语义切片器（注入 embedder 时启用真实语义相似度）
	e.splitters["semantic"] = NewSemanticSplitter(e.embedder)

	// 注册 Token 切片器
	e.splitters["token"] = NewTokenSplitter()
}

// AddSplitter 添加自定义切片器
func (e *Engine) AddSplitter(name string, splitter Splitter) {
	e.splitters[name] = splitter
}

// GetSplitter 获取切片器
func (e *Engine) GetSplitter(name string) (Splitter, error) {
	splitter, exists := e.splitters[name]
	if !exists {
		return nil, fmt.Errorf("splitter not found: %s", name)
	}
	return splitter, nil
}

// Split 执行切片
func (e *Engine) Split(content string, splitterName string, options map[string]interface{}) ([]common.Chunk, error) {
	// 获取切片器
	splitter, err := e.GetSplitter(splitterName)
	if err != nil {
		return nil, err
	}

	// 执行切片
	chunks, err := splitter.Split(content, options)
	if err != nil {
		return nil, fmt.Errorf("split failed: %w", err)
	}

	return chunks, nil
}

// SplitDocument 切片文档
func (e *Engine) SplitDocument(doc *common.Document, splitterName string, options map[string]interface{}) (*common.Document, error) {
	// 执行切片
	chunks, err := e.Split(doc.Content, splitterName, options)
	if err != nil {
		return nil, err
	}

	// 更新文档
	doc.Chunks = chunks
	doc.Metadata["chunked"] = true
	doc.Metadata["chunk_count"] = len(chunks)
	doc.Metadata["splitter"] = splitterName
	doc.Metadata["splitter_options"] = options

	return doc, nil
}

// GetSplitters 获取所有切片器
func (e *Engine) GetSplitters() []string {
	splitterNames := make([]string, 0, len(e.splitters))
	for name := range e.splitters {
		splitterNames = append(splitterNames, name)
	}
	return splitterNames
}
