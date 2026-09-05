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

type executionSignals struct {
	Score         int
	Reasons       []string
	TargetFiles   int
	RequiresAgent bool
	BroadTask     bool
}

func taskRequiresAgent(task string) (bool, []string) {
	lower := strings.ToLower(strings.TrimSpace(task))

	architecturalKeywords := []string{
		"refactor",
		"refactoring",
		"architecture",
		"architectural",
		"restructure",
		"reorganize",
		"redesign",
		"migration",
		"migrate",
		"split",
		"divide",
		"extract",
		"move to a package",
		"move into a package",
		"create package",
		"new package",

		"рефактор",
		"рефакторинг",
		"архитектур",
		"архитект",
		"реструктур",
		"перестрой",
		"перенастрой архитект",
		"перепроект",
		"миграц",
		"раздели",
		"разделить",
		"разбей",
		"разбить",
		"вынеси",
		"вынести",
		"перенеси",
		"перенести",
		"создай пакет",
		"добавь пакет",
		"новый пакет",
	}

	var reasons []string

	for _, keyword := range architecturalKeywords {
		if strings.Contains(lower, keyword) {
			reasons = append(
				reasons,
				"architectural or structural change",
			)
			break
		}
	}

	broadKeywords := []string{
		"entire project",
		"whole project",
		"all packages",
		"throughout the project",
		"system-wide",
		"across the project",

		"весь проект",
		"по всему проекту",
		"во всём проекте",
		"во всем проекте",
		"все пакеты",
		"во всех пакетах",
	}

	broad := containsAny(lower, broadKeywords)
	if broad {
		reasons = append(reasons, "broad project-wide scope")
	}

	return len(reasons) > 0, reasons
}

func (s *Service) executionSignals(task string) executionSignals {
	score, reasons := s.taskComplexityScore(task)

	targetFiles := extractTargetFiles(task)
	requiresAgent, agentReasons := taskRequiresAgent(task)

	reasons = append(reasons, agentReasons...)

	if len(targetFiles) > 3 {
		requiresAgent = true
		reasons = append(
			reasons,
			"many explicitly mentioned files",
		)
	}

	lower := strings.ToLower(task)

	broadKeywords := []string{
		"entire project",
		"whole project",
		"all packages",
		"throughout the project",
		"system-wide",
		"across the project",

		"весь проект",
		"по всему проекту",
		"во всём проекте",
		"во всем проекте",
		"все пакеты",
		"во всех пакетах",
	}

	broad := containsAny(lower, broadKeywords)

	return executionSignals{
		Score:         score,
		Reasons:       reasons,
		TargetFiles:   len(targetFiles),
		RequiresAgent: requiresAgent,
		BroadTask:     broad,
	}
}

func (s *Service) chooseExecutionStrategy(
	ctx context.Context,
	task string,
	opts Options,
	emit func(domain.Event),
) ExecutionStrategy {
	requested := normalizeExecutionMode(opts.Mode)
	requestedDepth := normalizeAgentDepth(string(opts.AgentDepth))

	// Явный выбор пользователя всегда имеет приоритет.
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

	signals := s.executionSignals(task)
	profile := s.modelProfile()

	// В автоматическом режиме сначала спрашиваем LLM-router.
	recommendation, err := s.llmExecutionStrategy(
		ctx,
		task,
		signals.Score,
		profile,
		emit,
	)

	if err == nil {
		return validateExecutionRecommendation(
			recommendation,
			signals,
		)
	}

	sendEvent(
		emit,
		domain.EventWarn,
		fmt.Sprintf(
			"Execution strategy LLM failed, using deterministic fallback: %v",
			err,
		),
	)

	return fallbackExecutionStrategy(signals)
}


