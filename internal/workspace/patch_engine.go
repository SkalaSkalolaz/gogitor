package workspace

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gogitor/internal/domain"
)

type PatchPolicy int

const (
	PatchPolicyStrict PatchPolicy = iota
	PatchPolicyBalanced
	PatchPolicyAdvanced
)

func (p PatchPolicy) String() string {
	switch p {
	case PatchPolicyStrict:
		return "strict"
	case PatchPolicyAdvanced:
		return "advanced"
	default:
		return "balanced"
	}
}

// ParsePatchPolicy преобразует строку из конфига в PatchPolicy.
func ParsePatchPolicy(s string) (PatchPolicy, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "strict":
		return PatchPolicyStrict, true
	case "balanced":
		return PatchPolicyBalanced, true
	case "advanced":
		return PatchPolicyAdvanced, true
	default:
		return PatchPolicyBalanced, false
	}
}

func PatchPolicyForModel(provider, model string, overrides map[string]string) PatchPolicy {
	p := strings.ToLower(strings.TrimSpace(provider))
	m := strings.ToLower(strings.TrimSpace(model))

	// 1. Сначала проверяем пользовательские переопределения (overrides из config.json)
	if len(overrides) > 0 {
		// Приоритет 1: точное совпадение по имени модели
		if policyStr, ok := overrides[m]; ok {
			if pol, valid := ParsePatchPolicy(policyStr); valid {
				return pol
			}
		}
		// Приоритет 2: провайдер + модель (например, "ollama/gemma3:4b")
		if policyStr, ok := overrides[p+"/"+m]; ok {
			if pol, valid := ParsePatchPolicy(policyStr); valid {
				return pol
			}
		}
		// Приоритет 3: только провайдер
		if policyStr, ok := overrides[p]; ok {
			if pol, valid := ParsePatchPolicy(policyStr); valid {
				return pol
			}
		}
		// Приоритет 4: поиск по подстроке в модели.
		type substringOverride struct {
			key    string
			policy string
		}

		var candidates []substringOverride

		for key, policyStr := range overrides {
			normalizedKey := strings.ToLower(strings.TrimSpace(key))

			if normalizedKey == "" ||
				strings.Contains(normalizedKey, "/") ||
				strings.Contains(normalizedKey, "+") {
				continue
			}

			if strings.Contains(m, normalizedKey) {
				candidates = append(candidates, substringOverride{
					key:    normalizedKey,
					policy: policyStr,
				})
			}
		}

		sort.Slice(candidates, func(i, j int) bool {
			if len(candidates[i].key) != len(candidates[j].key) {
				return len(candidates[i].key) > len(candidates[j].key)
			}

			return candidates[i].key < candidates[j].key
		})

		for _, candidate := range candidates {
			if pol, valid := ParsePatchPolicy(candidate.policy); valid {
				return pol
			}
		}
	}

	// 2. Облачные / внешние OpenAI-compatible модели считаем сильными (стандартная логика).
	if strings.HasPrefix(p, "openai+") ||
		strings.HasPrefix(p, "openai-compatible+") ||
		strings.Contains(m, "cloud") {
		return PatchPolicyAdvanced
	}

	// 3. Fallback: эвристика по размеру модели
	size := modelParameterCountB(m)
	if size > 0 {
		switch {
		case size <= 24:
			return PatchPolicyStrict
		case size <= 32:
			return PatchPolicyBalanced
		default:
			return PatchPolicyAdvanced
		}
	}

	return PatchPolicyBalanced
}

// Вспомогательная функция для парсинга строк из JSON
func parsePolicyString(s string) PatchPolicy {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "strict":
		return PatchPolicyStrict
	case "advanced":
		return PatchPolicyAdvanced
	default:
		return PatchPolicyBalanced
	}
}

var patchModelSizeRE = regexp.MustCompile(
	`(?:^|[^0-9])([0-9]+(?:\.[0-9]+)?)b(?:[^a-z0-9]|$)`,
)

func modelParameterCountB(model string) float64 {
	m := patchModelSizeRE.FindStringSubmatch(model)
	if len(m) != 2 {
		return 0
	}

	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil || v <= 0 {
		return 0
	}

	return v
}

type patchMatchResult struct {
	Start      int
	End        int
	Confidence float64
	Method     string
	SecondBest float64
}

