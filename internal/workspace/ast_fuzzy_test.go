package workspace

import "testing"

func TestFindASTAwareBlock_LiteralChange(t *testing.T) {
	orig := []string{
		"package main",
		"",
		"func process(x int) {",
		"\tif x > 0 {",
		"\t\tvalue := compute(x)",
		"\t\tif value > 0 {",
		"\t\t\tprintln(value)",
		"\t\t}",
		"\t\tprintln(\"done\")",
		"\t}",
		"}",
	}

	search := []string{
		"if x > 0 {",
		"\tvalue := compute(x + 1)",
		"\tif value > 0 {",
		"\t\tprintln(value)",
		"\t}",
		"\tprintln(\"done\")",
		"}",
	}

	match := findASTAwareBlock(
		orig,
		search,
	)

	if match == nil {
		t.Fatal(
			"expected AST-aware fuzzy match",
		)
	}

	// В orig блок начинается с строки 3
	// в zero-based indexing.
	if match.StartLine != 3 {
		t.Fatalf(
			"StartLine = %d, want 3",
			match.StartLine,
		)
	}

	if match.Similarity < 0.80 {
		t.Fatalf(
			"Similarity = %.3f, expected >= 0.80",
			match.Similarity,
		)
	}
}

func TestFindASTAwareBlock_RejectsDifferentStatementKind(
	t *testing.T,
) {
	orig := []string{
		"package main",
		"",
		"func process(x int) {",
		"\tfor ready {",
		"\t\tvalue := compute(x)",
		"\t\treturnValue(value)",
		"\t}",
		"\tprintln(\"done\")",
		"}",
	}

	search := []string{
		"if ready {",
		"\tvalue := compute(x)",
		"\treturnValue(value)",
		"}",
		"println(\"done\")",
	}

	match := findASTAwareBlock(
		orig,
		search,
	)

	if match != nil {
		t.Fatalf(
			"expected AST-aware matcher to reject if/for mismatch, got %+v",
			match,
		)
	}
}

func TestFindASTAwareBlock_RejectsDeclaration(
	t *testing.T,
) {
	orig := []string{
		"package main",
		"",
		"func A() {",
		"}",
		"",
		"func B() {",
		"}",
	}

	search := []string{
		"func B() {",
		"}",
	}

	match := findASTAwareBlock(
		orig,
		search,
	)

	if match != nil {
		t.Fatalf(
			"declarations must not be handled by AST-aware fuzzy: %+v",
			match,
		)
	}
}

func TestFindASTAwareBlock_RejectsWeakStructure(
	t *testing.T,
) {
	orig := []string{
		"package main",
		"",
		"func process(x int) {",
		"\tif ready {",
		"\t\ta := one(x)",
		"\t\tb := two(a)",
		"\t\tprintln(b)",
		"\t}",
		"}",
	}

	search := []string{
		"if ready {",
		"\tresult := one(x)",
		"\tprintln(result)",
		"}",
	}

	match := findASTAwareBlock(
		orig,
		search,
	)

	if match != nil {
		t.Fatalf(
			"expected structurally incompatible fragment to be rejected, got %+v",
			match,
		)
	}
}

func TestFindASTAwareBlock_IdentifierChangesKeepStructure(
	t *testing.T,
) {
	orig := []string{
		"package main",
		"",
		"func process(x int) {",
		"\tif x > 0 {",
		"\t\tvalue := compute(x)",
		"\t\tprintln(value)",
		"\t\tprintln(\"done\")",
		"\t}",
		"}",
	}

	search := []string{
		"if input > 0 {",
		"\tresult := compute(input)",
		"\tprintln(result)",
		"\tprintln(\"done\")",
		"}",
	}

	match := findASTAwareBlock(
		orig,
		search,
	)

	if match == nil {
		t.Fatal(
			"expected identifier-renamed structure to match",
		)
	}

	if match.StartLine != 3 {
		t.Fatalf(
			"StartLine = %d, want 3",
			match.StartLine,
		)
	}
}
