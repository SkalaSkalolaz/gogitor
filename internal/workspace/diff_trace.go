package workspace

import (
	"fmt"
	"strings"

	"gogitor/internal/domain"
)

// DiffTraceSink получает диагностические сообщения DIFF.
//
// Когда sink == nil, движок патчей работает в обычном режиме
// без дополнительного диагностического вывода.
type DiffTraceSink func(string)

// SetDiffTraceSink включает/выключает трассировку DIFF.
func (w *Workspace) SetDiffTraceSink(sink DiffTraceSink) {
	w.mu.Lock()
	w.diffTraceSink = sink
	w.mu.Unlock()
}

// getDiffTraceSink возвращает текущий sink трассировки.
func (w *Workspace) getDiffTraceSink() DiffTraceSink {
	w.mu.Lock()
	sink := w.diffTraceSink
	w.mu.Unlock()
	return sink
}

// diffTracef отправляет одну строку DIFF-трассы.
func (w *Workspace) diffTracef(format string, args ...any) {
	sink := w.getDiffTraceSink()
	if sink == nil {
		return
	}

	sink("[DIFF] " + fmt.Sprintf(format, args...))
}

// patchTrace хранит контекст одного конкретного SEARCH/REPLACE patch.
type patchTrace struct {
	sink DiffTraceSink

	phase string
	file  string

	index int
	total int

	policy PatchPolicy
	patch  domain.Patch

	method    string
	startLine int
}

func newPatchTrace(
	sink DiffTraceSink,
	phase string,
	file string,
	index int,
	total int,
	policy PatchPolicy,
	patch domain.Patch,
) *patchTrace {
	if sink == nil {
		return nil
	}

	return &patchTrace{
		sink:      sink,
		phase:     phase,
		file:      file,
		index:     index,
		total:     total,
		policy:    policy,
		patch:     patch,
		startLine: 0,
	}
}

func (t *patchTrace) emit(
	stage string,
	decision string,
	format string,
	args ...any,
) {
	if t == nil || t.sink == nil {
		return
	}

	detail := ""
	if format != "" {
		detail = " " + fmt.Sprintf(format, args...)
	}

	symbol := strings.TrimSpace(t.patch.Symbol)

	protocol := "SEARCH_REPLACE"
	if t.patch.ReplaceOnly {
		protocol = "REPLACE_ONLY"
	}

	message := fmt.Sprintf(
		"phase=%s file=%s patch=%d/%d stage=%s decision=%s policy=%s protocol=%s",
		t.phase,
		t.file,
		t.index,
		t.total,
		stage,
		decision,
		t.policy.String(),
		protocol,
	)

	if symbol != "" {
		message += " symbol=" + symbol
	}

	message += detail

	t.sink("[DIFF] " + message)
}

func patchLineCount(s string) int {
	s = strings.TrimSpace(
		strings.ReplaceAll(s, "\r\n", "\n"),
	)

	if s == "" {
		return 0
	}

	return strings.Count(s, "\n") + 1
}

func lineAtOffset(content string, offset int) int {
	if offset < 0 {
		return 0
	}

	if offset > len(content) {
		offset = len(content)
	}

	line := 1

	for i := 0; i < offset; i++ {
		if content[i] == '\n' {
			line++
		}
	}

	return line
}

// applyPatchesWithPolicyTraced — диагностический вариант
// applyPatchesWithPolicy.
func applyPatchesWithPolicyTraced(
	content string,
	patches []domain.Patch,
	policy PatchPolicy,
	minConfidenceOverride float64,
	phase string,
	file string,
	sink DiffTraceSink,
) (string, error) {
	content = normalizeNewlines(content)

	for i, p := range patches {
		trace := newPatchTrace(
			sink,
			phase,
			file,
			i+1,
			len(patches),
			policy,
			p,
		)

		trace.emit(
			"START",
			"RUN",
			"search_lines=%d replace_lines=%d",
			patchLineCount(p.Search),
			patchLineCount(p.Replace),
		)

		updated, err := applyOnePatchWithPolicyTraced(
			content,
			p,
			policy,
			minConfidenceOverride,
			trace,
		)
		if err != nil {
			return "", fmt.Errorf("patch %d: %w", i+1, err)
		}

		content = updated
	}

	return content, nil
}

