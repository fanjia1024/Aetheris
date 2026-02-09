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

func TestCheckpointStoreMem_Save_Load(t *testing.T) {
	ctx := context.Background()
	store := NewCheckpointStoreMem()
	cp := NewNodeCheckpoint("agent-1", "sess-1", "job-1", "n1", []byte("graph"), []byte("results"), nil)
	id, err := store.Save(ctx, cp)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if id == "" {
		t.Fatal("Save should return non-empty id")
	}
	loaded, err := store.Load(ctx, id)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded == nil || loaded.AgentID != "agent-1" || loaded.CursorNode != "n1" {
		t.Errorf("Load: %+v", loaded)
	}
}

func TestCheckpointStoreMem_Load_NotFound(t *testing.T) {
	ctx := context.Background()
	store := NewCheckpointStoreMem()
	loaded, err := store.Load(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded != nil {
		t.Errorf("Load nonexistent should return nil, got %+v", loaded)
	}
}

func TestCheckpointStoreMem_Save_WithID(t *testing.T) {
	ctx := context.Background()
	store := NewCheckpointStoreMem()
	cp := &Checkpoint{ID: "my-id", AgentID: "a1", SessionID: "s1"}
	id, err := store.Save(ctx, cp)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if id != "my-id" {
		t.Errorf("Save with ID should return same id, got %q", id)
	}
}

func TestCheckpointStoreMem_ListByAgent(t *testing.T) {
	ctx := context.Background()
	store := NewCheckpointStoreMem()
	_, _ = store.Save(ctx, NewNodeCheckpoint("a1", "s1", "", "n1", nil, nil, nil))
	_, _ = store.Save(ctx, NewNodeCheckpoint("a1", "s1", "", "n2", nil, nil, nil))
	_, _ = store.Save(ctx, NewNodeCheckpoint("a2", "s2", "", "n1", nil, nil, nil))
	list, err := store.ListByAgent(ctx, "a1")
	if err != nil {
		t.Fatalf("ListByAgent: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 for a1, got %d", len(list))
	}
	list2, _ := store.ListByAgent(ctx, "a2")
	if len(list2) != 1 {
		t.Errorf("expected 1 for a2, got %d", len(list2))
	}
}
