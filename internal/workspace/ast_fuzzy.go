package workspace

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"sort"
	"strings"
)

// astFuzzyMinStructure — минимальная структурная близость.
// Чем выше значение, тем консервативнее AST-aware fuzzy.
const astFuzzyMinStructure = 0.82

type astFragmentShape struct {
	NodeCounts  map[string]int
	TokenCounts map[string]int
	// Последовательность должна совпадать.
	TopLevelKinds []string
}

func findASTAwareBlock(
	origLines,
	searchLines []string,
) *fuzzyMatch {
	if len(searchLines) == 0 ||
		len(searchLines) > len(origLines) {
		return nil
	}

	// Полные declaration-блоки не должны fuzzy-сопоставляться.
	// Для func/type/const/var используется Symbol identity.
	if looksLikeTopLevelDeclaration(searchLines) {
		return nil
	}

	searchSource := strings.Join(
		searchLines,
		"\n",
	)

	searchShape, err :=
		parseASTFragmentShape(searchSource)

	if err != nil {
		return nil
	}

	if len(searchShape.TopLevelKinds) == 0 {
		return nil
	}

	// ------------------------------------------------------------
	// 1. Индексируем строки исходного файла.
	// ------------------------------------------------------------
	frequency := make(map[string][]int)

	for i, line := range origLines {
		key := normalizeLineForCompare(line)

		if strings.TrimSpace(key) == "" {
			continue
		}

		frequency[key] = append(
			frequency[key],
			i,
		)
	}

	// ------------------------------------------------------------
	// 2. Формируем возможные StartLine по уникальным anchors.
	// ------------------------------------------------------------
	starts := make(map[int]struct{})

	for rel, searchLine := range searchLines {
		key := normalizeLineForCompare(searchLine)

		if strings.TrimSpace(key) == "" {
			continue
		}

		positions := frequency[key]

		if len(positions) != 1 {
			continue
		}

		start := positions[0] - rel

		if start < 0 ||
			start+len(searchLines) > len(origLines) {
			continue
		}

		starts[start] = struct{}{}
	}

	if len(starts) == 0 {
		return nil
	}

	type candidate struct {
		start int

		// Буквальное совпадение строк.
		lineSimilarity float64

		// AST/token structural similarity.
		structuralSimilarity float64

		// Финальный confidence, который возвращается
		// в fuzzyMatch.Similarity.
		confidence float64
	}

	candidatesByStart := make(
		map[int]candidate,
	)

	for start := range starts {
		actualLines := origLines[start : start+len(searchLines)]

		lineSim := lineSimilarity(
			actualLines,
			searchLines,
		)

		actualSource := strings.Join(
			actualLines,
			"\n",
		)

		actualShape, err :=
			parseASTFragmentShape(actualSource)

		if err != nil {
			// Не удалось построить Go AST для кандидата.
			// Такой кандидат передаётся legacy fuzzy.
			continue
		}

		// --------------------------------------------------------
		// Структура верхнего уровня обязана совпадать.
		// --------------------------------------------------------
		if !sameTopLevelKinds(
			searchShape.TopLevelKinds,
			actualShape.TopLevelKinds,
		) {
			continue
		}

		structuralSim :=
			astFragmentSimilarity(
				searchShape,
				actualShape,
			)

		// --------------------------------------------------------
		// Это внутренний AST safety gate.
		// --------------------------------------------------------
		if structuralSim <
			astFuzzyMinStructure {
			continue
		}

		// --------------------------------------------------------
		// ФИНАЛЬНАЯ ОЦЕНКА.
		// --------------------------------------------------------
		const structuralWeight = 0.85
		const lineWeight = 0.15

		confidence :=
			structuralSim*structuralWeight +
				lineSim*lineWeight

		current, exists :=
			candidatesByStart[start]

		if !exists ||
			confidence > current.confidence {

			candidatesByStart[start] =
				candidate{
					start:                start,
					lineSimilarity:       lineSim,
					structuralSimilarity: structuralSim,
					confidence:           confidence,
				}
		}
	}

	if len(candidatesByStart) == 0 {
		return nil
	}

	// ------------------------------------------------------------
	// 3. Сортируем по итоговому confidence.
	// ------------------------------------------------------------
	candidates := make(
		[]candidate,
		0,
		len(candidatesByStart),
	)

	for _, c := range candidatesByStart {
		candidates = append(
			candidates,
			c,
		)
	}

	sort.Slice(
		candidates,
		func(i, j int) bool {
			if candidates[i].confidence !=
				candidates[j].confidence {

				return candidates[i].confidence >
					candidates[j].confidence
			}

			if candidates[i].structuralSimilarity !=
				candidates[j].structuralSimilarity {

				return candidates[i].structuralSimilarity >
					candidates[j].structuralSimilarity
			}

			if candidates[i].lineSimilarity !=
				candidates[j].lineSimilarity {

				return candidates[i].lineSimilarity >
					candidates[j].lineSimilarity
			}

			return candidates[i].start <
				candidates[j].start
		},
	)

	best := candidates[0]

	// ------------------------------------------------------------
	// 4. Ambiguity guard.
	// ------------------------------------------------------------
	secondBest := 0.0

	if len(candidates) > 1 {
		secondBest =
			candidates[1].confidence
	}

	return &fuzzyMatch{
		StartLine:  best.start,
		Similarity: best.confidence,
		SecondBest: secondBest,
	}
}

