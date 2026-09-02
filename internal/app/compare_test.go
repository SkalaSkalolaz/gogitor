package app

import (
	"testing"

	"gogitor/internal/domain"
)

func TestFormatApproachForPrompt(t *testing.T) {
	a := domain.Approach{ID: 1, Name: "Simple HTTP", Description: "Use stdlib"}
	if formatApproachForPrompt(a) == "" {
		t.Error("empty")
	}
}

func TestIsApproachModification(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"но без внешних зависимостей", true},
		{"однако с кэшированием", true},
		{"but with caching", true},
		{"however, use Redis", true},
		{"instead of REST, use gRPC", true},
		{"измени подход", true},
		{"убери базу данных", true},
		{"с поправкой на производительность", true},
		{"вариант 1", false},
		{"да", false},
		{"ok", false},
	} {
		if got := isApproachModification(tc.in); got != tc.want {
			t.Errorf("isApproachModification(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
