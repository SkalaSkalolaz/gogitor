package app

import (
	"context"
	"fmt"
	"strings"

	"gogitor/internal/agent"
	"gogitor/internal/domain"
	"gogitor/internal/i18n"
	"gogitor/internal/llm"
	"gogitor/internal/prompts"
	"gogitor/internal/search"
)

// ArticleMode — режим генерации статьи.
type ArticleMode int

const (
	ArticleModeSimple ArticleMode = iota
	ArticleModeComplex
)

// ArticleOptions — опции генерации статьи.
type ArticleOptions struct {
	Mode ArticleMode
}

// articleClassification — результат классификации запроса.
type articleClassification struct {
	Genre          string `json:"genre"`
	NeedWebSearch  bool   `json:"need_web_search"`
	NeedProjectCtx bool   `json:"need_project_context"`
}

// articlePlan — структура плана статьи.
type articlePlan struct {
	Title    string               `json:"title"`
	Sections []articlePlanSection `json:"sections"`
}

type articlePlanSection struct {
	Heading   string   `json:"heading"`
	KeyPoints []string `json:"key_points"`
}

// Article генерирует текст в режиме article.
func (s *Service) Article(
	ctx context.Context,
	topic string,
	opts ArticleOptions,
	emit func(domain.Event),
) domain.Result {
	ctx = agent.WithStatusFunc(ctx, s.agentStatusEmitter(emit))
	emitEvent(emit, domain.Event{
		Type:      domain.EventAgent,
		Message:   i18n.Localize("current stage: article"),
		TaskStage: domain.TaskStageArticle,
	})
	lang := "English"
	if DetectLanguage() == "ru" {
		lang = "Russian"
	}

	// Определяем жанр и параметры.
	genre, needWeb, needProject := s.articleClassify(ctx, topic, lang, emit)

	sendEvent(emit, domain.EventLog,
		fmt.Sprintf("Article mode: genre=%s, web_search=%v, project_context=%v",
			genre, needWeb, needProject))

	// Собираем контекст проекта.
	projectContext := ""
	if needProject {
		maxFiles, maxBytes := s.contextLimits()
		projectContext = s.WS.BuildSmartContext(topic, nil, maxFiles/4, maxBytes/4)
	}

	// Поиск в интернете.
	webContext := ""
	if needWeb {
		webContext = s.articleWebSearch(ctx, topic, emit)
	}

	// Объединяем контексты.
	combinedContext := projectContext
	if webContext != "" {
		if combinedContext != "" {
			combinedContext += "\n\n"
		}
		combinedContext += webContext
	}

	switch opts.Mode {
	case ArticleModeComplex:
		return s.articleComplex(ctx, topic, combinedContext, lang, genre, emit)
	default:
		return s.articleSimple(ctx, topic, combinedContext, lang, genre, emit)
	}
}

// articleClassify определяет жанр и параметры статьи.
func (s *Service) articleClassify(
	ctx context.Context,
	topic, lang string,
	emit func(domain.Event),
) (genre string, needWeb bool, needProject bool) {
	// Быстрая эвристика по ключевым словам.
	lower := strings.ToLower(topic)
	genre = "free"
	needWeb = false
	needProject = false

	if containsAny(lower, []string{
		"новост", "news", "релиз", "release", "выпуск", "анонс",
	}) {
		genre = "news"
		needWeb = true
	} else if containsAny(lower, []string{
		"рассказ", "история", "сказка", "story", "tale", "fiction",
	}) {
		genre = "story"
	} else if containsAny(lower, []string{
		"сравни", "обзор", "vs", "review", "comparison", "benchmark",
	}) {
		genre = "review"
		needWeb = true
	} else if containsAny(lower, []string{
		"как сделать", "инструкци", "how to", "tutorial", "гайд", "руководство",
	}) {
		genre = "howto"
	} else if containsAny(lower, []string{
		"опиши код", "что делает", "объясни код", "describe code", "explain function",
	}) {
		genre = "code_desc"
		needProject = true
	} else if containsAny(lower, []string{
		"статья", "article", "техническ", "technical", "документаци",
	}) {
		genre = "technical"
		needProject = s.WS.HasGoFiles()
	}

	// Для неоднозначных случаев пытаемся уточнить через LLM.
	if genre == "free" && s.Cfg.EffectiveContextTokens() > 16384 {
		classCtx := agent.WithRole(ctx, agent.RoleRouter)
		classCtx = agent.WithPriority(classCtx, agent.PriorityNormal)
		classCtx = agent.WithPurpose(classCtx, "article classification")
		classCtx = llm.WithReasoningDisabled(classCtx)
		prompt := prompts.ArticleClassify(topic, lang)
		var cls articleClassification
		if err := s.sendAgentJSON(classCtx, agent.RoleRouter, agent.PriorityNormal,
			"article classification", prompt, &cls); err == nil {
			if cls.Genre != "" {
				genre = cls.Genre
			}
			needWeb = cls.NeedWebSearch
			needProject = cls.NeedProjectCtx
		}
	}

	return genre, needWeb, needProject
}

