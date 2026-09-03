package app

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"gogitor/internal/agent"
	"gogitor/internal/domain"
	"gogitor/internal/llm"
	"gogitor/internal/prompts"
	"gogitor/internal/runner"
	"gogitor/internal/search"
	"gogitor/internal/textutil"
)

type AutoResearchKind string

const (
	AutoResearchGeneral       AutoResearchKind = "general"
	AutoResearchDependency    AutoResearchKind = "dependency"
	AutoResearchLibrary       AutoResearchKind = "library"
	AutoResearchAPI           AutoResearchKind = "api"
	AutoResearchMigration     AutoResearchKind = "migration"
	AutoResearchVersion       AutoResearchKind = "version"
	AutoResearchToolchain     AutoResearchKind = "toolchain"
	AutoResearchSecurity      AutoResearchKind = "security"
	AutoResearchLint          AutoResearchKind = "lint"
	AutoResearchArchitecture  AutoResearchKind = "architecture"
	AutoResearchPerformance   AutoResearchKind = "performance"
	AutoResearchDocumentation AutoResearchKind = "documentation"
	AutoResearchOS            AutoResearchKind = "os"
)

type AutoResearchRequest struct {
	Kind    AutoResearchKind
	Task    string
	Subject string
	Error   string
}

var autoResearchExternalRefRE = regexp.MustCompile(
	`(?i)\b(?:github\.com|gitlab\.com|bitbucket\.org|golang\.org/x|charm\.land)/[A-Za-z0-9._~:@%+\-/]+`,
)

var autoResearchLintCodeRE = regexp.MustCompile(
	`(?i)\b(?:SA\d{3,4}|ST\d{3,4}|S\d{3,4}|G\d{3,4}|SEC\d{3,4})\b`,
)

