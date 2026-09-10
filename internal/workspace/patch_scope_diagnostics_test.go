package workspace

import (
	"testing"

	"gogitor/internal/domain"
)

func TestDiagnoseSymbolScopedSearchFailureOutsideSymbol(
	t *testing.T,
) {
	content := `package main

type Store struct {
	value string
}

func (s *Store) Get() string {
	return s.value
}

func unrelated() {
	println("target")
}
`

	err := diagnoseSymbolScopedSearchFailure(
		content,
		"Store",
		`println("target")`,
	)

	if err == nil {
		t.Fatal("expected diagnostic error")
	}

	code :=
		domain.PatchErrorCodeFromError(err)

	if code !=
		domain.PatchErrorSearchOutsideSymbol {

		t.Fatalf(
			"error code = %q, want %q",
			code,
			domain.PatchErrorSearchOutsideSymbol,
		)
	}
}

func TestDiagnoseSymbolScopedSearchFailureCrossesBoundary(
	t *testing.T,
) {
	content := `package main

func main() {
	println("main")
}

func helper() {
	println("helper")
}
`

	err := diagnoseSymbolScopedSearchFailure(
		content,
		"main",
		`}

func helper() {`,
	)

	if err == nil {
		t.Fatal("expected diagnostic error")
	}

	code :=
		domain.PatchErrorCodeFromError(err)

	if code !=
		domain.PatchErrorSearchCrossesSymbolBoundary {

		t.Fatalf(
			"error code = %q, want %q",
			code,
			domain.PatchErrorSearchCrossesSymbolBoundary,
		)
	}
}

func TestDiagnoseSymbolScopedSearchFailureAmbiguous(
	t *testing.T,
) {
	content := `package main

func main() {
	println("x")
	println("x")
}
`

	err := diagnoseSymbolScopedSearchFailure(
		content,
		"main",
		`println("x")`,
	)

	if err == nil {
		t.Fatal("expected diagnostic error")
	}

	code :=
		domain.PatchErrorCodeFromError(err)

	if code !=
		domain.PatchErrorAmbiguousSearch {

		t.Fatalf(
			"error code = %q, want %q",
			code,
			domain.PatchErrorAmbiguousSearch,
		)
	}
}