func (m patchMatchResult) Margin() float64 {
	if m.SecondBest <= 0 {
		return m.Confidence
	}
	return m.Confidence - m.SecondBest
}

func fuzzyThresholds(policy PatchPolicy, override float64) (threshold, margin float64) {
	switch policy {
	case PatchPolicyAdvanced:
		threshold = 0.85
		margin = 0.05
	case PatchPolicyBalanced:
		threshold = 0.82
		margin = 0.08
	default:
		// Strict mode никогда не применяет fuzzy автоматически.
		threshold = 1.01
		margin = 1.00
	}

	if override > threshold {
		threshold = override
	}

	return threshold, margin
}

func applyPatchesWithPolicy(
	content string,
	patches []domain.Patch,
	policy PatchPolicy,
	minConfidenceOverride float64,
) (string, error) {
	content = normalizeNewlines(content)

	for i, p := range patches {
		updated, err := applyOnePatchWithPolicy(
			content,
			p,
			policy,
			minConfidenceOverride,
		)
		if err != nil {
			return "", fmt.Errorf("patch %d: %w", i+1, err)
		}
		content = updated
	}

	return content, nil
}

// applyOnePatch оставляем как compatibility wrapper,
// чтобы существующие unit-тесты и старый код продолжали работать.
func applyOnePatch(content string, p domain.Patch) (string, error) {
	return applyOnePatchWithPolicy(
		content,
		p,
		PatchPolicyBalanced,
		0,
	)
}

func applyOnePatchWithPolicy(
	content string,
	p domain.Patch,
	policy PatchPolicy,
	minConfidenceOverride float64,
) (string, error) {
	search := trimPatchLines(normalizeNewlines(p.Search))
	replace := trimPatchLines(normalizeNewlines(p.Replace))

	if search == "" {
		return "", fmt.Errorf("empty SEARCH block")
	}

	if patchSearchTooLarge(search, policy) {
		return "", fmt.Errorf(
			"strict SEARCH block is too large: %d lines, maximum 10",
			strings.Count(search, "\n")+1,
		)
	}

	if patchRequiresSymbol(p, policy) &&
		strings.TrimSpace(p.Symbol) == "" {
		return "", fmt.Errorf(
			"strict patch requires Symbol anchor for SEARCH block with %d lines",
			strings.Count(search, "\n")+1,
		)
	}

	if strings.TrimSpace(p.Symbol) != "" {
		start, end, err := findSymbolRange(content, p.Symbol)
		if err != nil {
			return "", err
		}

		local := content[start:end]

		updatedLocal, matched, err := applyPatchText(
			local,
			search,
			replace,
			policy,
			minConfidenceOverride,
		)
		if err == nil && matched {
			return content[:start] + updatedLocal + content[end:], nil
		}

		if err != nil {
			return "", fmt.Errorf(
				"symbol %q: %w",
				p.Symbol,
				err,
			)
		}

		return "", fmt.Errorf(
			"SEARCH block not found inside symbol %q",
			p.Symbol,
		)
	}

	updated, matched, err := applyPatchText(
		content,
		search,
		replace,
		policy,
		minConfidenceOverride,
	)
	if err != nil {
		return "", err
	}
	if !matched {
		return "", fmt.Errorf("SEARCH block not found")
	}

	return updated, nil
}

