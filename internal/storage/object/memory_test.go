package object

import (
	"bytes"
	"context"
	"io"
	"testing"
)

func TestMemoryStore_Put_Get_Delete(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	data := bytes.NewReader([]byte("hello"))
	if err := s.Put(ctx, "p1", data, 5, nil); err != nil {
		t.Fatalf("Put: %v", err)
	}
	rc, err := s.Get(ctx, "p1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer rc.Close()
	b, _ := io.ReadAll(rc)
	if string(b) != "hello" {
		t.Errorf("Get: got %q", string(b))
	}
	if err := s.Delete(ctx, "p1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get(ctx, "p1"); err == nil {
		t.Error("Get after Delete should error")
	}
}

func TestMemoryStore_Get_NotFound(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	_, err := s.Get(ctx, "missing")
	if err == nil {
		t.Error("Get missing should error")
	}
}
