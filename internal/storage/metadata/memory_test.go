package metadata

import (
	"context"
	"testing"
)

func TestMemoryStore_Create_Get(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	doc := &Document{ID: "doc1", Name: "a", Type: "pdf"}
	if err := s.Create(ctx, doc); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := s.Get(ctx, "doc1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != "doc1" || got.Name != "a" || got.CreatedAt == 0 {
		t.Errorf("Get: %+v", got)
	}
}

func TestMemoryStore_Create_DuplicateID(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	doc := &Document{ID: "d1"}
	_ = s.Create(ctx, doc)
	err := s.Create(ctx, &Document{ID: "d1"})
	if err == nil {
		t.Error("Create duplicate ID should error")
	}
}

func TestMemoryStore_Get_NotFound(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	_, err := s.Get(ctx, "nonexistent")
	if err == nil {
		t.Error("Get nonexistent should error")
	}
}

func TestMemoryStore_Update_Delete(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	doc := &Document{ID: "d1", Name: "old"}
	_ = s.Create(ctx, doc)
	doc.Name = "new"
	if err := s.Update(ctx, doc); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ := s.Get(ctx, "d1")
	if got.Name != "new" {
		t.Errorf("Update: got Name %q", got.Name)
	}
	if err := s.Delete(ctx, "d1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := s.Get(ctx, "d1")
	if err == nil {
		t.Error("Get after Delete should error")
	}
}

func TestMemoryStore_List(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	_ = s.Create(ctx, &Document{ID: "1", Name: "a"})
	_ = s.Create(ctx, &Document{ID: "2", Name: "b"})
	list, err := s.List(ctx, nil, &Pagination{Offset: 0, Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("List: expected 2, got %d", len(list))
	}
}