func applyPatchText(
	content string,
	search string,
	replace string,
	policy PatchPolicy,
	minConfidenceOverride float64,
) (string, bool, error) {
	// ------------------------------------------------------------
	// 1. EXACT
	// ------------------------------------------------------------
	count := strings.Count(content, search)

	if count == 1 {
		return strings.Replace(content, search, replace, 1), true, nil
	}

	if count > 1 {
		return "", false, fmt.Errorf(
			"SEARCH block is ambiguous (%d exact matches)",
			count,
		)
	}

	origLines := strings.Split(content, "\n")
	searchLines := strings.Split(search, "\n")

	// ------------------------------------------------------------
	// 2. RELAXED: tabs/spaces + indentation + trailing whitespace
	// ------------------------------------------------------------
	relaxed := findRelaxedMatches(
		origLines,
		searchLines,
	)

	if len(relaxed) == 1 {
		return replaceLineRange(
			origLines,
			relaxed[0],
			relaxed[0]+len(searchLines),
			replace,
		), true, nil
	}

	if len(relaxed) > 1 {
		return "", false, fmt.Errorf(
			"SEARCH block is ambiguous (%d relaxed matches)",
			len(relaxed),
		)
	}

	// ------------------------------------------------------------
	// 3. NORMALIZED: полностью игнорируем indentation
	// ------------------------------------------------------------
	normalized := findNormalizedMatches(
		origLines,
		searchLines,
	)

	if len(normalized) == 1 {
		return replaceLineRange(
			origLines,
			normalized[0],
			normalized[0]+len(searchLines),
			replace,
		), true, nil
	}

	if len(normalized) > 1 {
		return "", false, fmt.Errorf(
			"SEARCH block is ambiguous (%d normalized matches)",
			len(normalized),
		)
	}

	// ------------------------------------------------------------
	// 4. FUZZY — только Balanced / Advanced
	// ------------------------------------------------------------
	if policy == PatchPolicyStrict {
		return "", false, nil
	}

	// Одинокая fuzzy-строка слишком легко совпадает случайно.
	if len(searchLines) < 2 {
		return "", false, nil
	}

    if !hasUniqueExactAnchor(origLines, searchLines) {
    	return "", false, nil
    }

	threshold, requiredMargin := fuzzyThresholds(
		policy,
		minConfidenceOverride,
	)

	fuzzy := findClosestBlockWithMargin(
		origLines,
		searchLines,
		0.60,
	)

	if fuzzy == nil {
		return "", false, nil
	}

	if fuzzy.Similarity < threshold {
		return "", false, fmt.Errorf(
			"fuzzy SEARCH rejected: confidence %.2f below threshold %.2f",
			fuzzy.Similarity,
			threshold,
		)
	}

	margin := fuzzy.Similarity - fuzzy.SecondBest
	if fuzzy.SecondBest <= 0 {
		margin = fuzzy.Similarity
	}

	if margin < requiredMargin {
		return "", false, fmt.Errorf(
			"fuzzy SEARCH rejected: ambiguous candidates (best %.2f, second %.2f, margin %.2f, required %.2f)",
			fuzzy.Similarity,
			fuzzy.SecondBest,
			margin,
			requiredMargin,
		)
	}

	return replaceLineRange(
		origLines,
		fuzzy.StartLine,
		fuzzy.StartLine+len(searchLines),
		replace,
	), true, nil
}

func hasUniqueExactAnchor(
	origLines,
	searchLines []string,
) bool {
	frequency := make(map[string]int)

	for _, line := range origLines {
		key := normalizeLineForCompare(line)

		if strings.TrimSpace(key) == "" {
			continue
		}

		frequency[key]++
	}

	for _, line := range searchLines {
		key := normalizeLineForCompare(line)

		if strings.TrimSpace(key) == "" {
			continue
		}

		if frequency[key] == 1 {
			return true
		}
	}

	return false
}

func findRelaxedMatches(origLines, searchLines []string) []int {
	if len(searchLines) == 0 ||
		len(searchLines) > len(origLines) {
		return nil
	}

	const maxTrailingWsDiff = 8
	const maxIndentDiff = 12

	var matches []int

	for i := 0; i+len(searchLines) <= len(origLines); i++ {
		ok := true

		for j, searchLine := range searchLines {
			if !relaxedLineEqual(
				origLines[i+j],
				searchLine,
				maxTrailingWsDiff,
				maxIndentDiff,
			) {
				ok = false
				break
			}
		}

		if ok {
			matches = append(matches, i)
		}
	}

	return matches
}

func findNormalizedMatches(origLines, searchLines []string) []int {
	if len(searchLines) == 0 ||
		len(searchLines) > len(origLines) {
		return nil
	}

	normalizedSearch := make([]string, len(searchLines))
	for i, line := range searchLines {
		normalizedSearch[i] = normalizeLineForCompare(line)
	}

	var matches []int

	for i := 0; i+len(searchLines) <= len(origLines); i++ {
		ok := true

		for j, searchNorm := range normalizedSearch {
			if normalizeLineForCompare(origLines[i+j]) != searchNorm {
				ok = false
				break
			}
		}

		if ok {
			matches = append(matches, i)
		}
	}

	return matches
}

