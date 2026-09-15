package workspace

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"

	"gogitor/internal/i18n"
	"gogitor/internal/security"
)

// HintKind — тип «мягкой» подсказки.
type HintKind string

const (
	HintLongFunction HintKind = "long_function"
	HintDeepNesting  HintKind = "deep_nesting"
	HintUnusedCode   HintKind = "unused_code"
	HintIgnoredError HintKind = "ignored_error"
)

// Hint — одна подсказка для пользователя.
type Hint struct {
	Kind    HintKind
	File    string
	Line    int
	Symbol  string
	Message string // короткое сообщение
	Explain string // объяснение «почему это важно»
}

// Пороги подобраны так, чтобы новичок не получал шум.
const (
	longFunctionThreshold = 45
	deepNestingThreshold  = 4
)

// ScanHints сканирует Go-файлы проекта и возвращает мягкие подсказки.
// Не использует LLM. Максимум maxItems результатов.
func (w *Workspace) ScanHints(maxItems int) []Hint {
	if maxItems <= 0 {
		maxItems = 30
	}

	var hints []Hint

	for _, rel := range w.GoFiles(200) {
		if len(hints) >= maxItems {
			break
		}

		full, err := security.SafeJoin(w.Root, rel)
		if err != nil {
			continue
		}

		fileHints := scanFileHints(full, rel, maxItems-len(hints))
		hints = append(hints, fileHints...)
	}

	return hints
}

func scanFileHints(absPath, relPath string, limit int) []Hint {
	fset := token.NewFileSet()

	f, err := parser.ParseFile(fset, absPath, nil, parser.ParseComments)
	if err != nil {
		return nil
	}

	called := collectCalledNames(f)

	var hints []Hint

	for _, decl := range f.Decls {
		if len(hints) >= limit {
			break
		}

		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil || fn.Name == nil {
			continue
		}

		name := fn.Name.Name
		if name == "main" || name == "init" {
			continue
		}

		startLine := fset.Position(fn.Pos()).Line
		endLine := fset.Position(fn.End()).Line
		length := endLine - startLine + 1

		// 1. Длинная функция (только экспортируемые — их видит пользователь).
		if length > longFunctionThreshold && ast.IsExported(name) {
			hints = append(hints, Hint{
				Kind:    HintLongFunction,
				File:    relPath,
				Line:    startLine,
				Symbol:  name,
				Message: i18n.T("Function %s is %d lines long", name, length),
				Explain: i18n.T("Short functions are easier to read and to fix. Try to split a long function into a few smaller ones, each doing one thing."),
			})
		}

		// 2. Глубокая вложенность.
		if depth := maxBlockDepth(fn.Body.List, 0); depth > deepNestingThreshold {
			hints = append(hints, Hint{
				Kind:    HintDeepNesting,
				File:    relPath,
				Line:    startLine,
				Symbol:  name,
				Message: i18n.T("Function %s has %d nested levels", name, depth),
				Explain: i18n.T("When conditions are nested 5 levels deep, it is hard to follow the logic. Try early returns or split the function."),
			})
		}

		// 3. Неиспользуемая приватная функция.
		if !ast.IsExported(name) && !called[name] {
			hints = append(hints, Hint{
				Kind:    HintUnusedCode,
				File:    relPath,
				Line:    startLine,
				Symbol:  name,
				Message: i18n.T("Function %s is never called", name),
				Explain: i18n.T("It looks like leftover code. If you are sure it is not needed, you can delete it to keep the project clean."),
			})
		}

		// 4. Непроверенные ошибки.
		for _, line := range findIgnoredErrorLines(fset, fn.Body) {
			if len(hints) >= limit {
				break
			}
			hints = append(hints, Hint{
				Kind:    HintIgnoredError,
				File:    relPath,
				Line:    line,
				Symbol:  name,
				Message: i18n.T("An error in %s is not checked", name),
				Explain: i18n.T("When a function returns an error, it is usually checked with `if err != nil { ... }`. Otherwise the program may crash unexpectedly."),
			})
		}
	}

	return hints
}

