package jobstore

import (
	"context"
	"testing"
	"time"
)

type fakeLifecycleStore struct {
	JobStore
	expiredBatches [][]ToolInvocationRef
	archiveCalls   int
	deleteCalls    int
}

func (f *fakeLifecycleStore) ListExpiredToolInvocations(ctx context.Context, cutoff time.Time, limit int) ([]ToolInvocationRef, error) {
	if len(f.expiredBatches) == 0 {
		return nil, nil
	}
	batch := f.expiredBatches[0]
	f.expiredBatches = f.expiredBatches[1:]
	return append([]ToolInvocationRef(nil), batch...), nil
}

func (f *fakeLifecycleStore) ArchiveToolInvocations(ctx context.Context, refs []ToolInvocationRef) error {
	f.archiveCalls++
	return nil
}

func (f *fakeLifecycleStore) DeleteToolInvocations(ctx context.Context, refs []ToolInvocationRef) error {
	f.deleteCalls++
	return nil
}

func TestGC_NoopWhenDisabled(t *testing.T) {
	store := &fakeLifecycleStore{JobStore: NewMemoryStore()}
	err := GC(context.Background(), store, GCConfig{Enable: false})
	if err != nil {
		t.Fatalf("GC disabled should return nil, got: %v", err)
	}
	if store.archiveCalls != 0 || store.deleteCalls != 0 {
		t.Fatalf("expected no lifecycle calls, archive=%d delete=%d", store.archiveCalls, store.deleteCalls)
	}
}

func TestGC_ArchiveAndDelete(t *testing.T) {
	store := &fakeLifecycleStore{
		JobStore: NewMemoryStore(),
		expiredBatches: [][]ToolInvocationRef{
			{{ID: "inv_1"}, {ID: "inv_2"}},
		},
	}
	cfg := GCConfig{
		Enable:         true,
		TTLDays:        90,
		ArchiveEnabled: true,
		BatchSize:      1000,
	}
	if err := GC(context.Background(), store, cfg); err != nil {
		t.Fatalf("GC failed: %v", err)
	}
	if store.archiveCalls != 1 {
		t.Fatalf("archive calls = %d, want 1", store.archiveCalls)
	}
	if store.deleteCalls != 1 {
		t.Fatalf("delete calls = %d, want 1", store.deleteCalls)
	}
}

func TestGC_DeleteOnly(t *testing.T) {
	store := &fakeLifecycleStore{
		JobStore: NewMemoryStore(),
		expiredBatches: [][]ToolInvocationRef{
			{{ID: "inv_1"}},
		},
	}
	cfg := GCConfig{
		Enable:         true,
		ArchiveEnabled: false,
		BatchSize:      10,
	}
	if err := GC(context.Background(), store, cfg); err != nil {
		t.Fatalf("GC failed: %v", err)
	}
	if store.archiveCalls != 0 {
		t.Fatalf("archive calls = %d, want 0", store.archiveCalls)
	}
	if store.deleteCalls != 1 {
		t.Fatalf("delete calls = %d, want 1", store.deleteCalls)
	}
}
