package app

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"gogitor/internal/config"
)

// AgentDepth — глубина выполнения агента.
type AgentDepth string

const (
	AgentDepthNormal AgentDepth = "normal"
	AgentDepthDeep   AgentDepth = "deep"
	AgentDepthAuto   AgentDepth = "auto"
)

// EditMode определяет способ изменения существующих файлов.
type EditMode string

const (
	EditModeAuto  EditMode = "auto"
	EditModePatch EditMode = "patch"
	EditModeFull  EditMode = "full"
)

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

func normalizeEditMode(mode string) EditMode {
	m := strings.ToLower(strings.TrimSpace(mode))

	switch m {
	case "patch", "diff", "minimal":
		return EditModePatch

	case "full", "full-file", "full_file", "rewrite":
		return EditModeFull

	case "auto", "":
		return EditModeAuto

	default:
		return EditModeAuto
	}
}

func agentEditModeForTask(
	task string,
	requested EditMode,
) EditMode {
	mode := normalizeEditMode(
		string(requested),
	)

	if mode == EditModeAuto {
		mode = EditModePatch
	}

	// Явная просьба пользователя переписать файл целиком
	// имеет приоритет над безопасным PATCH по умолчанию.
	if taskRequestsWholeFileRewrite(task) {
		return EditModeFull
	}

	return mode
}

func taskRequestsWholeFileRewrite(task string) bool {
	lower := strings.ToLower(
		strings.TrimSpace(task),
	)

	keywords := []string{
		"rewrite the entire file",
		"rewrite entire file",
		"replace the entire file",
		"replace whole file",
		"full rewrite",
		"complete rewrite",
		"rewrite from scratch",
		"rebuild the file",
		"regenerate the file",
		"replace the whole file",

		"полностью перепиши файл",
		"перепиши весь файл",
		"перепиши файл целиком",
		"полностью замени файл",
		"замени весь файл",
		"замени файл целиком",
		"полная перезапись",
		"полностью переработай файл",
		"перепиши файл с нуля",
	}

	return containsAny(lower, keywords)
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
