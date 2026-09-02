package workspace

import (
	"fmt"
	"strings"
	"testing"

	"gogitor/internal/domain"
)

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
}

func TestStrictPatchDoesNotUseFuzzy(t *testing.T) {
	content := `package main

func main() {
	fmt.Println("hello")
}
`

	patch := domain.Patch{
		Symbol:  "main",
		Search:  `func main() {
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