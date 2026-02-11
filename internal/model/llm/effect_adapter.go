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

package llm

import (
	"context"
	"fmt"
	"time"

	"rag-platform/pkg/effects"
)

// EffectAdapter wraps an LLM Client with the effects system.
// It ensures all LLM calls go through the Effect layer for replayability.
type EffectAdapter struct {
	client   Client
	effects  effects.System
	recorder effects.EventRecorder
}

// NewEffectAdapter creates a new Effect-wrapped LLM client.
func NewEffectAdapter(client Client, sys effects.System) *EffectAdapter {
	return &EffectAdapter{
		client:  client,
		effects: sys,
	}
}

// WithRecorder sets the event recorder for this adapter.
func (a *EffectAdapter) WithRecorder(recorder effects.EventRecorder) *EffectAdapter {
	a.recorder = recorder
	return a
}

// Generate wraps Client.Generate with effects.
func (a *EffectAdapter) Generate(prompt string, options GenerateOptions) (string, error) {
	return a.GenerateWithContext(context.Background(), prompt, options)
}

// GenerateWithContext wraps Client.GenerateWithContext with effects.
func (a *EffectAdapter) GenerateWithContext(ctx context.Context, prompt string, options GenerateOptions) (string, error) {
	req := effects.LLMRequest{
		Model: a.client.Model(),
		Messages: []effects.LLMMessage{
			{Role: "user", Content: prompt},
		},
		Params: effects.LLMParams{
			Temperature:      &options.Temperature,
			MaxTokens:        &options.MaxTokens,
			TopP:             &options.TopP,
			FrequencyPenalty: &options.FrequencyPenalty,
			PresencePenalty:  &options.PresencePenalty,
			StopSequences:    options.Stop,
		},
	}

	fmt.Printf("DEBUG: EffectAdapter calling ExecuteLLM with client=%p\n", a.client)
	response, err := effects.ExecuteLLM(ctx, a.effects, req, func(ctx context.Context, r effects.LLMRequest) (effects.LLMResponse, error) {
		fmt.Printf("DEBUG: EffectAdapter caller called, client=%p\n", a.client)
		// Convert effect request to legacy options
		legacyOpts := GenerateOptions{
			Temperature:      getFloat64(r.Params.Temperature),
			MaxTokens:        getInt(r.Params.MaxTokens),
			TopP:             getFloat64(r.Params.TopP),
			FrequencyPenalty: getFloat64(r.Params.FrequencyPenalty),
			PresencePenalty:  getFloat64(r.Params.PresencePenalty),
			Stop:             r.Params.StopSequences,
		}

		// Convert messages
		messages := make([]Message, len(r.Messages))
		for i, m := range r.Messages {
			messages[i] = Message{Role: m.Role, Content: m.Content}
		}

		var content string
		var err error

		fmt.Printf("DEBUG: messages len=%d, r.Messages len=%d\n", len(messages), len(r.Messages))

		if len(messages) == 0 {
			// Use Generate for single prompt
			fmt.Printf("DEBUG: EffectAdapter calling client.GenerateWithContext, this=%p\n", a.client)
			content, err = a.client.GenerateWithContext(ctx, r.Messages[0].Content, legacyOpts)
			fmt.Printf("DEBUG: EffectAdapter client.GenerateWithContext returned, content=%q\n", content)
		} else {
			// Use Chat for multi-message
			fmt.Printf("DEBUG: EffectAdapter calling client.ChatWithContext\n")
			content, err = a.client.ChatWithContext(ctx, messages, legacyOpts)
		}

		fmt.Printf("DEBUG: EffectAdapter caller done, content=%q, err=%v\n", content, err)
		if err != nil {
			return effects.LLMResponse{}, err
		}

		return effects.LLMResponse{
			Content:    content,
			StopReason: "stop",
			Model:      r.Model,
		}, nil
	})

	fmt.Printf("DEBUG: EffectAdapter ExecuteLLM returned, response=%q, err=%v\n", response.Content, err)
	if err != nil {
		return "", err
	}

	return response.Content, nil
}

// Chat wraps Client.Chat with effects.
func (a *EffectAdapter) Chat(messages []Message, options GenerateOptions) (string, error) {
	return a.ChatWithContext(context.Background(), messages, options)
}

// ChatWithContext wraps Client.ChatWithContext with effects.
func (a *EffectAdapter) ChatWithContext(ctx context.Context, messages []Message, options GenerateOptions) (string, error) {
	// Convert to effect messages
	effectMsgs := make([]effects.LLMMessage, len(messages))
	for i, m := range messages {
		effectMsgs[i] = effects.LLMMessage{Role: m.Role, Content: m.Content}
	}

	req := effects.LLMRequest{
		Model:    a.client.Model(),
		Messages: effectMsgs,
		Params: effects.LLMParams{
			Temperature:      &options.Temperature,
			MaxTokens:        &options.MaxTokens,
			TopP:             &options.TopP,
			FrequencyPenalty: &options.FrequencyPenalty,
			PresencePenalty:  &options.PresencePenalty,
			StopSequences:    options.Stop,
		},
	}

	response, err := effects.ExecuteLLM(ctx, a.effects, req, func(ctx context.Context, r effects.LLMRequest) (effects.LLMResponse, error) {
		legacyOpts := GenerateOptions{
			Temperature:      getFloat64(r.Params.Temperature),
			MaxTokens:        getInt(r.Params.MaxTokens),
			TopP:             getFloat64(r.Params.TopP),
			FrequencyPenalty: getFloat64(r.Params.FrequencyPenalty),
			PresencePenalty:  getFloat64(r.Params.PresencePenalty),
			Stop:             r.Params.StopSequences,
		}

		legacyMsgs := make([]Message, len(r.Messages))
		for i, m := range r.Messages {
			legacyMsgs[i] = Message{Role: m.Role, Content: m.Content}
		}

		content, err := a.client.ChatWithContext(ctx, legacyMsgs, legacyOpts)
		if err != nil {
			return effects.LLMResponse{}, err
		}

		return effects.LLMResponse{
			Content:    content,
			StopReason: "stop",
			Model:      r.Model,
		}, nil
	})

	if err != nil {
		return "", err
	}

	return response.Content, nil
}

