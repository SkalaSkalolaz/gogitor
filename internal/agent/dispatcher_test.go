package agent

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func TestEstimateTokens(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want int
	}{
		{"", 0},
		{"hello", 2},
		{"hello world", 3},
	} {
		if got := estimateTokens(tc.in); got != tc.want {
			t.Errorf("estimateTokens(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestIsRetryable(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"canceled", context.Canceled, false},
		{"deadline", context.DeadlineExceeded, false},
		{"EOF", io.EOF, true},
		{"unexpected EOF", io.ErrUnexpectedEOF, true},
		{"dns", &net.DNSError{}, true},
		{"429", errors.New("HTTP 429 Too Many Requests"), true},
		{"502", errors.New("HTTP 502 Bad Gateway"), true},
		{"503", errors.New("HTTP 503"), true},
        {"model not found", errors.New("model not found"), false},
		{"random", errors.New("some error"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := isRetryable(tc.err); got != tc.want {
				t.Errorf("isRetryable(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestUsageAdd(t *testing.T) {
	u1 := Usage{Requests: 1, EstimatedTokens: 100, Duration: time.Second}
	u2 := Usage{Requests: 2, EstimatedTokens: 200, Duration: 2 * time.Second}
	sum := u1.Add(u2)
	if sum.Requests != 3 || sum.EstimatedTokens != 300 || sum.Duration != 3*time.Second {
		t.Errorf("sum = %+v", sum)
	}
}

func TestDispatcherImplementsLLM(t *testing.T) {
	var _ LLM = (*Dispatcher)(nil)
}

func TestPriorityOrder(t *testing.T) {
	if !(PriorityLow < PriorityNormal && PriorityNormal < PriorityHigh && PriorityHigh < PriorityCritical) {
		t.Error("priority order violated")
	}
}