func validateExecutionRecommendation(
	recommendation ExecutionStrategy,
	signals executionSignals,
) ExecutionStrategy {
	mode := normalizeExecutionMode(
		string(recommendation.Mode),
	)

	if mode == ExecutionModeAuto {
		mode = ExecutionModeSimple
	}

	confidence := recommendation.Confidence
	if confidence < 0 {
		confidence = 0
	}
	if confidence > 100 {
		confidence = 100
	}

	depth := normalizeAgentDepth(
		string(recommendation.AgentDepth),
	)

	if depth == AgentDepthAuto {
		depth = AgentDepthNormal
	}

	complexity := strings.ToLower(
		strings.TrimSpace(
			recommendation.Complexity,
		),
	)

	risk := strings.ToLower(
		strings.TrimSpace(
			recommendation.Risk,
		),
	)

	reason := strings.TrimSpace(
		recommendation.Reason,
	)

	// ------------------------------------------------------------
	// Agent разрешается только при наличии объективных оснований.
	// ------------------------------------------------------------

	agentAllowed :=
		signals.RequiresAgent ||
			signals.BroadTask ||
			signals.TargetFiles > 3 || 	(signals.Score >= 8 && 	confidence >= 70 && complexity == "high" && risk != "low")

	if mode == ExecutionModeAgent && !agentAllowed {
		mode = ExecutionModeSimple
		depth = AgentDepthNormal

		if reason == "" {
			reason = "LLM recommended agent"
		}

		reason =
			"Gogitor guard selected fast: " +
				reason
	}

	// ------------------------------------------------------------
	// Если LLM выбрала fast, но задача объективно архитектурная,
	// Gogitor повышает уровень до Agent.
	// ------------------------------------------------------------

	if mode == ExecutionModeSimple &&
		agentAllowed {

		mode = ExecutionModeAgent

		if signals.BroadTask ||
			signals.Score >= 8 ||
			risk == "high" {

			depth = AgentDepthDeep
		} else {
			depth = AgentDepthNormal
		}

		if reason == "" {
			reason = "LLM recommended fast"
		}

		reason =
			"Gogitor guard selected agent: " +
				reason
	}

	// ------------------------------------------------------------
	// Deep разрешается только для действительно тяжёлых задач.
	// Capability profile модели больше не может сам по себе
	// заставить Gogitor использовать deep.
	// ------------------------------------------------------------

	deepAllowed :=
		signals.BroadTask ||
			signals.Score >= 8 ||
			risk == "high"

	if depth == AgentDepthDeep &&
		!deepAllowed {

		depth = AgentDepthNormal

		if reason == "" {
			reason = "deep downgraded to normal by execution guard"
		} else {
			reason += "; deep downgraded to normal by execution guard"
		}
	}

	if mode == ExecutionModeSimple {
		depth = AgentDepthNormal
	}

	return ExecutionStrategy{
		Mode:       mode,
		AgentDepth: depth,
		Confidence: confidence,
		Complexity: complexity,
		Risk:       risk,
		Reason:     reason,
		Source:     recommendation.Source,
	}
}

