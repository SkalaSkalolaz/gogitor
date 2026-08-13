package workspace

import (
	"testing"

	"gogitor/internal/domain"
)

func TestApplyOnePatch_ExactMatch(t *testing.T) {
	content := "package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n"
	p := domain.Patch{
		Search:  "func main() {\n\tprintln(\"hello\")\n}",
		Replace: "func main() {\n\tprintln(\"world\")\n}",
	}
	result, err := applyOnePatch(content, p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "package main\n\nfunc main() {\n\tprintln(\"world\")\n}\n"
	if result != expected {
		t.Errorf("got:\n%q\nwant:\n%q", result, expected)
	}
}

func TestApplyOnePatch_TrailingWhitespaceTolerance(t *testing.T) {
	content := "package main\n\nfunc main() {   \n\tprintln(\"hello\")  \n}\n"
	p := domain.Patch{
		Search:  "func main() {\n\tprintln(\"hello\")\n}",
		Replace: "func main() {\n\tprintln(\"world\")\n}",
	}
	result, err := applyOnePatch(content, p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Fatal("empty result")
	}
}

func TestApplyOnePatch_IndentDifference(t *testing.T) {
	// LLM вернула SEARCH с другим отступом (4 пробела вместо таба)
	content := "package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n"
	p := domain.Patch{
		Search:  "func main() {\n    println(\"hello\")\n}",
		Replace: "func main() {\n    println(\"world\")\n}",
	}
	result, err := applyOnePatch(content, p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Fatal("empty result")
	}
}

func TestApplyOnePatch_TabsVsSpaces(t *testing.T) {
	// В файле табы, LLM вернула пробелы
	content := "package main\n\nfunc foo() {\n\tif true {\n\t\treturn\n\t}\n}\n"
	p := domain.Patch{
		Search:  "func foo() {\n    if true {\n        return\n    }\n}",
		Replace: "func foo() {\n    if false {\n        return\n    }\n}",
	}
	result, err := applyOnePatch(content, p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Fatal("empty result")
	}
}

func TestApplyOnePatch_FuzzyMatch(t *testing.T) {
	// Одна строка отличается (LLM ошиблась в комментарии)
	content := "package main\n\n// This is a helper function\nfunc helper() {\n\treturn\n}\n"
	p := domain.Patch{
		Search:  "// This is a helper\nfunc helper() {\n\treturn\n}",
		Replace: "// Updated helper\nfunc helper() {\n\treturn\n}",
	}
	result, err := applyOnePatch(content, p)
	if err != nil {
		t.Fatalf("fuzzy match should succeed: %v", err)
	}
	if result == "" {
		t.Fatal("empty result")
	}
}

func TestApplyOnePatch_NormalizedMatch(t *testing.T) {
	// LLM полностью изменила отступы (все строки без отступа)
	content := "package main\n\nfunc main() {\n\tprintln(\"hello\")\n\tprintln(\"world\")\n}\n"
	p := domain.Patch{
		Search:  "func main() {\nprintln(\"hello\")\nprintln(\"world\")\n}",
		Replace: "func main() {\nprintln(\"updated\")\n}",
	}
	result, err := applyOnePatch(content, p)
	if err != nil {
		t.Fatalf("normalized match should succeed: %v", err)
	}
	if result == "" {
		t.Fatal("empty result")
	}
}

func TestApplyOnePatch_NotFound(t *testing.T) {
	content := "package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n"
	p := domain.Patch{
		Search:  "func nonexistent() {\n\treturn\n}",
		Replace: "func nonexistent() {\n\treturn 42\n}",
	}
	_, err := applyOnePatch(content, p)
	if err == nil {
		t.Fatal("expected error for non-existent SEARCH block")
	}
}

func TestApplyOnePatch_Ambiguous(t *testing.T) {
	content := "package main\n\nfunc a() {\n\treturn\n}\n\nfunc b() {\n\treturn\n}\n"
	p := domain.Patch{
		Search:  "func a() {\n\treturn\n}",
		Replace: "func a() {\n\treturn 1\n}",
	}
	// Это не должно быть ambiguous, т.к. func a() уникален
	_, err := applyOnePatch(content, p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApplyOnePatch_EmptySearch(t *testing.T) {
	content := "package main\n"
	p := domain.Patch{
		Search:  "",
		Replace: "something",
	}
	_, err := applyOnePatch(content, p)
	if err == nil {
		t.Fatal("expected error for empty SEARCH")
	}
}

func TestRelaxedLineEqual(t *testing.T) {
	tests := []struct {
		name     string
		a, b     string
		trailMax int
		indMax   int
		want     bool
	}{
		{"exact", "hello", "hello", 8, 12, true},
		{"trailing ws", "hello   ", "hello", 8, 12, true},
		{"trailing ws over", "hello          ", "hello", 8, 12, false},
		{"indent diff ok", "    hello", "\thello", 8, 12, true},
		{"indent diff too big", "            hello", "hello", 8, 4, false},
		{"different content", "hello", "world", 8, 12, false},
		{"tabs vs spaces", "\t\tcode", "        code", 8, 12, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := relaxedLineEqual(tt.a, tt.b, tt.trailMax, tt.indMax)
			if got != tt.want {
				t.Errorf("relaxedLineEqual(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestNormalizeLineForCompare(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"\thello", "hello"},
		{"    hello   ", "hello"},
		{"\t\tcode", "code"},
		{"no indent", "no indent"},
	}
	for _, tt := range tests {
		got := normalizeLineForCompare(tt.input)
		if got != tt.want {
			t.Errorf("normalizeLineForCompare(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestLineSimilarity(t *testing.T) {
	a := []string{"func main() {", "\tprintln(\"hello\")", "}"}
	b := []string{"func main() {", "\tprintln(\"world\")", "}"}
	sim := lineSimilarity(a, b)
	// 2 из 3 строк совпадают
	if sim < 0.6 || sim > 0.7 {
		t.Errorf("expected similarity ~0.667, got %f", sim)
	}

	// Полное совпадение
	sim2 := lineSimilarity(a, a)
	if sim2 != 1.0 {
		t.Errorf("expected 1.0, got %f", sim2)
	}
}

func TestFindClosestBlock(t *testing.T) {
	orig := []string{
		"package main",
		"",
		"func main() {",
		"\tprintln(\"hello\")",
		"}",
	}
	search := []string{
		"func main() {",
		"\tprintln(\"world\")", // отличается
		"}",
	}
	match := findClosestBlock(orig, search, 0.60)
	if match == nil {
		t.Fatal("expected fuzzy match")
	}
	if match.StartLine != 2 {
		t.Errorf("expected start at line 2, got %d", match.StartLine)
	}
}

func TestApplyOnePatch_StrictRejectsFuzzy(t *testing.T) {
	content := "package main\n\n// This is a helper function\nfunc helper() {\n\treturn\n}\n"

	p := domain.Patch{
		Search:  "// This is a helper\nfunc helper() {\n\treturn\n}",
		Replace: "// Updated helper\nfunc helper() {\n\treturn\n}",
	}

	_, err := applyOnePatchWithPolicy(
		content,
		p,
		PatchPolicyStrict,
		0,
	)

	if err == nil {
		t.Fatal("strict policy must reject fuzzy-only patch")
	}
}

func TestApplyOnePatch_SymbolAnchor(t *testing.T) {
	content := `package main

func first() {
	fmt.Println("same")
}

func second() {
	fmt.Println("same")
}
`

	p := domain.Patch{
		Symbol:  "second",
		Search:  `fmt.Println("same")`,
		Replace: `fmt.Println("changed")`,
	}

	result, err := applyOnePatchWithPolicy(
		content,
		p,
		PatchPolicyStrict,
		0,
	)
	if err != nil {
		t.Fatalf("symbol patch failed: %v", err)
	}

	if !strings.Contains(result, `func first() {
	fmt.Println("same")
}`) {
		t.Fatal("first function was unexpectedly changed")
	}

	if !strings.Contains(result, `func second() {
	fmt.Println("changed")
}`) {
		t.Fatal("second function was not changed")
	}
}

func TestPatchPolicyForModel(t *testing.T) {
	tests := []struct {
		provider string
		model    string
		want     PatchPolicy
	}{
		{
			provider: "ollama",
			model:    "qwen3-coder:8b",
			want:     PatchPolicyStrict,
		},
		{
			provider: "ollama",
			model:    "qwen3-coder:14b",
			want:     PatchPolicyStrict,
		},
		{
			provider: "ollama",
			model:    "qwen3-coder:30b",
			want:     PatchPolicyBalanced,
		},
		{
			provider: "openai-compatible+http://localhost:8000/v1",
			model:    "local-model",
			want:     PatchPolicyAdvanced,
		},
		{
			provider: "ollama",
			model:    "gemma4:31b-cloud",
			want:     PatchPolicyAdvanced,
		},
	}

	for _, tt := range tests {
		got := PatchPolicyForModel(
			tt.provider,
			tt.model,
		)

		if got != tt.want {
			t.Errorf(
				"PatchPolicyForModel(%q, %q) = %v, want %v",
				tt.provider,
				tt.model,
				got,
				tt.want,
			)
		}
	}
}

func TestFuzzyMarginRejectsAmbiguousMatch(t *testing.T) {
	orig := []string{
		"func a() {",
		"\treturn value",
		"}",
		"",
		"func b() {",
		"\treturn value",
		"}",
	}

	search := []string{
		"func x() {",
		"\treturn value",
		"}",
	}

	match := findClosestBlockWithMargin(
		orig,
		search,
		0.80,
	)

	if match == nil {
		t.Fatal("expected candidate")
	}

	if match.SecondBest <= 0 {
		t.Fatal("expected second-best candidate")
	}

	if match.Similarity-match.SecondBest >= 0.08 {
		t.Fatalf(
			"expected ambiguous margin, got %.2f",
			match.Similarity-match.SecondBest,
		)
	}
}