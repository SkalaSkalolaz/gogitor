package codegen

import (
	"fmt"
	"path/filepath"
	"strings"

	"gogitor/internal/domain"
	"gogitor/internal/security"
)

const (
	patchSearchStart      = "<<<<<<< SEARCH"
	patchReplaceSeparator = "======="
	patchReplaceEnd       = ">>>>>>> REPLACE"
	patchReplaceOnlyStart = "<<<<<<< REPLACE_ONLY"
	patchReplaceOnlyEnd   = ">>>>>>> REPLACE_ONLY"
)

func ParseResponseWithOptions(response, fallbackPath string, allowFallback bool) []domain.FileChange {
	lines := strings.Split(response, "\n")
	var files []domain.FileChange
	var current *domain.FileChange
	var content []string

	flush := func() {
		if current == nil {
			return
		}
		current.Content = CleanCode(strings.Join(content, "\n"))
		if strings.TrimSpace(current.Content) != "" {
			files = append(files, *current)
		}
		current = nil
		content = nil
	}

	for _, line := range lines {
		if path := extractFileMarker(line); path != "" {
			flush()
			current = &domain.FileChange{Path: path}
			continue
		}
		if current != nil {
			content = append(content, line)
		}
	}
	flush()

	if len(files) > 0 {
		return files
	}

	if allowFallback && fallbackPath != "" {
		code := CleanCode(response)
		if code != "" && looksLikeCode(code) {
			return []domain.FileChange{
				{
					Path:    fallbackPath,
					Content: code,
				},
			}
		}
	}
	return nil
}

func extractFileMarker(line string) string {
	trimmed := strings.TrimSpace(line)
	idx := strings.Index(trimmed, "--- File:")
	if idx == -1 {
		return ""
	}
	rest := trimmed[idx+len("--- File:"):]
	rest = strings.TrimSpace(rest)
	if !strings.HasSuffix(rest, "---") {
		return ""
	}
	path := strings.TrimSuffix(rest, "---")
	path = strings.TrimSpace(path)
	if path == "" || strings.ContainsAny(path, "\r\n\x00") {
		return ""
	}
	return path
}

func CleanCode(content string) string {
	lines := strings.Split(content, "\n")
	hasFences := false
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			hasFences = true
			break
		}
	}

	var extracted []string
	if hasFences {
		inFence := false
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "```") {
				inFence = !inFence
				continue
			}
			if inFence {
				extracted = append(extracted, line)
			}
		}
		if len(extracted) == 0 {
			for _, line := range lines {
				trimmed := strings.TrimSpace(line)
				if !strings.HasPrefix(trimmed, "```") {
					extracted = append(extracted, line)
				}
			}
		}
	} else {
		extracted = lines
	}

	var cleaned []string
	for _, line := range extracted {
		if isPlaceholder(strings.TrimSpace(line)) {
			continue
		}
		cleaned = append(cleaned, line)
	}

	for len(cleaned) > 0 && strings.TrimSpace(cleaned[0]) == "" {
		cleaned = cleaned[1:]
	}
	for len(cleaned) > 0 && strings.TrimSpace(cleaned[len(cleaned)-1]) == "" {
		cleaned = cleaned[:len(cleaned)-1]
	}
	return strings.Join(cleaned, "\n")
}

func isPlaceholder(line string) bool {
	lower := strings.ToLower(line)
	placeholders := []string{
		"<code here>",
		"<code>",
		"</code>",
		"<your code here>",
		"<your code>",
		"<insert code here>",
		"<put code here>",
		"<implementation here>",
		"<implementation>",
	}
	for _, p := range placeholders {
		if lower == p {
			return true
		}
	}
	return false
}

func looksLikeCode(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "package ") ||
		strings.Contains(lower, "func ") ||
		strings.Contains(lower, "import ") ||
		strings.Contains(lower, "<!doctype html") ||
		strings.Contains(lower, "<html")
}

func FormatChanges(changes []domain.FileChange) string {
	var b strings.Builder
	for _, ch := range changes {
		b.WriteString("--- File: ")
		b.WriteString(ch.Path)
		b.WriteString(" ---\n")
		b.WriteString(ch.Content)
		b.WriteString("\n")
	}
	return b.String()
}