func fallbackExecutionStrategy(
	signals executionSignals,
) ExecutionStrategy {
	if signals.RequiresAgent ||
		signals.BroadTask ||
		signals.TargetFiles > 3 ||
		signals.Score >= 8 {

		depth := AgentDepthNormal

		if signals.BroadTask ||
			signals.Score >= 8 {

			depth = AgentDepthDeep
		}

		return ExecutionStrategy{
			Mode:       ExecutionModeAgent,
			AgentDepth: depth,
			Reason: fmt.Sprintf(
				"deterministic fallback: task requires orchestration (score %d)",
				signals.Score,
			),
			Source: "rules",
		}
	}

	return ExecutionStrategy{
		Mode:       ExecutionModeSimple,
		AgentDepth: AgentDepthNormal,
		Reason: fmt.Sprintf(
			"deterministic fallback: fast is sufficient (score %d)",
			signals.Score,
		),
		Source: "rules",
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

	rawMode := strings.TrimSpace(
		out.ExecutionMode,
	)

	mode := normalizeExecutionMode(rawMode)

	if mode == ExecutionModeAuto {
		return ExecutionStrategy{},
			fmt.Errorf(
				"execution router returned invalid mode %q",
				rawMode,
			)
	}

	depth := normalizeAgentDepth(
		out.AgentDepth,
	)

	if depth == AgentDepthAuto {
		depth = AgentDepthNormal
	}

	return ExecutionStrategy{
		Mode:       mode,
		AgentDepth: depth,
		Confidence: out.Confidence,
		Complexity: strings.ToLower(
			strings.TrimSpace(out.Complexity),
		),
		Risk: strings.ToLower(
			strings.TrimSpace(out.Risk),
		),
		Reason: strings.TrimSpace(
			out.Reason,
		),
		Source: "llm",
	}, nil
}

// taskComplexityScore оценивает сложность задачи детерминированно.
func (s *Service) taskComplexityScore(
	task string,
) (int, []string) {
	lower := strings.ToLower(task)

	score := 0
	var reasons []string

	// Длина запроса — слабый сигнал, а не основной критерий.
	if len(strings.Fields(task)) > 24 {
		score += 1
		reasons = append(
			reasons,
			"long task",
		)
	}

	// Только действительно структурные действия.
	highKeywords := []string{
		"refactor",
		"refactoring",
		"architecture",
		"architectural",
		"restructure",
		"reorganize",
		"redesign",
		"migration",
		"migrate",
		"split",
		"divide",
		"extract",
		"create package",
		"new package",
		"move to a package",

		"рефактор",
		"рефакторинг",
		"архитектур",
		"реструктур",
		"перестрой",
		"перепроект",
		"миграц",
		"раздели",
		"разделить",
		"разбей",
		"разбить",
		"вынеси",
		"вынести",
		"перенеси",
		"перенести",
		"создай пакет",
		"новый пакет",
	}

	if containsAny(lower, highKeywords) {
		score += 3
		reasons = append(
			reasons,
			"structural-change keywords",
		)
	}

	mediumKeywords := []string{
		"add",
		"modify",
		"update",
		"fix",
		"improve",
		"create",
		"change",
		"remove",
		"delete",

		"добавь",
		"добавить",
		"измени",
		"изменить",
		"обнови",
		"обновить",
		"исправь",
		"исправить",
		"улучши",
		"улучшить",
		"создай",
		"создать",
		"измени",
		"удали",
		"удалить",
	}

	if containsAny(lower, mediumKeywords) {
		score += 1
		reasons = append(
			reasons,
			"implementation keywords",
		)
	}

	files := extractTargetFiles(task)

	if len(files) > 1 {
		score += 1
		reasons = append(
			reasons,
			fmt.Sprintf(
				"mentions %d files",
				len(files),
			),
		)
	}

	if len(files) > 3 {
		score += 2
		reasons = append(
			reasons,
			"many mentioned files",
		)
	}

	if s.WS != nil && s.WS.HasGoFiles() {
		score += 1
		reasons = append(
			reasons,
			"existing Go project",
		)
	}

	if s.WS != nil {
		if idx := s.WS.ExistingIndex(); idx != nil &&
			idx.Ready() &&
			idx.FileCount() > 20 {

			score += 1
			reasons = append(
				reasons,
				"large indexed project",
			)
		}
	}

	if score > 10 {
		score = 10
	}

	return score, reasons
}

func (s *Service) modelProfile() modelProfile {
	cfgProfile := strings.ToLower(
		strings.TrimSpace(
			s.Cfg.AgentModelProfile,
		),
	)

	switch cfgProfile {
	case "small":
		return modelProfileSmall

	case "medium":
		return modelProfileMedium

	case "large":
		return modelProfileLarge
	}

	if capability, ok :=
		s.configuredAgentCapability(); ok {

		switch strings.ToLower(
			strings.TrimSpace(
				capability.Profile,
			),
		) {
		case "small":
			return modelProfileSmall

		case "medium":
			return modelProfileMedium

		case "large":
			return modelProfileLarge
		}
	}

	return s.detectModelProfile()
}

// modelProfile определяет условный класс модели.
func (s *Service) detectModelProfile() modelProfile {
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
