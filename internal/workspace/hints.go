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

// ─── Публичные типы (без изменений) ────────────────────────────────

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
	Message string
	Explain string
}

// Пороги подобраны так, чтобы новичок не получал шум.
const (
	longFunctionThreshold = 45
	deepNestingThreshold  = 4
)

// ─── Метаданные проекта ────────────────────────────────────────────

// funcSignature описывает сигнатуру функции для анализа
// возвращаемых значений.
type funcSignature struct {
	// returns — список возвращаемых типов в виде строк AST.
	// Для типа error представляется как "error".
	returns []string
}

// projectMetadata агрегирует информацию, необходимую для точной
// диагностики на уровне всего проекта.
type projectMetadata struct {
	called map[string]bool

	deferredOrGo map[string]bool

	usedAsCallback map[string]bool

	signatures map[string]funcSignature

	interfaceMethods map[string]bool
}

func newProjectMetadata() *projectMetadata {
	return &projectMetadata{
		called:           make(map[string]bool),
		deferredOrGo:     make(map[string]bool),
		usedAsCallback:   make(map[string]bool),
		signatures:       make(map[string]funcSignature),
		interfaceMethods: make(map[string]bool),
	}
}

// ─── Публичный API ─────────────────────────────────────────────────

// ScanHints сканирует Go-файлы проекта и возвращает мягкие подсказки.
// Не использует LLM. Максимум maxItems результатов.
func (w *Workspace) ScanHints(maxItems int) []Hint {
	if maxItems <= 0 {
		maxItems = 30
	}

	meta := w.collectProjectMetadata()

	var hints []Hint

	for _, rel := range w.GoFiles(200) {
		if len(hints) >= maxItems {
			break
		}

		full, err := security.SafeJoin(w.Root, rel)
		if err != nil {
			continue
		}

		fileHints := scanFileHints(
			full,
			rel,
			maxItems-len(hints),
			meta,
		)
		hints = append(hints, fileHints...)
	}

	return hints
}

// collectProjectMetadata парсит все Go-файлы проекта и собирает
// информацию, необходимую для точной диагностики.
func (w *Workspace) collectProjectMetadata() *projectMetadata {
	meta := newProjectMetadata()

	for _, rel := range w.GoFiles(500) {
		full, err := security.SafeJoin(w.Root, rel)
		if err != nil {
			continue
		}

		data, err := os.ReadFile(full)
		if err != nil {
			continue
		}

		fset := token.NewFileSet()

		f, err := parser.ParseFile(fset, full, data, parser.ParseComments)
		if err != nil {
			continue
		}

		collectSignatures(f, meta)
		collectInterfaceMethods(f, meta)
		collectCallSites(f, meta)
	}

	return meta
}

// ─── Сбор данных одного файла ──────────────────────────────────────

// collectSignatures собирает сигнатуры всех функций и методов файла.
func collectSignatures(f *ast.File, meta *projectMetadata) {
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil {
			continue
		}

		sig := funcSignature{}

		if fn.Type.Results != nil {
			for _, field := range fn.Type.Results.List {
				typ := typeString(field.Type)

				count := len(field.Names)
				if count == 0 {
					count = 1
				}

				for i := 0; i < count; i++ {
					sig.returns = append(sig.returns, typ)
				}
			}
		}

		name := fn.Name.Name
		meta.signatures[name] = sig

		if fn.Recv != nil && len(fn.Recv.List) > 0 {
			recv := receiverTypeName(fn.Recv.List[0].Type)
			if recv != "" {
				meta.signatures[recv+"."+name] = sig
			}
		}
	}
}

// collectInterfaceMethods собирает имена методов из интерфейсов.
// Такие методы считаются используемыми, даже если прямых вызовов нет.
func collectInterfaceMethods(f *ast.File, meta *projectMetadata) {
	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}

		for _, spec := range gen.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}

			iface, ok := ts.Type.(*ast.InterfaceType)
			if !ok || iface.Methods == nil {
				continue
			}

			for _, method := range iface.Methods.List {
				for _, nameIdent := range method.Names {
					meta.interfaceMethods[nameIdent.Name] = true
				}
			}
		}
	}
}

// collectCallSites собирает все места вызова функций и методов,
// а также использование функций как значений.
func collectCallSites(f *ast.File, meta *projectMetadata) {
	ast.Inspect(f, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CallExpr:
			recordCallTarget(node.Fun, meta.called)

			// Аргументы могут быть функциями-значениями.
			for _, arg := range node.Args {
				recordFuncValue(arg, meta.usedAsCallback)
			}

		case *ast.DeferStmt:
			recordCallTarget(node.Call.Fun, meta.deferredOrGo)

		case *ast.GoStmt:
			recordCallTarget(node.Call.Fun, meta.deferredOrGo)

		case *ast.AssignStmt:
			// RHS может содержать функцию-значение.
			for _, rhs := range node.Rhs {
				recordFuncValue(rhs, meta.usedAsCallback)
			}

		case *ast.ReturnStmt:
			for _, res := range node.Results {
				recordFuncValue(res, meta.usedAsCallback)
			}
		}

		return true
	})
}

// recordCallTarget записывает имя вызываемой функции/метода.
func recordCallTarget(expr ast.Expr, dst map[string]bool) {
	switch fn := expr.(type) {
	case *ast.Ident:
		dst[fn.Name] = true

	case *ast.SelectorExpr:
		if fn.Sel != nil {
			dst[fn.Sel.Name] = true
		}

	case *ast.ParenExpr:
		recordCallTarget(fn.X, dst)

	case *ast.IndexExpr:
		// Generic-инстанцирование: f[T](...)
		recordCallTarget(fn.X, dst)

	case *ast.IndexListExpr:
		recordCallTarget(fn.X, dst)
	}
}

