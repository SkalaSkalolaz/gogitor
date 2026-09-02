package app

import (
	"strings"
	"testing"
)

func TestParseErrorTrace_Panic(t *testing.T) {
	raw := "panic: runtime error: index out of range [3] with length 2\n\ngoroutine 1 [running]:\nmain.process()\n\t/home/user/project/main.go:42 +0x65\nmain.main()\n\t/home/user/project/main.go:10 +0x25"
	fc := ParseErrorTrace(raw)
	if fc.ErrorType != "panic" {
		t.Errorf("ErrorType = %q", fc.ErrorType)
	}
	if fc.Summary != "runtime error: index out of range [3] with length 2" {
		t.Errorf("Summary = %q", fc.Summary)
	}
	if len(fc.Frames) < 1 {
		t.Fatal("expected frames")
	}
}

func TestParseErrorTrace_BuildError(t *testing.T) {
	fc := ParseErrorTrace("./main.go:5:2: undefined: foo\n./util.go:10:1: syntax error")
	if fc.ErrorType != "build" {
		t.Errorf("ErrorType = %q", fc.ErrorType)
	}
	if len(fc.Frames) < 2 {
		t.Fatalf("expected 2 frames, got %d", len(fc.Frames))
	}
}

func TestParseErrorTrace_TestFailure(t *testing.T) {
	fc := ParseErrorTrace("--- FAIL: TestAdd (0.00s)\n    add_test.go:15: expected 3")
	if fc.ErrorType != "test" {
		t.Errorf("ErrorType = %q", fc.ErrorType)
	}
}

func TestLooksLikeStackTrace(t *testing.T) {
	for _, tc := range []struct {
		text string
		want bool
	}{
		{"panic: runtime error", true},
		{"fatal error: concurrent map writes", true},
		{"goroutine 1 [running]:", true},
		{"runtime error: index out of range", true},
		{"--- FAIL: TestFoo", true},
		{"hello world", false},
		{"", false},
	} {
		if got := looksLikeStackTrace(tc.text); got != tc.want {
			t.Errorf("looksLikeStackTrace(%q) = %v, want %v", tc.text, got, tc.want)
		}
	}
}

func TestIsExternalTracePath(t *testing.T) {
	for _, tc := range []struct {
		path string
		want bool
	}{
		{"/usr/local/go/src/runtime/panic.go", true},
		{"/go/src/net/http/server.go", true},
		{"/home/user/project/vendor/pkg/file.go", true},
		{"/home/user/project/main.go", false},
		{"main.go", false},
	} {
		if got := isExternalTracePath(tc.path); got != tc.want {
			t.Errorf("isExternalTracePath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestBuildFixTask(t *testing.T) {
	fc := &FixContext{
		RawError:  "panic: test",
		ErrorType: "panic",
		Summary:   "test",
		Frames:    []TraceFrame{{Function: "main.main", File: "main.go", Line: 10}},
	}
	task := buildFixTask(fc, []string{"main.go"})
	for _, expected := range []string{"panic: test", "main.go", "ROOT CAUSE"} {
		if !strings.Contains(task, expected) {
			t.Errorf("missing %q in task", expected)
		}
	}
}