func replaceLineRange(
	origLines []string,
	start int,
	end int,
	replace string,
) string {
	var replaceLines []string
	if replace != "" {
		replaceLines = strings.Split(replace, "\n")
	}

	newLines := make(
		[]string,
		0,
		len(origLines)-(end-start)+len(replaceLines),
	)

	newLines = append(
		newLines,
		origLines[:start]...,
	)

	newLines = append(
		newLines,
		replaceLines...,
	)

	newLines = append(
		newLines,
		origLines[end:]...,
	)

	return strings.Join(newLines, "\n")
}

// ------------------------------------------------------------
// AST / Symbol anchor
// ------------------------------------------------------------

func findSymbolRange(content, symbol string) (int, int, error) {
	symbol = normalizePatchSymbol(symbol)

	if symbol == "" {
		return 0, 0, fmt.Errorf("empty symbol anchor")
	}

	fset := token.NewFileSet()

	file, err := parser.ParseFile(
		fset,
		"patch-target.go",
		[]byte(content),
		parser.ParseComments,
	)
	if err != nil {
		return 0, 0, fmt.Errorf(
			"cannot parse Go file for symbol anchor %q: %w",
			symbol,
			err,
		)
	}

	type candidate struct {
		node ast.Node
		name string
	}

	var matches []candidate

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil {
			continue
		}

		name := fn.Name.Name

		if fn.Recv != nil &&
			len(fn.Recv.List) > 0 {
			receiver := receiverTypeName(
				fn.Recv.List[0].Type,
			)
			if receiver != "" {
				name = receiver + "." + name
			}
		}

		if symbol == name ||
			symbol == fn.Name.Name {
			matches = append(
				matches,
				candidate{
					node: fn,
					name: name,
				},
			)
		}
	}

	if len(matches) == 0 {
		return 0, 0, fmt.Errorf(
			"symbol %q not found",
			symbol,
		)
	}

	if len(matches) > 1 {
		return 0, 0, fmt.Errorf(
			"symbol %q is ambiguous (%d matches)",
			symbol,
			len(matches),
		)
	}

	fileInfo := fset.File(file.Pos())
	if fileInfo == nil {
		return 0, 0, fmt.Errorf(
			"cannot resolve token positions for symbol %q",
			symbol,
		)
	}

	start := fileInfo.Offset(matches[0].node.Pos())
	end := fileInfo.Offset(matches[0].node.End())

	if start < 0 ||
		end < start ||
		end > len(content) {
		return 0, 0, fmt.Errorf(
			"invalid source range for symbol %q",
			symbol,
		)
	}

	return start, end, nil
}

func normalizePatchSymbol(symbol string) string {
	symbol = strings.TrimSpace(symbol)
	if symbol == "" {
		return ""
	}

	symbol = strings.TrimPrefix(symbol, "func ")
	symbol = strings.TrimSpace(symbol)

	// Допускаем (*Service).Method и приводим к Service.Method.
	if strings.HasPrefix(symbol, "(*") {
		if idx := strings.Index(symbol, ")."); idx > 2 {
			symbol = symbol[2:idx] + symbol[idx+1:]
		}
	}

	return strings.TrimSpace(symbol)
}

func receiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return receiverTypeName(t.X)

	case *ast.Ident:
		return t.Name

	case *ast.IndexExpr:
		return receiverTypeName(t.X)

	case *ast.IndexListExpr:
		return receiverTypeName(t.X)
	}

	return ""
}

// ------------------------------------------------------------
// Existing helpers kept for compatibility with current tests
// ------------------------------------------------------------

func normalizeLineForCompare(s string) string {
	s = strings.ReplaceAll(s, "\t", "    ")
	s = strings.TrimLeft(s, " ")
	s = strings.TrimRight(s, " ")
	return s
}

func leadingIndent(s string) int {
	count := 0

	for _, ch := range s {
		switch ch {
		case ' ':
			count++
		case '\t':
			count += 4
		default:
			return count
		}
	}

	return count
}