// Model returns the underlying client's model name.
func (a *EffectAdapter) Model() string {
	return a.client.Model()
}

// Provider returns the underlying client's provider name.
func (a *EffectAdapter) Provider() string {
	return a.client.Provider()
}

// SetModel delegates to the underlying client.
func (a *EffectAdapter) SetModel(model string) {
	a.client.SetModel(model)
}

// SetAPIKey delegates to the underlying client.
func (a *EffectAdapter) SetAPIKey(apiKey string) {
	a.client.SetAPIKey(apiKey)
}

// Unwrap returns the underlying client.
func (a *EffectAdapter) Unwrap() Client {
	return a.client
}

// Helper functions for pointer conversions
func getFloat64(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

func getInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// EffectClient is a Client that uses effects.ExecuteLLM internally.
// This is a convenience type that wraps any LLM caller function.
type EffectClient struct {
	effects   effects.System
	generator effects.LLMCaller
	model     string
	provider  string
}

// NewEffectClient creates an LLM client that always uses the effects system.
func NewEffectClient(generator effects.LLMCaller, model, provider string, sys effects.System) *EffectClient {
	return &EffectClient{
		effects:   sys,
		generator: generator,
		model:     model,
		provider:  provider,
	}
}

// Generate implements Client.Generate using effects.
func (c *EffectClient) Generate(prompt string, options GenerateOptions) (string, error) {
	return c.GenerateWithContext(context.Background(), prompt, options)
}

// GenerateWithContext implements Client.GenerateWithContext using effects.
func (c *EffectClient) GenerateWithContext(ctx context.Context, prompt string, options GenerateOptions) (string, error) {
	req := effects.LLMRequest{
		Model: c.model,
		Messages: []effects.LLMMessage{
			{Role: "user", Content: prompt},
		},
		Params: effects.LLMParams{
			Temperature:      &options.Temperature,
			MaxTokens:        &options.MaxTokens,
			TopP:             &options.TopP,
			FrequencyPenalty: &options.FrequencyPenalty,
			PresencePenalty:  &options.PresencePenalty,
			StopSequences:    options.Stop,
		},
	}

	resp, err := effects.ExecuteLLM(ctx, c.effects, req, c.generator)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// Chat implements Client.Chat using effects.
func (c *EffectClient) Chat(messages []Message, options GenerateOptions) (string, error) {
	return c.ChatWithContext(context.Background(), messages, options)
}

// ChatWithContext implements Client.ChatWithContext using effects.
func (c *EffectClient) ChatWithContext(ctx context.Context, messages []Message, options GenerateOptions) (string, error) {
	effectMsgs := make([]effects.LLMMessage, len(messages))
	for i, m := range messages {
		effectMsgs[i] = effects.LLMMessage{Role: m.Role, Content: m.Content}
	}

	req := effects.LLMRequest{
		Model:    c.model,
		Messages: effectMsgs,
		Params: effects.LLMParams{
			Temperature:      &options.Temperature,
			MaxTokens:        &options.MaxTokens,
			TopP:             &options.TopP,
			FrequencyPenalty: &options.FrequencyPenalty,
			PresencePenalty:  &options.PresencePenalty,
			StopSequences:    options.Stop,
		},
	}

	resp, err := effects.ExecuteLLM(ctx, c.effects, req, c.generator)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// Model returns the model name.
func (c *EffectClient) Model() string {
	return c.model
}

// Provider returns the provider name.
func (c *EffectClient) Provider() string {
	return c.provider
}

// SetModel sets the model name.
func (c *EffectClient) SetModel(model string) {
	c.model = model
}

// SetAPIKey is a no-op for EffectClient (key should be in the generator closure).
func (c *EffectClient) SetAPIKey(apiKey string) {
	// No-op: API key should be handled by the generator closure
}

// EffectGenerateOptions converts GenerateOptions to LLMParams.
func EffectGenerateOptions(opts GenerateOptions) effects.LLMParams {
	return effects.LLMParams{
		Temperature:      &opts.Temperature,
		MaxTokens:        &opts.MaxTokens,
		TopP:             &opts.TopP,
		FrequencyPenalty: &opts.FrequencyPenalty,
		PresencePenalty:  &opts.PresencePenalty,
		StopSequences:    opts.Stop,
	}
}

// EffectMessages converts Messages to LLMMessage.
func EffectMessages(messages []Message) []effects.LLMMessage {
	result := make([]effects.LLMMessage, len(messages))
	for i, m := range messages {
		result[i] = effects.LLMMessage{Role: m.Role, Content: m.Content}
	}
	return result
}

// EffectDuration converts time.Duration to milliseconds for recording.
func EffectDuration(d time.Duration) int64 {
	return d.Milliseconds()
}