// applyOnePatchWithPolicyTraced — диагностический вариант
// текущего applyOnePatchWithPolicy.
func applyOnePatchWithPolicyTraced(
	content string,
	p domain.Patch,
	policy PatchPolicy,
	minConfidenceOverride float64,
	trace *patchTrace,
) (string, error) {
	if trace == nil {
		trace = &patchTrace{
			policy: policy,
			patch:  p,
		}
	}

	replaceOnly := p.ReplaceOnly

	trace.emit(
		"SOURCE",
		"RUN",
		"search_lines=%d replace_lines=%d",
		patchLineCount(p.Search),
		patchLineCount(p.Replace),
	)

	// Hash проверяется по исходным байтам,
	// до последующих нормализаций.
	if p.ExpectedSourceHash != "" {
		if hashBytes([]byte(content)) != p.ExpectedSourceHash {
			trace.emit(
				"SOURCE",
				"REJECT",
				"expected source hash does not match current file",
			)

			return "", fmt.Errorf(
				"expected source hash does not match current file",
			)
		}

		trace.emit(
			"SOURCE",
			"OK",
			"source_hash=verified",
		)
	}

	search := trimPatchLines(
		normalizeNewlines(p.Search),
	)

	replace := trimPatchLines(
		normalizeNewlines(p.Replace),
	)

	// REPLACE_ONLY.
	if replaceOnly && search == "" {
		if strings.TrimSpace(p.Symbol) == "" {
			trace.emit(
				"SYMBOL",
				"REJECT",
				"REPLACE_ONLY patch requires Symbol",
			)

			return "", fmt.Errorf(
				"REPLACE_ONLY patch requires Symbol",
			)
		}

		if p.ExpectedSymbolFingerprint != "" {
			fp, err := SymbolFingerprint(
				content,
				p.Symbol,
			)
			if err != nil {
				trace.emit(
					"SYMBOL",
					"REJECT",
					"fingerprint error=%q",
					err.Error(),
				)

				return "", err
			}

			if fp != p.ExpectedSymbolFingerprint {
				trace.emit(
					"SYMBOL",
					"REJECT",
					"symbol fingerprint changed",
				)

				return "", fmt.Errorf(
					"symbol %q changed since patch generation",
					p.Symbol,
				)
			}

			trace.emit(
				"SYMBOL",
				"OK",
				"fingerprint=verified",
			)
		}

		start, end, err := findSymbolRange(
			content,
			p.Symbol,
		)
		if err != nil {
			trace.emit(
				"SYMBOL",
				"REJECT",
				"error=%q",
				err.Error(),
			)

			return "", err
		}

		trace.emit(
			"SYMBOL",
			"OK",
			"range=%d-%d",
			lineAtOffset(content, start),
			lineAtOffset(content, end),
		)

		search = content[start:end]

		if err := validateReplacementForSymbol(
			replace,
			p.Symbol,
		); err != nil {
			trace.emit(
				"SYMBOL",
				"REJECT",
				"invalid replacement=%q",
				err.Error(),
			)

			return "", err
		}
	}

	if search == "" {
		trace.emit(
			"SEARCH",
			"REJECT",
			"empty SEARCH block",
		)

		return "", fmt.Errorf("empty SEARCH block")
	}

	trace.emit(
		"SEARCH",
		"OK",
		"search_lines=%d replace_lines=%d",
		patchLineCount(search),
		patchLineCount(replace),
	)

	// В strict режиме сохраняем текущее ограничение.
	if !replaceOnly &&
		patchSearchTooLarge(search, policy) {
		trace.emit(
			"LIMIT",
			"REJECT",
			"SEARCH block too large: %d lines",
			patchLineCount(search),
		)

		return "", fmt.Errorf(
			"strict SEARCH block is too large: %d lines, maximum 10",
			patchLineCount(search),
		)
	}

	if patchRequiresSymbol(p, policy) &&
		strings.TrimSpace(p.Symbol) == "" {
		trace.emit(
			"SYMBOL",
			"REJECT",
			"strict patch requires Symbol",
		)

		return "", fmt.Errorf(
			"strict patch requires Symbol anchor for SEARCH block with %d lines",
			patchLineCount(search),
		)
	}

	if strings.TrimSpace(p.Symbol) != "" {
		start, end, err := findSymbolRange(
			content,
			p.Symbol,
		)
		if err != nil {
			trace.emit(
				"SYMBOL",
				"REJECT",
				"error=%q",
				err.Error(),
			)

			return "", err
		}

		trace.emit(
			"SYMBOL",
			"OK",
			"range=%d-%d",
			lineAtOffset(content, start),
			lineAtOffset(content, end),
		)

		local := content[start:end]

		updatedLocal, matched, err :=
			applyPatchTextTraced(
				local,
				search,
				replace,
				policy,
				minConfidenceOverride,
				trace,
			)

		if err != nil {
			trace.emit(
				"APPLY",
				"REJECT",
				"symbol=%q error=%q",
				p.Symbol,
				err.Error(),
			)

			return "", fmt.Errorf(
				"symbol %q: %w",
				p.Symbol,
				err,
			)
		}

		if matched {
			trace.emit(
				"APPLY",
				"OK",
				"method=%s scope=%s start=%d",
				trace.method,
				"symbol",
				trace.startLine,
			)

			return content[:start] +
				updatedLocal +
				content[end:], nil
		}

		trace.emit(
			"APPLY",
			"MISS",
			"SEARCH block not found inside symbol",
		)

		return "", fmt.Errorf(
			"SEARCH block not found inside symbol %q",
			p.Symbol,
		)
	}

	updated, matched, err :=
		applyPatchTextTraced(
			content,
			search,
			replace,
			policy,
			minConfidenceOverride,
			trace,
		)

	if err != nil {
		trace.emit(
			"APPLY",
			"REJECT",
			"error=%q",
			err.Error(),
		)

		return "", err
	}

	if !matched {
		trace.emit(
			"APPLY",
			"MISS",
			"SEARCH block not found",
		)

		return "", fmt.Errorf(
			"SEARCH block not found",
		)
	}

	trace.emit(
		"APPLY",
		"OK",
		"method=%s start=%d",
		trace.method,
		trace.startLine,
	)

	return updated, nil
}