// articleWebSearch выполняет поиск в интернете для статьи.
func (s *Service) articleWebSearch(
	ctx context.Context,
	topic string,
	emit func(domain.Event),
) string {
	if s.SafeSearch == nil {
		return ""
	}

	sendEvent(emit, domain.EventLog, "Searching web for article context...")

	result, err := s.SafeSearch.Search(ctx, topic)
	if err != nil {
		sendEvent(emit, domain.EventWarn,
			fmt.Sprintf("Web search failed (non-fatal): %v", err))
		return ""
	}

	sendEvent(emit, domain.EventLog,
		fmt.Sprintf("Web search: found %d source(s)", len(result.Sources)))

	return search.FormatForPrompt(result)
}

// articleSimple — генерация короткого текста (один LLM-вызов).
func (s *Service) articleSimple(
	ctx context.Context,
	topic, context, lang, genre string,
	emit func(domain.Event),
) domain.Result {
	sendEvent(emit, domain.EventAgent, "current stage: article (simple)")
	sendEvent(emit, domain.EventLog, "Generating simple article")

	prompt := prompts.ArticleSimple(topic, context, lang, genre)

	response, err := s.sendLLMStreaming(
		ctx,
		prompt,
		emit,
		agent.RoleDefault,
		agent.PriorityNormal,
		"article_simple",
	)
	if err != nil {
		return domain.Result{
			Success: false,
			Mode:    "article",
			Errors:  []string{err.Error()},
		}
	}

	return domain.Result{
		Success:  true,
		Mode:     "article",
		Response: response,
	}
}

// articleComplex — генерация сложной статьи (план → секции → сборка).
func (s *Service) articleComplex(
	ctx context.Context,
	topic, context, lang, genre string,
	emit func(domain.Event),
) domain.Result {
	sendEvent(emit, domain.EventAgent, "current stage: article (complex)")

	maxSections := s.articleMaxSections()

	// ─── Шаг 1: Генерация плана ───────────────────────────────
	sendEvent(emit, domain.EventLog, "Generating article plan")

	plan, err := s.generateArticlePlan(ctx, topic, context, lang, genre, maxSections, emit)
	if err != nil {
		sendEvent(emit, domain.EventWarn,
			fmt.Sprintf("Plan generation failed, falling back to simple mode: %v", err))
		return s.articleSimple(ctx, topic, context, lang, genre, emit)
	}

	sendEvent(emit, domain.EventLog,
		fmt.Sprintf("Plan ready: %d sections", len(plan.Sections)))

	// Показываем план в TUI.
	planItems := make([]string, len(plan.Sections))
	for i, sec := range plan.Sections {
		planItems[i] = sec.Heading
	}
	sendPlanBoard(emit, plan.Title, nil, planItems)

	// ─── Шаг 2: Написание секций ──────────────────────────────
	var fullArticle strings.Builder
	var prevSections strings.Builder

	for i, sec := range plan.Sections {
		sendEvent(emit, domain.EventLog,
			fmt.Sprintf("Writing section %d/%d: %s", i+1, len(plan.Sections), sec.Heading))

		sendPlanStatus(emit, i+1, len(plan.Sections), sec.Heading, domain.PlanRunning, "")

		isLast := i == len(plan.Sections)-1
		prompt := prompts.ArticleSection(
			topic,
			plan.Title,
			sec.Heading,
			sec.KeyPoints,
			prevSections.String(),
			context,
			lang,
			genre,
			isLast,
		)

		sectionText, err := s.sendLLMStreaming(
			ctx,
			prompt,
			emit,
			agent.RoleDefault,
			agent.PriorityNormal,
			fmt.Sprintf("article_section_%d", i+1),
		)
		if err != nil {
			sendPlanStatus(emit, i+1, len(plan.Sections), sec.Heading,
				domain.PlanFailed, err.Error())
			sendEvent(emit, domain.EventWarn,
				fmt.Sprintf("Section %d failed: %v", i+1, err))
			continue
		}

		fullArticle.WriteString(sectionText)
		fullArticle.WriteString("\n\n")
		prevSections.WriteString(sectionText)
		prevSections.WriteString("\n\n")

		sendPlanStatus(emit, i+1, len(plan.Sections), sec.Heading,
			domain.PlanDone, "")
	}

	sendPlanSummary(emit, domain.PlanDone, len(plan.Sections), len(plan.Sections))

	return domain.Result{
		Success:  true,
		Mode:     "article",
		Response: fullArticle.String(),
	}
}

