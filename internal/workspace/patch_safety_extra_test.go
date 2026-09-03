package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gogitor/internal/domain"
)

func TestValidateImportGuardRejectsUnapprovedImportChange(t *testing.T) {
	before := `package main

import "fmt"

func main() {
	fmt.Println("hello")
}
`
	after := `package main

import (
	"fmt"
	"log"
)

func main() {
	fmt.Println("hello")
}
`

	patches := []domain.Patch{{
		Symbol: "main",
		Search: `func main() {
	fmt.Println("hello")
}`,
		Replace: `func main() {
	fmt.Println("hello")
}`,
	}}

	if err := validateImportGuard(
		before,
		after,
		patches,
		"main.go",
	); err == nil {
		t.Fatal("expected unapproved import change to be rejected")
	}
}

func TestValidateImportGuardAcceptsExplicitImportPatch(t *testing.T) {
	before := `package main

import "fmt"

func main() {
	fmt.Println("hello")
}
`
	after := `package main

import "log"

func main() {
	fmt.Println("hello")
}
`

	patches := []domain.Patch{{
		Search:  `import "fmt"`,
		Replace: `import "log"`,
	}}

	if err := validateImportGuard(
		before,
		after,
		patches,
		"main.go",
	); err != nil {
		t.Fatalf("unexpected import guard error: %v", err)
	}
}

func TestValidatePublicAPIGuardRejectsUnapprovedExportedChange(
	t *testing.T,
) {
	before := `package api

func Allowed() {}

func Other() {}
`

	after := `package api

func Allowed() {}

func Other() {
	println("changed")
}
`

	patches := []domain.Patch{{
		Symbol:  "Allowed",
		Search:  `func Allowed() {}`,
		Replace: `func Allowed() {}`,
	}}

	if err := validatePublicAPIGuard(
		before,
		after,
		patches,
		"api.go",
	); err == nil {
		t.Fatal("expected unrelated exported API change to be rejected")
	}
}

func TestValidatePublicAPIGuardAcceptsApprovedExportedChange(
	t *testing.T,
) {
	before := `package api

func Allowed() {}
`
	after := `package api

func Allowed() {
	println("changed")
}
`

	patches := []domain.Patch{{
		Symbol: "Allowed",
		Search: `func Allowed() {}`,
		Replace: `func Allowed() {
	println("changed")
}`,
	}}

	if err := validatePublicAPIGuard(
		before,
		after,
		patches,
		"api.go",
	); err != nil {
		t.Fatalf("unexpected public API guard error: %v", err)
	}
}

func TestValidateGoModGuardRejectsUnapprovedModuleChange(t *testing.T) {
	before := `module example.com/old

go 1.25
`
	after := `module example.com/new

go 1.25
`

	patches := []domain.Patch{{
		Search:  "go 1.25",
		Replace: "go 1.25",
	}}

	if err := validateGoModGuard(
		before,
		after,
		patches,
		"go.mod",
	); err == nil {
		t.Fatal("expected module-path change to be rejected")
	}
}

func TestValidateGoModGuardAcceptsExplicitModulePatch(t *testing.T) {
	before := `module example.com/old

go 1.25
`
	after := `module example.com/new

go 1.25
`

	patches := []domain.Patch{{
		Search:  "module example.com/old",
		Replace: "module example.com/new",
	}}

	if err := validateGoModGuard(
		before,
		after,
		patches,
		"go.mod",
	); err != nil {
		t.Fatalf("unexpected go.mod guard error: %v", err)
	}
}

func TestPreflightRejectsDeletedExpectedFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")

	content := `package main

func main() {}
`

	if err := os.WriteFile(
		path,
		[]byte(content),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	ws := New(root)

	snapshot, err :=
		ws.CaptureProjectSnapshot()
	if err != nil {
		t.Fatal(err)
	}

	changes := ws.BindSourceSnapshot(
		[]domain.FileChange{{
			Path:    "main.go",
			Content: content,
		}},
		snapshot,
	)

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	_, _, err = ws.PreflightChanges(
		root,
		changes,
		PatchPolicyStrict,
		0,
	)

	if err == nil {
		t.Fatal("expected deleted expected file to be rejected")
	}

	if !strings.Contains(
		err.Error(),
		"file disappeared after source snapshot",
	) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPreflightRejectsAppearedExpectedAbsentFile(t *testing.T) {
	root := t.TempDir()
	ws := New(root)

	snapshot, err :=
		ws.CaptureProjectSnapshot()
	if err != nil {
		t.Fatal(err)
	}

	changes := ws.BindSourceSnapshot(
		[]domain.FileChange{{
			Path:    "new.go",
			Content: "package main\n",
		}},
		snapshot,
	)

	path := filepath.Join(root, "new.go")

	if err := os.WriteFile(
		path,
		[]byte("package main\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	_, _, err = ws.PreflightChanges(
		root,
		changes,
		PatchPolicyStrict,
		0,
	)

	if err == nil {
		t.Fatal("expected appeared file to be rejected")
	}

	if !strings.Contains(
		err.Error(),
		"file appeared after source snapshot",
	) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPreparePatchRejectsSymbolMismatch(t *testing.T) {
	content := `package main

func main() {}
`

	patch := domain.Patch{
		ReplaceOnly: true,
		Symbol:      "main",
		Replace:     `func helper() {}`,
	}

	_, err := preparePatch(content, patch)
	if err == nil {
		t.Fatal("expected Symbol mismatch to be rejected")
	}

	if !strings.Contains(
		err.Error(),
		"does not match target Symbol",
	) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPatchLimits(t *testing.T) {
	tests := []struct {
		name       string
		policy     PatchPolicy
		wantBlocks int
		wantLines  int
		wantBytes  int
	}{
		{
			name:       "strict",
			policy:     PatchPolicyStrict,
			wantBlocks: 6,
			wantLines:  120,
			wantBytes:  12 * 1024,
		},
		{
			name:       "balanced",
			policy:     PatchPolicyBalanced,
			wantBlocks: 10,
			wantLines:  300,
			wantBytes:  24 * 1024,
		},
		{
			name:       "advanced",
			policy:     PatchPolicyAdvanced,
			wantBlocks: 16,
			wantLines:  600,
			wantBytes:  48 * 1024,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotBlocks, gotLines, gotBytes :=
				patchLimits(tt.policy)

			if gotBlocks != tt.wantBlocks {
				t.Fatalf(
					"maxBlocks = %d, want %d",
					gotBlocks,
					tt.wantBlocks,
				)
			}

			if gotLines != tt.wantLines {
				t.Fatalf(
					"maxLines = %d, want %d",
					gotLines,
					tt.wantLines,
				)
			}

			if gotBytes != tt.wantBytes {
				t.Fatalf(
					"maxBytes = %d, want %d",
					gotBytes,
					tt.wantBytes,
				)
			}
		})
	}
}

func TestFuzzyThresholds(t *testing.T) {
	tests := []struct {
		name      string
		policy    PatchPolicy
		threshold float64
		margin    float64
	}{
		{
			name:      "balanced",
			policy:    PatchPolicyBalanced,
			threshold: 0.82,
			margin:    0.08,
		},
		{
			name:      "advanced",
			policy:    PatchPolicyAdvanced,
			threshold: 0.85,
			margin:    0.05,
		},
		{
			name:      "strict",
			policy:    PatchPolicyStrict,
			threshold: 1.01,
			margin:    1.00,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotThreshold, gotMargin :=
				fuzzyThresholds(tt.policy, 0)

			if gotThreshold != tt.threshold {
				t.Fatalf(
					"threshold = %.2f, want %.2f",
					gotThreshold,
					tt.threshold,
				)
			}

			if gotMargin != tt.margin {
				t.Fatalf(
					"margin = %.2f, want %.2f",
					gotMargin,
					tt.margin,
				)
			}
		})
	}
}

func TestPreflightRejectsTooManyPatchBlocks(
	t *testing.T,
) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")

	lines := []string{
		"package main",
		"",
	}

	var patches []domain.Patch

	for i := 0; i < 11; i++ {
		oldLine := "// patch-line-" +
			string(rune('a'+i))

		newLine := "// changed-line-" +
			string(rune('a'+i))

		lines = append(
			lines,
			oldLine,
		)

		patches = append(
			patches,
			domain.Patch{
				Search:  oldLine,
				Replace: newLine,
			},
		)
	}

	lines = append(
		lines,
		"func main() {}",
	)

	content := strings.Join(
		lines,
		"\n",
	)

	if err := os.WriteFile(
		path,
		[]byte(content),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	ws := New(root)

	snapshot, err :=
		ws.CaptureProjectSnapshot()
	if err != nil {
		t.Fatal(err)
	}

	changes := ws.BindSourceSnapshot(
		[]domain.FileChange{{
			Path:    "main.go",
			Patches: patches,
		}},
		snapshot,
	)

	_, _, err = ws.PreflightChanges(
		root,
		changes,
		PatchPolicyBalanced,
		0,
	)

	if err == nil {
		t.Fatal("expected patch block limit rejection")
	}

	if !strings.Contains(
		err.Error(),
		"too many patch blocks",
	) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCopyToRootSafeRejectsStaleSource(
	t *testing.T,
) {
	root := t.TempDir()
	sandbox := t.TempDir()

	path := filepath.Join(root, "main.go")
	sandboxPath := filepath.Join(sandbox, "main.go")

	original := `package main

func main() {}
`

	changed := `package main

func main() {
	println("changed")
}
`

	if err := os.WriteFile(
		path,
		[]byte(original),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		sandboxPath,
		[]byte(changed),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	ws := New(root)

	snapshot, err :=
		ws.CaptureProjectSnapshot()
	if err != nil {
		t.Fatal(err)
	}

	changes := ws.BindSourceSnapshot(
		[]domain.FileChange{{
			Path:    "main.go",
			Content: changed,
		}},
		snapshot,
	)

	if err := os.WriteFile(
		path,
		[]byte(original+"// concurrent change\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	err = ws.CopyToRootSafe(
		sandbox,
		changes,
	)

	if err == nil {
		t.Fatal("expected stale source rejection")
	}

	if !strings.Contains(
		err.Error(),
		"source file changed after snapshot",
	) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateExpectedSourceAcceptsStableFile(
	t *testing.T,
) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")

	content := []byte("package main\n")

	if err := os.WriteFile(
		path,
		content,
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	hash := hashBytes(content)

	ch := domain.FileChange{
		Path:            "main.go",
		SourceHash:      hash,
		ExpectedPresent: true,
	}

	got, exists, err :=
		validateExpectedSource(root, ch)

	if err != nil {
		t.Fatal(err)
	}

	if !exists {
		t.Fatal("expected file to exist")
	}

	if string(got) != string(content) {
		t.Fatalf(
			"content = %q, want %q",
			string(got),
			string(content),
		)
	}
}

func TestWorkspaceSnapshotSkipsTemporaryFiles(
	t *testing.T,
) {
	root := t.TempDir()

	files := map[string]string{
		"main.go":             "package main\n",
		"ignored.gogitor.tmp": "temporary\n",
		"ignored.gogitor.bak": "backup\n",
	}

	for name, content := range files {
		if err := os.WriteFile(
			filepath.Join(root, name),
			[]byte(content),
			0o644,
		); err != nil {
			t.Fatal(err)
		}
	}

	ws := New(root)

	snapshot, err :=
		ws.CaptureProjectSnapshot()
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := snapshot["main.go"]; !ok {
		t.Fatal("main.go missing from snapshot")
	}

	if _, ok := snapshot["ignored.gogitor.tmp"]; ok {
		t.Fatal("temporary file must not be included")
	}

	if _, ok := snapshot["ignored.gogitor.bak"]; ok {
		t.Fatal("backup file must not be included")
	}
}
