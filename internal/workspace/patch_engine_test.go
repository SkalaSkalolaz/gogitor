package workspace

import (
	"fmt"
	"strings"
	"testing"

	"gogitor/internal/domain"
)


func TestSemanticScopeAllowsRelatedInterfaceAndImplementationMethod(
	t *testing.T,
) {
	before := `package main

type Repository interface {
	GetByID(id string) error
}

type InMemoryRepository struct{}

func (r *InMemoryRepository) GetByID(id string) error {
	return nil
}
`

	afterInterface := `package main

type Repository interface {
	GetByID(id string) error
	Delete(id string) error
}

type InMemoryRepository struct{}

func (r *InMemoryRepository) GetByID(id string) error {
	return nil
}
`

	afterMethod := `package main

type Repository interface {
	GetByID(id string) error
	Delete(id string) error
}

type InMemoryRepository struct{}

func (r *InMemoryRepository) GetByID(id string) error {
	return nil
}

func (r *InMemoryRepository) Delete(id string) error {
	return nil
}
`

	allowed :=
		make(map[string]bool)

	firstPatch := domain.Patch{
		Symbol: "Repository",
		Search: `type Repository interface {
	GetByID(id string) error
}`,
		Replace: `type Repository interface {
	GetByID(id string) error
	Delete(id string) error
}`,
	}

	if err := addPatchFootprintToScope(
		before,
		afterInterface,
		firstPatch,
		allowed,
	); err != nil {
		t.Fatal(err)
	}

	secondPatch := domain.Patch{
		Symbol: "InMemoryRepository.Delete",
		Replace: `func (r *InMemoryRepository) Delete(id string) error {
	return nil
}`,
	}

	if err := addPatchFootprintToScope(
		afterInterface,
		afterMethod,
		secondPatch,
		allowed,
	); err != nil {
		t.Fatal(err)
	}

	patches := []domain.Patch{
		firstPatch,
		secondPatch,
	}

	if err := validateSemanticScopeWithAllowed(
		before,
		afterMethod,
		patches,
		"repository.go",
		allowed,
	); err != nil {
		t.Fatalf(
			"related structural change rejected: %v",
			err,
		)
	}
}

func TestFindSymbolRangeSupportsTypeDeclaration(
	t *testing.T,
) {
	content := `package main

type Repository interface {
	GetByID(id string) error
}

func main() {}
`

	start, end, err :=
		findSymbolRange(
			content,
			"Repository",
		)

	if err != nil {
		t.Fatalf(
			"findSymbolRange failed: %v",
			err,
		)
	}

	got := content[start:end]

	if !strings.Contains(
		got,
		"type Repository interface",
	) {
		t.Fatalf(
			"unexpected range: %q",
			got,
		)
	}
}

func TestStrictPatchRequiresSymbolForLargeSearch(t *testing.T) {
	content := `package main

func main() {
	fmt.Println("hello")
	fmt.Println("world")
}

`

	patch := domain.Patch{
		Search: `func main() {
	fmt.Println("hello")
	fmt.Println("world")
}`,
		Replace: `func main() {
	fmt.Println("hello")
	fmt.Println("gogitor")
}`,
	}

	_, err := applyOnePatchWithPolicy(
		content,
		patch,
		PatchPolicyStrict,
		0,
	)

	if err == nil {
		t.Fatal("expected strict mode to require Symbol")
	}

	if !strings.Contains(err.Error(), "requires Symbol") {
		t.Fatalf("unexpected error: %v", err)
	}

	if code := domain.PatchErrorCodeFromError(err); code != domain.PatchErrorStrictSymbolRequired {

		t.Fatalf(
			"error code = %q, want %q",
			code,
			domain.PatchErrorStrictSymbolRequired,
		)
	}
}

func TestStrictPatchRejectsLargeSearch(t *testing.T) {
	var lines []string
	for i := 0; i < 11; i++ {
		lines = append(lines, fmt.Sprintf("line%d", i))
	}

	patch := domain.Patch{
		Search:  strings.Join(lines, "\n"),
		Replace: "replacement",
		Symbol:  "main",
	}

	_, err := applyOnePatchWithPolicy(
		strings.Join(lines, "\n"),
		patch,
		PatchPolicyStrict,
		0,
	)

	if err == nil {
		t.Fatal("expected large SEARCH block to be rejected")
	}

	if !strings.Contains(err.Error(), "too large") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(
		err.Error(),
		"patch_error_code=strict_search_too_large",
	) {
		t.Fatalf(
			"expected strict_search_too_large error code, got: %v",
			err,
		)
	}
}

