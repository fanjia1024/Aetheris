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

package messaging

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// StoreMem 内存实现：同时实现 AgentMessagingBus 与 InboxReader
type StoreMem struct {
	mu       sync.RWMutex
	messages []*Message
	byID     map[string]*Message
}

// NewStoreMem 创建内存版消息存储
func NewStoreMem() *StoreMem {
	return &StoreMem{
		messages: nil,
		byID:     make(map[string]*Message),
	}
}

func copyPayload(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (s *StoreMem) Send(ctx context.Context, fromAgentID, toAgentID string, payload map[string]any, opts *SendOptions) (string, error) {
	now := time.Now()
	kind := KindUser
	if opts != nil && opts.Kind != "" {
		kind = opts.Kind
	}
	channel := ""
	if opts != nil {
		channel = opts.Channel
	}
	id := "msg-" + uuid.New().String()
	delivered := now
	msg := &Message{
		ID:          id,
		FromAgentID: fromAgentID,
		ToAgentID:   toAgentID,
		Channel:     channel,
		Kind:        kind,
		Payload:     copyPayload(payload),
		CreatedAt:   now,
		DeliveredAt: &delivered,
	}
	if opts != nil {
		msg.CausationID = opts.CausationID
		msg.ScheduledAt = opts.ScheduledAt
		msg.ExpiresAt = opts.ExpiresAt
	}
	s.mu.Lock()
	s.messages = append(s.messages, msg)
	s.byID[id] = msg
	s.mu.Unlock()
	return id, nil
}

func (s *StoreMem) SendDelayed(ctx context.Context, toAgentID string, payload map[string]any, at time.Time, opts *SendOptions) (string, error) {
	now := time.Now()
	kind := KindTimer
	if opts != nil && opts.Kind != "" {
		kind = opts.Kind
	}
	channel := ""
	if opts != nil {
		channel = opts.Channel
	}
	id := "msg-" + uuid.New().String()
	msg := &Message{
		ID:          id,
		ToAgentID:   toAgentID,
		Channel:     channel,
		Kind:        kind,
		Payload:     copyPayload(payload),
		CreatedAt:   now,
		ScheduledAt: &at,
	}
	if opts != nil {
		msg.CausationID = opts.CausationID
		msg.ExpiresAt = opts.ExpiresAt
	}
	s.mu.Lock()
	s.messages = append(s.messages, msg)
	s.byID[id] = msg
	s.mu.Unlock()
	return id, nil
}

func (s *StoreMem) PeekInbox(ctx context.Context, agentID string, limit int) ([]*Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*Message
	now := time.Now()
	for _, m := range s.messages {
		if m.ToAgentID != agentID || m.ConsumedByJobID != "" {
			continue
		}
		if m.DeliveredAt == nil && m.ScheduledAt != nil && m.ScheduledAt.After(now) {
			continue
		}
		cp := *m
		cp.Payload = copyPayload(m.Payload)
		out = append(out, &cp)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (s *StoreMem) ConsumeInbox(ctx context.Context, agentID string, limit int) ([]*Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Message
	now := time.Now()
	for _, m := range s.messages {
		if m.ToAgentID != agentID || m.ConsumedByJobID != "" {
			continue
		}
		if m.DeliveredAt == nil && m.ScheduledAt != nil && m.ScheduledAt.After(now) {
			continue
		}
		cp := *m
		cp.Payload = copyPayload(m.Payload)
		out = append(out, &cp)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (s *StoreMem) MarkConsumed(ctx context.Context, messageID, jobID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.byID[messageID]
	if m != nil {
		m.ConsumedByJobID = jobID
		t := time.Now()
		m.ConsumedAt = &t
	}
	return nil
}

// ListAgentIDsWithUnconsumedMessages 返回有未消费消息的 to_agent_id 列表（design/plan.md Phase A）
func (s *StoreMem) ListAgentIDsWithUnconsumedMessages(ctx context.Context, limit int) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen := make(map[string]struct{})
	now := time.Now()
	for _, m := range s.messages {
		if m.ConsumedByJobID != "" {
			continue
		}
		if m.DeliveredAt == nil && m.ScheduledAt != nil && m.ScheduledAt.After(now) {
			continue
		}
		if m.ToAgentID != "" {
			seen[m.ToAgentID] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}
