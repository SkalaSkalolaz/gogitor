package app

import (
	"testing"
	"time"

	"gogitor/internal/agent"
)

func TestPromptBucket(t *testing.T) {
	for _, tc := range []struct {
		size int
		want string
	}{
		{10, "s"}, {2000, "m"}, {10000, "l"}, {100000, "xl"},
	} {
		prompt := string(make([]byte, tc.size))
		if got := promptBucket(prompt); got != tc.want {
			t.Errorf("promptBucket(%d bytes) = %q, want %q", tc.size, got, tc.want)
		}
	}
}

func TestPurposeCategory(t *testing.T) {
	for _, tc := range []struct{ purpose, want string }{
		{"code subtask 1/3", "code"}, {"create plan", "plan"},
		{"review changes", "review"}, {"verify completion", "verify"},
		{"detect intent", "intent"}, {"chat response", "chat"},
		{"analyze code", "analyze"}, {"web search", "search"},
		{"generate commit message", "commit"}, {"compare approaches", "compare"},
		{"something else", "other"},
	} {
		if got := purposeCategory(tc.purpose); got != tc.want {
			t.Errorf("purposeCategory(%q) = %q, want %q", tc.purpose, got, tc.want)
		}
	}
}

func TestHeuristicDuration(t *testing.T) {
	d := heuristicDuration(100)
	if d < 1500*time.Millisecond || d > 180000*time.Millisecond {
		t.Errorf("duration = %v", d)
	}
}

func TestHeuristicSubtaskDuration(t *testing.T) {
	single := heuristicSubtaskDuration(100, false)
	multi := heuristicSubtaskDuration(100, true)
	if single <= 0 || multi <= single {
		t.Errorf("single=%v multi=%v", single, multi)
	}
}

func TestStatsKey(t *testing.T) {
	key := statsKey(agent.RoleCoder, "code subtask", "prompt")
	if key == "" {
		t.Error("empty key")
	}
}
