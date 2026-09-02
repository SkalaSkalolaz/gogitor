package computer

import (
	"testing"
)

func TestAuditLog_FormatHistory(t *testing.T) {
	l := &AuditLog{entries: []AuditEntry{
		{Command: "ls", Risk: "low", ExitCode: 0},
		{Command: "rm f", Risk: "high", ExitCode: 1},
		{Command: "rm -rf /", Risk: "forbidden", ExitCode: -1},
	}}
	if l.FormatHistory(10) == "" {
		t.Error("empty")
	}
	if (&AuditLog{}).FormatHistory(10) != "" {
		t.Error("empty log should return empty")
	}
}

func TestAuditLog_FormatHistory_Limit(t *testing.T) {
	l := &AuditLog{entries: make([]AuditEntry, 100)}
	for i := range l.entries {
		l.entries[i] = AuditEntry{Command: "ls", Risk: "low"}
	}
	formatted := l.FormatHistory(5)
	lines := 0
	for _, c := range formatted {
		if c == '\n' {
			lines++
		}
	}
	if lines > 5 {
		t.Errorf("expected ≤5 lines, got %d", lines)
	}
}