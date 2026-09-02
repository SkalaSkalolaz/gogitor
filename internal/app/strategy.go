package app

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"gogitor/internal/agent"
	"gogitor/internal/config"
	"gogitor/internal/domain"
	"gogitor/internal/llm"
	"gogitor/internal/prompts"
)

// ExecutionMode — режим выполнения задачи.
type ExecutionMode string

const (
	ExecutionModeAuto   ExecutionMode = "auto"
	ExecutionModeSimple ExecutionMode = "simple"
	ExecutionModeAgent  ExecutionMode = "agent"
)

// AgentDepth — глубина выполнения агента.
type AgentDepth string

const (
	AgentDepthNormal AgentDepth = "normal"
	AgentDepthDeep   AgentDepth = "deep"
	AgentDepthAuto   AgentDepth = "auto"
)

// ExecutionStrategy — результат выбора режима.
type ExecutionStrategy struct {
	Mode       ExecutionMode
	AgentDepth AgentDepth
	Confidence int
	Complexity string
	Risk       string
	Reason     string
	Source     string
}

type modelProfile string

const (
	modelProfileSmall   modelProfile = "small"
	modelProfileMedium  modelProfile = "medium"
	modelProfileLarge   modelProfile = "large"
	modelProfileUnknown modelProfile = "unknown"
)

var modelSizeRE = regexp.MustCompile(
	`(?:^|[^0-9])([0-9]+(?:\.[0-9]+)?)b(?:[^a-z0-9]|$)`,
)

