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
