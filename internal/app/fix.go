package app

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gogitor/internal/domain"
	"gogitor/internal/textutil"
)

// ─── Структуры ───────────────────────────────────────────────────────

type TraceFrame struct {
	Function string
	File     string
	Line     int
	Message  string
}

type FixContext struct {
	RawError  string
	ErrorType string
	Summary   string
	Frames    []TraceFrame
}

// ─── Регулярные выражения ────────────────────────────────────────────

var (
	fixFuncRE      = regexp.MustCompile(`^([a-zA-Z_][\w./]*(?:\.\(\*[\w]+\))?[\w.]*)\(`)
	fixTraceFileRE = regexp.MustCompile(`^\s+(.+\.go):(\d+)`)
	fixBuildErrRE  = regexp.MustCompile(`([^\s:]+\.go):(\d+)(?::(\d+))?:\s*(.*)`)
	fixPanicRE     = regexp.MustCompile(`^panic:\s+(.*)`)
	fixFatalRE     = regexp.MustCompile(`^fatal error:\s+(.*)`)
	fixGoroutineRE = regexp.MustCompile(`^goroutine\s+\d+\s+\[`)
	fixTestFailRE  = regexp.MustCompile(`^--- FAIL:\s+(\S+)`)
	fixTestFileRE  = regexp.MustCompile(`^\s+([^\s:]+\.go):(\d+):\s*(.*)`)
)

// ─── Парсинг ─────────────────────────────────────────────────────────

func ParseErrorTrace(rawError string) *FixContext {
	fc := &FixContext{
		RawError:  rawError,
		ErrorType: "unknown",
	}
	lines := strings.Split(rawError, "\n")

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if m := fixPanicRE.FindStringSubmatch(trimmed); m != nil {
			fc.ErrorType = "panic"
			fc.Summary = m[1]
			break
		}
		if m := fixFatalRE.FindStringSubmatch(trimmed); m != nil {
			fc.ErrorType = "panic"
			fc.Summary = m[1]
			break
		}
		if fixTestFailRE.MatchString(trimmed) {
			fc.ErrorType = "test"
		}
	}
	if fc.ErrorType == "unknown" {
		for _, line := range lines {
			if fixBuildErrRE.MatchString(strings.TrimSpace(line)) {
				fc.ErrorType = "build"
				break
			}
		}
	}
	if fc.ErrorType == "unknown" {
		fc.ErrorType = "runtime"
	}

	var currentFunc string
	inGoroutine := false

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		if fixGoroutineRE.MatchString(trimmed) {
			inGoroutine = true
			continue
		}

		if inGoroutine {
			if fixFuncRE.MatchString(trimmed) {
				currentFunc = trimmed
				if idx := strings.Index(currentFunc, "("); idx > 0 {
					currentFunc = currentFunc[:idx]
				}
				continue
			}
			if m := fixTraceFileRE.FindStringSubmatch(line); m != nil {
				lineNum, _ := strconv.Atoi(m[2])
				fc.Frames = append(fc.Frames, TraceFrame{
					Function: currentFunc,
					File:     m[1],
					Line:     lineNum,
				})
				currentFunc = ""
				continue
			}
		}

		if m := fixBuildErrRE.FindStringSubmatch(trimmed); m != nil {
			lineNum, _ := strconv.Atoi(m[2])
			fc.Frames = append(fc.Frames, TraceFrame{
				File:    m[1],
				Line:    lineNum,
				Message: m[4],
			})
			continue
		}

		if m := fixTestFileRE.FindStringSubmatch(line); m != nil {
			lineNum, _ := strconv.Atoi(m[2])
			fc.Frames = append(fc.Frames, TraceFrame{
				File:    m[1],
				Line:    lineNum,
				Message: m[3],
			})
		}
	}

	fc.Frames = deduplicateFrames(fc.Frames)
	return fc
}

func deduplicateFrames(frames []TraceFrame) []TraceFrame {
	seen := make(map[string]bool)
	var out []TraceFrame
	for _, f := range frames {
		key := fmt.Sprintf("%s:%d", f.File, f.Line)
		if !seen[key] {
			seen[key] = true
			out = append(out, f)
		}
	}
	return out
}

// ─── Резолв файлов ───────────────────────────────────────────────────

func (s *Service) resolveTraceFiles(fc *FixContext) []string {
	var files []string
	seen := make(map[string]bool)

	for _, frame := range fc.Frames {
		if frame.File == "" {
			continue
		}
		if isExternalTracePath(frame.File) {
			continue
		}
		rel := s.normalizeTraceFilePath(frame.File)
		if rel == "" {
			continue
		}
		if s.fileExistsRoot(rel) && !seen[rel] {
			seen[rel] = true
			files = append(files, rel)
		}
	}
	return files
}