func modelParameterCountB(name string) float64 {
	m := modelSizeRE.FindStringSubmatch(
		strings.ToLower(strings.TrimSpace(name)),
	)
	if len(m) != 2 {
		return 0
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil || v <= 0 {
		return 0
	}
	return v
}

func normalizeExecutionMode(mode string) ExecutionMode {
	m := strings.ToLower(strings.TrimSpace(mode))
	switch m {
	case "simple", "fast", "быстро", "quick":
		return ExecutionModeSimple
	case "agent", "агент", "multi-agent", "multiagent":
		return ExecutionModeAgent
	case "auto", "", "default":
		return ExecutionModeAuto
	default:
		return ExecutionModeAuto
	}
}

func normalizeAgentDepth(depth string) AgentDepth {
	switch strings.ToLower(strings.TrimSpace(depth)) {
	case "deep", "strict", "enhanced":
		return AgentDepthDeep
	case "normal", "standard":
		return AgentDepthNormal
	default:
		return AgentDepthAuto
	}
}

func (s *Service) agentDepthForTask(task string) AgentDepth {
	score, _ := s.taskComplexityScore(task)

	threshold := s.Cfg.AgentDeepComplexityThreshold
	if threshold <= 0 {
		threshold = 6
	}

	profile := s.modelProfile()

	if score >= threshold {
		return AgentDepthDeep
	}

	// Для небольших моделей даже средняя по score задача
	// заслуживает усиленного harness.
	if profile == modelProfileSmall && score >= 4 {
		return AgentDepthDeep
	}

	return AgentDepthNormal
}

// chooseExecutionStrategy выбирает режим выполнения задачи.
func (s *Service) chooseExecutionStrategy(
	ctx context.Context,
	task string,
	opts Options,
	emit func(domain.Event),
) ExecutionStrategy {
	requested := normalizeExecutionMode(opts.Mode)
	requestedDepth := normalizeAgentDepth(string(opts.AgentDepth))

	// Явно выбранный пользователем режим.
	if requested != ExecutionModeAuto {
		if requested == ExecutionModeSimple {
			return ExecutionStrategy{
				Mode:       ExecutionModeSimple,
				AgentDepth: AgentDepthNormal,
				Reason:     "explicit simple mode",
				Source:     "user",
			}
		}
		depth := requestedDepth
		if depth == AgentDepthAuto {
			depth = AgentDepthNormal
		}
		return ExecutionStrategy{
			Mode:       ExecutionModeAgent,
			AgentDepth: depth,
			Reason:     "explicit agent mode",
			Source:     "user",
		}
	}

	score, reasons := s.taskComplexityScore(task)
	reason := strings.Join(reasons, "; ")
	profile := s.modelProfile()
	local := s.isLocalModelEndpoint()

	threshold := s.Cfg.AgentDeepComplexityThreshold
	if threshold <= 0 {
		threshold = 6
	}

	// Простые задачи — без оркестрации.
	if score <= 2 {
		return ExecutionStrategy{
			Mode:       ExecutionModeSimple,
			AgentDepth: AgentDepthNormal,
			Reason:     "low complexity: " + reason,
			Source:     "rules",
		}
	}

	// Для локальных моделей сложные задачи → агент.
	if local && score >= threshold {
		return ExecutionStrategy{
			Mode:       ExecutionModeAgent,
			AgentDepth: AgentDepthDeep,
			Reason:     fmt.Sprintf("local model, high complexity score %d", score),
			Source:     "rules",
		}
	}

	// Средние задачи для локальных моделей.
	if local && score >= 4 {
		return ExecutionStrategy{
			Mode:       ExecutionModeAgent,
			AgentDepth: AgentDepthNormal,
			Reason:     fmt.Sprintf("local model, medium complexity score %d", score),
			Source:     "rules",
		}
	}

	// Для внешних провайдеров — LLM-подсказка.
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

	// Fallback: сложную задачу отдаём агенту.
	if score >= threshold {
		return ExecutionStrategy{
			Mode:       ExecutionModeAgent,
			AgentDepth: AgentDepthDeep,
			Reason:     fmt.Sprintf("high complexity fallback, score %d", score),
			Source:     "rules",
		}
	}

	return ExecutionStrategy{
		Mode:       ExecutionModeAgent,
		AgentDepth: AgentDepthNormal,
		Reason:     "default heuristic",
		Source:     "rules",
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
		AgentDepth    string `json:"agent_depth"`
		Confidence    int    `json:"confidence"`
		Complexity    string `json:"complexity"`
		Risk          string `json:"risk"`
		Reason        string `json:"reason"`
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
	depth := normalizeAgentDepth(out.AgentDepth)
	if depth == AgentDepthAuto {
		depth = AgentDepthNormal
	}

	// Защитные ограничения.
	if score >= 8 && mode == ExecutionModeSimple {
		mode = ExecutionModeAgent
		depth = AgentDepthDeep
	}
	if score <= 2 && mode == ExecutionModeAgent {
		mode = ExecutionModeSimple
		depth = AgentDepthNormal
	}

	return ExecutionStrategy{
		Mode:       mode,
		AgentDepth: depth,
		Confidence: out.Confidence,
		Complexity: out.Complexity,
		Risk:       out.Risk,
		Reason:     out.Reason,
		Source:     "llm",
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
		"refactor", "architecture", "split", "module", "api",
		"server", "auth", "database", "middleware", "interface",
		"migration", "test", "tests",
		"рефактор", "архитект", "раздели", "вынеси",
		"много файлов", "сервер", "база", "тест",
	}
	if containsAny(lower, highKeywords) {
		score += 3
		reasons = append(reasons, "high-complexity keywords")
	}
	mediumKeywords := []string{
		"add", "modify", "update", "fix", "improve",
		"добавь", "измени", "исправь", "улучши",
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
	cfgProfile := strings.ToLower(strings.TrimSpace(s.Cfg.AgentModelProfile))
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
		"70b", "72b", "123b", "236b", "405b",
		"gpt-4", "gpt-5", "claude-3", "claude-4", "o1", "o3",
	}
	if containsAny(lower, largeKeywords) {
		return modelProfileLarge
	}
	if size := modelParameterCountB(s.Cfg.Model); size > 0 {
		switch {
		case size <= 9:
			return modelProfileSmall
		case size <= 32:
			return modelProfileMedium
		default:
			return modelProfileLarge
		}
	}
	mediumKeywords := []string{"12b", "13b", "14b", "20b", "27b", "30b", "31b", "32b"}
	if containsAny(lower, mediumKeywords) {
		return modelProfileMedium
	}
	smallKeywords := []string{"1b", "2b", "3b", "4b", "7b", "8b", "9b"}
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
	// Локальный endpoint не означает локальную модель.
	// Если модель явно обозначена как cloud/remote/hosted,
	// считаем её внешней даже при Ollama localhost.
	if isClearlyRemoteModel(s.Cfg.Model) {
		return false
	}

	provider := strings.ToLower(
		strings.TrimSpace(s.Cfg.Provider),
	)

	if provider == "ollama" {
		base := s.Cfg.OllamaURL
		if base == "" {
			base = "http://localhost:11434"
		}

		return urlHostIsLocal(base)
	}

	if strings.HasPrefix(provider, "http://") ||
		strings.HasPrefix(provider, "https://") {
		return urlHostIsLocal(provider)
	}

	if base, ok :=
		config.OpenAIBaseFromProvider(
			s.Cfg.Provider,
		); ok {
		return urlHostIsLocal(base)
	}

	return false
}

func isClearlyRemoteModel(model string) bool {
	lower := strings.ToLower(
		strings.TrimSpace(model),
	)

	if lower == "" {
		return false
	}

	markers := []string{
		"cloud",
		"remote",
		"hosted",
		"online",
	}

	return containsAny(lower, markers)
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

func isRemovedWorkflowMode(
	mode string,
) bool {
	switch strings.ToLower(
		strings.TrimSpace(mode),
	) {
	case "workflow",
		"harness",
		"workflow-lite",
		"воркфлоу":
		return true

	default:
		return false
	}
}
