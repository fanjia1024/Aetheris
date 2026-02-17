package api

import (
	"testing"

	"github.com/cloudwego/eino/schema"

	"rag-platform/internal/agent/runtime"
)

func TestSessionRoleToSchema(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want schema.RoleType
	}{
		{name: "user", in: "user", want: schema.User},
		{name: "assistant", in: "assistant", want: schema.Assistant},
		{name: "system", in: "system", want: schema.System},
		{name: "custom", in: "tool", want: schema.RoleType("tool")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sessionRoleToSchema(tt.in)
			if got != tt.want {
				t.Fatalf("sessionRoleToSchema(%q)=%q, want=%q", tt.in, got, tt.want)
			}
		})
	}
}

func TestLastUserMessage(t *testing.T) {
	t.Run("nil session", func(t *testing.T) {
		if got := lastUserMessage(nil); got != "" {
			t.Fatalf("expected empty result, got %q", got)
		}
	})

	t.Run("no user message", func(t *testing.T) {
		s := runtime.NewSession("", "agent-1")
		s.AddMessage("assistant", "hello")
		if got := lastUserMessage(s); got != "" {
			t.Fatalf("expected empty result, got %q", got)
		}
	})

	t.Run("returns latest user message", func(t *testing.T) {
		s := runtime.NewSession("", "agent-1")
		s.AddMessage("user", "first question")
		s.AddMessage("assistant", "first answer")
		s.AddMessage("user", "second question")
		s.AddMessage("assistant", "second answer")

		got := lastUserMessage(s)
		if got != "second question" {
			t.Fatalf("expected latest user message, got %q", got)
		}
	})
}