func classifyAutoResearch(
	task string,
	errorText string,
) AutoResearchKind {
	text := strings.ToLower(
		strings.TrimSpace(task + "\n" + errorText),
	)

	// ------------------------------------------------------------
	// 1. Dependency — самый высокий приоритет.
	// Уже существует в текущем проекте после предыдущего
	// dependency-recovery изменения.
	// ------------------------------------------------------------
	if strings.TrimSpace(errorText) != "" &&
		runner.IsDependencyFetchError(errorText) {
		return AutoResearchDependency
	}

	// ------------------------------------------------------------
	// 2. Security.
	// ------------------------------------------------------------
	if containsAny(text, []string{
		"cve-",
		"ghsa-",
		"vulnerability",
		"vulnerabilities",
		"security advisory",
		"security issue",
		"security alert",
		"уязвим",
		"уязвимость",
		"уязвимости",
		"безопасност",
	}) {
		return AutoResearchSecurity
	}

	// ------------------------------------------------------------
	// 3. Migration / breaking changes / deprecated API.
	// ------------------------------------------------------------
	if containsAny(text, []string{
		"breaking change",
		"breaking changes",
		"deprecated",
		"deprecation",
		"migration",
		"migrate",
		"upgrade from",
		"upgrading from",
		"заменить старый api",
		"устарел",
		"устаревш",
		"миграци",
	}) {
		return AutoResearchMigration
	}

	// ------------------------------------------------------------
	// 4. Toolchain / Go version.
	// ------------------------------------------------------------
	if containsAny(text, []string{
		"go version",
		"go toolchain",
		"toolchain directive",
		"requires go ",
		"requires go1.",
		"unsupported go version",
		"unsupported language version",
		"module requires go",
		"версия go",
		"версия golang",
		"toolchain",
	}) {
		return AutoResearchToolchain
	}

	// ------------------------------------------------------------
	// 5. Version compatibility.
	// ------------------------------------------------------------
	if containsAny(text, []string{
		"version mismatch",
		"incompatible version",
		"version conflict",
		"compatible version",
		"which version",
		"latest version",
		"current version",
		"какую версию",
		"текущая версия",
		"актуальная версия",
		"совместим",
	}) {
		return AutoResearchVersion
	}

	// ------------------------------------------------------------
	// 6. Lint.
	// ------------------------------------------------------------
	if strings.Contains(text, "golangci-lint") ||
		strings.Contains(text, "staticcheck") ||
		strings.Contains(text, "errcheck") ||
		strings.Contains(text, "gosec") ||
		strings.Contains(text, "revive") ||
		autoResearchLintCodeRE.MatchString(text) {
		return AutoResearchLint
	}

	// ------------------------------------------------------------
	// 7. Performance.
	// ------------------------------------------------------------
	if containsAny(text, []string{
		"benchmark",
		"benchmarking",
		"performance",
		"throughput",
		"latency",
		"memory usage",
		"cpu usage",
		"allocation",
		"allocations",
		"оптимизир",
		"производительн",
		"быстрее",
		"latency",
	}) {
		return AutoResearchPerformance
	}

	// ------------------------------------------------------------
	// 8. Documentation.
	// ------------------------------------------------------------
	if containsAny(text, []string{
		"readme",
		"documentation",
		"docs",
		"document",
		"changelog",
		"release notes",
		"документаци",
		"readme.md",
		"readme_ru.md",
	}) {
		return AutoResearchDocumentation
	}

	// ------------------------------------------------------------
	// 9. API.
	// ------------------------------------------------------------
	if autoResearchExternalRefRE.MatchString(text) &&
		containsAny(text, []string{
			"api",
			"endpoint",
			"sdk",
			"client",
			"request",
			"response",
			"webhook",
			"grpc",
			"rest",
		}) {
		return AutoResearchAPI
	}

	// ------------------------------------------------------------
	// 10. Library.
	// ------------------------------------------------------------
	if autoResearchExternalRefRE.MatchString(text) ||
		containsAny(text, []string{
			"third-party",
			"third party",
			"external library",
			"external package",
			"dependency",
			"library",
			"framework",
			"driver",
			"orm",
			"библиотек",
			"внешн пакет",
			"внешняя библиотека",
			"зависимост",
			"sdk",
		}) {
		return AutoResearchLibrary
	}

	// ------------------------------------------------------------
	// 11. OS / platform.
	// ------------------------------------------------------------
	if containsAny(text, []string{
		"linux",
		"ubuntu",
		"debian",
		"fedora",
		"arch linux",
		"macos",
		"darwin",
		"wsl",
		"tty",
		"terminal",
		"clipboard",
		"cgo",
		"operating system",
		"операционной системы",
	}) {
		return AutoResearchOS
	}

	// ------------------------------------------------------------
	// 12. Architecture.
	// Только классификация. В обычном code loop автоматически
	// такой поиск включаться не должен — для Agent его должен
	// решать Planner через needs_search.
	// ------------------------------------------------------------
	if containsAny(text, []string{
		"architecture",
		"architectural",
		"design pattern",
		"trade-off",
		"tradeoffs",
		"compare approaches",
		"архитектур",
		"архитектурный",
		"сравни подходы",
		"варианты реализации",
	}) {
		return AutoResearchArchitecture
	}

	return AutoResearchGeneral
}

func shouldAutoResearchCodeTask(task string) bool {
	text := strings.ToLower(strings.TrimSpace(task))

	if text == "" {
		return false
	}

	// Не запускаем второй раз, если результат research уже
	// был встроен в текущую задачу.
	if strings.Contains(
		text,
		"=== untrusted auto-research ===",
	) {
		return false
	}

	kind := classifyAutoResearch(text, "")

	switch kind {
	case AutoResearchDependency,
		AutoResearchAPI,
		AutoResearchLibrary,
		AutoResearchMigration,
		AutoResearchVersion,
		AutoResearchToolchain,
		AutoResearchSecurity,
		AutoResearchLint,
		AutoResearchPerformance,
		AutoResearchDocumentation,
		AutoResearchOS:
		return true

	case AutoResearchArchitecture:
		// Архитектурный поиск лучше контролировать
		// через Planner -> needs_search.
		return false

	default:
		return false
	}
}

func shouldAutoResearchArticle(
	topic string,
	genre string,
) bool {
	lower := strings.ToLower(
		strings.TrimSpace(topic),
	)

	if genre == "news" ||
		genre == "review" {
		return true
	}

	if genre != "technical" &&
		genre != "howto" {
		return false
	}

	return containsAny(lower, []string{
		"latest",
		"current",
		"version",
		"release",
		"release notes",
		"api",
		"library",
		"module",
		"dependency",
		"migration",
		"deprecated",
		"readme",
		"documentation",
		"golang",
		"go ",
		"github",
		"актуаль",
		"последн",
		"верси",
		"релиз",
		"api",
		"библиотек",
		"модул",
		"зависимост",
		"миграци",
		"документаци",
	})
}

