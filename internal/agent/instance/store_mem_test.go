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

package instance

import (
	"context"
	"testing"
)

func TestStoreMem_GetCreateUpdate(t *testing.T) {
	ctx := context.Background()
	s := NewStoreMem()
	// Get missing returns nil
	got, err := s.Get(ctx, "agent-1")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("Get missing: got %v", got)
	}
	// Create
	inst := &AgentInstance{ID: "agent-1", Name: "Test", Status: StatusIdle}
	if err := s.Create(ctx, inst); err != nil {
		t.Fatal(err)
	}
	got, err = s.Get(ctx, "agent-1")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID != "agent-1" || got.Name != "Test" || got.Status != StatusIdle {
		t.Fatalf("Get after Create: got %+v", got)
	}
	// UpdateStatus
	if err := s.UpdateStatus(ctx, "agent-1", StatusRunning); err != nil {
		t.Fatal(err)
	}
	got, _ = s.Get(ctx, "agent-1")
	if got.Status != StatusRunning {
		t.Fatalf("after UpdateStatus: got status %q", got.Status)
	}
	// ListByTenant
	list, err := s.ListByTenant(ctx, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != "agent-1" {
		t.Fatalf("ListByTenant: got %+v", list)
	}
}