// generateArticlePlan генерирует план статьи.
func (s *Service) generateArticlePlan(
	ctx context.Context,
	topic, context, lang, genre string,
	maxSections int,
	emit func(domain.Event),
) (*articlePlan, error) {
	planCtx := agent.WithRole(ctx, agent.RolePlanner)
	planCtx = agent.WithPriority(planCtx, agent.PriorityHigh)
	planCtx = agent.WithPurpose(planCtx, "article plan")

	prompt := prompts.ArticlePlan(topic, context, lang, genre, maxSections)

	var plan articlePlan
	err := s.sendAgentJSON(planCtx, agent.RolePlanner, agent.PriorityHigh,
		"article plan", prompt, &plan)
	if err == nil && len(plan.Sections) > 0 {
		if plan.Title == "" {
			plan.Title = topic
		}
		if len(plan.Sections) > maxSections {
			plan.Sections = plan.Sections[:maxSections]
		}
		return &plan, nil
	}

	// Fallback для малых моделей.
	sendEvent(emit, domain.EventLog, "JSON plan failed, trying text fallback")
	return s.generateArticlePlanFallback(ctx, topic, lang, maxSections)
}

// generateArticlePlanFallback парсит текстовый план.
func (s *Service) generateArticlePlanFallback(
	ctx context.Context,
	topic, lang string,
	maxSections int,
) (*articlePlan, error) {
	fbCtx := agent.WithRole(ctx, agent.RolePlanner)
	fbCtx = agent.WithPriority(fbCtx, agent.PriorityHigh)
	fbCtx = agent.WithPurpose(fbCtx, "article plan fallback")

	prompt := prompts.ArticlePlanFallback(topic, lang, maxSections)
	response, err := s.LLM.Send(fbCtx, prompt)
	if err != nil {
		return nil, err
	}

	plan := &articlePlan{Title: topic}
	lines := strings.Split(response, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		heading := extractSectionHeading(line)
		if heading != "" {
			plan.Sections = append(plan.Sections, articlePlanSection{
				Heading:   heading,
				KeyPoints: []string{heading},
			})
			if len(plan.Sections) >= maxSections {
				break
			}
		}
	}

	if len(plan.Sections) == 0 {
		return nil, fmt.Errorf("no sections found in plan response")
	}
	return plan, nil
}

// extractSectionHeading извлекает заголовок секции из строки.
func extractSectionHeading(line string) string {
	lower := strings.ToLower(line)
	for _, prefix := range []string{"section ", "секция ", "раздел "} {
		if strings.HasPrefix(lower, prefix) {
			rest := line[len(prefix):]
			for i, ch := range rest {
				if ch == ':' || ch == '.' || ch == '-' {
					return strings.TrimSpace(rest[i+1:])
				}
				if ch < '0' || ch > '9' {
					break
				}
			}
		}
	}
	if len(line) > 2 && line[0] >= '1' && line[0] <= '9' {
		if line[1] == '.' || line[1] == ')' {
			return strings.TrimSpace(line[2:])
		}
	}
	return ""
}

// articleMaxSections возвращает максимальное число секций
// в зависимости от размера контекста модели.
func (s *Service) articleMaxSections() int {
	ctxTokens := s.Cfg.EffectiveContextTokens()
	switch {
	case ctxTokens <= 8192:
		return 3
	case ctxTokens <= 32768:
		return 4
	case ctxTokens <= 65536:
		return 5
	case ctxTokens <= 131072:
		return 6
	default:
		return 7
	}
}
