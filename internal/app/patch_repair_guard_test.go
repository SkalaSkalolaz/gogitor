package app

import (
	"gogitor/internal/domain"
	"strings"
	"testing"
)

func TestValidateRepairTargetContractRejectsSymbolDrift(
	t *testing.T,
) {
	previous := []domain.FileChange{
		{
			Path: "internal/repository/paste.go",
			Patches: []domain.Patch{
				{
					Symbol:  "Store",
					Search:  "old",
					Replace: "new",
				},
			},
		},
	}

	repaired := []domain.FileChange{
		{
			Path: "internal/repository/paste.go",
			Patches: []domain.Patch{
				{
					Symbol:  "Get",
					Search:  "old",
					Replace: "new",
				},
			},
		},
	}

	err :=
		validateRepairTargetContract(
			previous,
			repaired,
			domain.PatchErrorSearchOutsideSymbol,
		)

	if err == nil {
		t.Fatal("expected symbol drift error")
	}

	code :=
		domain.PatchErrorCodeFromError(err)

	if code !=
		domain.PatchErrorRepairSymbolDrift {

		t.Fatalf(
			"error code = %q, want %q",
			code,
			domain.PatchErrorRepairSymbolDrift,
		)
	}
}

func TestValidateRepairTargetContractAllowsSymbolChangeAfterSymbolNotFound(
	t *testing.T,
) {
	previous := []domain.FileChange{
		{
			Path: "main.go",
			Patches: []domain.Patch{
				{
					Symbol:  "OldFunction",
					Search:  "x",
					Replace: "y",
				},
			},
		},
	}

	repaired := []domain.FileChange{
		{
			Path: "main.go",
			Patches: []domain.Patch{
				{
					Symbol:  "RealFunction",
					Search:  "x",
					Replace: "y",
				},
			},
		},
	}

	err :=
		validateRepairTargetContract(
			previous,
			repaired,
			domain.PatchErrorSymbolNotFound,
		)

	if err != nil {
		t.Fatalf(
			"unexpected error: %v",
			err,
		)
	}
}

func TestValidateRepairTargetContractRejectsFileDrift(
	t *testing.T,
) {
	previous := []domain.FileChange{
		{
			Path: "internal/repository/paste.go",
			Patches: []domain.Patch{
				{
					Symbol: "Store",
				},
			},
		},
	}

	repaired := []domain.FileChange{
		{
			Path: "cmd/main.go",
			Patches: []domain.Patch{
				{
					Symbol: "main",
				},
			},
		},
	}

	err :=
		validateRepairTargetContract(
			previous,
			repaired,
			domain.PatchErrorSearchOutsideSymbol,
		)

	if err == nil {
		t.Fatal("expected file drift error")
	}

	code :=
		domain.PatchErrorCodeFromError(err)

	if code !=
		domain.PatchErrorRepairFileDrift {

		t.Fatalf(
			"error code = %q, want %q",
			code,
			domain.PatchErrorRepairFileDrift,
		)
	}
}

func TestPatchRepairStatePreservesPrimaryErrorAfterInvalidResponse(
	t *testing.T,
) {
	changes := []domain.FileChange{
		{
			Path: "main.go",
			Patches: []domain.Patch{
				{
					Symbol:  "handleListPastes",
					Search:  "old",
					Replace: "new",
				},
			},
		},
	}

	state := patchRepairState{}

	state.rememberRejectedCandidate(
		changes,
		"ORIGINAL PATCH CONTENT",
		"patch_error_code=no_op_patch: patch produced no effective change",
	)

	state.noteInvalidResponse(
		"LLM returned explanation instead of patch",
	)

	if got := state.repairCode(); got != domain.PatchErrorNoOpPatch {

		t.Fatalf(
			"repairCode() = %q, want %q",
			got,
			domain.PatchErrorNoOpPatch,
		)
	}

	if got := state.repairPatch(); got != "ORIGINAL PATCH CONTENT" {

		t.Fatalf(
			"repairPatch() = %q, want original patch",
			got,
		)
	}

	if !state.LastResponseInvalid {
		t.Fatal(
			"expected LastResponseInvalid=true",
		)
	}

	ctx := state.recoveryContext()

	for _, want := range []string{
		"no_op_patch",
		"ORIGINAL PATCH CONTENT",
		"previous repair response was invalid",
	} {
		if !strings.Contains(
			ctx,
			want,
		) {
			t.Fatalf(
				"recovery context missing %q:\n%s",
				want,
				ctx,
			)
		}
	}
}

func TestSplitCompoundAgentSubtask(
	t *testing.T,
) {
	source := `package main

func (pb *Pastebin) handleListPastes() {
}

func (pb *Pastebin) handlePastesPage() {
}
`

	sub := fullPlanSubtask{
		Task: "Update handleListPastes and handlePastesPage in cmd/pastebin/main.go",
		Acceptance: []string{
			"preserve existing behavior",
		},
	}

	got :=
		splitCompoundAgentSubtask(
			sub,
			source,
		)

	if len(got) != 2 {
		t.Fatalf(
			"got %d subtasks, want 2",
			len(got),
		)
	}

	if !strings.Contains(
		got[0].Task,
		"handleListPastes",
	) {
		t.Fatalf(
			"first subtask does not target handleListPastes: %q",
			got[0].Task,
		)
	}

	if strings.Contains(
		got[0].Task,
		"handlePastesPage",
	) {
		t.Fatalf(
			"first subtask still contains second symbol: %q",
			got[0].Task,
		)
	}

	if !strings.Contains(
		got[1].Task,
		"handlePastesPage",
	) {
		t.Fatalf(
			"second subtask does not target handlePastesPage: %q",
			got[1].Task,
		)
	}
}
