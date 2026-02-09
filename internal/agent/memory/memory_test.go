package memory

import (
	"context"
	"errors"
	"testing"
	"time"
)

type mockMemory struct {
	recallItems []MemoryItem
	recallErr   error
	stored      []MemoryItem
}

func (m *mockMemory) Recall(ctx context.Context, query string) ([]MemoryItem, error) {
	if m.recallErr != nil {
		return nil, m.recallErr
	}
	return m.recallItems, nil
}
func (m *mockMemory) Store(ctx context.Context, item MemoryItem) error {
	m.stored = append(m.stored, item)
	return nil
}

func TestCompositeMemory_Recall_Merge(t *testing.T) {
	ctx := context.Background()
	m1 := &mockMemory{recallItems: []MemoryItem{{Type: "a", Content: "c1"}}}
	m2 := &mockMemory{recallItems: []MemoryItem{{Type: "b", Content: "c2"}}}
	c := NewCompositeMemory(m1, m2)
	items, err := c.Recall(ctx, "q")
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Content != "c1" || items[1].Content != "c2" {
		t.Errorf("items: %+v", items)
	}
}

func TestCompositeMemory_Recall_SkipErrorBackend(t *testing.T) {
	ctx := context.Background()
	m1 := &mockMemory{recallErr: errors.New("fail")}
	m2 := &mockMemory{recallItems: []MemoryItem{{Content: "ok"}}}
	c := NewCompositeMemory(m1, m2)
	items, err := c.Recall(ctx, "q")
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(items) != 1 || items[0].Content != "ok" {
		t.Errorf("expected one item from m2, got %+v", items)
	}
}

func TestCompositeMemory_Store_AllBackends(t *testing.T) {
	ctx := context.Background()
	m1 := &mockMemory{}
	m2 := &mockMemory{}
	c := NewCompositeMemory(m1, m2)
	item := MemoryItem{Type: "working", Content: "x", At: time.Now()}
	if err := c.Store(ctx, item); err != nil {
		t.Fatalf("Store: %v", err)
	}
	if len(m1.stored) != 1 || len(m2.stored) != 1 {
		t.Errorf("Store should write to all backends: m1=%d m2=%d", len(m1.stored), len(m2.stored))
	}
}

func TestNewCompositeMemory_NoBackends(t *testing.T) {
	c := NewCompositeMemory()
	ctx := context.Background()
	items, err := c.Recall(ctx, "q")
	if err != nil || len(items) != 0 {
		t.Errorf("Recall with no backends: err=%v items=%d", err, len(items))
	}
	if err := c.Store(ctx, MemoryItem{}); err != nil {
		t.Errorf("Store with no backends: %v", err)
	}
}
