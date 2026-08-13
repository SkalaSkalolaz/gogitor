package app

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"gogitor/internal/agent"
	"gogitor/internal/domain"
	"gogitor/internal/i18n"
	"gogitor/internal/prompts"
)

// generateComparison генерирует сравнительный анализ подходов через LLM.
func (s *Service) generateComparison(
	ctx context.Context,
	query string,
	emit func(domain.Event),
) (*domain.ComparisonResult, error) {
	ctx = agent.WithRole(ctx, agent.RolePlanner)
	ctx = agent.WithPriority(ctx, agent.PriorityHigh)
	ctx = agent.WithPurpose(ctx, "compare approaches")

	maxFiles, maxBytes := s.contextLimits()
	projectContext := s.WS.BuildSmartContext(query, nil, maxFiles/2, maxBytes/2)

	prompt := prompts.CompareApproaches(query, projectContext)
	response, err := s.LLM.Send(ctx, prompt)
	if err != nil {
		return nil, err
	}

	var comparison domain.ComparisonResult
	if err := parseAgentJSON(response, &comparison); err != nil {
		return nil, fmt.Errorf("cannot parse comparison response: %w", err)
	}

	if len(comparison.Approaches) < 2 {
		return nil, fmt.Errorf("LLM returned fewer than 2 approaches")
	}

	// Ограничиваем максимум 3 подхода
	if len(comparison.Approaches) > 3 {
		comparison.Approaches = comparison.Approaches[:3]
	}

	// Нормализуем ID
	for i := range comparison.Approaches {
		comparison.Approaches[i].ID = i + 1
	}

	return &comparison, nil
}

// parseApproachSelection пытается интерпретировать ввод как выбор подхода.
func (s *Service) parseApproachSelection(query string) (string, bool) {
	if s.pendingComparison == nil || s.pendingComparison.Comparison == nil {
		return "", false
	}

	q := strings.TrimSpace(query)
	qLower := strings.ToLower(q)
	approaches := s.pendingComparison.Comparison.Approaches

	if len(approaches) == 0 {
		return "", false
	}

	// 1. Прямой номер: "1", "2", "3"
	if n, err := strconv.Atoi(qLower); err == nil && n >= 1 && n <= len(approaches) {
		return formatApproachForPrompt(approaches[n-1]), true
	}

	// 2. Подтверждение → рекомендуемый подход
	confirmWords := []string{
		"да", "yes", "ок", "ok", "давай", "go", "хорошо", "well",
		"рекомендуемый", "recommended", "рекомендация", "recommendation",
		"согласен", "agree", "принимаю", "accept",
	}
	for _, w := range confirmWords {
		if qLower == w {
			for _, a := range approaches {
				if a.Recommended {
					return formatApproachForPrompt(a), true
				}
			}
			return formatApproachForPrompt(approaches[0]), true
		}
	}

	// 3. "вариант N", "variant N", "подход N", "approach N"
	for _, a := range approaches {
		num := strconv.Itoa(a.ID)
		for _, prefix := range []string{"вариант", "variant", "подход", "approach"} {
			if strings.Contains(qLower, prefix+" "+num) {
				return formatApproachForPrompt(a), true
			}
		}
	}

	// 4. Порядковые слова
	ordinals := map[string]int{
		"первый": 1, "первую": 1, "первое": 1, "первого": 1,
		"второй": 2, "вторую": 2, "второе": 2, "второго": 2,
		"третий": 3, "третью": 3, "третье": 3, "третьего": 3,
		"first": 1, "second": 2, "third": 3,
	}
	for word, id := range ordinals {
		if strings.Contains(qLower, word) && id <= len(approaches) {
			return formatApproachForPrompt(approaches[id-1]), true
		}
	}

    // 5. Явная модификация подхода.
    if isApproachModification(qLower) {
    	recommended := ""
    	for _, a := range approaches {
    		if a.Recommended {
    			recommended = formatApproachForPrompt(a)
    			break
    		}
    	}
    	if recommended != "" {
    		return recommended + "\nUSER MODIFICATION:\n" + q, true
    	}
    	return q, true
    }
    


	return "", false
}

// formatComparison формирует человекочитаемое представление сравнения.
func (s *Service) formatComparison(c *domain.ComparisonResult) string {
	var b strings.Builder
	b.WriteString("> " + i18n.T("Complex task detected: multi-agent mode enabled") + "\n")
	b.WriteString("## " + i18n.T("Comparative Analysis of Approaches") + "\n")
	b.WriteString("| # | " + i18n.T("Approach") + " | " + i18n.T("Complexity") + " | " +
		i18n.T("Performance") + " | " + i18n.T("Readability") + " | " +
		i18n.T("Dependencies") + " | " + i18n.T("Testability") + " |\n")
	b.WriteString("|---|----------|------------|-------------|-------------|--------------|-------------|\n")
	for _, a := range c.Approaches {
		rec := ""
		if a.Recommended {
			rec = " ⭐"
		}
		fmt.Fprintf(&b, "| %d%s | %s | %s | %s | %s | %s | %s |\n",
			a.ID, rec, a.Name, a.Complexity, a.Performance,
			a.Readability, a.Dependencies, a.Testability)
	}
	b.WriteString("\n### " + i18n.T("Details") + "\n")
	for _, a := range c.Approaches {
		rec := ""
		if a.Recommended {
			rec = " ⭐ " + i18n.T("RECOMMENDED")
		}
		b.WriteString("**" + fmt.Sprintf(i18n.T("Variant %d: %s"), a.ID, a.Name) + "**" + rec + "\n")
		fmt.Fprintf(&b, "%s\n", a.Description)
		if a.Justification != "" {
			fmt.Fprintf(&b, "*%s* %s\n", i18n.T("Justification:"), a.Justification)
		}
		if a.Tradeoffs != "" {
			fmt.Fprintf(&b, "*%s* %s\n", i18n.T("Trade-offs:"), a.Tradeoffs)
		}
		b.WriteString("\n")
	}
	if c.Recommendation != "" {
		fmt.Fprintf(&b, "### %s\n%s\n", i18n.T("Recommendation"), c.Recommendation)
	}
	b.WriteString("---\n")
	fmt.Fprintf(&b, "**%s** %s\n",
		i18n.T("Select a variant:"),
		i18n.T("type a number (1-%d), or describe your own approach, or type \"yes\" to accept the recommendation.", len(c.Approaches)))
	return b.String()
}

