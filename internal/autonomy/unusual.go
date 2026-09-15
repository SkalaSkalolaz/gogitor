package autonomy

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gogitor/internal/domain"
	"gogitor/internal/i18n"
	"gogitor/internal/prompts"
	"gogitor/internal/runner"
	"gogitor/internal/security"
)

// UnusualCheckResult — результат проверки одной функции.
type UnusualCheckResult struct {
	Func    string
	File    string
	Line    int
	Skipped string // почему пропустили (например, сложные параметры)
	Passed  bool
	Crash   string // короткое сообщение о падении
}

// FindFuzzCandidates ищет экспортированные функции с простыми параметрами.
// Это те функции, для которых можно легко придумать «необычные данные».
func (g *TestGenerator) FindFuzzCandidates(maxFuncs int) []UntestedFunc {
	if maxFuncs <= 0 {
		maxFuncs = 3
	}

	var out []UntestedFunc

	for _, rel := range g.ws.GoFiles(200) {
		if len(out) >= maxFuncs {
			break
		}

		if strings.HasSuffix(rel, "_test.go") {
			continue
		}

		full, err := security.SafeJoin(g.ws.Root, rel)
		if err != nil {
			continue
		}

		data, err := os.ReadFile(full)
		if err != nil {
			continue
		}

		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, rel, data, parser.ParseComments)
		if err != nil {
			continue
		}

		for _, decl := range f.Decls {
			if len(out) >= maxFuncs {
				break
			}

			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || fn.Name == nil {
				continue
			}

			if !ast.IsExported(fn.Name.Name) {
				continue
			}

			if !hasSimpleParams(fn.Type.Params) {
				continue
			}

			start := fset.Position(fn.Pos()).Offset
			end := fset.Position(fn.End()).Offset

			source := string(data[start:end])
			if len(source) > 2000 {
				source = source[:2000] + "\n// ... (truncated)"
			}

			out = append(out, UntestedFunc{
				File:    rel,
				Package: f.Name.Name,
				Name:    fn.Name.Name,
				Line:    fset.Position(fn.Pos()).Line,
				Source:  source,
			})
		}
	}

	return out
}

// hasSimpleParams проверяет, что у функции хотя бы один параметр
// простого типа: string, []byte, int, bool. Такие функции легко
// «накормить» необычными данными.
func hasSimpleParams(params *ast.FieldList) bool {
	if params == nil {
		return false
	}

	for _, field := range params.List {
		if isSimpleType(field.Type) {
			return true
		}
	}
	return false
}

func isSimpleType(expr ast.Expr) bool {
	switch t := expr.(type) {
	case *ast.Ident:
		switch t.Name {
		case "string", "int", "int64", "bool", "byte", "rune", "float64":
			return true
		}
	case *ast.ArrayType:
		if id, ok := t.Elt.(*ast.Ident); ok && id.Name == "byte" {
			return true
		}
	}
	return false
}

