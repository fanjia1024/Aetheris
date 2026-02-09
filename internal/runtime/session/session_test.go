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

package session

import (
	"testing"
)

func TestNew(t *testing.T) {
	s := New("sid1")
	if s == nil || s.ID != "sid1" {
		t.Errorf("New: %+v", s)
	}
	if s.WorkingState == nil || s.Metadata == nil {
		t.Error("WorkingState and Metadata should be initialized")
	}
	s2 := New("")
	if s2.ID == "" {
		t.Error("empty id should generate id")
	}
}

func TestSession_AddMessage_CopyMessages(t *testing.T) {
	s := New("s1")
	s.AddMessage("user", "hello")
	s.AddMessage("assistant", "hi")
	msgs := s.CopyMessages()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "hello" {
		t.Errorf("first message: %+v", msgs[0])
	}
	if msgs[1].Role != "assistant" || msgs[1].Content != "hi" {
		t.Errorf("second message: %+v", msgs[1])
	}
}

func TestSession_AddObservation_CopyToolCalls(t *testing.T) {
	s := New("s1")
	s.AddObservation("tool1", map[string]any{"q": "x"}, "out", "")
	calls := s.CopyToolCalls()
	if len(calls) != 1 || calls[0].Tool != "tool1" || calls[0].Output != "out" {
		t.Errorf("CopyToolCalls: %+v", calls)
	}
}

func TestSession_WorkingStateGet_WorkingStateSet(t *testing.T) {
	s := New("s1")
	s.WorkingStateSet("k1", "v1")
	v, ok := s.WorkingStateGet("k1")
	if !ok || v != "v1" {
		t.Errorf("WorkingStateGet: v=%v ok=%v", v, ok)
	}
	_, ok = s.WorkingStateGet("missing")
	if ok {
		t.Error("WorkingStateGet missing should be false")
	}
}
