package computer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// AuditEntry — запись в журнале аудита.
type AuditEntry struct {
	Timestamp string `json:"timestamp"`
	Command   string `json:"command"`
	Risk      string `json:"risk"`
	Reason    string `json:"reason"`
	ExitCode  int    `json:"exit_code"`
	Duration  string `json:"duration"`
	TimedOut  bool   `json:"timed_out"`
	Confirmed bool   `json:"confirmed"`
}

// AuditLog — журнал аудита.
type AuditLog struct {
	path    string
	entries []AuditEntry
}

func NewAuditLog(workDir string) *AuditLog {
	dir := filepath.Join(workDir, ".gogitor")
	_ = os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, "computer_audit.json")
	l := &AuditLog{path: path}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &l.entries)
	}
	return l
}

func (l *AuditLog) Record(r *CommandResult, confirmed bool) {
	l.entries = append(l.entries, AuditEntry{
		Timestamp: time.Now().Format(time.RFC3339),
		Command:   r.Command,
		Risk:      r.Risk.String(),
		Reason:    r.Reason,
		ExitCode:  r.ExitCode,
		Duration:  r.Duration.Round(time.Millisecond).String(),
		TimedOut:  r.TimedOut,
		Confirmed: confirmed,
	})
	if len(l.entries) > 500 {
		l.entries = l.entries[len(l.entries)-500:]
	}
	l.save()
}

func (l *AuditLog) RecordBlocked(command, risk, reason string) {
	l.entries = append(l.entries, AuditEntry{
		Timestamp: time.Now().Format(time.RFC3339),
		Command:   command,
		Risk:      risk,
		Reason:    reason,
		ExitCode:  -1,
	})
	if len(l.entries) > 500 {
		l.entries = l.entries[len(l.entries)-500:]
	}
	l.save()
}

func (l *AuditLog) FormatHistory(max int) string {
	if max <= 0 {
		max = 20
	}
	entries := l.entries
	if len(entries) > max {
		entries = entries[len(entries)-max:]
	}
	var b string
	for _, e := range entries {
		icon := "✓"
		if e.ExitCode != 0 {
			icon = "✗"
		}
		if e.Risk == "forbidden" {
			icon = "⊘"
		}
		b += fmt.Sprintf("%s [%s] %s (%s)\n", icon, e.Risk, e.Command, e.Timestamp)
	}
	return b
}

func (l *AuditLog) save() {
	data, _ := json.MarshalIndent(l.entries, "", "  ")
	tmp := l.path + ".tmp"
	if os.WriteFile(tmp, data, 0o644) == nil {
		_ = os.Rename(tmp, l.path)
	}
}