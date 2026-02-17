package eino

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"
)

func TestInferToolOrUnavailable_FallbackWhenInferFails(t *testing.T) {
	orig := inferStringTool
	t.Cleanup(func() { inferStringTool = orig })

	inferStringTool = func(name, desc string, fn func(context.Context, string) (string, error)) (tool.InvokableTool, error) {
		return nil, errors.New("boom")
	}

	tl := inferToolOrUnavailable("retriever", "desc", func(ctx context.Context, input string) (string, error) {
		return "ok", nil
	})

	invokable, ok := tl.(tool.InvokableTool)
	if !ok {
		t.Fatalf("expected tool.InvokableTool, got %T", tl)
	}

	info, err := invokable.Info(context.Background())
	if err != nil {
		t.Fatalf("unexpected info error: %v", err)
	}
	if info == nil || info.Name != "retriever" {
		t.Fatalf("unexpected tool info: %#v", info)
	}

	_, err = invokable.InvokableRun(context.Background(), `{"input":"q"}`)
	if err == nil {
		t.Fatal("expected fallback tool error, got nil")
	}
	if !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("expected unavailable error, got: %v", err)
	}
}
