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

package app

import (
	"fmt"
	"strings"

	"rag-platform/internal/model/embedding"
	"rag-platform/internal/model/llm"
	"rag-platform/pkg/config"
)

// NewLLMClientFromConfig 根据 config.Model 的 defaults.llm 创建 LLM 客户端（如 "openai.gpt_35_turbo"）
func NewLLMClientFromConfig(cfg *config.Config) (llm.Client, error) {
	if cfg == nil || cfg.Model.Defaults.LLM == "" {
		return nil, nil
	}
	provider, modelKey, err := parseDefaultKey(cfg.Model.Defaults.LLM)
	if err != nil {
		return nil, err
	}
	pc, ok := cfg.Model.LLM.Providers[provider]
	if !ok {
		return nil, fmt.Errorf("LLM provider %q not configured", provider)
	}
	mi, ok := pc.Models[modelKey]
	if !ok {
		return nil, fmt.Errorf("LLM model %q not configured in provider %q", modelKey, provider)
	}
	apiKey := pc.APIKey
	if apiKey == "" {
		return nil, fmt.Errorf("LLM provider %q api_key not configured", provider)
	}
	baseURL := pc.BaseURL
	return llm.NewClient(provider, mi.Name, apiKey, baseURL)
}

// NewQueryEmbedderFromConfig 根据 config.Model 的 defaults.embedding 创建用于 query 向量化的 Embedder
func NewQueryEmbedderFromConfig(cfg *config.Config) (*embedding.Embedder, error) {
	if cfg == nil || cfg.Model.Defaults.Embedding == "" {
		return nil, nil
	}
	provider, modelKey, err := parseDefaultKey(cfg.Model.Defaults.Embedding)
	if err != nil {
		return nil, err
	}
	pc, ok := cfg.Model.Embedding.Providers[provider]
	if !ok {
		return nil, fmt.Errorf("Embedding provider %q not configured", provider)
	}
	mi, ok := pc.Models[modelKey]
	if !ok {
		return nil, fmt.Errorf("Embedding model %q not configured in provider %q", modelKey, provider)
	}
	dimension := mi.Dimension
	if dimension <= 0 {
		dimension = 1536
	}
	return embedding.NewEmbedder(mi.Name, dimension), nil
}

func parseDefaultKey(key string) (provider, modelKey string, err error) {
	parts := strings.SplitN(key, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("default key 格式应为 provider.model_key，如 openai.gpt_35_turbo，当前: %q", key)
	}
	return parts[0], parts[1], nil
}