// без требования, чтобы SEARCH сам был полноценным Go-файлом.
func parseASTFragmentShape(
	source string,
) (astFragmentShape, error) {
	source = normalizeNewlines(source)

	shape := astFragmentShape{
		NodeCounts:  make(map[string]int),
		TokenCounts: make(map[string]int),
	}

	wrapped :=
		"package __gogitor_ast_fuzzy__\n\n" +
			"func __gogitor_fragment__() {\n" +
			source +
			"\n}\n"

	fset := token.NewFileSet()

	file, err := parser.ParseFile(
		fset,
		"ast-fuzzy.go",
		[]byte(wrapped),
		parser.ParseComments,
	)

	if err != nil {
		return shape, fmt.Errorf(
			"cannot parse Go fragment: %w",
			err,
		)
	}

	if len(file.Decls) != 1 {
		return shape, fmt.Errorf(
			"unexpected declaration count: %d",
			len(file.Decls),
		)
	}

	fn, ok := file.Decls[0].(*ast.FuncDecl)
	if !ok || fn.Body == nil {
		return shape, fmt.Errorf(
			"fragment wrapper is not a function",
		)
	}

	for _, stmt := range fn.Body.List {
		shape.TopLevelKinds =
			append(
				shape.TopLevelKinds,
				astNodeKind(stmt),
			)

		ast.Inspect(
			stmt,
			func(node ast.Node) bool {
				if node == nil {
					return true
				}

				shape.NodeCounts[astNodeKind(node)]++

				return true
			},
		)
	}

	collectTokenShape(
		source,
		shape.TokenCounts,
	)

	return shape, nil
}

// looksLikeTopLevelDeclaration определяет случаи,
func looksLikeTopLevelDeclaration(
	lines []string,
) bool {
	source :=
		strings.TrimSpace(
			strings.Join(lines, "\n"),
		)

	source = strings.TrimSpace(
		strings.TrimLeft(source, "/"),
	)

	for _, prefix := range []string{
		"func ",
		"type ",
		"const ",
		"var ",
		"import ",
		"package ",
	} {
		if strings.HasPrefix(source, prefix) {
			return true
		}
	}

	return false
}

func astNodeKind(node ast.Node) string {
	return fmt.Sprintf(
		"%T",
		node,
	)
}

// sameTopLevelKinds не позволяет, например,
func sameTopLevelKinds(
	a,
	b []string,
) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

func collectTokenShape(
	source string,
	counts map[string]int,
) {
	source = normalizeNewlines(source)

	fset := token.NewFileSet()

	file := fset.AddFile(
		"ast-fuzzy.go",
		-1,
		len(source),
	)

	var s scanner.Scanner

	s.Init(
		file,
		[]byte(source),
		nil,
		0,
	)

	for {
		_, tok, _ := s.Scan()

		if tok == token.EOF {
			return
		}

		// При mode=0 комментарии не выдаются,
		// но оставляем проверку для безопасности.
		if tok == token.COMMENT {
			continue
		}

		key := tokenShapeKey(tok)

		if key == "" {
			continue
		}

		counts[key]++
	}
}

func tokenShapeKey(tok token.Token) string {
	switch tok {
	case token.IDENT:
		return "IDENT"

	case token.INT,
		token.FLOAT,
		token.IMAG,
		token.CHAR,
		token.STRING:

		return "LIT:" + tok.String()

	default:
		return "TOK:" + tok.String()
	}
}

// astFragmentSimilarity сравнивает две AST shapes.
func astFragmentSimilarity(
	a,
	b astFragmentShape,
) float64 {
	nodeScore :=
		multisetDiceSimilarity(
			a.NodeCounts,
			b.NodeCounts,
		)

	tokenScore :=
		multisetDiceSimilarity(
			a.TokenCounts,
			b.TokenCounts,
		)

	return (nodeScore + tokenScore) / 2
}

func multisetDiceSimilarity(
	a,
	b map[string]int,
) float64 {
	if len(a) == 0 &&
		len(b) == 0 {

		return 1.0
	}

	totalA := 0
	totalB := 0
	common := 0

	for key, count := range a {
		totalA += count

		if other, ok := b[key]; ok {
			if count < other {
				common += count
			} else {
				common += other
			}
		}
	}

	for _, count := range b {
		totalB += count
	}

	if totalA == 0 ||
		totalB == 0 {

		return 0
	}

	return float64(2*common) /
		float64(totalA+totalB)
}
