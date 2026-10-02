package main

import "testing"

func TestTruncateRunes(t *testing.T) {
	tests := []struct {
		in   string
		n    int
		want string
	}{
		{"abc", 5, "abc"},
		{"abcdef", 3, "abc"},
		{"ñañaña", 4, "ñaña"}, // multi-byte runes are not split
		{"", 3, ""},
	}
	for _, tt := range tests {
		if got := truncateRunes(tt.in, tt.n); got != tt.want {
			t.Errorf("truncateRunes(%q, %d) = %q, want %q", tt.in, tt.n, got, tt.want)
		}
	}
}