func TestStrictPatchDoesNotUseFuzzy(t *testing.T) {
	content := `package main

func main() {
	fmt.Println("hello")
}
`

	patch := domain.Patch{
		Symbol: "main",
		Search: `func main() {
	fmt.Println("helo")
}`,
		Replace: `func main() {
	fmt.Println("world")
}`,
	}

	_, err := applyOnePatchWithPolicy(
		content,
		patch,
		PatchPolicyStrict,
		0,
	)

	if err == nil {
		t.Fatal("expected fuzzy mismatch to be rejected in strict mode")
	}
}

func TestPatchPolicyForModel_SubstringOverridePrefersSpecificMatch(t *testing.T) {
	overrides := map[string]string{
		"q":    "balanced",
		"qwen": "strict",
	}

	got := PatchPolicyForModel("ollama", "Qwen3:27b", overrides)

	if got != PatchPolicyStrict {
		t.Fatalf(
			"PatchPolicyForModel() = %v, want %v",
			got,
			PatchPolicyStrict,
		)
	}
}

func TestFindSymbolRangeReturnsSymbolNotFoundCode(
	t *testing.T,
) {
	content := `package main

func main() {
	println("hello")
}
`

	_, _, err := findSymbolRange(
		content,
		"ImportBlock",
	)

	if err == nil {
		t.Fatal(
			"expected symbol-not-found error",
		)
	}

	code :=
		domain.PatchErrorCodeFromError(err)

	if code != domain.PatchErrorSymbolNotFound {
		t.Fatalf(
			"code = %q, want %q",
			code,
			domain.PatchErrorSymbolNotFound,
		)
	}
}

func TestApplyPatchesWithPolicyCore_AllowsMultiplePatchesWithSameSourceHash(
	t *testing.T,
) {
	content := `package main

func main() {
	println("hello")
	println("world")
}
`

	sourceHash :=
		hashBytes([]byte(content))

	patches := []domain.Patch{
		{
			Search:             `println("hello")`,
			Replace:            `println("hi")`,
			ExpectedSourceHash: sourceHash,
		},
		{
			Search:             `println("world")`,
			Replace:            `println("bye")`,
			ExpectedSourceHash: sourceHash,
		},
	}

	updated, err :=
		applyPatchesWithPolicyCore(
			content,
			patches,
			PatchPolicyBalanced,
			0,
			domain.DefaultDiffMatchingConfig(),
			"TEST",
			"main.go",
			nil,
		)

	if err != nil {
		t.Fatalf(
			"multiple patches with shared source hash failed: %v",
			err,
		)
	}

	if !strings.Contains(
		updated,
		`println("hi")`,
	) {
		t.Fatal(
			"first patch was not applied",
		)
	}

	if !strings.Contains(
		updated,
		`println("bye")`,
	) {
		t.Fatal(
			"second patch was not applied",
		)
	}
}

func TestApplyPatchesWithPolicyCoreRejectsWrongInitialSourceHash(
	t *testing.T,
) {
	content := `package main

func main() {
	println("hello")
}
`

	wrongHash :=
		hashBytes(
			[]byte("different source"),
		)

	patches := []domain.Patch{
		{
			Search:             `println("hello")`,
			Replace:            `println("world")`,
			ExpectedSourceHash: wrongHash,
		},
		{
			Search:             `println("world")`,
			Replace:            `println("bye")`,
			ExpectedSourceHash: wrongHash,
		},
	}

	_, err :=
		applyPatchesWithPolicyCore(
			content,
			patches,
			PatchPolicyBalanced,
			0,
			domain.DefaultDiffMatchingConfig(),
			"TEST",
			"main.go",
			nil,
		)

	if err == nil {
		t.Fatal(
			"expected initial source hash mismatch",
		)
	}

	if !strings.Contains(
		err.Error(),
		"source hash",
	) {
		t.Fatalf(
			"unexpected error: %v",
			err,
		)
	}
}