// formatApproachForPrompt форматирует подход для передачи в промпты.
func formatApproachForPrompt(a domain.Approach) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Approach %d: %s\n", a.ID, a.Name)
	fmt.Fprintf(&b, "Description: %s\n", a.Description)
	fmt.Fprintf(&b, "Complexity: %s\n", a.Complexity)
	fmt.Fprintf(&b, "Performance: %s\n", a.Performance)
	fmt.Fprintf(&b, "Readability: %s\n", a.Readability)
	fmt.Fprintf(&b, "Dependencies: %s\n", a.Dependencies)
	fmt.Fprintf(&b, "Testability: %s\n", a.Testability)
	fmt.Fprintf(&b, "Trade-offs: %s\n", a.Tradeoffs)
	return b.String()
}

func isApproachModification(lower string) bool {
	contrastMarkers := []string{
		"но ", "но,", "но.", "но:",
		"однако ", "однако,", "однако:",
		"вместо ",
		"but ", "but,", "but:",
		"however", "instead",
	}
	for _, m := range contrastMarkers {
		if strings.Contains(lower, m) {
			return true
		}
	}

	modifyVerbs := []string{
		"измени ", "изменить ", "измени:",
		"убери ", "убрать ", "убери:",
		"замени ", "заменить ", "замени:",
		"добавь к ", "добавь к:",
		"modify ", "change ", "remove ", "replace ",
	}
	for _, m := range modifyVerbs {
		if strings.Contains(lower, m) {
			return true
		}
	}

	modifyNouns := []string{
		"поправк",       // поправка, поправкой, поправки
		"корректиров",   // корректировка, корректировкой
		"модификац",     // модификация, модификацией
		"amendment", "correction", "modification",
	}
	for _, m := range modifyNouns {
		if strings.Contains(lower, m) {
			return true
		}
	}

	phrases := []string{
		"с поправкой", "с изменением", "но с ",
		"к подход", "к варианту",
		"with amendment", "with change", "but with",
		"to the approach", "to the variant",
	}
	for _, m := range phrases {
		if strings.Contains(lower, m) {
			return true
		}
	}

	return false
}

// approachSelectionResult — ответ LLM-селектора выбора подхода.
type approachSelectionResult struct {
	Action       string `json:"action"`       // "select" или "new_task"
	ApproachID   int    `json:"approach_id"`  // 1-based ID подхода
	Modification string `json:"modification"` // модификация, если есть
	Reason       string `json:"reason"`       // краткое пояснение интерпретации
}

func (s *Service) selectApproachViaLLM(
	ctx context.Context,
	userInput string,
	emit func(domain.Event),
) (string, bool) {
	if s.pendingComparison == nil || s.pendingComparison.Comparison == nil {
		return "", false
	}

	comparison := s.pendingComparison.Comparison
	if len(comparison.Approaches) == 0 {
		return "", false
	}

	sendEvent(
		emit,
		domain.EventLog,
		i18n.T("Interpreting approach selection via LLM..."),
	)

	prompt := prompts.ApproachSelection(
		comparison.Approaches,
		comparison.Recommendation,
		userInput,
	)

	ctx = agent.WithRole(ctx, agent.RoleRouter)
	ctx = agent.WithPriority(ctx, agent.PriorityHigh)
	ctx = agent.WithPurpose(ctx, "interpret approach selection")

	response, err := s.LLM.Send(ctx, prompt)
	if err != nil {
		sendEvent(
			emit,
			domain.EventWarn,
			i18n.T("Approach selection LLM failed, falling back: %v", err),
		)
		return "", false
	}

	var result approachSelectionResult
	if err := parseAgentJSON(response, &result); err != nil {
		sendEvent(
			emit,
			domain.EventWarn,
			i18n.T("Cannot parse approach selection response: %v", err),
		)
		return "", false
	}

	// Если LLM решила, что это новая задача — не выбираем подход.
	if result.Action != "select" {
		return "", false
	}

	// Валидация ID.
	if result.ApproachID < 1 || result.ApproachID > len(comparison.Approaches) {
		sendEvent(
			emit,
			domain.EventWarn,
			i18n.T(
				"LLM returned invalid approach_id=%d (valid: 1-%d)",
				result.ApproachID,
				len(comparison.Approaches),
			),
		)
		return "", false
	}

	approach := comparison.Approaches[result.ApproachID-1]
	formatted := formatApproachForPrompt(approach)

	// Если пользователь запросил модификацию — добавляем её.
	if strings.TrimSpace(result.Modification) != "" {
		formatted += "\nUSER MODIFICATION:\n" + result.Modification
	}

	if result.Reason != "" {
		sendEvent(
			emit,
			domain.EventLog,
			i18n.T("Selection reason: %s", result.Reason),
		)
	}

	return formatted, true
}