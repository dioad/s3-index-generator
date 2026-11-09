package main

import "testing"

func TestHumanizeBytes(t *testing.T) {
	tests := []struct {
		size int64
		want string
	}{
		{0, "0 B"},
		{1023, "1023 B"},
		{1024, "1.0 KiB"},
		{1536, "1.5 KiB"},
		{1048576, "1.0 MiB"},
		{5 * 1024 * 1024 * 1024, "5.0 GiB"},
	}

	for _, tt := range tests {
		if got := humanizeBytes(tt.size); got != tt.want {
			t.Errorf("humanizeBytes(%d) = %q, want %q", tt.size, got, tt.want)
		}
	}
}