func Validate(changes []domain.FileChange, root string) error {
	if len(changes) == 0 {
		return fmt.Errorf("no file changes found")
	}
	seenPaths := make(map[string]int, len(changes))

	for i, ch := range changes {
		path := strings.TrimSpace(ch.Path)

		if path == "" {
			return fmt.Errorf(
				"empty file path in LLM response",
			)
		}

		normalizedPath := filepath.Clean(path)

		if prev, ok := seenPaths[normalizedPath]; ok {
			return fmt.Errorf(
				"duplicate file change for %q (entries %d and %d)",
				path,
				prev+1,
				i+1,
			)
		}

		seenPaths[normalizedPath] = i

		if len(ch.Patches) == 0 {
			if strings.TrimSpace(ch.Content) == "" {
				return fmt.Errorf("empty content for file %s", ch.Path)
			}
		} else {
			for i, p := range ch.Patches {
				if p.ReplaceOnly {
					if strings.TrimSpace(p.Search) != "" {
						return fmt.Errorf(
							"REPLACE_ONLY patch %d for file %s must not contain SEARCH",
							i+1,
							ch.Path,
						)
					}

					if strings.TrimSpace(p.Symbol) == "" {
						return fmt.Errorf(
							"REPLACE_ONLY patch %d for file %s requires Symbol",
							i+1,
							ch.Path,
						)
					}

					if strings.TrimSpace(p.Replace) == "" {
						return fmt.Errorf(
							"empty REPLACE_ONLY body %d for file %s",
							i+1,
							ch.Path,
						)
					}

					continue
				}

				if strings.TrimSpace(p.Search) == "" {
					return fmt.Errorf(
						"empty SEARCH block %d for file %s",
						i+1,
						ch.Path,
					)
				}
			}
		}
		if _, err := security.SafeJoin(root, ch.Path); err != nil {
			return fmt.Errorf("invalid file path %s: %w", ch.Path, err)
		}
	}

	return nil
}

// ─── Толерантное распознавание маркеров ─────────────────────────────────

// isSearchMarker проверяет, является ли строка началом SEARCH-блока.
// Допускает: "<<<<<<< SEARCH", "<<<<<<<SEARCH", "<<<< SEARCH" и т.д.
func isSearchMarker(trimmed string) bool {
	if !strings.HasPrefix(trimmed, "<") {
		return false
	}
	// Убираем все '<' в начале.
	rest := strings.TrimLeft(trimmed, "<")
	rest = strings.TrimSpace(rest)
	upper := strings.ToUpper(rest)
	return upper == "SEARCH" || strings.HasPrefix(upper, "SEARCH")
}

// isReplaceSeparator проверяет, является ли строка разделителем.
// Допускает: "=======", "======= ", " =======" и т.д.
func isReplaceSeparator(trimmed string) bool {
	return strings.Trim(trimmed, "= ") == "" && strings.Contains(trimmed, "=") && len(trimmed) >= 3
}

// isReplaceEnd проверяет, является ли строка концом REPLACE-блока.
// Допускает: ">>>>>>> REPLACE", ">>>>>>>REPLACE", ">>>> REPLACE" и т.д.
func isReplaceEnd(trimmed string) bool {
	if !strings.HasPrefix(trimmed, ">") {
		return false
	}
	rest := strings.TrimLeft(trimmed, ">")
	rest = strings.TrimSpace(rest)
	upper := strings.ToUpper(rest)
	return upper == "REPLACE" || strings.HasPrefix(upper, "REPLACE")
}

func isReplaceOnlyStart(trimmed string) bool {
	if !strings.HasPrefix(trimmed, "<") {
		return false
	}

	rest := strings.TrimSpace(
		strings.TrimLeft(trimmed, "<"),
	)

	return strings.EqualFold(
		rest,
		"REPLACE_ONLY",
	)
}

func isReplaceOnlyEnd(trimmed string) bool {
	if !strings.HasPrefix(trimmed, ">") {
		return false
	}

	rest := strings.TrimSpace(
		strings.TrimLeft(trimmed, ">"),
	)

	return strings.EqualFold(
		rest,
		"REPLACE_ONLY",
	)
}

