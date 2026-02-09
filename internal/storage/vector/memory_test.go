package vector

import (
	"context"
	"testing"
)

func TestMemoryStore_Create_Add_Search(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	idx := &Index{Name: "idx1", Dimension: 2, Distance: "cosine"}
	if err := s.Create(ctx, idx); err != nil {
		t.Fatalf("Create: %v", err)
	}
	vecs := []*Vector{
		{ID: "v1", Values: []float64{1, 0}},
		{ID: "v2", Values: []float64{0, 1}},
	}
	if err := s.Add(ctx, "idx1", vecs); err != nil {
		t.Fatalf("Add: %v", err)
	}
	results, err := s.Search(ctx, "idx1", []float64{1, 0}, &SearchOptions{TopK: 2})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) < 1 {
		t.Fatalf("Search: expected at least 1 result, got %d", len(results))
	}
	if results[0].ID != "v1" {
		t.Errorf("Search: expected v1 first (cosine sim), got %s", results[0].ID)
	}
}

func TestMemoryStore_Create_DuplicateIndex(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	idx := &Index{Name: "x", Dimension: 2}
	_ = s.Create(ctx, idx)
	err := s.Create(ctx, idx)
	if err == nil {
		t.Error("Create duplicate index should error")
	}
}

func TestMemoryStore_Add_IndexNotFound(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	err := s.Add(ctx, "missing", []*Vector{{ID: "v1", Values: []float64{1}}})
	if err == nil {
		t.Error("Add to missing index should error")
	}
}

func TestMemoryStore_Add_DimensionMismatch(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	_ = s.Create(ctx, &Index{Name: "i", Dimension: 2})
	err := s.Add(ctx, "i", []*Vector{{ID: "v1", Values: []float64{1, 0, 0}}})
	if err == nil {
		t.Error("Add with wrong dimension should error")
	}
}

func TestMemoryStore_Search_IndexNotFound(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	_, err := s.Search(ctx, "missing", []float64{1}, nil)
	if err == nil {
		t.Error("Search missing index should error")
	}
}
