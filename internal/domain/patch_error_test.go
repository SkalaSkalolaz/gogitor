package domain

import (
	"errors"
	"testing"
)

func TestPatchError(t *testing.T) {
	err := NewPatchError(
		PatchErrorDuplicateFileChange,
		`duplicate file change for "main.go"`,
	)

	want :=
		`patch_error_code=duplicate_file_change: duplicate file change for "main.go"`

	if err.Error() != want {
		t.Fatalf(
			"error = %q, want %q",
			err.Error(),
			want,
		)
	}
}

func TestPatchErrorCodeFromError(t *testing.T) {
	err := NewPatchError(
		PatchErrorStrictSymbolRequired,
		"strict patch requires Symbol",
	)

	code := PatchErrorCodeFromError(err)

	if code != PatchErrorStrictSymbolRequired {
		t.Fatalf(
			"code = %q, want %q",
			code,
			PatchErrorStrictSymbolRequired,
		)
	}
}

func TestPatchErrorCodeFromWrappedError(t *testing.T) {
	base := NewPatchError(
		PatchErrorDuplicateFileChange,
		"duplicate file change",
	)

	err := errors.New(
		"patch failed: " + base.Error(),
	)

	code := PatchErrorCodeFromText(
		err.Error(),
	)

	if code != PatchErrorDuplicateFileChange {
		t.Fatalf(
			"code = %q, want %q",
			code,
			PatchErrorDuplicateFileChange,
		)
	}
}

func TestPatchErrorCodeFromUnknownText(t *testing.T) {
	code := PatchErrorCodeFromText(
		"ordinary error without patch code",
	)

	if code != "" {
		t.Fatalf(
			"code = %q, want empty",
			code,
		)
	}
}
