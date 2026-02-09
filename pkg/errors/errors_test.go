// Copyright 2026 fanjia1024
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