// collectCalledNames собирает все имена, которые где-то вызываются.
func collectCalledNames(f *ast.File) map[string]bool {
	called := make(map[string]bool)

	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		switch fn := call.Fun.(type) {
		case *ast.Ident:
			called[fn.Name] = true
		case *ast.SelectorExpr:
			called[fn.Sel.Name] = true
		}
		return true
	})

	return called
}

// maxBlockDepth считает максимальную вложенность if/for/switch.
func maxBlockDepth(stmts []ast.Stmt, current int) int {
	max := current

	for _, s := range stmts {
		var child int

		switch st := s.(type) {
		case *ast.IfStmt:
			child = maxBlockDepth(st.Body.List, current+1)
		case *ast.ForStmt:
			child = maxBlockDepth(st.Body.List, current+1)
		case *ast.RangeStmt:
			child = maxBlockDepth(st.Body.List, current+1)
		case *ast.SwitchStmt:
			for _, c := range st.Body.List {
				if cc, ok := c.(*ast.CaseClause); ok {
					if d := maxBlockDepth(cc.Body, current+1); d > child {
						child = d
					}
				}
			}
		case *ast.SelectStmt:
			for _, c := range st.Body.List {
				if cc, ok := c.(*ast.CommClause); ok {
					if d := maxBlockDepth(cc.Body, current+1); d > child {
						child = d
					}
				}
			}
		case *ast.BlockStmt:
			child = maxBlockDepth(st.List, current)
		}

		if child > max {
			max = child
		}
	}

	return max
}

// findIgnoredErrorLines ищет `_ = foo()` или `_, _ = foo()`.
func findIgnoredErrorLines(fset *token.FileSet, body *ast.BlockStmt) []int {
	var lines []int

	ast.Inspect(body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}

		hasBlank := false
		for _, lhs := range assign.Lhs {
			if id, ok := lhs.(*ast.Ident); ok && id.Name == "_" {
				hasBlank = true
				break
			}
		}
		if !hasBlank {
			return true
		}

		for _, rhs := range assign.Rhs {
			if _, ok := rhs.(*ast.CallExpr); ok {
				lines = append(lines, fset.Position(rhs.Pos()).Line)
				break
			}
		}
		return true
	})

	return lines
}

// FormatHints формирует человекочитаемый ответ для TUI.
func FormatHints(hints []Hint) string {
	if len(hints) == 0 {
		return i18n.T("Your code looks clean — no obvious issues found.")
	}

	byKind := make(map[HintKind][]Hint)
	for _, h := range hints {
		byKind[h.Kind] = append(byKind[h.Kind], h)
	}

	var b strings.Builder

	b.WriteString(i18n.T("I looked at your code and found a few things worth a look:") + "\n\n")

	order := []HintKind{
		HintIgnoredError,
		HintLongFunction,
		HintDeepNesting,
		HintUnusedCode,
	}

	for _, kind := range order {
		items := byKind[kind]
		if len(items) == 0 {
			continue
		}

		fmt.Fprintf(&b, "### %s\n\n", headerForHintKind(kind))

		// Объяснение показываем один раз на группу.
		if len(items) > 0 && items[0].Explain != "" {
			b.WriteString("> " + items[0].Explain + "\n\n")
		}

		for _, h := range items {
			fmt.Fprintf(&b, "- `%s:%d` — %s\n", h.File, h.Line, h.Message)
		}
		b.WriteString("\n")
	}

	b.WriteString(i18n.T("Nothing here is a bug — just ideas to make the code easier to read. Ask me to explain any of them if you want."))

	return strings.TrimSpace(b.String())
}

func headerForHintKind(kind HintKind) string {
	switch kind {
	case HintIgnoredError:
		return i18n.T("Errors that are not checked")
	case HintLongFunction:
		return i18n.T("Long functions")
	case HintDeepNesting:
		return i18n.T("Deeply nested code")
	case HintUnusedCode:
		return i18n.T("Code that is never used")
	default:
		return string(kind)
	}
}

var _ = os.Stat // silence import if not used elsewhere