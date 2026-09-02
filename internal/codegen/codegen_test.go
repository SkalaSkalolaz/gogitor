package codegen

import (
	"strings"
	"testing"

	"gogitor/internal/domain"
)

func TestParseResponseWithOptions_SingleFile(t *testing.T) {
	response := "--- File: main.go ---\npackage main\n\nfunc main() {}\n"
	files := ParseResponseWithOptions(response, "fallback.go", false)
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].Path != "main.go" {
		t.Errorf("path = %q, want main.go", files[0].Path)
	}
	if !strings.Contains(files[0].Content, "package main") {
		t.Error("expected package main in content")
	}
}

func TestParseResponseWithOptions_MultipleFiles(t *testing.T) {
	response := "--- File: main.go ---\npackage main\n--- File: util.go ---\npackage util\n"
	files := ParseResponseWithOptions(response, "", false)
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
	if files[0].Path != "main.go" || files[1].Path != "util.go" {
		t.Errorf("paths: %q, %q", files[0].Path, files[1].Path)
	}
}

func TestParseResponseWithOptions_Fallback(t *testing.T) {
	response := "package main\n\nfunc main() {}"
	files := ParseResponseWithOptions(response, "main.go", true)
	if len(files) != 1 {
		t.Fatalf("expected 1 file via fallback, got %d", len(files))
	}
	if files[0].Path != "main.go" {
		t.Errorf("fallback path = %q", files[0].Path)
	}
}

func TestParseResponseWithOptions_NoFallback(t *testing.T) {
	response := "package main\n\nfunc main() {}"
	files := ParseResponseWithOptions(response, "main.go", false)
	if len(files) != 0 {
		t.Fatalf("expected 0 files without fallback, got %d", len(files))
	}
}

func TestParseResponseWithPatches_SinglePatch(t *testing.T) {
	response := `--- Patch: main.go ---
<<<<<<< SEARCH
old code
=======
new code
>>>>>>> REPLACE
`
	files := ParseResponseWithPatches(response)
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	f := files[0]
	if f.Path != "main.go" {
		t.Errorf("path = %q", f.Path)
	}
	if !f.PatchMode {
		t.Error("expected PatchMode=true")
	}
	if len(f.Patches) != 1 {
		t.Fatalf("expected 1 patch, got %d", len(f.Patches))
	}
	if f.Patches[0].Search != "old code" {
		t.Errorf("search = %q", f.Patches[0].Search)
	}
	if f.Patches[0].Replace != "new code" {
		t.Errorf("replace = %q", f.Patches[0].Replace)
	}
}

func TestParseResponseWithPatches_MultiplePatches(t *testing.T) {
	response := `--- Patch: main.go ---
<<<<<<< SEARCH
aaa
=======
bbb
>>>>>>> REPLACE
<<<<<<< SEARCH
ccc
=======
ddd
>>>>>>> REPLACE
`
	files := ParseResponseWithPatches(response)
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if len(files[0].Patches) != 2 {
		t.Fatalf("expected 2 patches, got %d", len(files[0].Patches))
	}
}

func TestParseResponseWithPatches_WithSymbol(t *testing.T) {
	response := `--- Patch: main.go ---
--- Symbol: main ---
<<<<<<< SEARCH
old
=======
new
>>>>>>> REPLACE
`
	files := ParseResponseWithPatches(response)
	if len(files) != 1 || len(files[0].Patches) != 1 {
		t.Fatal("unexpected structure")
	}
	if files[0].Patches[0].Symbol != "main" {
		t.Errorf("symbol = %q, want main", files[0].Patches[0].Symbol)
	}
}

func TestParseResponseWithPatches_MixedFileAndPatch(t *testing.T) {
	response := `--- File: new.go ---
package new
--- Patch: existing.go ---
<<<<<<< SEARCH
old
=======
new
>>>>>>> REPLACE
`
	files := ParseResponseWithPatches(response)
	if len(files) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(files))
	}
	if files[0].Path != "new.go" {
		t.Errorf("first path = %q", files[0].Path)
	}
	if files[1].Path != "existing.go" || !files[1].PatchMode {
		t.Errorf("second: path=%q patchMode=%v", files[1].Path, files[1].PatchMode)
	}
}

func TestCleanCode(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no fences", "package main\nfunc main() {}", "package main\nfunc main() {}"},
		{"with fences", "```go\npackage main\n```", "package main"},
		{"with placeholder", "package main\n<code here>\nfunc main() {}", "package main\nfunc main() {}"},
		{"trim empty lines", "\n\npackage main\n\n", "package main"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CleanCode(tt.input)
			if got != tt.want {
				t.Errorf("CleanCode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractFileMarker(t *testing.T) {
	tests := []struct {
		line string
		want string
	}{
		{"--- File: main.go ---", "main.go"},
		{"--- File: internal/app/app.go ---", "internal/app/app.go"},
		{"--- File: ---", ""},
		{"--- File: main.go", ""},
		{"not a marker", ""},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			got := extractFileMarker(tt.line)
			if got != tt.want {
				t.Errorf("extractFileMarker(%q) = %q, want %q", tt.line, got, tt.want)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	t.Run("empty changes", func(t *testing.T) {
		if err := Validate(nil, "/tmp"); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("empty path", func(t *testing.T) {
		if err := Validate([]domain.FileChange{{Path: "", Content: "x"}}, "/tmp"); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("empty content", func(t *testing.T) {
		if err := Validate([]domain.FileChange{{Path: "main.go", Content: ""}}, "/tmp"); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("path traversal", func(t *testing.T) {
		if err := Validate([]domain.FileChange{{Path: "../../etc/passwd", Content: "x"}}, "/tmp"); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("valid", func(t *testing.T) {
		if err := Validate([]domain.FileChange{{Path: "main.go", Content: "package main"}}, "/tmp"); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	t.Run("empty search in patch", func(t *testing.T) {
		if err := Validate([]domain.FileChange{{Path: "main.go", Patches: []domain.Patch{{Search: "", Replace: "x"}}}}, "/tmp"); err == nil {
			t.Error("expected error")
		}
	})
}

func TestFormatChanges(t *testing.T) {
	changes := []domain.FileChange{{Path: "main.go", Content: "package main"}}
	got := FormatChanges(changes)
	if !strings.Contains(got, "--- File: main.go ---") {
		t.Error("expected file marker")
	}
}