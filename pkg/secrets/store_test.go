package secrets

import (
	"context"
	"strings"
	"testing"
)

func TestNewStore(t *testing.T) {
	tests := []struct {
		name        string
		provider    string
		wantErr     bool
		errContains string
	}{
		{name: "memory", provider: "memory", wantErr: false},
		{name: "env", provider: "env", wantErr: false},
		{name: "vault not implemented", provider: "vault", wantErr: true, errContains: "not implemented"},
		{name: "k8s not implemented", provider: "k8s", wantErr: true, errContains: "not implemented"},
		{name: "unknown provider", provider: "unknown", wantErr: true, errContains: "unsupported secret provider"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store, err := NewStore(Config{Provider: tc.provider})
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tc.errContains != "" && !strings.Contains(err.Error(), tc.errContains) {
					t.Fatalf("error = %q, want contains %q", err.Error(), tc.errContains)
				}
				if store != nil {
					t.Fatalf("store should be nil when error occurs")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if store == nil {
				t.Fatalf("store should not be nil")
			}
		})
	}
}

func TestMemoryAndEnvStoreBasicContract(t *testing.T) {
	ctx := context.Background()
	stores := []Store{NewMemoryStore(), NewEnvStore()}

	for _, s := range stores {
		if err := s.Set(ctx, "secret_test_key", "value"); err != nil {
			t.Fatalf("set secret failed: %v", err)
		}
		got, err := s.Get(ctx, "secret_test_key")
		if err != nil {
			t.Fatalf("get secret failed: %v", err)
		}
		if got != "value" {
			t.Fatalf("get secret = %q, want value", got)
		}
		if err := s.Delete(ctx, "secret_test_key"); err != nil {
			t.Fatalf("delete secret failed: %v", err)
		}
		_, err = s.Get(ctx, "secret_test_key")
		if err == nil {
			t.Fatalf("expected error after delete")
		}
	}
}
