package runner

import (
	"strings"
	"testing"

	"gogitor/internal/domain"
)

func TestParseGoTestOutput(t *testing.T) {
	tests := []struct {
		name       string
		output     string
		wantPassed int
		wantFailed int
	}{
		{"all pass", "--- PASS: TestA (0.00s)\n--- PASS: TestB (0.00s)\n", 2, 0},
		{"mixed", "--- PASS: TestA (0.00s)\n--- FAIL: TestB (0.00s)\n", 1, 1},
		{"all fail", "--- FAIL: TestA (0.00s)\n--- FAIL: TestB (0.00s)\n", 0, 2},
		{"empty", "", 0, 0},
		{"no test files", "?   \tpkg\t[no test files]\n", 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, f := parseGoTestOutput(tt.output)
			if p != tt.wantPassed || f != tt.wantFailed {
				t.Errorf("passed=%d failed=%d, want %d/%d", p, f, tt.wantPassed, tt.wantFailed)
			}
		})
	}
}

func TestParseGoCoverage(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		wantCov float64
		hasOut  bool
	}{
		{"single", "ok  \tpkg\tcoverage: 85.5% of statements", 85.5, true},
		{"multiple", "ok \tpkg1\tcoverage: 80.0%\nok \tpkg2\tcoverage: 60.0%", 70.0, true},
		{"none", "ok  \tpkg\t0.001s", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cov, out := parseGoCoverage(tt.output)
			if cov != tt.wantCov {
				t.Errorf("coverage = %f, want %f", cov, tt.wantCov)
			}
			if (out != "") != tt.hasOut {
				t.Errorf("output present = %v, want %v", out != "", tt.hasOut)
			}
		})
	}
}

func TestSanitizeModuleName(t *testing.T) {
	tests := []struct{ input, want string }{
		{"MyProject", "myproject"},
		{"my project", "my-project"},
		{"my/project", "my-project"},
		{"", "app"},
		{".", "app"},
		{"/", "app"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := sanitizeModuleName(tt.input); got != tt.want {
				t.Errorf("sanitizeModuleName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseGoBuildErrors(t *testing.T) {
	got := parseGoBuildErrors("./main.go:5:2: undefined: foo\n./util.go:10:1: syntax error")
	if got == "" {
		t.Fatal("expected non-empty")
	}
	lines := strings.Split(got, "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines, got %d", len(lines))
	}
	if parseGoBuildErrors("no errors here") != "" {
		t.Error("expected empty for no .go: lines")
	}
}

func TestCountLintIssues(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   int
	}{
		{"issues", "main.go:10:2: comment [revive]\nutil.go:5:1: errcheck [errcheck]", 2},
		{"empty", "", 0},
		{"warnings", "WARN [runner] something\nlevel=warning", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CountLintIssues(tt.output); got != tt.want {
				t.Errorf("CountLintIssues() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestParseTidyOutput(t *testing.T) {
	if parseTidyOutput("") != "" {
		t.Error("expected empty for empty input")
	}
	if parseTidyOutput("go: downloading github.com/pkg/errors v0.9.1") == "" {
		t.Error("expected non-empty for download line")
	}
	if parseTidyOutput("irrelevant output") != "" {
		t.Error("expected empty for irrelevant output")
	}
}

func TestDetectExternalImport(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"external", "package main\nimport \"github.com/pkg/errors\"", true},
		{"stdlib", "package main\nimport \"fmt\"\nimport \"os\"", false},
		{"internal", "package main\nimport \"internal/pkg\"", false},
		{"comment", "package main\n// import \"github.com/pkg/errors\"", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detectExternalImport(tt.content); got != tt.want {
				t.Errorf("detectExternalImport() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFunctionFromTestName(t *testing.T) {
	tests := []struct{ input, want string }{
		{"TestMyFunction", "MyFunction"},
		{"TestMyFunction/subcase", "MyFunction"},
		{"ExampleMyFunction", "MyFunction"},
		{"BenchmarkMyFunction", "MyFunction"},
		{"FuzzMyFunction", "MyFunction"},
		{"Test", "Test"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := functionFromTestName(tt.input); got != tt.want {
				t.Errorf("functionFromTestName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatFeedback(t *testing.T) {
	t.Run("all passed", func(t *testing.T) {
		got := FormatFeedback(domain.TestsStatus{Passed: 5, Output: "ok"})
		if !strings.Contains(got, "All tests passed") {
			t.Error("expected 'All tests passed'")
		}
	})
	t.Run("with failures", func(t *testing.T) {
		got := FormatFeedback(domain.TestsStatus{
			Passed: 1, Failed: 1,
			Failures: []domain.TestFailure{{Test: "TestFoo", File: "foo_test.go", Line: 42, Message: "expected 1, got 2"}},
		})
		if !strings.Contains(got, "TEST FAILURES") || !strings.Contains(got, "TestFoo") {
			t.Error("expected failure details")
		}
	})
}

func TestParseGoTestFailures(t *testing.T) {
	output := "=== RUN   TestAdd\n    add_test.go:15: expected 3, got 4\n--- FAIL: TestAdd (0.00s)\n"
	failures := parseGoTestFailures(output)
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(failures))
	}
	f := failures[0]
	if f.Test != "TestAdd" {
		t.Errorf("test = %q", f.Test)
	}
	if f.Function != "Add" {
		t.Errorf("function = %q", f.Function)
	}
	if f.File != "add_test.go" || f.Line != 15 {
		t.Errorf("location = %q:%d", f.File, f.Line)
	}
}