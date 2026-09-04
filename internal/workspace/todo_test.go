package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLineCommentStart(t *testing.T) {
	for _, tc := range []struct {
		name, line string
		want       int
	}{
		{"at start", "// comment", 0},
		{"after code", "code // comment", 5},
		{"no comment", "no comment here", -1},
		{"in string", `"not // a comment"`, -1},
		{"after string", `x := "str" // real`, 11},
		{"empty", "", -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := lineCommentStart(tc.line); got != tc.want {
				t.Errorf("lineCommentStart(%q) = %d, want %d", tc.line, got, tc.want)
			}
		})
	}
}

func TestScanFileTODOs(t *testing.T) {
	dir := t.TempDir()
	content := "package main\n\n// TODO: implement this\nfunc foo() {}\n\n// FIXME: broken\nfunc bar() {}\n\n// HACK: workaround\nfunc baz() {}\n\n// Normal comment\nfunc qux() {}\n"
	path := filepath.Join(dir, "main.go")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	items := scanFileTODOs(path, "main.go", 10)
	if len(items) != 3 {
		t.Fatalf("expected 3, got %d", len(items))
	}
	if items[0].Kind != "TODO" || items[1].Kind != "FIXME" || items[2].Kind != "HACK" {
		t.Error("kinds mismatch")
	}
}

func TestScanFileTODOs_Limit(t *testing.T) {
	dir := t.TempDir()
	content := "package main\n// TODO: first\n// TODO: second\n// TODO: third\n"
	path := filepath.Join(dir, "main.go")
	os.WriteFile(path, []byte(content), 0o644)
	if items := scanFileTODOs(path, "main.go", 2); len(items) != 2 {
		t.Errorf("expected 2, got %d", len(items))
	}
}

func TestFormatTODOs(t *testing.T) {
	items := []TODOItem{
		{File: "main.go", Line: 10, Kind: "TODO", Text: "implement"},
		{File: "util.go", Line: 20, Kind: "FIXME", Text: "fix"},
	}
	if FormatTODOs(items) == "" {
		t.Error("empty")
	}
	if FormatTODOs(nil) != "" {
		t.Error("nil should return empty")
	}
}
