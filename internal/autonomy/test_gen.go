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

    "gogitor/internal/i18n"
	"gogitor/internal/domain"
	"gogitor/internal/runner"
	"gogitor/internal/security"
	"gogitor/internal/workspace"
)

// LLMClient — минимальный интерфейс для отправки запросов к модели.
// Совместим с agent.Dispatcher и llm.Client.
type LLMClient interface {
	Send(ctx context.Context, prompt string) (string, error)
}

// UntestedFunc — функция без тестового покрытия.
type UntestedFunc struct {
	File     string
	Package  string
	Name     string
	Receiver string
	Line     int
	Source   string
}

// TestGenerator — генератор тестов для функций без покрытия.
// Использует узкие однозадачные промпты, совместимые с моделями 20B+.
type TestGenerator struct {
	ws  *workspace.Workspace
	llm LLMClient
}

func NewTestGenerator(ws *workspace.Workspace, llm LLMClient) *TestGenerator {
	return &TestGenerator{ws: ws, llm: llm}
}

// FindUntested находит экспортированные функции без соответствующего _test.go.
// Полностью детерминированно, через AST, без LLM.
func (g *TestGenerator) FindUntested(maxFuncs int) []UntestedFunc {
	if maxFuncs <= 0 {
		maxFuncs = 5
	}
	goFiles := g.ws.GoFiles(200)
	var untested []UntestedFunc

	for _, rel := range goFiles {
		if len(untested) >= maxFuncs {
			break
		}
		// Пропускаем тестовые файлы
		if strings.HasSuffix(rel, "_test.go") {
			continue
		}
		// Проверяем наличие _test.go для этого файла
		dir := filepath.Dir(rel)
		base := strings.TrimSuffix(filepath.Base(rel), ".go")
		testRelPath := filepath.Join(dir, base+"_test.go")
		if _, err := os.Stat(filepath.Join(g.ws.Root, testRelPath)); err == nil {
			continue // тестовый файл существует
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
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name == nil {
				continue
			}
			// Только экспортированные функции
			if !ast.IsExported(fn.Name.Name) {
				continue
			}
			// Пропускаем main и init
			if fn.Name.Name == "main" || fn.Name.Name == "init" {
				continue
			}

			// Извлекаем исходный код функции
			start := fset.Position(fn.Pos()).Offset
			end := fset.Position(fn.End()).Offset
			source := string(data[start:end])
			if len(source) > 600 {
				source = source[:600] + "\n// ... (truncated)"
			}

			receiver := ""
			if fn.Recv != nil && len(fn.Recv.List) > 0 {
				receiver = receiverTypeName(fn.Recv.List[0].Type)
			}

			untested = append(untested, UntestedFunc{
				File:     rel,
				Package:  f.Name.Name,
				Name:     fn.Name.Name,
				Receiver: receiver,
				Line:     fset.Position(fn.Pos()).Line,
				Source:   source,
			})
			if len(untested) >= maxFuncs {
				break
			}
		}
	}
	return untested
}

func receiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return receiverTypeName(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr:
		return receiverTypeName(t.X)
	}
	return ""
}

// GenerateForFunc генерирует тест для одной функции через узкий промпт.
// Промпт сформулирован так, чтобы модель 20B+ могла справиться:
// одна функция, один тест, чёткие правила.
func (g *TestGenerator) GenerateForFunc(ctx context.Context, fn UntestedFunc) (string, error) {
	funcName := fn.Name
	if fn.Receiver != "" {
		funcName = fn.Receiver + "." + fn.Name
	}

	prompt := fmt.Sprintf(`You are a Go test writer. Write a single test function for the following Go function.
Return ONLY the test function code. No explanations, no markdown fences, no package declaration, no import block.

FUNCTION (package %s, symbol %s):
%s

RULES:
1. Write exactly one test function named Test%s.
2. Use table-driven tests with t.Run subtests.
3. Include at least 3 test cases: happy path, boundary value, and error/edge case.
4. Use only the standard library "testing" package. Do not import any other package.
5. If the function returns an error, test both nil and non-nil error paths.
6. If the function has no parameters and no return values, write a smoke test that calls it.
7. Return ONLY the function body starting with "func Test%s(t *testing.T) {".
`, fn.Package, funcName, fn.Source, fn.Name, fn.Name)

	response, err := g.llm.Send(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("LLM request failed: %w", err)
	}
	code := cleanTestCode(response)
	if code == "" {
		return "", fmt.Errorf("LLM returned empty test code")
	}
	return code, nil
}

