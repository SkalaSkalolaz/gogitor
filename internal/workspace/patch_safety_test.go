package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gogitor/internal/domain"
)

func TestPatchProtocolForModel(t *testing.T) {
	tests := []struct {
		provider string
		model    string
		want     PatchProtocol
	}{
		{"ollama", "gemma3:4b", PatchProtocolReplaceOnly},
		{"ollama", "gemma4:12b", PatchProtocolReplaceOnly},
		{"ollama", "gpt-oss:20b", PatchProtocolSearchReplace},
		{"ollama", "qwen3.8:27b", PatchProtocolSearchReplace},
		{"openai-compatible+http://localhost:8000/v1", "local-model", PatchProtocolSearchReplace},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := PatchProtocolForModel(tt.provider, tt.model, "")
			if got != tt.want {
				t.Fatalf("PatchProtocolForModel() = %v, want %v", got, tt.want)
			}
		})
	}

	if got := PatchProtocolForModel("ollama", "gemma3:4b", "search_replace"); got != PatchProtocolSearchReplace {
		t.Fatalf("explicit search_replace override ignored: %v", got)
	}
}

func TestCaptureAndBindSourceSnapshot(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	content := `package main

func main() {
	println("hello")
}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	ws := New(root)
	snapshot, err := ws.CaptureProjectSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot["main.go"] == "" {
		t.Fatal("main.go was not included in snapshot")
	}

	changes := []domain.FileChange{{
		Path: "main.go",
		Patches: []domain.Patch{{
			ReplaceOnly: true,
			Symbol:      "main",
			Replace: `func main() {
	println("world")
}`,
		}},
	}}
	bound := ws.BindSourceSnapshot(changes, snapshot)
	if len(bound) != 1 || len(bound[0].Patches) != 1 {
		t.Fatal("unexpected bound change structure")
	}
	if !bound[0].ExpectedPresent || bound[0].ExpectedAbsent {
		t.Fatalf("unexpected presence flags: %+v", bound[0])
	}
	if bound[0].SourceHash == "" {
		t.Fatal("missing source hash")
	}
	if bound[0].Patches[0].ExpectedSymbolFingerprint == "" {
		t.Fatal("missing symbol fingerprint")
	}
}

func TestPreflightReplaceOnly(t *testing.T) {
	root := t.TempDir()
	content := `package main

func main() {
	println("hello")
}
`
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	ws := New(root)
	snapshot, err := ws.CaptureProjectSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	changes := ws.BindSourceSnapshot([]domain.FileChange{{
		Path: "main.go",
		Patches: []domain.Patch{{
			ReplaceOnly: true,
			Symbol:      "main",
			Replace: `func main() {
	println("world")
}`,
		}},
	}}, snapshot)

	prepared, report, err := ws.PreflightChanges(root, changes, PatchPolicyStrict, 0)
	if err != nil {
		t.Fatalf("preflight failed: %v", err)
	}
	if report.PatchBlocks != 1 {
		t.Fatalf("PatchBlocks = %d, want 1", report.PatchBlocks)
	}
	if prepared[0].Patches[0].Search == "" {
		t.Fatal("REPLACE_ONLY was not resolved to trusted SEARCH")
	}
	if prepared[0].Patches[0].Search != `func main() {
	println("hello")
}` {
		t.Fatalf("unexpected trusted SEARCH: %q", prepared[0].Patches[0].Search)
	}

	updated, err := applyOnePatchWithPolicy(content, prepared[0].Patches[0], PatchPolicyStrict, 0)
	if err != nil {
		t.Fatalf("prepared patch failed: %v", err)
	}
	if !strings.Contains(updated, `println("world")`) {
		t.Fatal("replacement not applied")
	}
}

func TestPreflightRejectsStaleSource(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ws := New(root)
	snapshot, err := ws.CaptureProjectSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	changes := ws.BindSourceSnapshot([]domain.FileChange{{
		Path: "main.go",
		Patches: []domain.Patch{{
			ReplaceOnly: true,
			Symbol:      "main",
			Replace:     "func main() { println(\"world\") }",
		}},
	}}, snapshot)

	if err := os.WriteFile(path, []byte("package main\n\nfunc main() { println(\"changed\") }\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, err = ws.PreflightChanges(root, changes, PatchPolicyStrict, 0)
	if err == nil {
		t.Fatal("expected stale source error")
	}
	if !strings.Contains(err.Error(), "stale change") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSemanticScopeRejectsUnrelatedFunction(t *testing.T) {
	before := `package main

func A() {
	println("a")
}

func B() {
	println("b")
}
`
	after := `package main

func A() {
	println("changed")
}

func B() {
	println("b2")
}
`

	patches := []domain.Patch{{
		Symbol: "A",
		Search: `func A() {
	println("a")
}`,
		Replace: `func A() {
	println("changed")
}`,
	}}
	if err := validateSemanticScope(before, after, patches, "main.go"); err == nil {
		t.Fatal("expected unrelated declaration rejection")
	}
}

func TestFindRebasedBlock(t *testing.T) {
	orig := []string{
		"package main",
		"",
		"func A() {",
		"\tprintln(\"a\")",
		"}",
		"",
		"func B() {",
		"\tprintln(\"b\")",
		"}",
	}
	search := []string{
		"func B() {",
		"\tprintln(\"b\")",
		"}",
	}
	m := findRebasedBlock(orig, search)
	if m == nil {
		t.Fatal("expected rebased block")
	}
	if m.StartLine != 6 {
		t.Fatalf("StartLine = %d, want 6", m.StartLine)
	}
}

func TestFindRebasedBlock_MultipleAnchorsSameBlock(
	t *testing.T,
) {
	orig := []string{
		"package main",
		"",
		"func A() {",
		"\tprintln(\"a\")",
		"}",
		"",
		"func B() {",
		"\tprintln(\"b\")",
		"\tprintln(\"c\")",
		"}",
	}

	search := []string{
		"func B() {",
		"\tprintln(\"b\")",
		"\tprintln(\"c\")",
		"}",
	}

	m := findRebasedBlock(
		orig,
		search,
	)

	if m == nil {
		t.Fatal(
			"expected rebased block",
		)
	}

	if m.StartLine != 6 {
		t.Fatalf(
			"StartLine = %d, want 6",
			m.StartLine,
		)
	}

	if m.Similarity != 1.0 {
		t.Fatalf(
			"Similarity = %.2f, want 1.0",
			m.Similarity,
		)
	}
}

func TestFindRebasedBlock_WhitespaceChange(
	t *testing.T,
) {
	orig := []string{
		"package main",
		"",
		"func B() {",
		"    println(\"b\")",
		"    println(\"c\")",
		"}",
	}

	search := []string{
		"func B() {",
		"\tprintln(\"b\")",
		"\tprintln(\"c\")",
		"}",
	}

	m := findRebasedBlock(
		orig,
		search,
	)

	if m == nil {
		t.Fatal(
			"expected rebased block despite indentation difference",
		)
	}

	if m.StartLine != 2 {
		t.Fatalf(
			"StartLine = %d, want 2",
			m.StartLine,
		)
	}
}

func TestFindRebasedBlock_RejectsAmbiguousLocations(
	t *testing.T,
) {
	orig := []string{
		"package main",
		"",
		"func A() {",
		"\tprintln(\"same\")",
		"}",
		"",
		"func B() {",
		"\tprintln(\"same\")",
		"}",
	}

	search := []string{
		"\tprintln(\"same\")",
		"}",
	}

	m := findRebasedBlock(
		orig,
		search,
	)

	if m != nil {
		t.Fatalf(
			"expected ambiguous REBASE to be rejected, got %+v",
			m,
		)
	}
}

func TestFindRebasedBlock_RejectsWeakMatch(
	t *testing.T,
) {
	orig := []string{
		"package main",
		"",
		"func B() {",
		"\tprintln(\"different\")",
		"\tprintln(\"another\")",
		"}",
	}

	search := []string{
		"func B() {",
		"\tprintln(\"b\")",
		"\tprintln(\"c\")",
		"}",
	}

	m := findRebasedBlock(
		orig,
		search,
	)

	if m != nil {
		t.Fatalf(
			"expected weak REBASE match to be rejected, got %+v",
			m,
		)
	}
}

func TestAffectedPackageDirs(t *testing.T) {
	ws := New(t.TempDir())
	got := ws.AffectedPackageDirs(ws.Root, []domain.FileChange{
		{Path: "internal/app/app.go"},
		{Path: "internal/app/app_test.go"},
		{Path: "main.go"},
		{Path: "README.md"},
	})
	want := []string{".", "internal/app"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestPreflightChanges_AllowsMultiplePatchesWithSameSourceHash(
	t *testing.T,
) {
	root := t.TempDir()

	content := `package main

func main() {
	println("hello")
	println("world")
}
`

	path := filepath.Join(
		root,
		"main.go",
	)

	if err := os.WriteFile(
		path,
		[]byte(content),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	ws := New(root)

	hash :=
		hashBytes([]byte(content))

	changes := []domain.FileChange{
		{
			Path:            "main.go",
			ExpectedPresent: true,
			SourceHash:      hash,
			Patches: []domain.Patch{
				{
					Search:             `println("hello")`,
					Replace:            `println("hi")`,
					ExpectedSourceHash: hash,
				},
				{
					Search:             `println("world")`,
					Replace:            `println("bye")`,
					ExpectedSourceHash: hash,
				},
			},
		},
	}

	prepared, _, err :=
		ws.PreflightChanges(
			root,
			changes,
			PatchPolicyBalanced,
			0,
		)

	if err != nil {
		t.Fatalf(
			"preflight failed: %v",
			err,
		)
	}

	got := prepared[0].Patches

	if len(got) != 2 {
		t.Fatalf(
			"patch count = %d, want 2",
			len(got),
		)
	}
}
