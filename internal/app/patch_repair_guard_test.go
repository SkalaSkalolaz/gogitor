package app

import (
	"testing"

	"gogitor/internal/domain"
)

func TestValidateRepairTargetContractRejectsSymbolDrift(
	t *testing.T,
) {
	previous := []domain.FileChange{
		{
			Path: "internal/repository/paste.go",
			Patches: []domain.Patch{
				{
					Symbol: "Store",
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
					Symbol: "Get",
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
					Symbol: "OldFunction",
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
					Symbol: "RealFunction",
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