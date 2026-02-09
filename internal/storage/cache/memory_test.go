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

package cache

import (
	"context"
	"testing"
)

func TestMemoryStore_Set_Get_Delete(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	if err := s.Set(ctx, "k1", "v1", 0); err != nil {
		t.Fatalf("Set: %v", err)
	}
	var v string
	if err := s.Get(ctx, "k1", &v); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if v != "v1" {
		t.Errorf("Get: got %q", v)
	}
	if err := s.Delete(ctx, "k1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := s.Get(ctx, "k1", &v); err == nil {
		t.Error("Get after Delete should error")
	}
}

func TestMemoryStore_Get_NotFound(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	var v string
	if err := s.Get(ctx, "missing", &v); err == nil {
		t.Error("Get missing should error")
	}
}

func TestMemoryStore_Exists(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	ok, err := s.Exists(ctx, "k")
	if err != nil || ok {
		t.Errorf("Exists missing: ok=%v err=%v", ok, err)
	}
	_ = s.Set(ctx, "k", "v", 0)
	ok, err = s.Exists(ctx, "k")
	if err != nil || !ok {
		t.Errorf("Exists present: ok=%v err=%v", ok, err)
	}
}

func TestMemoryStore_Clear(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	_ = s.Set(ctx, "k1", "v1", 0)
	if err := s.Clear(ctx); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	var v string
	if err := s.Get(ctx, "k1", &v); err == nil {
		t.Error("Get after Clear should error")
	}
}

// Expiration 由实现用 Unix 秒判断，短 TTL 可能仍在同一秒内未过期，此处不测过期以保持稳定
