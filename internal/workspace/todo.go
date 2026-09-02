package workspace

import (
	"bufio"
	"os"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"gogitor/internal/i18n"
)

// TODOItem — найденная метка в исходном коде.
type TODOItem struct {
	File string
	Line int
	Kind string // TODO, FIXME, HACK, XXX, BUG
	Text string
}

var todoRE = regexp.MustCompile(
	`(?i)\b(TODO|FIXME|HACK|XXX|BUG)\b\s*:?\s*(.*)`,
)

// ScanTODOs сканирует Go-файлы проекта на TODO/FIXME/HACK/XXX/BUG.
// Не использует LLM. Максимум maxItems результатов.
func (w *Workspace) ScanTODOs(maxItems int) []TODOItem {
	if maxItems <= 0 {
		maxItems = 30
	}
	var items []TODOItem
	_ = filepath.Walk(w.Root, func(path string, info os.FileInfo, err error) error {
		if err != nil || len(items) >= maxItems {
			return nil
		}
		if info.IsDir() {
			if shouldSkipDir(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".go") {
			return nil
		}
		rel, relErr := filepath.Rel(w.Root, path)
		if relErr != nil {
			rel = info.Name()
		}
		fileItems := scanFileTODOs(path, rel, maxItems-len(items))
		items = append(items, fileItems...)
		return nil
	})
	return items
}

// lineCommentStart возвращает индекс начала однострочного комментария "//"
// вне строковых и рунных литералов. Возвращает -1, если комментария нет.
func lineCommentStart(line string) int {
	inString := false
	inRune := false
	escaped := false
	for i := 0; i < len(line); i++ {
		ch := line[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && (inString || inRune) {
			escaped = true
			continue
		}
		if ch == '"' && !inRune {
			inString = !inString
			continue
		}
		if ch == '\'' && !inString {
			inRune = !inRune
			continue
		}
		if !inString && !inRune && ch == '/' && i+1 < len(line) && line[i+1] == '/' {
			return i
		}
	}
	return -1
}

func scanFileTODOs(absPath, relPath string, limit int) []TODOItem {
	f, err := os.Open(absPath)
	if err != nil {
		return nil
	}
	defer f.Close()

	var items []TODOItem
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() && len(items) < limit {
		lineNum++
		line := scanner.Text()

		// Ищем маркер только в комментарии.
		commentIdx := lineCommentStart(line)
		if commentIdx == -1 {
			continue
		}
		comment := line[commentIdx+2:] // текст после "//"

		m := todoRE.FindStringSubmatch(comment)
		if m == nil {
			continue
		}

		kind := strings.ToUpper(m[1])
		text := strings.TrimSpace(m[2])
		if len(text) > 120 {
			text = text[:120] + "..."
		}
		items = append(items, TODOItem{
			File: relPath,
			Line: lineNum,
			Kind: kind,
			Text: text,
		})
	}
	return items
}

// FormatTODOs формирует строку-подсказку для TUI.
func FormatTODOs(items []TODOItem) string {
	if len(items) == 0 {
		return ""
	}
	counts := map[string]int{}
	for _, item := range items {
		counts[item.Kind]++
	}
	var parts []string
	for _, kind := range []string{"TODO", "FIXME", "HACK", "BUG", "XXX"} {
		if counts[kind] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", counts[kind], kind))
		}
	}
	summary := strings.Join(parts, ", ")

	var b strings.Builder
	b.WriteString(i18n.T("Found %s:", summary) + "\n")
	maxShow := 5
	if len(items) < maxShow {
		maxShow = len(items)
	}
	for i := 0; i < maxShow; i++ {
		item := items[i]
		b.WriteString(fmt.Sprintf("  %s:%d [%s] %s\n", item.File, item.Line, item.Kind, item.Text))
	}
	if len(items) > maxShow {
		b.WriteString("  " + i18n.T("... and %d more", len(items)-maxShow) + "\n")
	}
	b.WriteString(i18n.T("Type ':todo' to see all, or ask to fix any of them."))
	return b.String()
}