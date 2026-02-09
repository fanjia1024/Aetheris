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

package utils

import (
	"testing"
)

func TestCoalesceString(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want string
	}{
		{"empty slice", []string{}, ""},
		{"all empty", []string{"", "", ""}, ""},
		{"first non-empty", []string{"a", "", "c"}, "a"},
		{"second non-empty", []string{"", "b", "c"}, "b"},
		{"single", []string{"x"}, "x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CoalesceString(tt.in...)
			if got != tt.want {
				t.Errorf("CoalesceString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDefaultInt(t *testing.T) {
	tests := []struct {
		v, defaultVal, want int
	}{
		{0, 10, 10},
		{1, 10, 1},
		{-1, 10, -1},
		{100, 5, 100},
	}
	for _, tt := range tests {
		got := DefaultInt(tt.v, tt.defaultVal)
		if got != tt.want {
			t.Errorf("DefaultInt(%d, %d) = %d, want %d", tt.v, tt.defaultVal, got, tt.want)
		}
	}
}
