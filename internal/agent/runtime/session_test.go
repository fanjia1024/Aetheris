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

package runtime

import (
	"testing"
	"time"
)

func TestNewSession(t *testing.T) {
	s := NewSession("sid1", "agent-1")
	if s == nil || s.ID != "sid1" || s.AgentID != "agent-1" {
		t.Errorf("NewSession: %+v", s)
	}
	if s.Variables == nil {
		t.Error("Variables should be initialized")
	}
	s2 := NewSession("", "")
	if s2.ID == "" {
		t.Error("empty id should generate session id")
	}
}

func TestSession_AddMessage_GetCurrentTask_GetLastCheckpoint(t *testing.T) {
	s := NewSession("s1", "a1")
	s.AddMessage("user", "hello")
	s.AddMessage("assistant", "hi")
	msgs := s.CopyMessages()
	if len(msgs) != 2 || msgs[0].Role != "user" || msgs[0].Content != "hello" {
		t.Errorf("CopyMessages: %+v", msgs)
	}
	if s.GetCurrentTask() != "" {
		t.Error("initial CurrentTask should be empty")
	}
	s.SetCurrentTask("task1")
	if s.GetCurrentTask() != "task1" {
		t.Errorf("GetCurrentTask: %q", s.GetCurrentTask())
	}
	s.SetLastCheckpoint("cp-1")
	if s.GetLastCheckpoint() != "cp-1" {
		t.Errorf("GetLastCheckpoint: %q", s.GetLastCheckpoint())
	}
	_ = s.GetUpdatedAt()
}

func TestSession_SetVariable_GetVariable(t *testing.T) {
	s := NewSession("s1", "a1")
	s.SetVariable("k1", "v1")
	v, ok := s.GetVariable("k1")
	if !ok || v != "v1" {
		t.Errorf("GetVariable: v=%v ok=%v", v, ok)
	}
	_, ok = s.GetVariable("missing")
	if ok {
		t.Error("GetVariable missing should be false")
	}
}

func TestSession_GetUpdatedAt(t *testing.T) {
	s := NewSession("s1", "a1")
	before := time.Now()
	s.AddMessage("user", "x")
	after := s.GetUpdatedAt()
	if after.Before(before) {
		t.Error("UpdatedAt should be after AddMessage")
	}
}