func autoResearchSubject(
	req AutoResearchRequest,
) string {
	if strings.TrimSpace(req.Subject) != "" {
		return strings.TrimSpace(req.Subject)
	}

	if strings.TrimSpace(req.Task) != "" {
		return truncate(req.Task, 300)
	}

	return ""
}

func (s *Service) autoResearch(
	ctx context.Context,
	req AutoResearchRequest,
	emit func(domain.Event),
) (string, error) {
	if s == nil ||
		s.Cfg == nil ||
		!s.Cfg.AutoSearch {
		return "", nil
	}

	if s.SafeSearch == nil {
		return "", nil
	}

	req.Kind = AutoResearchKind(
		strings.ToLower(
			strings.TrimSpace(
				string(req.Kind),
			),
		),
	)

	if req.Kind == "" {
		req.Kind = AutoResearchGeneral
	}

	subject := autoResearchSubject(req)

	sendEvent(
		emit,
		domain.EventLog,
		fmt.Sprintf(
			"Auto-search: research type: %s",
			req.Kind,
		),
	)

	sendEvent(
		emit,
		domain.EventLog,
		"Auto-search: generating focused research query...",
	)

	queryCtx := agent.WithRole(
		ctx,
		agent.RoleRouter,
	)
	queryCtx = agent.WithPriority(
		queryCtx,
		agent.PriorityNormal,
	)
	queryCtx = agent.WithPurpose(
		queryCtx,
		"auto research query",
	)
	queryCtx = llm.WithReasoningDisabled(
		queryCtx,
	)

	searchQuery := ""

	if generated, err := s.LLM.Send(
		queryCtx,
		prompts.AutoResearchQuery(
			string(req.Kind),
			subject,
			req.Task,
			req.Error,
		),
	); err == nil {
		searchQuery = strings.TrimSpace(generated)
	}

	if searchQuery == "" {
		searchQuery = subject
	}

	searchQuery = textutil.LimitRunes(
		searchQuery,
		450,
		"...",
	)

	sendEvent(
		emit,
		domain.EventLog,
		"Auto-search: query: "+searchQuery,
	)

	result, err := s.SafeSearch.Search(
		ctx,
		searchQuery,
	)
	if err != nil {
		return "", err
	}

	if result == nil {
		return "", nil
	}

	sendEvent(
		emit,
		domain.EventLog,
		fmt.Sprintf(
			"Auto-search: found %d source(s)",
			len(result.Sources),
		),
	)

	formatted := search.FormatForPrompt(result)

	formatted = textutil.LimitRunes(
		formatted,
		12000,
		"...",
	)

	if strings.TrimSpace(formatted) == "" {
		return "", nil
	}

	var out strings.Builder

	out.WriteString(
		"=== UNTRUSTED AUTO-RESEARCH ===\n",
	)

	fmt.Fprintf(
		&out,
		"RESEARCH TYPE: %s\n",
		req.Kind,
	)

	if subject != "" {
		fmt.Fprintf(
			&out,
			"SUBJECT: %s\n",
			subject,
		)
	}

	fmt.Fprintf(
		&out,
		"SEARCH QUERY: %s\n\n",
		searchQuery,
	)

	out.WriteString(
		"SOURCE MATERIAL:\n",
	)
	out.WriteString(formatted)

	out.WriteString(
		"\n=== END UNTRUSTED AUTO-RESEARCH ===",
	)

	return out.String(), nil
}

func (s *Service) autoResearchForCodeTask(
	ctx context.Context,
	task string,
	emit func(domain.Event),
) (string, error) {
	if !shouldAutoResearchCodeTask(task) {
		return "", nil
	}

	kind := classifyAutoResearch(
		task,
		"",
	)

	return s.autoResearch(
		ctx,
		AutoResearchRequest{
			Kind: kind,
			Task: task,
		},
		emit,
	)
}
