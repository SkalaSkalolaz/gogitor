package codegen

import (
	"strings"
	"testing"

	"gogitor/internal/domain"
)

func TestParseResponseWithPatches_ReplaceOnly(t *testing.T) {
	response := `--- Patch: main.go ---
--- Symbol: main ---
<<<<<<< REPLACE_ONLY
func main() {
	println("world")
}
>>>>>>> REPLACE_ONLY
`

	files := ParseResponseWithPatches(response)

	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}

	if len(files[0].Patches) != 1 {
		t.Fatalf(
			"expected 1 patch, got %d",
			len(files[0].Patches),
		)
	}

	patch := files[0].Patches[0]

	if !patch.ReplaceOnly {
		t.Fatal("expected ReplaceOnly=true")
	}

	if patch.Symbol != "main" {
		t.Fatalf(
			"Symbol = %q, want main",
			patch.Symbol,
		)
	}

	if patch.Search != "" {
		t.Fatalf(
			"REPLACE_ONLY Search = %q, want empty",
			patch.Search,
		)
	}

	if !strings.Contains(
		patch.Replace,
		`println("world")`,
	) {
		t.Fatalf(
			"unexpected replacement: %q",
			patch.Replace,
		)
	}
}

func TestValidateAcceptsValidReplaceOnly(t *testing.T) {
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

	if err := Validate(
		changes,
		"/tmp/project",
	); err != nil {
		t.Fatalf(
			"unexpected validation error: %v",
			err,
		)
	}
}

func TestValidateRejectsReplaceOnlyWithSearch(t *testing.T) {
	changes := []domain.FileChange{{
		Path: "main.go",
		Patches: []domain.Patch{{
			ReplaceOnly: true,
			Symbol:      "main",
			Search:      "old",
			Replace:     "new",
		}},
	}}

	err := Validate(
		changes,
		"/tmp/project",
	)

	if err == nil {
		t.Fatal(
			"expected REPLACE_ONLY with SEARCH to be rejected",
		)
	}

	if !strings.Contains(
		err.Error(),
		"must not contain SEARCH",
	) {
		t.Fatalf(
			"unexpected error: %v",
			err,
		)
	}
}

func TestValidateRejectsReplaceOnlyWithoutSymbol(
	t *testing.T,
) {
	changes := []domain.FileChange{{
		Path: "main.go",
		Patches: []domain.Patch{{
			ReplaceOnly: true,
			Replace:     `func main() {}`,
		}},
	}}

	err := Validate(
		changes,
		"/tmp/project",
	)

	if err == nil {
		t.Fatal(
			"expected REPLACE_ONLY without Symbol to be rejected",
		)
	}

	if !strings.Contains(
		err.Error(),
		"requires Symbol",
	) {
		t.Fatalf(
			"unexpected error: %v",
			err,
		)
	}
}

func TestParseResponseWithPatches_MixedProtocols(
	t *testing.T,
) {
	response := `--- Patch: main.go ---
--- Symbol: main ---
<<<<<<< REPLACE_ONLY
func main() {
	println("world")
}
>>>>>>> REPLACE_ONLY
<<<<<<< SEARCH
old
=======
new
>>>>>>> REPLACE
`

	files := ParseResponseWithPatches(response)

	if len(files) != 1 {
		t.Fatalf(
			"expected 1 file, got %d",
			len(files),
		)
	}

	if len(files[0].Patches) != 2 {
		t.Fatalf(
			"expected 2 patches, got %d",
			len(files[0].Patches),
		)
	}

	if !files[0].Patches[0].ReplaceOnly {
		t.Fatal("first patch should be REPLACE_ONLY")
	}

	if files[0].Patches[1].ReplaceOnly {
		t.Fatal("second patch should be SEARCH/REPLACE")
	}
}

func TestValidateRejectsEmptyReplaceOnlyBody(
	t *testing.T,
) {
	changes := []domain.FileChange{{
		Path: "main.go",
		Patches: []domain.Patch{{
			ReplaceOnly: true,
			Symbol:      "main",
			Replace:     "   ",
		}},
	}}

	err := Validate(
		changes,
		"/tmp/project",
	)

	if err == nil {
		t.Fatal(
			"expected empty REPLACE_ONLY body rejection",
		)
	}

	if !strings.Contains(
		err.Error(),
		"empty REPLACE_ONLY body",
	) {
		t.Fatalf(
			"unexpected error: %v",
			err,
		)
	}
}
