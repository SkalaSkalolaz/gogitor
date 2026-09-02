package autonomy

import (
	"context"
	"fmt"
	"os"
	"strings"

    "gogitor/internal/i18n"
	"gogitor/internal/domain"
	"gogitor/internal/runner"
	"gogitor/internal/security"
	"gogitor/internal/workspace"
)

// MutationType — тип мутации.
type MutationType string

const (
	MutRelational MutationType = "relational" // > ↔ >=, < ↔ <=
	MutLogical    MutationType = "logical"    // && ↔ ||
	MutEquality   MutationType = "equality"   // == ↔ !=
)

// Mutation — одна мутация.
type Mutation struct {
	File     string
	Line     int
	Original string
	Mutated  string
	Type     MutationType
	Killed   bool
	Error    string
}

// MutationReport — отчёт мутационного тестирования.
type MutationReport struct {
	Mutations []Mutation
	Killed    int
	Survived  int
	Errors    int
}

// Mutator — детерминированный генератор и исполнитель мутаций.
// Не использует LLM. Мутации генерируются текстовой заменой операторов.
// Каждая мутация применяется в песочнице, проверяется тестами и откатывается.
type Mutator struct {
	ws     *workspace.Workspace
	runner *runner.Runner
	limit  int
}

func NewMutator(ws *workspace.Workspace, r *runner.Runner, limit int) *Mutator {
	if limit <= 0 {
		limit = 20
	}
	return &Mutator{ws: ws, runner: r, limit: limit}
}

// mutationRules — детерминированные правила мутаций.
// Порядок важен: более длинные операторы проверяются первыми,
// чтобы избежать ложных срабатываний (например, >= не должен
// порождать мутацию для > внутри себя).
var mutationRules = []struct {
	From string
	To   string
	Type MutationType
}{
	{">=", ">", MutRelational},
	{"<=", "<", MutRelational},
	{"&&", "||", MutLogical},
	{"||", "&&", MutLogical},
	{"==", "!=", MutEquality},
	{"!=", "==", MutEquality},
}

// GenerateMutations сканирует Go-файлы и генерирует мутации.
// Полностью детерминированно, без LLM.
func (m *Mutator) GenerateMutations() []Mutation {
	var mutations []Mutation
	goFiles := m.ws.GoFiles(100)

	for _, rel := range goFiles {
		if len(mutations) >= m.limit {
			break
		}
		full, err := security.SafeJoin(m.ws.Root, rel)
		if err != nil {
			continue
		}
		data, err := os.ReadFile(full)
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")

		for lineNum, line := range lines {
			if len(mutations) >= m.limit {
				break
			}
			trimmed := strings.TrimSpace(line)
			// Пропускаем комментарии и пустые строки
			if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") {
				continue
			}
			for _, rule := range mutationRules {
				idx := strings.Index(line, rule.From)
				if idx == -1 {
					continue
				}
				// Защита от ложных срабатываний
				if isFalsePositive(line, idx, rule.From) {
					continue
				}
				mutated := line[:idx] + rule.To + line[idx+len(rule.From):]
				mutations = append(mutations, Mutation{
					File:     rel,
					Line:     lineNum + 1,
					Original: line,
					Mutated:  mutated,
					Type:     rule.Type,
				})
				break // одна мутация на строку
			}
		}
	}
	return mutations
}

// isFalsePositive проверяет, не является ли найденный оператор частью другого.
func isFalsePositive(line string, idx int, op string) bool {
	switch op {
	case ">=":
		// Проверяем, что перед >= нет ! или другого символа
		if idx > 0 && (line[idx-1] == '!' || line[idx-1] == '<') {
			return true
		}
	case "<=":
		if idx > 0 && (line[idx-1] == '>' || line[idx-1] == '-') {
			return true
		}
	case "==":
		// Проверяем, что это не часть != или <=
		if idx > 0 && (line[idx-1] == '!' || line[idx-1] == '<' || line[idx-1] == '>') {
			return true
		}
		// Проверяем, что это не часть ===
		if idx+2 < len(line) && line[idx+2] == '=' {
			return true
		}
	case "!=":
		if idx > 0 && line[idx-1] == '!' {
			return true
		}
	case "&&":
		// Убеждаемся, что это не часть строки или комментария
		if idx > 0 && line[idx-1] == '&' {
			return true
		}
	case "||":
		if idx > 0 && line[idx-1] == '|' {
			return true
		}
	}
	return false
}