// recordFuncValue записывает имя функции, использованной как значение.
func recordFuncValue(expr ast.Expr, dst map[string]bool) {
	switch e := expr.(type) {
	case *ast.Ident:
		dst[e.Name] = true

	case *ast.SelectorExpr:
		if e.Sel != nil {
			dst[e.Sel.Name] = true
		}
	}
}

// typeString возвращает текстовое представление AST-типа.
func typeString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name

	case *ast.StarExpr:
		return "*" + typeString(t.X)

	case *ast.SelectorExpr:
		if x, ok := t.X.(*ast.Ident); ok && t.Sel != nil {
			return x.Name + "." + t.Sel.Name
		}

	case *ast.ArrayType:
		return "[]" + typeString(t.Elt)

	case *ast.MapType:
		return "map[" + typeString(t.Key) + "]" + typeString(t.Value)

	case *ast.ChanType:
		return "chan " + typeString(t.Value)

	case *ast.InterfaceType:
		return "interface{}"

	case *ast.Ellipsis:
		return "..." + typeString(t.Elt)
	}

	return ""
}

// ─── Пофайловый анализ ─────────────────────────────────────────────

func scanFileHints(
	absPath, relPath string,
	limit int,
	meta *projectMetadata,
) []Hint {
	fset := token.NewFileSet()

	f, err := parser.ParseFile(fset, absPath, nil, parser.ParseComments)
	if err != nil {
		return nil
	}

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
		if isServiceFunc(name) {
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
		if !ast.IsExported(name) && !isCalledAnywhere(name, fn, meta) {
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
		for _, line := range findIgnoredErrorLines(fset, fn.Body, meta) {
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

// isServiceFunc сообщает, относится ли функция к служебным.
// Служебные функции пропускаются во всех проверках.
func isServiceFunc(name string) bool {
	switch name {
	case "main", "init":
		return true
	}

	for _, prefix := range []string{
		"Test", "Benchmark", "Fuzz", "Example",
	} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}

	return false
}

// isCalledAnywhere определяет, используется ли функция/метод
// где-либо в проекте.
func isCalledAnywhere(
	name string,
	fn *ast.FuncDecl,
	meta *projectMetadata,
) bool {
	if meta == nil {
		return false
	}

	// Метод на экспортируемом типе — часть публичного API,
	// он доступен из других пакетов.
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		recv := receiverTypeName(fn.Recv.List[0].Type)
		if ast.IsExported(recv) {
			return true
		}
	}

	// Прямой вызов по имени.
	if meta.called[name] {
		return true
	}

	// Вызов через defer/go.
	if meta.deferredOrGo[name] {
		return true
	}

	// Функция передана как значение (callback).
	if meta.usedAsCallback[name] {
		return true
	}

	// Реализация интерфейсного метода.
	if meta.interfaceMethods[name] {
		return true
	}

	return false
}

// findIgnoredErrorLines находит строки, где возвращаемое значение
// типа error отбрасывается через `_`.
func findIgnoredErrorLines(
	fset *token.FileSet,
	body *ast.BlockStmt,
	meta *projectMetadata,
) []int {
	if meta == nil {
		return nil
	}

	var lines []int

	ast.Inspect(body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}

		if len(assign.Lhs) == 0 || len(assign.Rhs) != 1 {
			return true
		}

		// Проверяем, отбрасывается ли последнее значение через _.
		last := assign.Lhs[len(assign.Lhs)-1]

		id, ok := last.(*ast.Ident)
		if !ok || id.Name != "_" {
			return true
		}

		call, ok := assign.Rhs[0].(*ast.CallExpr)
		if !ok {
			return true
		}

		sig := resolveCallSignature(call, meta)
		if sig == nil || len(sig.returns) == 0 {
			return true
		}

		// По конвенции Go error всегда последний возвращаемый тип.
		lastReturn := sig.returns[len(sig.returns)-1]
		if lastReturn != "error" {
			return true
		}

		// Позиций LHS должно быть не больше, чем возвращаемых значений.
		if len(assign.Lhs) > len(sig.returns) {
			return true
		}

		lines = append(lines, fset.Position(call.Pos()).Line)
		return true
	})

	return lines
}

// resolveCallSignature пытается найти сигнатуру вызываемой функции.
// Возвращает nil, если функция не определена в проекте.
func resolveCallSignature(
	call *ast.CallExpr,
	meta *projectMetadata,
) *funcSignature {
	if meta == nil {
		return nil
	}

	switch fn := call.Fun.(type) {
	case *ast.Ident:
		if sig, ok := meta.signatures[fn.Name]; ok {
			return &sig
		}

	case *ast.SelectorExpr:
		if fn.Sel == nil {
			return nil
		}
		name := fn.Sel.Name

		// Попытка найти "Type.Method".
		if x, ok := fn.X.(*ast.Ident); ok {
			if sig, ok := meta.signatures[x.Name+"."+name]; ok {
				return &sig
			}
		}

		// Общий поиск по имени метода.
		if sig, ok := meta.signatures[name]; ok {
			return &sig
		}
	}

	return nil
}

// ─── Вспомогательные функции (без изменений) ──────────────────────

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
		if items[0].Explain != "" {
			fmt.Fprintf(&b, "> %s\n\n", items[0].Explain)
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