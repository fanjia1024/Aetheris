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
		return nil, fmt.Errorf("LLM provider %q 未配置", provider)
	}
	mi, ok := pc.Models[modelKey]
	if !ok {
		return nil, fmt.Errorf("LLM model %q 未在 provider %q 中配置", modelKey, provider)
	}
	apiKey := pc.APIKey
	if apiKey == "" {
		return nil, fmt.Errorf("LLM provider %q 的 api_key 未配置", provider)
	}
	return llm.NewClient(provider, mi.Name, apiKey)
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
		return nil, fmt.Errorf("Embedding provider %q 未配置", provider)
	}
	mi, ok := pc.Models[modelKey]
	if !ok {
		return nil, fmt.Errorf("Embedding model %q 未在 provider %q 中配置", modelKey, provider)
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
