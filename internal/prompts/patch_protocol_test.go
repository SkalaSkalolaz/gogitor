package prompts

import (
	"strings"
	"testing"
)

func TestCodeModifyDiffForModelWithProtocol_ReplaceOnly(
	t *testing.T,
) {
	prompt := CodeModifyDiffForModelWithProtocol(
		"change main",
		"package main",
		"balanced",
		"replace_only",
	)

	for _, want := range []string{
		"PATCH PROTOCOL: REPLACE_ONLY",
		"--- Symbol: FunctionName ---",
		"<<<<<<< REPLACE_ONLY",
		">>>>>>> REPLACE_ONLY",
		"DO NOT output SEARCH",
		"Gogitor will reconstruct SEARCH",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf(
				"prompt does not contain %q",
				want,
			)
		}
	}
}

func TestCodeModifyDiffForModelWithProtocol_SearchReplace(
	t *testing.T,
) {
	prompt := CodeModifyDiffForModelWithProtocol(
		"change main",
		"package main",
		"balanced",
		"search_replace",
	)

	if strings.Contains(
		prompt,
		"PATCH PROTOCOL: REPLACE_ONLY",
	) {
		t.Fatal(
			"search_replace protocol must not add REPLACE_ONLY section",
		)
	}

	if !strings.Contains(
		prompt,
		"SEARCH/REPLACE",
	) {
		t.Fatal(
			"expected base SEARCH/REPLACE instructions",
		)
	}
}

func TestCodeFixPatchWithProtocol_ReplaceOnly(
	t *testing.T,
) {
	prompt := CodeFixPatchWithProtocol(
		"fix main",
		"package main",
		"bad patch",
		"test failure",
		"replace_only",
	)

	for _, want := range []string{
		"PATCH REPAIR PROTOCOL: REPLACE_ONLY",
		"<<<<<<< REPLACE_ONLY",
		">>>>>>> REPLACE_ONLY",
		"Do not reproduce SEARCH",
		"NEVER return a complete file",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf(
				"prompt does not contain %q",
				want,
			)
		}
	}
}

func TestPatchAuditPromptContainsContract(t *testing.T) {
	prompt := PatchAudit(
		"change Handler.ServeHTTP",
		"package main",
		`--- Patch: handler.go ---
--- Symbol: Handler.ServeHTTP ---
<<<<<<< REPLACE_ONLY
func (h *Handler) ServeHTTP() {}
>>>>>>> REPLACE_ONLY`,
	)

	for _, want := range []string{
		"You are a strict code patch auditor",
		`"approved"`,
		`"scope_ok"`,
		`"symbol_ok"`,
		`"unrelated_changes"`,
		"ORIGINAL TASK:",
		"PROJECT SOURCE:",
		"PATCH:",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf(
				"PatchAudit prompt does not contain %q",
				want,
			)
		}
	}
}

func TestCodeModifyDiffForModelWithProtocol_OverrideIsCaseInsensitive(
	t *testing.T,
) {
	prompt := CodeModifyDiffForModelWithProtocol(
		"change",
		"package main",
		"balanced",
		"REPLACE_ONLY",
	)

	if !strings.Contains(
		prompt,
		"PATCH PROTOCOL: REPLACE_ONLY",
	) {
		t.Fatal(
			"REPLACE_ONLY protocol should be case-insensitive",
		)
	}
}
