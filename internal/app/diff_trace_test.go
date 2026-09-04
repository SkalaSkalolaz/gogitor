package app

import (
	"strings"
	"testing"

	"gogitor/internal/domain"
)

func TestEmitDiffMatchingConfigTrace(t *testing.T) {
	var events []domain.Event
	cfg := domain.DiffMatchingConfig{
		ASTWeight:          0.40,
		LineWeight:         0.60,
		ASTMinStructure:    0.82,
		FuzzyBaseThreshold: 0.60,
		BalancedThreshold:  0.82,
		BalancedMargin:     0.08,
		AdvancedThreshold:  0.85,
		AdvancedMargin:     0.05,
	}
	emitDiffMatchingConfigTrace(
		func(e domain.Event) {
			events = append(events, e)
		},
		cfg,
	)
	if len(events) != 1 {
		t.Fatalf(
			"expected 1 event, got %d",
			len(events),
		)
	}
	want :=
		"[DIFF] matching-config " +
			"ast_weight=0.40 " +
			"line_weight=0.60 " +
			"ast_min_structure=0.82 " +
			"base=0.60 " +
			"balanced=0.82/0.08 " +
			"advanced=0.85/0.05"
	if events[0].Type != domain.EventLog {
		t.Fatalf(
			"event type = %q, want %q",
			events[0].Type,
			domain.EventLog,
		)
	}
	if events[0].Message != want {
		t.Fatalf(
			"message = %q\nwant = %q",
			events[0].Message,
			want,
		)
	}
	_ = strings.TrimSpace // suppress unused if needed
}