package app

import (
	"strings"
	"testing"

	"gogitor/internal/domain"
)

func TestAppendReviewerSuggestions(t *testing.T) {
	var dst []string

	appendReviewerSuggestions(
		&dst,
		agentReview{
			Suggestions: []string{
				"Add caching layer",
				"  ",                       // пустая — должна быть пропущена
				"Consider sync.Pool",
			},
		},
		2,
		5,
	)

	if len(dst) != 2 {
		t.Fatalf("got %d suggestions, want 2: %v", len(dst), dst)
	}
	if dst[0] != "[2/5] Add caching layer" {
		t.Fatalf("unexpected first: %q", dst[0])
	}
	if dst[1] != "[2/5] Consider sync.Pool" {
		t.Fatalf("unexpected second: %q", dst[1])
	}
}

func TestFormatAgentTaskReportShowsSuggestions(t *testing.T) {
	res := domain.Result{
		Success: true,
		Mode:    "agent",
		ReviewerSuggestions: []string{
			"[1/2] extract helper",
			"[2/2] add test",
		},
	}

	out := formatAgentTaskReport(res, AgentDepthNormal, 2, 2)

	if !strings.Contains(out, "REVIEWER SUGGESTIONS (optional, not applied)") {
		t.Fatalf("header missing:\n%s", out)
	}
	if !strings.Contains(out, "[1/2] extract helper") ||
		!strings.Contains(out, "[2/2] add test") {
		t.Fatalf("suggestion bodies missing:\n%s", out)
	}
}

func TestFormatAgentTaskReportOmitsEmptySuggestions(t *testing.T) {
	res := domain.Result{Success: true, Mode: "agent"}
	out := formatAgentTaskReport(res, AgentDepthNormal, 0, 0)

	if strings.Contains(out, "REVIEWER SUGGESTIONS") {
		t.Fatalf("unexpected section in:\n%s", out)
	}
}