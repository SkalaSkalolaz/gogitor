package app

import (
	"context"
	"fmt"
	"net/url"
	"strings"

    "gogitor/internal/llm"
	"gogitor/internal/agent"
	"gogitor/internal/config"
	"gogitor/internal/domain"
	"gogitor/internal/prompts"
)

// ExecutionMode — режим выполнения задачи.
type ExecutionMode string

const (
	ExecutionModeAuto     ExecutionMode = "auto"
	ExecutionModeFast     ExecutionMode = "fast"
	ExecutionModeAgent    ExecutionMode = "agent"
	ExecutionModeWorkflow ExecutionMode = "workflow"
)

// ExecutionStrategy — результат выбора режима.
type ExecutionStrategy struct {
	Mode       ExecutionMode
	Confidence int
	Complexity string
	Risk       string
	Reason     string
	Source     string
	AskUser    bool
}

type modelProfile string

const (
	modelProfileSmall   modelProfile = "small"
	modelProfileMedium  modelProfile = "medium"
	modelProfileLarge   modelProfile = "large"
	modelProfileUnknown modelProfile = "unknown"
)

// normalizeExecutionMode приводит пользовательский режим к каноническому виду.
func normalizeExecutionMode(mode string) ExecutionMode {
	m := strings.ToLower(strings.TrimSpace(mode))

	switch m {
	case "fast", "быстро", "quick", "simple":
		return ExecutionModeFast
	case "agent", "агент", "multi-agent", "multiagent":
		return ExecutionModeAgent
	case "workflow", "harness", "воркфлоу", "workflow-lite":
		return ExecutionModeWorkflow
	case "auto", "", "default", "off":
		return ExecutionModeAuto
	default:
		return ExecutionModeAuto
	}
}

// chooseExecutionStrategy выбирает режим выполнения задачи.
//
// Приоритеты:
// 1. Явный пользовательский режим.
// 2. Детерминированные правила.
// 3. LLM-подсказка для внешних провайдеров.
// 4. Безопасный fallback.
func (s *Service) chooseExecutionStrategy(
	ctx context.Context,
	task string,
	opts Options,
	emit func(domain.Event),
) ExecutionStrategy {
	requested := normalizeExecutionMode(opts.Mode)

	if requested == ExecutionModeAuto && strings.TrimSpace(s.Cfg.WorkflowMode) != "" {
		requested = normalizeExecutionMode(s.Cfg.WorkflowMode)
	}

	if requested != ExecutionModeAuto {
		return ExecutionStrategy{
			Mode:    requested,
			Reason:  "explicit execution mode requested",
			Source:  "user",
			AskUser: false,
		}
	}

	score, reasons := s.taskComplexityScore(task)
	reason := strings.Join(reasons, "; ")
	profile := s.modelProfile()
	local := s.isLocalModelEndpoint()

	threshold := s.Cfg.WorkflowLocalComplexThreshold
	if threshold <= 0 {
		threshold = 6
	}

	// Простые задачи всегда быстрее дешевле выполнять без оркестрации.
	if score <= 2 {
		return ExecutionStrategy{
			Mode:    ExecutionModeFast,
			Reason:  "low complexity: " + reason,
			Source:  "rules",
			AskUser: false,
		}
	}

	// Для локальных small/medium моделей сложные задачи лучше сразу
	// отправлять в workflow-lite, чтобы уменьшить риск потери контекста.
	if local &&
		(profile == modelProfileSmall || profile == modelProfileMedium) &&
		score >= threshold {
		return ExecutionStrategy{
			Mode:    ExecutionModeWorkflow,
			Reason:  fmt.Sprintf("local model and high complexity score %d", score),
			Source:  "rules",
			AskUser: false,
		}
	}

	// Локальные medium-модели для средних задач обычно лучше работают
	// в agent режиме, а не в полном workflow.
	if local && score >= 4 {
		return ExecutionStrategy{
			Mode:    ExecutionModeAgent,
			Reason:  fmt.Sprintf("local model and medium complexity score %d", score),
			Source:  "rules",
			AskUser: false,
		}
	}

	// Для внешних провайдеров можно спросить LLM, но только если задача
	// уже выглядит средней или сложной.
	if !local && score >= 4 {
		strategy, err := s.llmExecutionStrategy(ctx, task, score, profile, emit)
		if err == nil {
			return strategy
		}

		sendEvent(
			emit,
			domain.EventWarn,
			fmt.Sprintf("Execution strategy LLM failed, fallback to rules: %v", err),
		)
	}

	// Последний fallback: сложную задачу безопаснее отдать агенту.
	if score >= threshold {
		return ExecutionStrategy{
			Mode:    ExecutionModeAgent,
			Reason:  fmt.Sprintf("high complexity fallback, score %d", score),
			Source:  "rules",
			AskUser: false,
		}
	}

	return ExecutionStrategy{
		Mode:    ExecutionModeAuto,
		Reason:  "default heuristic",
		Source:  "rules",
		AskUser: false,
	}
}

