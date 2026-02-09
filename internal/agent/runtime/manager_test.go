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
	"context"
	"testing"
)

func TestManager_Create_Get_List_Delete(t *testing.T) {
	ctx := context.Background()
	m := NewManager()
	agent, err := m.Create(ctx, "my-agent", nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if agent == nil || agent.Name != "my-agent" {
		t.Errorf("Create: %+v", agent)
	}
	if agent.Session == nil {
		t.Fatal("Create with nil session should set Session")
	}
	if agent.Session.AgentID != agent.ID {
		t.Errorf("Session.AgentID should be set to agent ID")
	}
	got, err := m.Get(ctx, agent.ID)
	if err != nil || got != agent {
		t.Errorf("Get: err=%v got=%v", err, got)
	}
	list, err := m.List(ctx)
	if err != nil || len(list) != 1 {
		t.Errorf("List: err=%v len=%d", err, len(list))
	}
	if err := m.Delete(ctx, agent.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got2, _ := m.Get(ctx, agent.ID)
	if got2 != nil {
		t.Error("Get after Delete should return nil")
	}
}

func TestManager_Create_WithSession(t *testing.T) {
	ctx := context.Background()
	m := NewManager()
	sess := NewSession("s1", "")
	agent, err := m.Create(ctx, "a", sess, nil, nil, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if agent.Session != sess {
		t.Error("Create with session should use that session")
	}
	if sess.AgentID != agent.ID {
		t.Errorf("session AgentID should be set to %q", agent.ID)
	}
}