func ParseResponseWithPatches(response string) []domain.FileChange {
	lines := strings.Split(response, "\n")

	var files []domain.FileChange
	var current *domain.FileChange
	var content []string

	inPatch := false
	inSearch := false
	inReplace := false
	inReplaceOnly := false

	var searchLines []string
	var replaceLines []string

	// Symbol, расположенный после --- Patch: ... ---,
	// ожидает открытия следующего patch block.
	var pendingSymbol string

	// Symbol, реально принадлежащий текущему patch block.
	var currentPatchSymbol string

	flushPatch := func() {
		if !inPatch || current == nil {
			inPatch = false
			inSearch = false
			inReplace = false
			inReplaceOnly = false

			searchLines = nil
			replaceLines = nil

			currentPatchSymbol = ""

			return
		}

		p := domain.Patch{
			Search: trimPatch(
				strings.Join(searchLines, "\n"),
			),
			Replace: trimPatch(
				strings.Join(replaceLines, "\n"),
			),
			Symbol:      currentPatchSymbol,
			ReplaceOnly: inReplaceOnly,
		}

		// В REPLACE_ONLY SEARCH принципиально отсутствует.
		if inReplaceOnly {
			p.Search = ""
		}

		if strings.TrimSpace(p.Search) != "" ||
			strings.TrimSpace(p.Replace) != "" {

			current.Patches =
				append(
					current.Patches,
					p,
				)
		}

		inPatch = false
		inSearch = false
		inReplace = false
		inReplaceOnly = false

		searchLines = nil
		replaceLines = nil

		currentPatchSymbol = ""
	}

	flushFile := func() {
		if current == nil {
			return
		}

		if len(current.Patches) > 0 {
			current.PatchMode = true
			current.Content = ""
		} else {
			current.Content =
				CleanCode(
					strings.Join(
						content,
						"\n",
					),
				)
		}

		if len(current.Patches) > 0 ||
			strings.TrimSpace(current.Content) != "" {

			files = append(
				files,
				*current,
			)
		}

		current = nil
		content = nil

		pendingSymbol = ""
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// ---------------------------------------------------------
		// PATCH FILE HEADER
		// ---------------------------------------------------------
		if path :=
			extractMarker(
				trimmed,
				"--- Patch:",
			); path != "" {

			flushPatch()
			flushFile()

			current = &domain.FileChange{
				Path:      path,
				PatchMode: true,
			}

			continue
		}

		// ---------------------------------------------------------
		// SYMBOL
		// ---------------------------------------------------------
		if symbol :=
			extractPatchSymbol(trimmed); symbol != "" {

			if inPatch {
				// Теоретически допускаем Symbol внутри блока.
				currentPatchSymbol = symbol
			} else {
				// Нормальный случай:
				// --- Symbol: main ---
				// <<<<<<< SEARCH
				pendingSymbol = symbol
			}

			continue
		}

		// ---------------------------------------------------------
		// FULL FILE HEADER
		// ---------------------------------------------------------
		if path :=
			extractMarker(
				trimmed,
				"--- File:",
			); path != "" {

			flushPatch()
			flushFile()

			current = &domain.FileChange{
				Path: path,
			}

			continue
		}

		if current == nil {
			continue
		}

		// ---------------------------------------------------------
		// PATCH CONTENT
		// ---------------------------------------------------------
		if current.PatchMode || inPatch {

			switch {

			// REPLACE_ONLY
			case isReplaceOnlyStart(trimmed):
				flushPatch()

				inPatch = true
				inSearch = false
				inReplace = false
				inReplaceOnly = true

				currentPatchSymbol = pendingSymbol
				pendingSymbol = ""

				continue

			// SEARCH
			case isSearchMarker(trimmed):
				flushPatch()

				inPatch = true
				inSearch = true
				inReplace = false
				inReplaceOnly = false

				// Критический момент:
				// Symbol фиксируется именно здесь.
				currentPatchSymbol = pendingSymbol
				pendingSymbol = ""

				continue

			// SEARCH -> REPLACE
			case isReplaceSeparator(trimmed) &&
				inPatch &&
				inSearch:

				inSearch = false
				inReplace = true

				continue

			// REPLACE_ONLY end
			case isReplaceOnlyEnd(trimmed) &&
				inPatch &&
				inReplaceOnly:

				flushPatch()

				continue

			// обычный REPLACE end
			case isReplaceEnd(trimmed) &&
				inPatch &&
				inReplace:

				flushPatch()

				continue
			}

			if inPatch {

				if inSearch {
					searchLines =
						append(
							searchLines,
							line,
						)

				} else if inReplace ||
					inReplaceOnly {

					replaceLines =
						append(
							replaceLines,
							line,
						)
				}

				continue
			}

			continue
		}

		content = append(
			content,
			line,
		)
	}

	flushPatch()
	flushFile()

	return files
}

func extractMarker(line, prefix string) string {
	idx := strings.Index(line, prefix)
	if idx == -1 {
		return ""
	}
	rest := strings.TrimSpace(line[idx+len(prefix):])
	if !strings.HasSuffix(rest, "---") {
		return ""
	}
	path := strings.TrimSpace(strings.TrimSuffix(rest, "---"))
	if path == "" || strings.ContainsAny(path, "\r\n\x00") {
		return ""
	}
	return path
}

func extractPatchSymbol(line string) string {
	const prefix = "--- Symbol:"

	trimmed := strings.TrimSpace(line)
	idx := strings.Index(trimmed, prefix)
	if idx == -1 {
		return ""
	}

	rest := strings.TrimSpace(
		trimmed[idx+len(prefix):],
	)

	if strings.HasSuffix(rest, "---") {
		rest = strings.TrimSpace(
			strings.TrimSuffix(rest, "---"),
		)
	}

	if rest == "" ||
		strings.ContainsAny(rest, "\r\n\x00") {
		return ""
	}

	return rest
}

func trimPatch(s string) string {
	lines := strings.Split(s, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}
