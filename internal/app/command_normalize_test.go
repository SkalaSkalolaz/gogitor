package app

import "testing"

func TestNormalizeTUICommand(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"canonical", ":agent", ":agent"},
		{"legacy spelling", "agent", ":agent"},
		{"spaces and case", "  TASK-DIFF ", ":task-diff"},
		{"empty", "   ", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeTUICommand(tt.in); got != tt.want {
				t.Fatalf("normalizeTUICommand(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
