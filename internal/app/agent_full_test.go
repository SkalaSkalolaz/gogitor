package app

import (
	"strings"
	"testing"
	"os"
	"path/filepath"

	"gogitor/internal/config"
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

func TestPersistResearchFallback_SavesLongMarkdown(t *testing.T) {
	root := t.TempDir()

	svc := &Service{
		Cfg: &config.Config{WorkDir: root},
	}

	body := strings.Repeat("## Section\n- point\n", 40) // > 200 chars

	path, didSave, err := svc.persistResearchFallback(
		".gogitor/research/api.md",
		1,
		"find API docs",
		domain.Result{Success: true, Response: body},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !didSave {
		t.Fatal("expected fallback to save")
	}
	if path != ".gogitor/research/api.md" {
		t.Fatalf("path = %q", path)
	}
	if _, err := os.Stat(filepath.Join(root, path)); err != nil {
		t.Fatalf("file not created: %v", err)
	}

	// Второй вызов не должен перезаписывать.
	_, didSave2, _ := svc.persistResearchFallback(
		".gogitor/research/api.md", 1, "find API docs",
		domain.Result{Success: true, Response: body},
	)
	if didSave2 {
		t.Fatal("fallback must not overwrite existing file")
	}
}

func TestPersistResearchFallback_RejectsBoilerplate(t *testing.T) {
	svc := &Service{Cfg: &config.Config{WorkDir: t.TempDir()}}

	_, didSave, err := svc.persistResearchFallback(
		".gogitor/research/api.md",
		1,
		"modify code",
		domain.Result{Success: true, Response: "Applied changes: 1 patched (DIFF)."},
	)
	if err != nil {
		t.Fatal(err)
	}
	if didSave {
		t.Fatal("boilerplate must not be saved as research")
	}
}