// cleanTestCode удаляет markdown-обёртки и лишние строки из ответа модели.
func cleanTestCode(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Удаляем markdown-обёртки
	if strings.Contains(s, "```") {
		lines := strings.Split(s, "\n")
		var code []string
		inBlock := false
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "```") {
				inBlock = !inBlock
				continue
			}
			if inBlock {
				code = append(code, line)
			}
		}
		if len(code) > 0 {
			s = strings.Join(code, "\n")
		}
	}
	// Удаляем возможные префиксы
	for _, prefix := range []string{"```go", "```"} {
		s = strings.TrimPrefix(s, prefix)
	}
	return strings.TrimSpace(s)
}

// ApplyTest записывает сгенерированный тест в файл и проверяет его.
// Если тесты падают — файл удаляется (откат).
// Если тесты проходят — файл остаётся.
func (g *TestGenerator) ApplyTest(
	ctx context.Context,
	fn UntestedFunc,
	testCode string,
	r *runner.Runner,
	emit func(domain.Event),
) (string, error) {
	dir := filepath.Dir(fn.File)
	base := strings.TrimSuffix(filepath.Base(fn.File), ".go")
	testRelPath := filepath.Join(dir, base+"_test.go")
	testFullPath := filepath.Join(g.ws.Root, testRelPath)

	// Не перезаписываем существующий тестовый файл
	if _, err := os.Stat(testFullPath); err == nil {
		return "", fmt.Errorf("test file already exists: %s", testRelPath)
	}

	// Формируем содержимое тестового файла
	content := fmt.Sprintf("package %s\n\nimport (\n\t\"testing\"\n)\n\n%s\n", fn.Package, testCode)

	// Записываем файл
	if err := os.WriteFile(testFullPath, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("cannot write test file: %w", err)
	}

    if emit != nil {
        emit(domain.Event{
            Type:    domain.EventLog,
            Message: i18n.T("Test file created: %s — running tests...", testRelPath),
        })
    }
	// Проверяем: запускаем тесты
	tests, err := r.Test(ctx, g.ws.Root)
    if err != nil || tests.Failed > 0 {
        os.Remove(testFullPath)
        if emit != nil {
            emit(domain.Event{
                Type:    domain.EventWarn,
                Message: i18n.T("Generated tests failed (%d passed, %d failed). File removed.",
                    tests.Passed, tests.Failed),
            })
        }
        return "", fmt.Errorf("generated tests failed: %d passed, %d failed", tests.Passed, tests.Failed)
    }
    if emit != nil {
        emit(domain.Event{
            Type:    domain.EventLog,
            Message: i18n.T("Tests pass (%d passed). File kept: %s", tests.Passed, testRelPath),
        })
    }
	return testRelPath, nil
}

// FormatUntested формирует список функций без тестов.
func FormatUntested(funcs []UntestedFunc) string {
	if len(funcs) == 0 {
		return "No untested exported functions found."
	}
	var b strings.Builder
	b.WriteString("## Untested Exported Functions\n\n")
	for _, fn := range funcs {
		name := fn.Name
		if fn.Receiver != "" {
			name = fn.Receiver + "." + fn.Name
		}
		fmt.Fprintf(&b, "- `%s` in `%s:%d`\n", name, fn.File, fn.Line)
	}
	b.WriteString("\nUse `:autogen-tests` to generate tests for these functions.\n")
	return b.String()
}