// Run выполняет мутационное тестирование.
// Для каждой мутации: копирует проект в песочницу, применяет мутацию,
// запускает тесты, фиксирует результат, удаляет песочницу.
// Если тесты упали — мутация «убита» (хорошо).
// Если тесты прошли — мутация «выжила» (тесты слабые).
func (m *Mutator) Run(ctx context.Context, mutations []Mutation, emit func(domain.Event)) *MutationReport {
	report := &MutationReport{Mutations: mutations}
	total := len(mutations)

	for i := range mutations {
		if ctx.Err() != nil {
			break
		}

        if emit != nil {
            emit(domain.Event{
                Type:    domain.EventLog,
                Message: i18n.T("Mutation %d/%d: %s:%d (%s)",
                    i+1, total, mutations[i].File, mutations[i].Line, mutations[i].Type),
            })
        }
		// Создаём песочницу
		sandbox, err := m.ws.PrepareSandbox(ctx)
		if err != nil {
			mutations[i].Error = "sandbox: " + err.Error()
			report.Errors++
			continue
		}

		// Применяем мутацию в песочнице
		target, err := security.SafeJoin(sandbox, mutations[i].File)
		if err != nil {
			os.RemoveAll(sandbox)
			mutations[i].Error = "path: " + err.Error()
			report.Errors++
			continue
		}
		data, err := os.ReadFile(target)
		if err != nil {
			os.RemoveAll(sandbox)
			mutations[i].Error = "read: " + err.Error()
			report.Errors++
			continue
		}
		content := string(data)
		idx := strings.Index(content, mutations[i].Original)
		if idx == -1 {
			os.RemoveAll(sandbox)
			mutations[i].Error = "original line not found in sandbox"
			report.Errors++
			continue
		}
		mutated := content[:idx] + mutations[i].Mutated + content[idx+len(mutations[i].Original):]
		if err := os.WriteFile(target, []byte(mutated), 0o644); err != nil {
			os.RemoveAll(sandbox)
			mutations[i].Error = "write: " + err.Error()
			report.Errors++
			continue
		}

		// Запускаем тесты в песочнице
		tests, testErr := m.runner.Test(ctx, sandbox)
		os.RemoveAll(sandbox)

		if testErr != nil || tests.Failed > 0 {
			// Тесты упали → мутация убита → тесты сильные
			mutations[i].Killed = true
			report.Killed++
		} else {
			// Тесты прошли → мутация выжила → тесты слабые
			report.Survived++
		}
	}
	return report
}

// Format формирует человекочитаемый отчёт.
func (r *MutationReport) Format() string {
	total := r.Killed + r.Survived
	if total == 0 {
		return "## Mutation Testing Report\nNo mutations were tested (no applicable operators found or all errors)."
	}
	score := float64(r.Killed) / float64(total) * 100
	var b strings.Builder
	b.WriteString("## Mutation Testing Report\n\n")
	fmt.Fprintf(&b, "**Mutation Score: %.1f%%** (%d killed / %d survived / %d total", score, r.Killed, r.Survived, total)
	if r.Errors > 0 {
		fmt.Fprintf(&b, ", %d errors", r.Errors)
	}
	b.WriteString(")\n\n")

	// Сначала выжившие (проблемные)
	hasSurvived := false
	for _, m := range r.Mutations {
		if !m.Killed && m.Error == "" {
			if !hasSurvived {
				b.WriteString("### ✗ Survived mutations (weak tests)\n")
				hasSurvived = true
			}
			fmt.Fprintf(&b, "- `%s:%d` (%s): `%s` → `%s`\n",
				m.File, m.Line, m.Type,
				strings.TrimSpace(m.Original), strings.TrimSpace(m.Mutated))
		}
	}
	if hasSurvived {
		b.WriteString("\n")
	}

	// Убитые
	hasKilled := false
	for _, m := range r.Mutations {
		if m.Killed {
			if !hasKilled {
				b.WriteString("### ✓ Killed mutations (strong tests)\n")
				hasKilled = true
			}
			fmt.Fprintf(&b, "- `%s:%d` (%s)\n", m.File, m.Line, m.Type)
		}
	}
	if hasKilled {
		b.WriteString("\n")
	}

	// Ошибки
	if r.Errors > 0 {
		b.WriteString("### ⚠ Errors\n")
		for _, m := range r.Mutations {
			if m.Error != "" {
				fmt.Fprintf(&b, "- `%s:%d`: %s\n", m.File, m.Line, m.Error)
			}
		}
	}
	return b.String()
}