// llmExecutionStrategy запрашивает стратегию у LLM.
func (s *Service) llmExecutionStrategy(
	ctx context.Context,
	task string,
	score int,
	profile modelProfile,
	emit func(domain.Event),
) (ExecutionStrategy, error) {
	prompt := prompts.ExecutionStrategy(
		task,
		s.projectSummary(),
		string(profile),
		score,
	)
	if !s.Cfg.ReasoningRouter {
		ctx = llm.WithReasoningDisabled(ctx)
	}
	var out struct {
		ExecutionMode string `json:"execution_mode"`
		Confidence    int    `json:"confidence"`
		Complexity    string `json:"complexity"`
		Risk          string `json:"risk"`
		Reason        string `json:"reason"`
		AskUser       bool   `json:"ask_user"`
	}
	err := s.sendAgentJSON(
		ctx,
		agent.RoleRouter,
		agent.PriorityHigh,
		"choose execution strategy",
		prompt,
		&out,
	)
	if err != nil {
		return ExecutionStrategy{}, err
	}
	mode := normalizeExecutionMode(out.ExecutionMode)
	if mode == ExecutionModeAuto {
		mode = ExecutionModeAgent
	}
	// Защитные ограничения: не даём LLM выбрать слишком лёгкий режим
	// для потенциально сложной задачи.
	if score >= 8 && mode == ExecutionModeFast {
		mode = ExecutionModeAgent
		if out.Reason == "" {
			out.Reason = "forced agent due to high complexity"
		} else {
			out.Reason += "; forced agent due to high complexity"
		}
	}
	// И слишком тяжёлый режим для тривиальной задачи.
	if score <= 2 && mode == ExecutionModeWorkflow {
		mode = ExecutionModeFast
		if out.Reason == "" {
			out.Reason = "forced fast due to low complexity"
		} else {
			out.Reason += "; forced fast due to low complexity"
		}
	}
	if out.AskUser && s.Cfg.WorkflowAskUser {
		sendEvent(
			emit,
			domain.EventWarn,
			"LLM suggested asking user before choosing execution mode; continuing with "+string(mode),
		)
	}
	return ExecutionStrategy{
		Mode:       mode,
		Confidence: out.Confidence,
		Complexity: out.Complexity,
		Risk:       out.Risk,
		Reason:     out.Reason,
		Source:     "llm",
		AskUser:    out.AskUser,
	}, nil
}