func relaxedLineEqual(
	a,
	b string,
	maxTrailingDiff,
	maxIndentDiff int,
) bool {
	if a == b {
		return true
	}

	aNorm := strings.ReplaceAll(a, "\t", "    ")
	bNorm := strings.ReplaceAll(b, "\t", "    ")

	if aNorm == bNorm {
		return true
	}

	aContent := strings.TrimSpace(aNorm)
	bContent := strings.TrimSpace(bNorm)

	if aContent != bContent {
		return false
	}

	aIndent := len(aNorm) - len(strings.TrimLeft(aNorm, " "))
	bIndent := len(bNorm) - len(strings.TrimLeft(bNorm, " "))

	indentDiff := aIndent - bIndent
	if indentDiff < 0 {
		indentDiff = -indentDiff
	}

	if indentDiff > maxIndentDiff {
		return false
	}

	aTrail := len(aNorm) - len(strings.TrimRight(aNorm, " "))
	bTrail := len(bNorm) - len(strings.TrimRight(bNorm, " "))

	trailDiff := aTrail - bTrail
	if trailDiff < 0 {
		trailDiff = -trailDiff
	}

	return trailDiff <= maxTrailingDiff
}

func lineSimilarity(aLines, bLines []string) float64 {
	if len(aLines) == 0 && len(bLines) == 0 {
		return 1.0
	}

	if len(aLines) != len(bLines) {
		return 0.0
	}

	matchCount := 0

	for i := range aLines {
		aNorm := normalizeLineForCompare(aLines[i])
		bNorm := normalizeLineForCompare(bLines[i])

		if aNorm == bNorm {
			matchCount++
		}
	}

	return float64(matchCount) / float64(len(aLines))
}

type fuzzyMatch struct {
	StartLine  int
	Similarity float64
	SecondBest float64
}

func findClosestBlock(
	origLines,
	searchLines []string,
	threshold float64,
) *fuzzyMatch {
	return findClosestBlockWithMargin(
		origLines,
		searchLines,
		threshold,
	)
}

func findClosestBlockWithMargin(
	origLines,
	searchLines []string,
	threshold float64,
) *fuzzyMatch {
	if len(searchLines) == 0 ||
		len(searchLines) > len(origLines) {
		return nil
	}

	best := -1.0
	second := -1.0
	bestLine := -1

	for i := 0; i+len(searchLines) <= len(origLines); i++ {
		candidate := origLines[i : i+len(searchLines)]
		sim := lineSimilarity(candidate, searchLines)

		if sim < threshold {
			continue
		}

		if sim > best {
			second = best
			best = sim
			bestLine = i
			continue
		}

		if sim > second {
			second = sim
		}
	}

	if bestLine < 0 {
		return nil
	}

	if second < 0 {
		second = 0
	}

	return &fuzzyMatch{
		StartLine:  bestLine,
		Similarity: best,
		SecondBest: second,
	}
}

func normalizeNewlines(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

func trimPatchLines(s string) string {
	lines := strings.Split(s, "\n")

	for len(lines) > 0 &&
		strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}

	for len(lines) > 0 &&
		strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}

	return strings.Join(lines, "\n")
}

func PatchConfidence(content, search string) float64 {
	search = trimPatchLines(normalizeNewlines(search))

	if search == "" {
		return 0
	}

	if strings.Count(content, search) == 1 {
		return 1.0
	}

	origLines := strings.Split(content, "\n")
	searchLines := strings.Split(search, "\n")

	relaxed := findRelaxedMatches(
		origLines,
		searchLines,
	)

	if len(relaxed) == 1 {
		return 0.95
	}

	normalized := findNormalizedMatches(
		origLines,
		searchLines,
	)

	if len(normalized) == 1 {
		return 0.98
	}

	fuzzy := findClosestBlockWithMargin(
		origLines,
		searchLines,
		0.80,
	)

	if fuzzy != nil {
		margin := fuzzy.Similarity - fuzzy.SecondBest
		if fuzzy.SecondBest <= 0 {
			margin = fuzzy.Similarity
		}

		if margin < 0 {
			margin = 0
		}

		// Это оценка, а не вероятность.
		return fuzzy.Similarity * 0.9 * (0.5 + margin)
	}

	return 0
}

func patchRequiresSymbol(p domain.Patch, policy PatchPolicy) bool {
	if policy != PatchPolicyStrict {
		return false
	}

	lines := strings.Count(strings.TrimSpace(p.Search), "\n") + 1

	return lines >= 4
}

func patchSearchTooLarge(search string, policy PatchPolicy) bool {
	if policy != PatchPolicyStrict {
		return false
	}

	lines := strings.Count(strings.TrimSpace(search), "\n") + 1
	return lines > 10
}