// RunUnusualTests выполняет проверку нескольких функций на устойчивость
// к необычным данным. Файлы-тесты временные и удаляются после запуска.
func (g *TestGenerator) RunUnusualTests(
	ctx context.Context,
	r *runner.Runner,
	maxFuncs int,
	durationPerFunc time.Duration,
	emit func(domain.Event),
) []UnusualCheckResult {
	if maxFuncs <= 0 {
		maxFuncs = 3
	}
	if durationPerFunc <= 0 {
		durationPerFunc = 5 * time.Second
	}

	candidates := g.FindFuzzCandidates(maxFuncs)
	if len(candidates) == 0 {
		return nil
	}

	var results []UnusualCheckResult

	for _, fn := range candidates {
		if ctx.Err() != nil {
			break
		}

		fuzzName := "Fuzz" + fn.Name

		if emit != nil {
			emit(domain.Event{
				Type:    domain.EventLog,
				Message: i18n.T("Checking %s with unusual input...", fn.Name),
			})
		}
		// 1. LLM генерирует fuzz-тест.
		prompt := prompts.UnusualTestPrompt(fn.Name, fn.Package, fn.Source)

		response, err := g.llm.Send(ctx, prompt)
		if err != nil {
			results = append(results, UnusualCheckResult{
				Func:    fn.Name,
				File:    fn.File,
				Line:    fn.Line,
				Skipped: i18n.T("could not prepare a check"),
			})
			continue
		}

		testCode := cleanTestCode(response)
		if testCode == "" {
			results = append(results, UnusualCheckResult{
				Func:    fn.Name,
				File:    fn.File,
				Line:    fn.Line,
				Skipped: i18n.T("check could not be generated"),
			})
			continue
		}

		// 2. Пишем временный файл рядом с исходником.
		dir := filepath.Dir(fn.File)
		base := strings.TrimSuffix(filepath.Base(fn.File), ".go")
		testRel := filepath.Join(dir, base+"_unusual_test.go")
		testFull, joinErr := security.SafeJoin(g.ws.Root, testRel)
		if joinErr != nil {
			results = append(results, UnusualCheckResult{
				Func:    fn.Name,
				File:    fn.File,
				Line:    fn.Line,
				Skipped: i18n.T("could not write temporary file"),
			})
			continue
		}

		body := fmt.Sprintf(
			"package %s\n\nimport \"testing\"\n\n%s\n",
			fn.Package,
			testCode,
		)

		if err := os.WriteFile(testFull, []byte(body), 0o644); err != nil {
			results = append(results, UnusualCheckResult{
				Func:    fn.Name,
				File:    fn.File,
				Line:    fn.Line,
				Skipped: i18n.T("could not write temporary file"),
			})
			continue
		}

		// 3. Запускаем и удаляем файл в любом случае.
		output, runErr := r.RunFuzzTarget(ctx, dir, fuzzName, durationPerFunc)
		_ = os.Remove(testFull)

		if runErr != nil {
			// Ищем в выводе строку с паникой — она самая полезная.
			crash := extractCrashLine(output)
			if crash == "" {
				crash = i18n.T("the program broke on unusual input")
			}
			results = append(results, UnusualCheckResult{
				Func:   fn.Name,
				File:   fn.File,
				Line:   fn.Line,
				Passed: false,
				Crash:  crash,
			})
			continue
		}

		results = append(results, UnusualCheckResult{
			Func:   fn.Name,
			File:   fn.File,
			Line:   fn.Line,
			Passed: true,
		})
	}

	return results
}

// extractCrashLine вытаскивает из вывода go test первую строку с panic.
func extractCrashLine(output string) string {
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "panic:") {
			return trimmed
		}
		if strings.Contains(trimmed, "runtime error:") {
			return trimmed
		}
	}
	return ""
}

// FormatUnusualResults формирует человекочитаемый ответ.
func FormatUnusualResults(results []UnusualCheckResult) string {
	if len(results) == 0 {
		return i18n.T("Nothing to check with unusual data — no simple functions found.")
	}

	var b strings.Builder

	passed := 0
	failed := 0
	skipped := 0

	for _, r := range results {
		switch {
		case r.Skipped != "":
			skipped++
		case r.Passed:
			passed++
		default:
			failed++
		}
	}

	if failed == 0 && skipped == 0 {
		b.WriteString(i18n.T("Checked a few functions with unusual input. Everything held up.") + "\n\n")
	} else if failed == 0 {
		b.WriteString(i18n.T("Checked a few functions with unusual input. Everything held up, but a couple were skipped.") + "\n\n")
	} else {
		b.WriteString(i18n.T("The program did not survive unusual input in a few places.") + "\n\n")
	}

	for _, r := range results {
		switch {
		case r.Skipped != "":
			fmt.Fprintf(&b, "- ⏭ `%s` — %s\n", r.Func, r.Skipped)
		case r.Passed:
			fmt.Fprintf(&b, "- ✓ `%s`\n", r.Func)
		default:
			fmt.Fprintf(&b, "- ✗ `%s:%d` — %s\n", r.File, r.Line, r.Crash)
		}
	}

	if failed > 0 {
		b.WriteString("\n")
		b.WriteString(i18n.T("Run `:fix` with the message above, and I will try to make the function handle strange input gracefully."))
	}

	return strings.TrimSpace(b.String())
}