// taskComplexityScore оценивает сложность задачи детерминированно.
func (s *Service) taskComplexityScore(task string) (int, []string) {
	lower := strings.ToLower(task)
	score := 0
	var reasons []string

	if len(strings.Fields(task)) > 14 {
		score += 2
		reasons = append(reasons, "long task")
	}

	highKeywords := []string{
		"refactor",
		"architecture",
		"split",
		"module",
		"api",
		"server",
		"auth",
		"database",
		"middleware",
		"interface",
		"migration",
		"test",
		"tests",
		"рефактор",
		"архитект",
		"раздели",
		"вынеси",
		"много файлов",
		"сервер",
		"база",
		"тест",
	}

	if containsAny(lower, highKeywords) {
		score += 3
		reasons = append(reasons, "high-complexity keywords")
	}

	mediumKeywords := []string{
		"add",
		"modify",
		"update",
		"fix",
		"improve",
		"добавь",
		"измени",
		"исправь",
		"улучши",
	}

	if containsAny(lower, mediumKeywords) {
		score += 1
		reasons = append(reasons, "medium-complexity keywords")
	}

	files := extractTargetFiles(task)
	if len(files) > 1 {
		score += 2
		reasons = append(reasons, fmt.Sprintf("mentions %d files", len(files)))
	}
	if len(files) > 3 {
		score += 1
		reasons = append(reasons, "many mentioned files")
	}

	if s.WS.HasGoFiles() {
		score += 1
		reasons = append(reasons, "existing Go project")
	}

	if idx := s.WS.ExistingIndex(); idx != nil && idx.Ready() && idx.FileCount() > 20 {
		score += 1
		reasons = append(reasons, "large indexed project")
	}

	if score > 10 {
		score = 10
	}

	return score, reasons
}

// modelProfile определяет условный класс модели.
func (s *Service) modelProfile() modelProfile {
	cfgProfile := strings.ToLower(strings.TrimSpace(s.Cfg.WorkflowModelProfile))

	switch cfgProfile {
	case "small":
		return modelProfileSmall
	case "medium":
		return modelProfileMedium
	case "large":
		return modelProfileLarge
	}

	lower := strings.ToLower(s.Cfg.Model)

	largeKeywords := []string{
		"70b",
		"72b",
		"123b",
		"236b",
		"405b",
		"gpt-4",
		"gpt-5",
		"claude-3",
		"claude-4",
		"o1",
		"o3",
	}

	if containsAny(lower, largeKeywords) {
		return modelProfileLarge
	}

	mediumKeywords := []string{
		"12b",
		"13b",
		"14b",
		"20b",
		"27b",
		"30b",
		"31b",
		"32b",
	}

	if containsAny(lower, mediumKeywords) {
		return modelProfileMedium
	}

	smallKeywords := []string{
		"1b",
		"2b",
		"3b",
		"4b",
		"7b",
		"8b",
		"9b",
	}

	if containsAny(lower, smallKeywords) {
		return modelProfileSmall
	}

	ctxTokens := s.Cfg.EffectiveContextTokens()

	switch {
	case ctxTokens <= 32768:
		return modelProfileSmall
	case ctxTokens <= 131072:
		return modelProfileMedium
	default:
		return modelProfileLarge
	}
}

// isLocalModelEndpoint определяет, похож ли endpoint на локальный.
func (s *Service) isLocalModelEndpoint() bool {
	provider := strings.ToLower(strings.TrimSpace(s.Cfg.Provider))

	if provider == "ollama" {
		base := s.Cfg.OllamaURL
		if base == "" {
			base = "http://localhost:11434"
		}
		return urlHostIsLocal(base)
	}

	if strings.HasPrefix(provider, "http://") || strings.HasPrefix(provider, "https://") {
		return urlHostIsLocal(provider)
	}

	if base, ok := config.OpenAIBaseFromProvider(s.Cfg.Provider); ok {
		return urlHostIsLocal(base)
	}

	return false
}

func urlHostIsLocal(rawURL string) bool {
	if strings.TrimSpace(rawURL) == "" {
		return true
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	host := strings.ToLower(u.Hostname())

	switch host {
	case "", "localhost", "127.0.0.1", "0.0.0.0", "::1":
		return true
	}

	if strings.HasPrefix(host, "192.168.") ||
		strings.HasPrefix(host, "10.") ||
		strings.HasPrefix(host, "172.") {
		return true
	}

	return false
}