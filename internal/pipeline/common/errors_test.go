package common

import (
	"context"
	"errors"
	"testing"
)

func TestPipelineError_Error(t *testing.T) {
	t.Run("without cause", func(t *testing.T) {
		e := NewPipelineError("ingest", "failed", nil)
		s := e.Error()
		if s == "" || len(s) < 10 {
			t.Errorf("Error() = %q", s)
		}
		if !errors.As(e, new(*PipelineError)) {
			t.Error("should be *PipelineError")
		}
	})
	t.Run("with cause", func(t *testing.T) {
		cause := errors.New("io error")
		e := NewPipelineError("parse", "file", cause)
		s := e.Error()
		if s == "" {
			t.Error("Error() should not be empty")
		}
		if e.Unwrap() != cause {
			t.Error("Unwrap() should return cause")
		}
	})
}

func TestIsPipelineError_GetPipelineError(t *testing.T) {
	e := NewPipelineError("stage", "msg", nil)
	if !IsPipelineError(e) {
		t.Error("IsPipelineError should be true")
	}
	got, ok := GetPipelineError(e)
	if !ok || got != e {
		t.Errorf("GetPipelineError: ok=%v got=%v", ok, got)
	}
	if !IsPipelineError(errors.New("other")) {
		// other is not PipelineError
	}
	_, ok = GetPipelineError(errors.New("other"))
	if ok {
		t.Error("GetPipelineError(other) should be false")
	}
}

func TestValidationError_Error(t *testing.T) {
	e := NewValidationError("field1", "required")
	s := e.Error()
	if s == "" || len(s) < 5 {
		t.Errorf("Error() = %q", s)
	}
}

func TestIsValidationError_GetValidationError(t *testing.T) {
	e := NewValidationError("f", "m")
	if !IsValidationError(e) {
		t.Error("IsValidationError should be true")
	}
	got, ok := GetValidationError(e)
	if !ok || got != e {
		t.Errorf("GetValidationError: ok=%v got=%v", ok, got)
	}
	_, ok = GetValidationError(errors.New("other"))
	if ok {
		t.Error("GetValidationError(other) should be false")
	}
}

func TestNewPipelineContext(t *testing.T) {
	ctx := NewPipelineContext(context.Background(), "id1")
	if ctx == nil || ctx.ID != "id1" || ctx.Status != "running" || ctx.Metadata == nil {
		t.Errorf("NewPipelineContext: %+v", ctx)
	}
}
