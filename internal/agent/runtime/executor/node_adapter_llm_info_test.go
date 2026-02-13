package executor

import (
	"context"
	"testing"
)

type fakeLLMGen struct{}

func (f *fakeLLMGen) Generate(ctx context.Context, prompt string) (string, error) {
	return "ok", nil
}

type fakeLLMGenWithMeta struct{}

func (f *fakeLLMGenWithMeta) Generate(ctx context.Context, prompt string) (string, error) {
	return "ok", nil
}

func (f *fakeLLMGenWithMeta) ModelInfo(ctx context.Context) LLMModelInfo {
	return LLMModelInfo{Model: "gpt-test", Provider: "openai", Temperature: 0.2}
}

func TestResolveLLMModelInfo_Default(t *testing.T) {
	info := resolveLLMModelInfo(context.Background(), &fakeLLMGen{})
	if info.Model == "" || info.Provider == "" {
		t.Fatalf("expected default model/provider, got %+v", info)
	}
}

func TestResolveLLMModelInfo_FromProvider(t *testing.T) {
	info := resolveLLMModelInfo(context.Background(), &fakeLLMGenWithMeta{})
	if info.Model != "gpt-test" || info.Provider != "openai" || info.Temperature != 0.2 {
		t.Fatalf("unexpected llm info: %+v", info)
	}
}