func isExternalTracePath(path string) bool {
	if strings.Contains(path, "/go/src/") || strings.Contains(path, `\go\src\`) {
		return true
	}
	if strings.HasPrefix(path, "/usr/local/go/") {
		return true
	}
	if strings.Contains(path, "/vendor/") {
		return true
	}
	dir := filepath.Dir(path)
	if filepath.Base(dir) == "runtime" {
		return true
	}
	return false
}

func (s *Service) normalizeTraceFilePath(path string) string {
	if idx := strings.LastIndex(path, ":"); idx > 0 {
		if _, err := strconv.Atoi(path[idx+1:]); err == nil {
			path = path[:idx]
		}
	}
	if filepath.IsAbs(path) {
		if rel, err := filepath.Rel(s.Cfg.WorkDir, path); err == nil && !strings.HasPrefix(rel, "..") {
			return rel
		}
		return ""
	}
	return strings.TrimPrefix(path, "./")
}

// ─── Формирование задачи ─────────────────────────────────────────────

func buildFixTask(fc *FixContext, targetFiles []string) string {
	var b strings.Builder

	b.WriteString("Fix the following error in the Go project.\n\n")

	raw := fc.RawError
	const maxTraceLen = 8000
	if len(raw) > maxTraceLen {
		raw = textutil.TruncateStringBytes(raw, maxTraceLen) + "\n... (trace truncated)"
	}
	b.WriteString("ERROR OUTPUT:\n")
	b.WriteString(raw)
	b.WriteString("\n")

	if fc.ErrorType != "unknown" || fc.Summary != "" {
		b.WriteString("\nDIAGNOSIS:\n")
		if fc.ErrorType != "unknown" {
			fmt.Fprintf(&b, "  Error type: %s\n", fc.ErrorType)
		}
		if fc.Summary != "" {
			fmt.Fprintf(&b, "  Summary: %s\n", fc.Summary)
		}
	}

	if len(fc.Frames) > 0 {
		b.WriteString("\nERROR LOCATIONS:\n")
		for _, f := range fc.Frames {
			switch {
			case f.Function != "":
				fmt.Fprintf(&b, "  - %s at %s:%d\n", f.Function, f.File, f.Line)
			case f.Message != "":
				fmt.Fprintf(&b, "  - %s:%d: %s\n", f.File, f.Line, f.Message)
			default:
				fmt.Fprintf(&b, "  - %s:%d\n", f.File, f.Line)
			}
		}
	}

	if len(targetFiles) > 0 {
		b.WriteString("\nTARGET FILES TO FIX:\n")
		for _, f := range targetFiles {
			b.WriteString("  - " + f + "\n")
		}
	}

	b.WriteString(`
INSTRUCTIONS:
- Fix the ROOT CAUSE of the error shown above.
- Preserve existing behavior for all non-error paths.
- Add proper bounds checking, nil checks, or error handling as appropriate.
- Do NOT rewrite code arbitrarily — fix only what causes the error.
`)
	return b.String()
}

// ─── Основной метод ──────────────────────────────────────────────────

func (s *Service) FixError(
	ctx context.Context,
	errorText string,
	opts Options,
	emit func(domain.Event),
) domain.Result {
	// Все строки на английском — sendEvent автоматически локализует через i18n.Localize
	sendEvent(emit, domain.EventLog, "Parsing error trace")
	fc := ParseErrorTrace(errorText)

	sendEvent(emit, domain.EventLog, fmt.Sprintf(
		"Error type: %s | frames: %d", fc.ErrorType, len(fc.Frames),
	))

	targetFiles := s.resolveTraceFiles(fc)
	if len(targetFiles) > 0 {
		sendEvent(emit, domain.EventLog, fmt.Sprintf(
			"Identified source files: %s", strings.Join(targetFiles, ", "),
		))
	} else {
		sendEvent(emit, domain.EventWarn,
			"No project files found in trace; using general project context")
	}

	task := buildFixTask(fc, targetFiles)
	return s.ExecuteCode(ctx, task, opts, emit)
}

// ─── Эвристика ───────────────────────────────────────────────────────

func looksLikeStackTrace(text string) bool {
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "panic:"):
		return true
	case strings.Contains(lower, "fatal error:"):
		return true
	case strings.Contains(lower, "goroutine ") && strings.Contains(lower, "[running]"):
		return true
	case strings.Contains(lower, "runtime error:"):
		return true
	case strings.Contains(lower, "--- fail:"):
		return true
	}
	return false
}