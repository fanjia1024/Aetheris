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

package model

import (
	"fmt"
	"sync"

	"rag-platform/internal/model/embedding"
	"rag-platform/internal/model/llm"
	"rag-platform/internal/model/vision"
)

// Registry 模型注册表，支持按名称/类型解析 LLM、Embedding、Vision，便于运行时切换
var (
	llmRegistry       = make(map[string]llm.Client)
	embeddingRegistry = make(map[string]*embedding.Embedder)
	visionRegistry    = make(map[string]vision.Client)
	registryMu        sync.RWMutex
)

// RegisterLLM 注册 LLM 实现
func RegisterLLM(name string, c llm.Client) {
	registryMu.Lock()
	defer registryMu.Unlock()
	llmRegistry[name] = c
}

// GetLLM 按名称获取 LLM
func GetLLM(name string) (llm.Client, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	c, ok := llmRegistry[name]
	if !ok {
		return nil, fmt.Errorf("LLM not registered: %s", name)
	}
	return c, nil
}

// RegisterEmbedding 注册 Embedding 实现
func RegisterEmbedding(name string, e *embedding.Embedder) {
	registryMu.Lock()
	defer registryMu.Unlock()
	embeddingRegistry[name] = e
}

// GetEmbedding 按名称获取 Embedding
func GetEmbedding(name string) (*embedding.Embedder, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	e, ok := embeddingRegistry[name]
	if !ok {
		return nil, fmt.Errorf("Embedding not registered: %s", name)
	}
	return e, nil
}

// RegisterVision 注册 Vision 实现
func RegisterVision(name string, c vision.Client) {
	registryMu.Lock()
	defer registryMu.Unlock()
	visionRegistry[name] = c
}

// GetVision 按名称获取 Vision
func GetVision(name string) (vision.Client, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	c, ok := visionRegistry[name]
	if !ok {
		return nil, fmt.Errorf("Vision not registered: %s", name)
	}
	return c, nil
}
