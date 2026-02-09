package errors

import (
	"errors"
	"testing"
)

func TestWrap(t *testing.T) {
	if Wrap(nil, "msg") != nil {
		t.Error("Wrap(nil, msg) should return nil")
	}
	err := errors.New("base")
	wrapped := Wrap(err, "context")
	if wrapped == nil {
		t.Fatal("Wrap(err, msg) should not return nil")
	}
	if !errors.Is(wrapped, err) {
		t.Error("wrapped error should unwrap to base")
	}
}

func TestWrapf(t *testing.T) {
	if Wrapf(nil, "format %s", "x") != nil {
		t.Error("Wrapf(nil, ...) should return nil")
	}
	err := errors.New("base")
	wrapped := Wrapf(err, "id=%s", "a")
	if wrapped == nil {
		t.Fatal("Wrapf(err, ...) should not return nil")
	}
	if !errors.Is(wrapped, err) {
		t.Error("wrapped error should unwrap to base")
	}
}

func TestSentinels(t *testing.T) {
	if !errors.Is(ErrNotFound, ErrNotFound) {
		t.Error("ErrNotFound should be Is ErrNotFound")
	}
	if !errors.Is(ErrInvalidArg, ErrInvalidArg) {
		t.Error("ErrInvalidArg should be Is ErrInvalidArg")
	}
}