func applyPatchTextTraced(
	content string,
	search string,
	replace string,
	policy PatchPolicy,
	minConfidenceOverride float64,
	trace *patchTrace,
) (string, bool, error) {
	if trace == nil {
		trace = &patchTrace{
			policy: policy,
		}
	}

	// 1. EXACT.
	count := strings.Count(content, search)

	if count == 1 {
		trace.method = "EXACT"

		offset := strings.Index(content, search)
		trace.startLine = lineAtOffset(content, offset)

		trace.emit(
			"EXACT",
			"OK",
			"matches=1 start=%d",
			trace.startLine,
		)

		return strings.Replace(
			content,
			search,
			replace,
			1,
		), true, nil
	}

	if count > 1 {
		trace.emit(
			"EXACT",
			"REJECT",
			"ambiguous_matches=%d",
			count,
		)

		return "", false, fmt.Errorf(
			"SEARCH block is ambiguous (%d exact matches)",
			count,
		)
	}

	trace.emit(
		"EXACT",
		"MISS",
		"matches=0",
	)

	origLines := strings.Split(
		content,
		"\n",
	)

	searchLines := strings.Split(
		search,
		"\n",
	)

	// 2. RELAXED.
	relaxed := findRelaxedMatches(
		origLines,
		searchLines,
	)

	if len(relaxed) == 1 {
		trace.method = "RELAXED"
		trace.startLine = relaxed[0] + 1

		trace.emit(
			"RELAXED",
			"OK",
			"matches=1 start=%d",
			trace.startLine,
		)

		return replaceLineRange(
			origLines,
			relaxed[0],
			relaxed[0]+len(searchLines),
			replace,
		), true, nil
	}

	if len(relaxed) > 1 {
		trace.emit(
			"RELAXED",
			"REJECT",
			"ambiguous_matches=%d",
			len(relaxed),
		)

		return "", false, fmt.Errorf(
			"SEARCH block is ambiguous (%d relaxed matches)",
			len(relaxed),
		)
	}

	trace.emit(
		"RELAXED",
		"MISS",
		"matches=0",
	)

	// 3. NORMALIZED.
	normalized := findNormalizedMatches(
		origLines,
		searchLines,
	)

	if len(normalized) == 1 {
		trace.method = "NORMALIZED"
		trace.startLine = normalized[0] + 1

		trace.emit(
			"NORMALIZED",
			"OK",
			"matches=1 start=%d",
			trace.startLine,
		)

		return replaceLineRange(
			origLines,
			normalized[0],
			normalized[0]+len(searchLines),
			replace,
		), true, nil
	}

	if len(normalized) > 1 {
		trace.emit(
			"NORMALIZED",
			"REJECT",
			"ambiguous_matches=%d",
			len(normalized),
		)

		return "", false, fmt.Errorf(
			"SEARCH block is ambiguous (%d normalized matches)",
			len(normalized),
		)
	}

	trace.emit(
		"NORMALIZED",
		"MISS",
		"matches=0",
	)

	// 4. REBASE.
	if policy != PatchPolicyStrict {
		if rebased := findRebasedBlock(
			origLines,
			searchLines,
		); rebased != nil {
			trace.method = "REBASE"
			trace.startLine = rebased.StartLine + 1

			trace.emit(
				"REBASE",
				"OK",
				"start=%d similarity=%.3f",
				trace.startLine,
				rebased.Similarity,
			)

			return replaceLineRange(
				origLines,
				rebased.StartLine,
				rebased.StartLine+len(searchLines),
				replace,
			), true, nil
		}
	}

	if policy != PatchPolicyStrict {
		trace.emit(
			"REBASE",
			"MISS",
			"no safe rebased block",
		)
	} else {
		trace.emit(
			"REBASE",
			"SKIP",
			"strict policy",
		)
	}

	// ------------------------------------------------------------
	// 5. AST-AWARE FUZZY.
	// ------------------------------------------------------------
	threshold, requiredMargin :=
		fuzzyThresholds(
			policy,
			minConfidenceOverride,
		)

	if policy == PatchPolicyStrict {
		trace.emit(
			"AST_FUZZY",
			"SKIP",
			"strict policy",
		)
	} else if len(searchLines) < 2 {
		trace.emit(
			"AST_FUZZY",
			"SKIP",
			"search has fewer than 2 lines",
		)
	} else {
		astMatch := findASTAwareBlock(
			origLines,
			searchLines,
		)

		if astMatch == nil {
			trace.emit(
				"AST_FUZZY",
				"MISS",
				"no structurally compatible candidate",
			)
		} else {
			margin := astMatch.Similarity -
				astMatch.SecondBest

			if astMatch.SecondBest <= 0 {
				margin = astMatch.Similarity
			}

			if margin < 0 {
				margin = 0
			}

			trace.emit(
				"AST_FUZZY",
				"RUN",
				"best=%.3f second=%.3f margin=%.3f threshold=%.3f required_margin=%.3f candidate_start=%d",
				astMatch.Similarity,
				astMatch.SecondBest,
				margin,
				threshold,
				requiredMargin,
				astMatch.StartLine+1,
			)

			if astMatch.Similarity < threshold {
				trace.emit(
					"AST_FUZZY",
					"REJECT",
					"confidence below threshold",
				)
			} else if margin < requiredMargin {
				trace.emit(
					"AST_FUZZY",
					"REJECT",
					"ambiguous candidates",
				)
			} else {
				trace.method = "AST_FUZZY"
				trace.startLine = astMatch.StartLine + 1

				trace.emit(
					"AST_FUZZY",
					"OK",
					"start=%d confidence=%.3f margin=%.3f",
					trace.startLine,
					astMatch.Similarity,
					margin,
				)

				return replaceLineRange(
					origLines,
					astMatch.StartLine,
					astMatch.StartLine+len(searchLines),
					replace,
				), true, nil
			}
		}
	}

	// ------------------------------------------------------------
	// 6. LEGACY FUZZY.
	// ------------------------------------------------------------
	if policy == PatchPolicyStrict {
		trace.emit(
			"FUZZY",
			"SKIP",
			"strict policy",
		)

		return "", false, nil
	}

	if len(searchLines) < 2 {
		trace.emit(
			"FUZZY",
			"SKIP",
			"search has fewer than 2 lines",
		)

		return "", false, nil
	}

	if !hasUniqueExactAnchor(
		origLines,
		searchLines,
	) {
		trace.emit(
			"FUZZY",
			"SKIP",
			"no unique exact anchor",
		)

		return "", false, nil
	}

	fuzzy := findClosestBlockWithMargin(
		origLines,
		searchLines,
		0.60,
	)

	if fuzzy == nil {
		trace.emit(
			"FUZZY",
			"MISS",
			"no candidate above base threshold",
		)

		return "", false, nil
	}

	margin := fuzzy.Similarity -
		fuzzy.SecondBest

	if fuzzy.SecondBest <= 0 {
		margin = fuzzy.Similarity
	}

	if margin < 0 {
		margin = 0
	}

	trace.emit(
		"FUZZY",
		"RUN",
		"best=%.3f second=%.3f margin=%.3f threshold=%.3f required_margin=%.3f candidate_start=%d",
		fuzzy.Similarity,
		fuzzy.SecondBest,
		margin,
		threshold,
		requiredMargin,
		fuzzy.StartLine+1,
	)

	if fuzzy.Similarity < threshold {
		trace.emit(
			"FUZZY",
			"REJECT",
			"confidence below threshold",
		)

		return "", false, fmt.Errorf(
			"fuzzy SEARCH rejected: confidence %.2f below threshold %.2f",
			fuzzy.Similarity,
			threshold,
		)
	}

	if margin < requiredMargin {
		trace.emit(
			"FUZZY",
			"REJECT",
			"ambiguous candidates",
		)

		return "", false, fmt.Errorf(
			"fuzzy SEARCH rejected: ambiguous candidates (best %.2f, second %.2f, margin %.2f, required %.2f)",
			fuzzy.Similarity,
			fuzzy.SecondBest,
			margin,
			requiredMargin,
		)
	}

	trace.method = "FUZZY"
	trace.startLine = fuzzy.StartLine + 1

	trace.emit(
		"FUZZY",
		"OK",
		"start=%d confidence=%.3f margin=%.3f",
		trace.startLine,
		fuzzy.Similarity,
		margin,
	)

	return replaceLineRange(
		origLines,
		fuzzy.StartLine,
		fuzzy.StartLine+len(searchLines),
		replace,
	), true, nil
}
