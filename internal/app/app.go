package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	// "net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gogitor/internal/agent"
	"gogitor/internal/autonomy"
	"gogitor/internal/codegen"
	"gogitor/internal/computer"
	"gogitor/internal/config"
	"gogitor/internal/domain"
	"gogitor/internal/git"
	"gogitor/internal/github"
	"gogitor/internal/i18n"
	"gogitor/internal/llm"
	"gogitor/internal/prompts"
	"gogitor/internal/runner"
	"gogitor/internal/search"
	"gogitor/internal/security"
	"gogitor/internal/textutil"
	"gogitor/internal/workspace"
)

type Options struct {
	DryRun            bool
	NoCommit          bool
	NoTests           bool
	NoCompare         bool
	ProgressItem      int
	ProgressTotal     int
	Mode              string
	AgentDepth        AgentDepth
	InterviewAnswers  []prompts.AgentInterviewAnswer
	AgentResumePlan   *fullPlan
	AgentResumeFrom   int
	AgentResumeSource string
}

type PendingComparison struct {
	Task       string
	Comparison *domain.ComparisonResult
	Opts       Options
	CreatedAt  time.Time
}

type historyItem struct {
	Query  string
	Answer string
}

type codeContextInfo struct {
	Context         string
	HasExisting     bool
	ExistingTargets []string
}

type agentInterviewQuestion struct {
	ID       int    `json:"id"`
	Question string `json:"question"`
	Why      string `json:"why"`
	Default  string `json:"default"`
}

type agentInterviewResult struct {
	Questions   []agentInterviewQuestion `json:"questions"`
	Assumptions []string                 `json:"assumptions"`
}

type PendingInterview struct {
	Task      string
	Questions []agentInterviewQuestion
	CreatedAt time.Time
}

type Service struct {
	Cfg               *config.Config
	Log               *slog.Logger
	LLM               LLMClient
	Agents            *agent.Dispatcher
	WS                *workspace.Workspace
	Runner            *runner.Runner
	Git               *git.Git
	GitHub            *github.Client
	Search            *search.Searcher
	Stats             *llmStats
	lastPreTaskHead   string
	history           []historyItem
	SafeSearch        *search.SafeSearcher
	ComputerExecutor  *computer.Executor
	ComputerAudit     *computer.AuditLog
	ComputerOS        computer.OSInfo
	pendingComparison *PendingComparison
	pendingInterview  *PendingInterview
	Autonomy          *autonomy.Controller
	RawTask           bool
}

type LLMClient interface {
	Send(ctx context.Context, prompt string) (string, error)
}

var (
	dependencyImportLineRE = regexp.MustCompile(
		`(?m)^\s+([^\s:]+):\s+`,
	)

	dependencyProvidesRE = regexp.MustCompile(
		`(?m)(?:no required module provides package|cannot find module providing package)\s+([^\s]+)`,
	)

	dependencyReadGoModRE = regexp.MustCompile(
		`(?m)\breading\s+([^\s]+?)/go\.mod\b`,
	)
)

func isDependencyFetchError(text string) bool {
	lower := strings.ToLower(text)

	markers := []string{
		"git ls-remote",
		"permission denied (publickey)",
		"could not read from remote repository",
		"no required module provides package",
		"cannot find module providing package",
		"unrecognized import path",
		"does not contain package",
		"reading github.com/",
		"reading charm.land/",
		"go mod tidy",
	}

	for _, marker := range markers {
		if strings.Contains(lower, marker) {
			return true
		}
	}

	return false
}

func extractDependencyImportPath(text string) string {
	if m := dependencyProvidesRE.FindStringSubmatch(text); len(m) == 2 {
		return strings.Trim(
			m[1],
			".,;:()[]{}\"'",
		)
	}

	if m := dependencyImportLineRE.FindStringSubmatch(text); len(m) == 2 {
		value := strings.Trim(
			m[1],
			".,;:()[]{}\"'",
		)

		if strings.Contains(value, "/") {
			return value
		}
	}

	if m := dependencyReadGoModRE.FindStringSubmatch(text); len(m) == 2 {
		value := strings.Trim(
			m[1],
			".,;:()[]{}\"'",
		)

		if strings.Contains(value, "/") {
			return value
		}
	}

	return ""
}

func New(cfg *config.Config, log *slog.Logger) *Service {
	baseLLM := llm.NewClient(cfg, log)

	stats := loadLLMStats(cfg.WorkDir)

	dispCfg := dispatcherConfig(cfg)
	dispCfg.StatsHook = func(req agent.Request, usage agent.Usage, err error) {
		if err == nil {
			stats.record(req.Role, req.Purpose, req.Prompt, usage.Duration, usage.EstimatedTokens)
		}
	}

	dispatcher := agent.NewDispatcher(baseLLM, dispCfg)

	searcher := search.New(log)

	if cfg.EffectiveContextTokens() > 65536 {
		searcher.SetMaxContent(100000)
	}
	if cfg.EffectiveContextTokens() > 131072 {
		searcher.SetMaxContent(200000)
	}

	safeCfg := search.DefaultSafeSearchConfig()
	safeCfg.Enabled = cfg.AutoSearch
	safeSearcher := search.NewSafeSearcher(searcher, safeCfg)
	svc := &Service{
		Cfg:        cfg,
		Log:        log,
		LLM:        dispatcher,
		Agents:     dispatcher,
		WS:         workspace.New(cfg.WorkDir),
		Runner:     newRunnerWithDeps(cfg, log),
		Git:        git.New(cfg.WorkDir, log),
		GitHub:     github.NewClient(cfg.GitHubToken, log),
		Search:     searcher,
		SafeSearch: safeSearcher,
		Stats:      stats,
		ComputerOS: computer.DetectOS(),
	}
	// ─── Autonomy: инициализация после создания svc ─────────────
	execCfg := computer.DefaultExecutorConfig(cfg.WorkDir)
	execCfg.AllowSudo = cfg.ComputerAllowSudo
	execCfg.ConfirmHighRisk = cfg.ComputerConfirmHigh
	if cfg.ComputerCommandTimeout > 0 {
		execCfg.CommandTimeout = time.Duration(cfg.ComputerCommandTimeout) * time.Second
	}
	if cfg.ComputerMaxOutput > 0 {
		execCfg.MaxOutputBytes = cfg.ComputerMaxOutput
	}
	execCfg.ConfirmFunc = func(command string, risk security.RiskLevel, reason string) bool {
		log.Warn("computer mode: HIGH risk command auto-confirmed",
			"command", command, "risk", risk.String(), "reason", reason)
		return true
	}
	svc.ComputerExecutor = computer.NewExecutor(execCfg)
	svc.ComputerAudit = computer.NewAuditLog(cfg.WorkDir)
	svc.Autonomy = autonomy.NewController(cfg, svc.WS, svc.Runner, dispatcher, log)
	if cfg.AutonomyEnabled {
		svc.Autonomy.Monitor.Start()
	}
	return svc
}

func newRunnerWithDeps(cfg *config.Config, log *slog.Logger) *runner.Runner {
	r := runner.New(120*time.Second, log)
	if cfg.DepsMode != "" {
		r.DepsMode = cfg.DepsMode
	}
	return r
}

func (s *Service) searchForSubtask(
	ctx context.Context,
	task string,
	emit func(domain.Event),
) (string, error) {
	sendEvent(emit, domain.EventLog,
		"Auto-search: looking up reference material for subtask...")

	result, err := s.SafeSearch.Search(ctx, task)
	if err != nil {
		sendEvent(emit, domain.EventWarn,
			fmt.Sprintf("Auto-search failed (non-fatal): %v", err))
		return "", err
	}

	formatted := search.FormatForPrompt(result)
	sendEvent(emit, domain.EventLog,
		fmt.Sprintf("Auto-search: found %d source(s)", len(result.Sources)))
	return formatted, nil
}

func (s *Service) searchForDependency(
	ctx context.Context,
	sandbox string,
	importPath string,
	errorText string,
	emit func(domain.Event),
) (string, error) {
	if !s.Cfg.AutoSearch {
		return "", nil
	}

	if s.SafeSearch == nil {
		return "", nil
	}

	importPath = strings.TrimSpace(importPath)
	if importPath == "" {
		return "", nil
	}

	sendEvent(
		emit,
		domain.EventLog,
		"Auto-search: dependency resolution failed; researching package/module...",
	)

	var b strings.Builder

	fmt.Fprintf(
		&b,
		`Find authoritative information about the Go dependency "%s".

The project failed while resolving this dependency.

Determine:
1. whether "%s" is a valid current Go import path;
2. which Go module provides that package;
3. whether the module path differs from the import path;
4. whether a major-version suffix such as /v2 is required;
5. which currently valid version should be used;
6. whether the reported error is caused by an invalid package path,
   a module/version mismatch, or Git/SSH transport/authentication.

IMPORTANT:
- Prefer pkg.go.dev, go.dev, the official project repository,
  or other authoritative technical sources.
- the official project repository
- official project documentation
- Do not invent a module path or version.
- The search result will be used only as technical evidence for
  a later code-repair step.

PACKAGE:
%s

ERROR:
%s
`,
		importPath,
		importPath,
		importPath,
		truncate(errorText, 3000),
	)

	if data, err := os.ReadFile(
		filepath.Join(sandbox, "go.mod"),
	); err == nil {
		goMod := textutil.TruncateStringBytes(
			string(data),
			5000,
		)

		b.WriteString("\nCURRENT go.mod:\n")
		b.WriteString(goMod)
		b.WriteString("\n")
	}

	question := b.String()

	searchQuery := question

	sqCtx := llm.WithReasoningDisabled(ctx)

	if generated, err := s.LLM.Send(
		sqCtx,
		prompts.SearchQuery(question),
	); err == nil {
		generated = strings.TrimSpace(generated)

		if generated != "" {
			searchQuery = generated
		}
	}

	sendEvent(
		emit,
		domain.EventLog,
		fmt.Sprintf(
			"Auto-search: dependency query: %s",
			truncate(searchQuery, 300),
		),
	)

	result, err := s.SafeSearch.Search(
		ctx,
		searchQuery,
	)
	if err != nil {
		return "", err
	}

	formatted := search.FormatForPrompt(result)

	// Не даём одному dependency-research раздувать
	// repair prompt до полного лимита search subsystem.
	formatted = textutil.TruncateStringBytes(
		formatted,
		12000,
	)

	if strings.TrimSpace(formatted) == "" {
		return "", nil
	}

	var out strings.Builder

	out.WriteString(
		"AUTO-SEARCH DEPENDENCY RESEARCH:\n",
	)
	out.WriteString("Package: ")
	out.WriteString(importPath)
	out.WriteString("\n\n")
	out.WriteString(formatted)

	sendEvent(
		emit,
		domain.EventLog,
		"Auto-search: dependency research added to repair context",
	)

	return out.String(), nil
}

// Close освобождает ресурсы сервиса: LLM-диспетчер и файловый watcher проекта.
func (s *Service) Close() {
	if s.Autonomy != nil {
		s.Autonomy.Close()
	}
	if s.Agents != nil {
		s.Agents.Close()
	}
	if s.WS != nil {
		_ = s.WS.Close()
	}
}

// LLMSnapshot — сводка использования LLM для отображения в TUI.
type LLMSnapshot struct {
	Requests        int
	EstimatedTokens int
	Duration        time.Duration
}

// LLMSnapshotData возвращает сводку использования LLM.
func (s *Service) LLMSnapshotData() LLMSnapshot {
	if s.Agents == nil {
		return LLMSnapshot{}
	}
	session, _ := s.Agents.Snapshot()
	return LLMSnapshot{
		Requests:        session.Requests,
		EstimatedTokens: session.EstimatedTokens,
		Duration:        session.Duration,
	}
}

func (s *Service) ProcessEvents(ctx context.Context, query string, emit func(domain.Event)) domain.Result {
	if emit == nil {
		emit = func(domain.Event) {}
	}
	ctx = agent.WithStatusFunc(ctx, s.agentStatusEmitter(emit))
	q := strings.TrimSpace(query)
	if q == "" {
		return domain.Result{
			Success: false,
			Errors:  []string{"empty query"},
		}
	}
	if strings.HasPrefix(q, ":") {
		return s.handleCommand(ctx, q, emit)
	}

	// Автоопределение изображений в запросе
	imagePaths := ExtractImagePaths(q)
	if len(imagePaths) > 0 {
		var images [][]byte
		for _, ip := range imagePaths {
			data, err := ReadImageFile(ip)
			if err != nil {
				sendEvent(emit, domain.EventWarn, fmt.Sprintf("Cannot read image: %v", err))
				continue
			}
			images = append(images, data...)
		}
		if len(images) > 0 {
			// Удаляем пути к изображениям из текстового запроса
			cleanQuery := q
			for _, ip := range imagePaths {
				cleanQuery = strings.ReplaceAll(cleanQuery, ip, "")
			}
			cleanQuery = strings.TrimSpace(cleanQuery)
			if cleanQuery == "" {
				cleanQuery = "Опиши, что изображено на картинке."
			}
			sendEvent(emit, domain.EventIntent, "Mode: image analysis")
			res := s.AnalyzeWithImages(ctx, cleanQuery, images, emit)
			res.RefinedTask = cleanQuery
			res.Mode = "analyze"
			return res
		}
	}

	if looksLikeStackTrace(q) {
		sendEvent(emit, domain.EventIntent, "Mode: fix (auto-detected error trace)")
		res := s.FixError(ctx, q, Options{}, emit)
		res.RefinedTask = q
		res.Mode = "fix"
		return res
	}

	if s.pendingComparison != nil {
		// Проверяем таймаут (5 минут)
		if time.Since(s.pendingComparison.CreatedAt) > 5*time.Minute {
			s.pendingComparison = nil
		} else if selection, ok := s.parseApproachSelection(q); ok {
			// ─── Быстрый путь: ключевые слова сработали ───────────────
			return s.executeSelectedApproach(ctx, selection, emit)
		} else if selection, ok := s.selectApproachViaLLM(ctx, q, emit); ok {
			// ─── LLM-путь: свободный ввод распознан моделью ───────────
			return s.executeSelectedApproach(ctx, selection, emit)
		} else {
			// Не похоже на выбор — очищаем и обрабатываем как новую задачу
			s.pendingComparison = nil
		}
	}

	if s.pendingInterview != nil {
		if time.Since(
			s.pendingInterview.CreatedAt,
		) > 10*time.Minute {
			s.pendingInterview = nil

			return domain.Result{
				Success: false,
				Mode:    "agent-interview",
				Errors: []string{
					"agent interview expired; please start it again",
				},
			}
		}

		pending := s.pendingInterview

		s.pendingInterview = nil

		return s.ContinueAgentInterview(
			ctx,
			pending.Task,
			pending.Questions,
			q,
			emit,
		)
	}

	intent := s.resolveIntent(ctx, q, emit, s.RawTask)
	task := effectiveTask(q, intent.Task, s.RawTask)
	if intent.Mode == "code" && s.looksLikeFileCodeTask(q) && !s.looksLikeFileCodeTask(task) {
		task = q
	}
	sendEvent(emit, domain.EventIntent, fmt.Sprintf("Mode: %s", intent.Mode))
	sendEvent(emit, domain.EventIntent, fmt.Sprintf("Refined task: %s", task))
	if strings.TrimSpace(intent.Reason) != "" && s.Cfg.Debug {
		sendEvent(emit, domain.EventLog, fmt.Sprintf("Intent reason: %s", intent.Reason))
	}

	var res domain.Result
	switch intent.Mode {
	case "article":
		res = s.Article(ctx, task, ArticleOptions{Mode: ArticleModeSimple}, emit)
	case "code":
		res = s.ExecuteCode(ctx, task, Options{}, emit)
	case "fix":
		res = s.FixError(ctx, task, Options{}, emit)
	case "analyze":
		res = s.Analyze(ctx, task, emit)
	case "search":
		res = s.SearchAnswer(ctx, task, emit)
	case "run":
		file := firstTargetFile(task, q)
		res = s.RunFile(ctx, file, emit)
	case "test":
		if strings.Contains(strings.ToLower(task), "lint") {
			res = s.RunLint(ctx, emit)
		} else {
			res = s.RunTests(ctx, emit)
		}
	case "git":
		sub := normalizeGitSubcommand(task)
		if sub == "" {
			sub = normalizeGitSubcommand(q)
		}
		if sub == "" {
			sub = "status"
		}
		res = s.handleCommand(ctx, ":git "+sub, emit)
	case "computer":
		res = s.handleCommand(ctx, ":computer "+task, emit)
	default:
		res = s.Chat(ctx, task, emit)
	}
	res.RefinedTask = task
	res.IntentReason = intent.Reason
	if res.Mode == "" {
		res.Mode = intent.Mode
	}
	return res
}

// executeSelectedApproach запускает мультиагентное выполнение с выбранным подходом.
func (s *Service) executeSelectedApproach(ctx context.Context, selection string, emit func(domain.Event)) domain.Result {
	task := s.pendingComparison.Task
	opts := s.pendingComparison.Opts
	s.pendingComparison = nil

	mem := loadAgentMemory(s.Cfg.WorkDir)
	mem.addDecisionWithAlternatives(
		fmt.Sprintf("Selected approach: %s", truncate(selection, 200)),
		task,
		nil,
		"user-selection",
	)
	_ = mem.save(s.Cfg.WorkDir)

	sendEvent(emit, domain.EventIntent,
		fmt.Sprintf("Selected approach: %s", truncate(selection, 300)))

	res := s.executeAgentFull(ctx, task, selection, opts, emit)
	res.RefinedTask = task
	res.SelectedApproach = selection
	res.Mode = "code"
	return res
}

func (s *Service) handleCommand(ctx context.Context, query string, emit func(domain.Event)) domain.Result {
	parts := strings.Fields(query)
	if len(parts) == 0 {
		return domain.Result{Success: false, Errors: []string{"empty command"}}
	}

	cmd := strings.ToLower(parts[0])
	args := parts[1:]
	argString := strings.TrimSpace(strings.Join(args, " "))

	if len(args) > 0 && strings.ToLower(args[len(args)-1]) == "help" {
		return HelpForCommand(strings.TrimPrefix(cmd, ":"))
	}
	switch cmd {
	case ":reasoning":
		if len(args) > 0 {
			// ➕ НОВОЕ: обработка ":reasoning router [on|off]"
			if strings.ToLower(args[0]) == "router" {
				if len(args) > 1 {
					switch strings.ToLower(args[1]) {
					case "on", "true", "1":
						s.Cfg.ReasoningRouter = true
						return domain.Result{Success: true, Mode: "command",
							Response: i18n.T("Router reasoning enabled.")}
					case "off", "false", "0":
						s.Cfg.ReasoningRouter = false
						return domain.Result{Success: true, Mode: "command",
							Response: i18n.T("Router reasoning disabled.")}
					}
				}
				rStatus := "off"
				if s.Cfg.ReasoningRouter {
					rStatus = "on"
				}
				return domain.Result{Success: true, Mode: "command",
					Response: i18n.T("Router reasoning: %s", rStatus)}
			}
			switch strings.ToLower(args[0]) {
			case "on", "true", "1":
				s.Cfg.ReasoningEnabled = true
				return domain.Result{Success: true, Mode: "command",
					Response: i18n.T("Reasoning mode enabled.")}
			case "off", "false", "0":
				s.Cfg.ReasoningEnabled = false
				return domain.Result{Success: true, Mode: "command",
					Response: i18n.T("Reasoning mode disabled.")}
			}
		}
		status := "off"
		if s.Cfg.ReasoningEnabled {
			status = "on"
		}
		rStatus := "off"
		if s.Cfg.ReasoningRouter {
			rStatus = "on"
		}
		return domain.Result{Success: true, Mode: "command",
			Response: i18n.T("Reasoning: %s (effort: %s, router: %s)",
				status, s.Cfg.ReasoningEffort, rStatus)}
	case ":article":
		if argString == "" {
			return domain.Result{Success: false, Mode: "article", Errors: []string{"usage: :article <topic>"}}
		}
		// Определяем режим по ключевым словам.
		mode := ArticleModeSimple
		lower := strings.ToLower(argString)
		if strings.Contains(lower, "--full") || strings.Contains(lower, "--complex") ||
			strings.Contains(lower, "подробн") || strings.Contains(lower, "полн") {
			mode = ArticleModeComplex
			argString = strings.ReplaceAll(argString, "--full", "")
			argString = strings.ReplaceAll(argString, "--complex", "")
			argString = strings.TrimSpace(argString)
		}
		return s.Article(ctx, argString, ArticleOptions{Mode: mode}, emit)
	case ":suggest":
		return s.Suggest(ctx, emit)
	case ":vet":
		return s.RunVet(ctx, emit)
	case ":todo":
		return s.ScanTODO(ctx, emit)
	case ":autonomy":
		return s.handleAutonomyCommand(ctx, args, emit)
	case ":mutate":
		return s.handleMutateCommand(ctx, args, emit)
	case ":autogen-tests":
		return s.handleTestGenCommand(ctx, args, emit)
	case ":computer":
		if argString == "" {
			return domain.Result{
				Success: false,
				Mode:    "computer",
				Errors:  []string{"usage: :computer <task>"},
			}
		}
		if !s.Cfg.ComputerEnabled {
			return domain.Result{
				Success: false,
				Mode:    "computer",
				Errors: []string{
					"computer mode is disabled; use --computer flag, " +
						"set GOGITOR_COMPUTER_ENABLED=true, " +
						`or "computer_enabled": true in .gogitor.json`,
				},
			}
		}
		return s.ExecuteComputer(ctx, argString, emit)
	case ":help", ":h":
		if len(args) > 0 {
			return HelpForCommand(args[0])
		}
		return domain.Result{
			Success:  true,
			Mode:     "help",
			Response: HelpText(),
		}
	case ":clear":
		s.history = nil
		return domain.Result{
			Success:  true,
			Mode:     "clear",
			Response: "Conversation context cleared.",
		}

	case ":code":
		if argString == "" {
			return domain.Result{Success: false, Mode: "code", Errors: []string{"usage: :code <task>"}}
		}
		return s.ExecuteCode(ctx, argString, Options{}, emit)

	case ":fast":
		if argString == "" {
			return domain.Result{Success: false, Mode: "code", Errors: []string{"usage: :fast <task>"}}
		}
		return s.ExecuteCode(ctx, argString, Options{Mode: "fast"}, emit)

	case ":agent":
		if argString == "" {
			return domain.Result{
				Success: false, Mode: "agent",
				Errors: []string{"usage: :agent <task> | :agent deep <task>"},
			}
		}
		lowerArgs := strings.ToLower(strings.TrimSpace(argString))
		if lowerArgs == "undo" {
			return s.ExecuteAgentUndo(
				ctx,
				emit,
			)
		}
		if lowerArgs == "resume" {
			return s.ExecuteAgentResume(
				ctx,
				emit,
			)
		}
		if lowerArgs == "report" {
			return s.ExecuteAgentReport(
				ctx,
				emit,
			)
		}
		if lowerArgs == "reflect" {
			return s.ExecuteAgentReflect(ctx, emit)
		}
		if strings.HasPrefix(lowerArgs, "interview ") {
			task := strings.TrimSpace(argString[len("interview"):])
			return s.ExecuteAgentInterview(ctx, task, emit)
		}
		depth := AgentDepthNormal
		task := argString
		if strings.HasPrefix(lowerArgs, "deep ") {
			depth = AgentDepthDeep
			task = strings.TrimSpace(argString[len("deep"):])
		}
		if task == "" {
			return domain.Result{
				Success: false, Mode: "agent",
				Errors: []string{"usage: :agent <task> | :agent deep <task>"},
			}
		}
		return s.ExecuteCode(ctx, task, Options{Mode: "agent", AgentDepth: depth}, emit)
	case ":fix":
		if argString == "" {
			return domain.Result{
				Success: false, Mode: "fix",
				Errors: []string{"usage: :fix <error output / stack trace>"},
			}
		}
		return s.FixError(ctx, argString, Options{}, emit)

	case ":ask":
		if argString == "" {
			return domain.Result{Success: false, Mode: "chat", Errors: []string{"usage: :ask <question>"}}
		}
		return s.Chat(ctx, argString, emit)

	case ":analyze", ":analysis":
		if argString == "" {
			return domain.Result{Success: false, Mode: "analyze", Errors: []string{"usage: :analyze <question>"}}
		}
		return s.Analyze(ctx, argString, emit)

	case ":search":
		if argString == "" {
			return domain.Result{Success: false, Mode: "search", Errors: []string{"usage: :search <query>"}}
		}
		return s.SearchAnswer(ctx, argString, emit)
	case ":load":
		return s.handleLoad(ctx, argString, emit)
	case ":run":
		file := ""
		if len(args) > 0 {
			file = args[0]
		}
		return s.RunFile(ctx, file, emit)

	case ":test":
		if len(args) > 0 && strings.ToLower(args[0]) == "lint" {
			return s.RunLint(ctx, emit)
		}
		return s.RunTests(ctx, emit)

	case ":decisions", ":journal":
		return s.DecisionJournal(ctx, emit)

	case "task-diff":
		return s.GitDiffTask(ctx, emit)
	case ":git":
		return s.handleGitCommand(ctx, args, emit)
	default:
		return domain.Result{
			Success: false,
			Mode:    "command",
			Errors:  []string{fmt.Sprintf("unknown command %s; type :help", cmd)},
		}
	}
}

func (s *Service) ExecuteAgentReport(
	ctx context.Context,
	emit func(domain.Event),
) domain.Result {
	dir, err :=
		s.findLatestAgentSessionDir()

	if err != nil {
		return domain.Result{
			Success: false,
			Mode:    "agent-report",
			Errors: []string{
				err.Error(),
			},
		}
	}

	data, err :=
		os.ReadFile(
			filepath.Join(
				dir,
				"result.json",
			),
		)

	if err != nil {
		return domain.Result{
			Success: false,
			Mode:    "agent-report",
			Errors: []string{
				fmt.Sprintf(
					"cannot read Agent result: %v",
					err,
				),
			},
		}
	}

	var result domain.Result

	if err := json.Unmarshal(
		data,
		&result,
	); err != nil {
		return domain.Result{
			Success: false,
			Mode:    "agent-report",
			Errors: []string{
				fmt.Sprintf(
					"cannot parse Agent result: %v",
					err,
				),
			},
		}
	}

	state, _ :=
		loadAgentState(dir)

	depth := AgentDepthNormal

	completedSubtasks := 0
	totalSubtasks := 0

	if state != nil {
		completedSubtasks =
			state.CompletedSubtasks

		totalSubtasks =
			state.TotalSubtasks
	}

	report := formatAgentTaskReport(
		result,
		depth,
		completedSubtasks,
		totalSubtasks,
	)

	sendEvent(
		emit,
		domain.EventLog,
		"Loaded Agent task report: "+
			filepath.Base(dir),
	)

	return domain.Result{
		Success:  true,
		Mode:     "agent-report",
		Response: report,
	}
}

func (s *Service) ExecuteAgentUndo(
	ctx context.Context,
	emit func(domain.Event),
) domain.Result {
	result := domain.Result{
		Mode: "agent-undo",
	}

	if !s.Git.IsRepo(ctx) {
		result.AddError(
			"current project is not a Git repository",
		)
		return result
	}

	dir, state, err :=
		s.findLatestUndoableAgentSession()

	if err != nil {
		result.AddError(err.Error())
		return result
	}

	head, err :=
		s.Git.HeadHash(ctx)

	if err != nil {
		result.AddError(
			fmt.Sprintf(
				"cannot determine current HEAD: %v",
				err,
			),
		)
		return result
	}

	// Не разрешаем Undo, если после Agent уже появился
	// другой commit.
	if strings.TrimSpace(head) !=
		strings.TrimSpace(state.GitCommit) {
		result.AddError(
			"the last Agent commit is no longer HEAD; refusing automatic undo",
		)

		result.AddError(
			"Use ':git revert <hash>' manually if you really want to undo it.",
		)

		return result
	}

	sendEvent(
		emit,
		domain.EventWarn,
		fmt.Sprintf(
			"Undoing last Agent commit %s...",
			state.GitCommit,
		),
	)

	_, err =
		s.Git.Revert(
			ctx,
			state.GitCommit,
		)

	if err != nil {
		result.AddError(
			fmt.Sprintf(
				"Agent undo failed: %v",
				err,
			),
		)

		return result
	}

	undoHead, _ :=
		s.Git.HeadHash(ctx)

	state.Status = "undone"
	state.UndoCommit = undoHead

	statePath := filepath.Join(
		dir,
		"state.json",
	)

	data, marshalErr :=
		json.MarshalIndent(
			state,
			"",
			"  ",
		)

	if marshalErr == nil {
		_ = os.WriteFile(
			statePath,
			data,
			0o644,
		)
	}

	s.WS.RefreshIndex()

	result.Success = true

	result.Response = fmt.Sprintf(
		"Last Agent change was reverted.\nAgent commit: %s\nUndo commit: %s",
		state.GitCommit,
		undoHead,
	)

	return result
}

func (s *Service) handleGitCommand(
	ctx context.Context,
	args []string,
	emit func(domain.Event),
) domain.Result {
	emitEvent(emit, domain.Event{
		Type:      domain.EventLog,
		Message:   i18n.Localize("Running git command"),
		TaskStage: domain.TaskStageGit,
	})

	if len(args) == 0 {
		return s.GitStatus(ctx, emit)
	}

	switch strings.ToLower(args[0]) {
	case "revert":
		return s.GitRevert(ctx, args[1:], emit)

	case "reset":
		return s.GitReset(ctx, args[1:], emit)

	case "pr":
		return s.GitPR(ctx, emit)

	case "issue", "issues":
		return s.GitIssue(ctx, emit)

	case "changelog":
		return s.GitChangelog(ctx, emit)

	case "pr-comment":
		return s.GitPRComment(ctx, args[1:], emit)

	case "status":
		return s.GitStatus(ctx, emit)

	case "diff":
		if s.lastPreTaskHead != "" {
			return s.GitDiffTask(ctx, emit)
		}
		return s.GitDiff(ctx, emit)

	case "commit":
		splitFiles, hasSplit := ParseCommitSplitArgs(args[1:])
		if hasSplit {
			return s.GitCommitSplit(ctx, splitFiles, emit)
		}
		return s.GitCommit(ctx, emit)

	case "init":
		return s.GitInit(ctx, emit)

	case "log":
		return s.GitLog(ctx, emit)

	case "create":
		return s.GitCreate(ctx, args[1:], emit)

	case "checkout":
		return s.GitCheckout(ctx, args[1:], emit)

	case "diff-task":
		return s.GitDiffTask(ctx, emit)

	case "branch":
		return s.GitBranch(ctx, args[1:], emit)

	case "merge":
		branch := ""
		if len(args) > 1 {
			branch = args[1]
		}
		return s.GitMerge(ctx, branch, emit)

	case "push":
		branch := ""
		if len(args) > 1 {
			branch = args[1]
		}
		return s.GitPush(ctx, branch, emit)

	case "pull":
		branch := ""
		if len(args) > 1 {
			branch = args[1]
		}
		return s.GitPull(ctx, branch, emit)

	case "fetch":
		return s.GitFetch(ctx, emit)

	case "clone":
		url := ""
		if len(args) > 1 {
			url = args[1]
		}
		return s.GitClone(ctx, url, emit)

	case "remote":
		return s.GitRemote(ctx, args[1:], emit)

	default:
		return domain.Result{
			Success: false,
			Mode:    "git",
			Errors: []string{
				"unknown git command; supported: status, diff, commit, init, log, checkout, branch, merge, push, pull, fetch, clone, remote",
			},
		}
	}
}

// GitDiffTask показывает накопительный diff последней задачи.
func (s *Service) GitDiffTask(ctx context.Context, emit func(domain.Event)) domain.Result {
	result := domain.Result{Mode: "git"}
	if !s.Git.IsRepo(ctx) {
		result.Success = true
		result.Response = "Not a git repository."
		return result
	}
	if s.lastPreTaskHead == "" {
		result.Success = true
		result.Response = "No previous task diff available. Run a code task first."
		return result
	}
	diff, err := s.Git.DiffRange(ctx, s.lastPreTaskHead, "HEAD")
	if err != nil {
		// Fallback: показать последний коммит
		diff, err = s.Git.DiffLast(ctx)
		if err != nil {
			result.AddError(err.Error())
			return result
		}
	}
	if strings.TrimSpace(diff) == "" {
		diff = "No changes."
	}
	result.Success = true
	result.Response = diff
	return result
}

func (s *Service) Chat(ctx context.Context, query string, emit func(domain.Event)) domain.Result {
	ctx = agent.WithStatusFunc(ctx, s.agentStatusEmitter(emit))

	emitEvent(emit, domain.Event{
		Type:      domain.EventAgent,
		Message:   i18n.Localize("current stage: chat"),
		TaskStage: domain.TaskStageChat,
	})

	sendEvent(emit, domain.EventLog, "Sending chat request to LLM")

	history := s.historyString()
	prompt := prompts.Chat(history, query)

	response, err := s.sendLLMStreaming(
		ctx,
		prompt,
		emit,
		agent.RoleDefault,
		agent.PriorityNormal,
		"chat",
	)

	if err != nil {
		return domain.Result{
			Success: false,
			Mode:    "chat",
			Errors:  []string{err.Error()},
		}
	}

	s.addHistory(query, response)

	return domain.Result{
		Success:  true,
		Mode:     "chat",
		Response: response,
	}
}

func (s *Service) Analyze(ctx context.Context, query string, emit func(domain.Event)) domain.Result {
	ctx = agent.WithStatusFunc(ctx, s.agentStatusEmitter(emit))

	emitEvent(emit, domain.Event{
		Type:      domain.EventAgent,
		Message:   i18n.Localize("current stage: analyze"),
		TaskStage: domain.TaskStageAnalyze,
	})
	sendEvent(emit, domain.EventLog, "Reading project files")

	// Умный выбор файлов по запросу анализа.
	maxFiles, maxBytes := s.contextLimits()
	// Для analyze используем 75% от code-лимитов
	analyzeFiles := maxFiles * 3 / 4
	analyzeBytes := maxBytes * 3 / 4
	projectContext := s.WS.BuildSmartContext(query, nil, analyzeFiles, analyzeBytes)

	sendEvent(emit, domain.EventLog, "Sending analysis request to LLM")
	prompt := prompts.Analyze(query, projectContext)

	response, err := s.sendLLMStreaming(
		ctx,
		prompt,
		emit,
		agent.RoleDefault,
		agent.PriorityNormal,
		"chat",
	)

	if err != nil {
		return domain.Result{
			Success: false,
			Mode:    "analyze",
			Errors:  []string{err.Error()},
		}
	}
	s.addHistory(query, response)
	return domain.Result{
		Success:  true,
		Mode:     "analyze",
		Response: response,
	}
}

// AnalyzeWithImages анализирует изображение с текстовым запросом.
func (s *Service) AnalyzeWithImages(ctx context.Context, query string, images [][]byte, emit func(domain.Event)) domain.Result {
	ctx = agent.WithStatusFunc(ctx, s.agentStatusEmitter(emit))
	emitEvent(emit, domain.Event{
		Type:      domain.EventAgent,
		Message:   i18n.Localize("current stage: image analysis"),
		TaskStage: domain.TaskStageAnalyze,
	})
	sendEvent(emit, domain.EventLog, "Sending image analysis request to LLM")
	prompt := prompts.AnalyzeImage(query, "")
	response, err := s.sendLLMStreamingWithImages(
		ctx, prompt, images, emit,
		agent.RoleDefault, agent.PriorityNormal, "image_analysis",
	)
	if err != nil {
		return domain.Result{
			Success: false,
			Mode:    "analyze",
			Errors:  []string{err.Error()},
		}
	}
	s.addHistory(query, response)
	return domain.Result{
		Success:  true,
		Mode:     "analyze",
		Response: response,
	}
}

// ReadImageFile читает файл изображения и возвращает его содержимое.
func ReadImageFile(path string) ([][]byte, error) {
	expandedPath := path
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			expandedPath = filepath.Join(home, path[2:])
		}
	}
	info, err := os.Stat(expandedPath)
	if err != nil {
		return nil, fmt.Errorf("image file not found: %s", expandedPath)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("path is a directory: %s", expandedPath)
	}
	// Ограничение 4 МБ
	const maxImageSize = 4 << 20
	if info.Size() > maxImageSize {
		return nil, fmt.Errorf("image file too large (%d bytes, max %d)", info.Size(), maxImageSize)
	}
	data, err := os.ReadFile(expandedPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read image file: %w", err)
	}
	return [][]byte{data}, nil
}

// imageExtensions — расширения файлов, которые считаются изображениями.
var imageExtensions = []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp"}

// IsImagePath проверяет, является ли строка путём к изображению.
func IsImagePath(s string) bool {
	lower := strings.ToLower(s)
	for _, ext := range imageExtensions {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// ExtractImagePaths извлекает пути к изображениям из текста запроса.
func ExtractImagePaths(query string) []string {
	var paths []string
	for _, word := range strings.Fields(query) {
		word = strings.Trim(word, ".,;:()[]{}\"'`")
		if IsImagePath(word) {
			paths = append(paths, word)
		}
	}
	return paths
}

func (s *Service) SearchAnswer(ctx context.Context, query string, emit func(domain.Event)) domain.Result {
	ctx = agent.WithStatusFunc(ctx, s.agentStatusEmitter(emit))
	emitEvent(emit, domain.Event{
		Type:      domain.EventAgent,
		Message:   i18n.Localize("current stage: search"),
		TaskStage: domain.TaskStageSearch,
	})

	sendEvent(emit, domain.EventLog, "Generating search query")

	searchQuery := query

	prompt := prompts.SearchQuery(query)
	sqCtx := llm.WithReasoningDisabled(ctx)
	if generated, err := s.LLM.Send(sqCtx, prompt); err == nil {
		generated = strings.TrimSpace(generated)
		if generated != "" {
			searchQuery = generated
		}
	}

	sendEvent(emit, domain.EventLog, fmt.Sprintf("Searching web: %s", searchQuery))

	searchResult, err := s.Search.Search(ctx, searchQuery)
	if err != nil {
		return domain.Result{
			Success: false,
			Mode:    "search",
			Errors:  []string{err.Error()},
		}
	}

	sendEvent(emit, domain.EventLog, "Generating answer from search results")

	answerPrompt := prompts.SearchAnswer(query, searchResult.Content)

	response, err := s.sendLLMStreaming(
		ctx,
		answerPrompt,
		emit,
		agent.RoleDefault,
		agent.PriorityNormal,
		"chat",
	)

	if err != nil {
		return domain.Result{
			Success: false,
			Mode:    "search",
			Errors:  []string{err.Error()},
		}
	}

	var sources []string
	for _, src := range searchResult.Sources {
		sources = append(sources, fmt.Sprintf("- %s: %s", src.Title, src.URL))
	}

	if len(sources) > 0 {
		response = response + "\n" + i18n.T("Sources:") + "\n" + strings.Join(sources, "\n")
	}

	s.addHistory(query, response)

	return domain.Result{
		Success:  true,
		Mode:     "search",
		Response: response,
	}
}

func (s *Service) ExecuteCode(ctx context.Context, query string, opts Options, emit func(domain.Event)) domain.Result {
	ctx = agent.WithStatusFunc(ctx, s.agentStatusEmitter(emit))

	if isRemovedWorkflowMode(opts.Mode) {
		return domain.Result{
			Success: false,
			Mode:    "code",
			Errors: []string{
				"workflow mode was removed; use agent or agent deep",
			},
		}
	}

	if opts.DryRun {
		sendEvent(emit, domain.EventWarn, "Dry-run mode enabled: changes will be validated but not applied")
	}
	analysisOnly := s.isAnalysisOnlyTask(query)

	if analysisOnly {
		sendEvent(emit, domain.EventLog, "Analysis-only task detected: no file changes will be made")
		return s.executeSimple(ctx, query, opts, emit)
	}

	strategy := s.chooseExecutionStrategy(ctx, query, opts, emit)

	sendEvent(
		emit,
		domain.EventLog,
		fmt.Sprintf(
			"Execution strategy: %s (source=%s, reason=%s)",
			strategy.Mode,
			strategy.Source,
			strategy.Reason,
		),
	)

	switch strategy.Mode {
	case ExecutionModeSimple:
		return s.executeSimple(ctx, query, opts, emit)
	case ExecutionModeAgent:
		agentOpts := opts
		if strategy.AgentDepth != "" && strategy.AgentDepth != AgentDepthAuto {
			agentOpts.AgentDepth = strategy.AgentDepth
		}
		return s.executeAgentFull(ctx, query, "", agentOpts, emit)
	default:
		return s.executeSimple(ctx, query, opts, emit)
	}
}

func (s *Service) executeSimple(ctx context.Context, query string, opts Options, emit func(domain.Event)) domain.Result {
	if agent.RoleFromContext(ctx) == agent.RoleDefault {
		ctx = agent.WithRole(ctx, agent.RoleCoder)
	}
	result := domain.Result{
		Success: false,
		Mode:    "code",
		DryRun:  opts.DryRun,
	}
	preTaskHead := s.captureHead(ctx)
	if s.isAnalysisOnlyTask(query) {
		sendEvent(emit, domain.EventLog, "Analysis-only task detected: no file changes will be made")
		analysis := s.Analyze(ctx, query, emit)
		result.Success = analysis.Success
		result.Response = analysis.Response
		result.Errors = analysis.Errors
		result.Warnings = analysis.Warnings
		result.Iterations = 1
		return result
	}
	maxIterations := s.Cfg.MaxIterations
	if maxIterations <= 0 {
		maxIterations = 3
	}

	patchPolicy :=
		s.patchPolicyForOptions(opts)

	if opts.AgentDepth == AgentDepthDeep {
		sendEvent(
			emit,
			domain.EventLog,
			"Deep Agent: strict patch policy enabled",
		)
	}
	targetFiles := extractTargetFiles(query)
	cc := s.buildCodeContext(query, targetFiles)
	originalContext := cc.Context
	defaultPath := defaultPath(query, targetFiles)
	var lastErrors []string
	var lastChanges []domain.FileChange

	// ─── Новые переменные для управления patch-режимом ──────────────
	forceFull := false
	patchAttempted := false
	patchFixAttempts := 0
	maxPatchFixAttempts := 3
	if patchPolicy == workspace.PatchPolicyStrict {
		maxPatchFixAttempts = 4
	}
	var lastPatchContent string
	patchAppliedSuccessfully := false
	patchRepairPending := false

	// Оценка ETA для простых задач (не multi-agent подзадач).
	if opts.ProgressItem == 0 && s.Stats != nil && emit != nil {
		simpleETA := s.Stats.estimateSubtask(query, false)
		emit(domain.Event{
			Type:    domain.EventProgress,
			Message: query,
			Progress: &domain.ProgressUpdate{
				Stage:      truncate(query, 80),
				ETASeconds: int(simpleETA.Seconds() + 0.5),
			},
		})
	}

	for i := 1; i <= maxIterations; i++ {
		result.Iterations = i
		result.FilesCreated = nil
		result.FilesModified = nil
		result.OutputFiles = nil
		emitEvent(emit, domain.Event{
			Type:      domain.EventLog,
			Message:   fmt.Sprintf("Iteration %d/%d", i, maxIterations),
			TaskStage: domain.TaskStageCoding,
		})

		emitEvent(emit, domain.Event{
			Type:      domain.EventProgress,
			Message:   fmt.Sprintf("Iteration %d/%d", i, maxIterations),
			TaskStage: domain.TaskStageCoding,
			Progress: &domain.ProgressUpdate{
				Stage:      "coding",
				ItemIndex:  i,
				TotalItems: maxIterations,
			},
		})

		var prompt string
		allowFallback := false
		usePatchPrompt := false
		if i == 1 {
			// Проверяем, существуют ли целевые файлы.
			// Если ни один целевой файл не существует, патч-режим не нужен.
			targetFiles := extractTargetFiles(query)
			existingTargets := s.WS.ExistingFiles(targetFiles)
			allTargetsExist := len(targetFiles) == 0 || len(existingTargets) == len(targetFiles)

			switch {
			case originalContext != "" && !allTargetsExist:
				// Целевые файлы не существуют → режим создания, а не патчей.
				sendEvent(emit, domain.EventLog, "Target files do not exist, using create mode")
				prompt = prompts.CodeCreateInExistingProject(query, originalContext)
			case originalContext != "" && (len(cc.ExistingTargets) > 0 || s.needsModify(query) || s.isSplitOrRefactor(query)):
				if !forceFull {
					sendEvent(emit, domain.EventLog, "Using patch mode for existing project files")
					prompt = prompts.CodeModifyDiffForModel(
						query,
						originalContext,
						patchPolicy.String(),
					)
					usePatchPrompt = true
				} else {
					sendEvent(emit, domain.EventLog, "Using modification mode based on existing project files")
					prompt = prompts.CodeModify(query, originalContext)
				}
			case originalContext != "":
				sendEvent(emit, domain.EventLog, "Using create-in-existing-project mode")
				prompt = prompts.CodeCreateInExistingProject(query, originalContext)
			default:
				sendEvent(emit, domain.EventLog, "Using create mode for empty project")
				prompt = prompts.CodeCreate(query, "")
				allowFallback = true
			}

		} else if (patchAppliedSuccessfully || patchRepairPending) &&
			!forceFull &&
			patchFixAttempts < maxPatchFixAttempts {
			// ─── НОВОЕ: исправление патча вместо полного файла ─────
			patchFixAttempts++
			sendEvent(emit, domain.EventLog,
				fmt.Sprintf("Patch applied but caused error. Requesting patch fix (attempt %d/%d)",
					patchFixAttempts, maxPatchFixAttempts))
			prompt = prompts.CodeFixPatch(query, originalContext, lastPatchContent, strings.Join(lastErrors, "\n"))
			usePatchPrompt = true
			patchRepairPending = false
			patchAppliedSuccessfully = false
		} else {
			contextForFix := originalContext
			if !forceFull && !patchAttempted && len(lastChanges) > 0 {
				contextForFix = codegen.FormatChanges(lastChanges)
			}
			if forceFull || patchAttempted {
				sendEvent(emit, domain.EventLog, "Falling back to full-file mode")
			}
			prompt = prompts.CodeFix(query, contextForFix, strings.Join(lastErrors, "\n"))
			usePatchPrompt = false
		}

		prompt = s.appendProjectInstructions(prompt)

		sendEvent(
			emit,
			domain.EventLog,
			"LLM request",
		)

		if opts.ProgressItem > 0 {
			emitEvent(emit, domain.Event{
				Type:      domain.EventProgress,
				Message:   fmt.Sprintf("Iteration %d/%d", i, maxIterations),
				TaskStage: domain.TaskStageCoding,
				Progress: &domain.ProgressUpdate{
					Stage:      "coding",
					ItemIndex:  opts.ProgressItem,
					TotalItems: opts.ProgressTotal,
				},
			})
		} else {
			emitEvent(emit, domain.Event{
				Type:      domain.EventProgress,
				Message:   fmt.Sprintf("Iteration %d/%d", i, maxIterations),
				TaskStage: domain.TaskStageCoding,
				Progress: &domain.ProgressUpdate{
					Stage:      "coding",
					ItemIndex:  i,
					TotalItems: maxIterations,
				},
			})
		}
		response, err := s.LLM.Send(ctx, prompt)
		if err != nil {
			if ctx.Err() != nil {
				result.AddError(ctx.Err().Error())
				break
			}

			lastErrors = []string{err.Error()}
			continue
		}

		if usePatchPrompt {
			lastPatchContent = strings.TrimSpace(response)
		}

		var changes []domain.FileChange
		if usePatchPrompt {
			changes = codegen.ParseResponseWithPatches(response)
			patchAttempted = true
		} else {
			changes = codegen.ParseResponseWithOptions(response, defaultPath, allowFallback)
		}
		patchModeChanges := hasPatches(changes)

		if len(changes) == 0 {
			if usePatchPrompt {
				lastErrors = []string{
					"LLM did not return a valid SEARCH/REPLACE patch. Expected format: --- Patch: path --- with SEARCH/REPLACE blocks.",
				}

				if patchFixAttempts < maxPatchFixAttempts {
					patchRepairPending = true

					sendEvent(
						emit,
						domain.EventWarn,
						"Patch repair required: model did not return a valid patch.",
					)
				} else {
					forceFull = true
				}

				continue
			}

			lastErrors = []string{
				"LLM did not return file blocks. Expected format: --- File: path ---",
			}
			continue
		}

		// Patch можно применять только к существующим файлам.
		if patchModeChanges {
			var missing []string
			for _, ch := range changes {
				if len(ch.Patches) > 0 && !s.fileExistsRoot(ch.Path) {
					missing = append(missing, ch.Path)
				}
			}
			if len(missing) > 0 {
				lastErrors = []string{
					fmt.Sprintf("patch target files do not exist: %s", strings.Join(missing, ", ")),
				}
				forceFull = true
				continue
			}
		}
		if err := codegen.Validate(changes, s.Cfg.WorkDir); err != nil {
			lastErrors = []string{err.Error()}

			if patchModeChanges || usePatchPrompt {
				if patchFixAttempts < maxPatchFixAttempts {
					patchRepairPending = true

					sendEvent(
						emit,
						domain.EventWarn,
						fmt.Sprintf(
							"Patch validation failed. Will request corrected patch (%d/%d).",
							patchFixAttempts+1,
							maxPatchFixAttempts,
						),
					)
				} else {
					forceFull = true
				}
			}

			continue
		}
		lastChanges = changes

		// Сохраняем содержимое патча для возможного запроса на исправление.
		if patchModeChanges {
			lastPatchContent = formatPatchContent(changes)
		}

		createdSet := map[string]bool{}
		modifiedSet := map[string]bool{}
		patchedSet := map[string]bool{}
		fullRewrittenSet := map[string]bool{}
		var createdList, modifiedList []string
		var patchedList, fullRewrittenList []string
		for _, ch := range changes {
			exists := s.fileExistsRoot(ch.Path)
			if len(ch.Patches) > 0 {
				if !patchedSet[ch.Path] {
					patchedSet[ch.Path] = true
					patchedList = append(patchedList, ch.Path)
				}
			} else {
				if exists {
					if !fullRewrittenSet[ch.Path] {
						fullRewrittenSet[ch.Path] = true
						fullRewrittenList = append(fullRewrittenList, ch.Path)
					}
				}
			}
			if exists {
				if !modifiedSet[ch.Path] {
					modifiedSet[ch.Path] = true
					modifiedList = append(modifiedList, ch.Path)
				}
			} else {
				if !createdSet[ch.Path] {
					createdSet[ch.Path] = true
					createdList = append(createdList, ch.Path)
				}
			}
		}
		result.FilesPatched = patchedList
		result.FilesFullRewritten = fullRewrittenList

		var summaryParts []string
		if len(patchedList) > 0 {
			summaryParts = append(summaryParts, fmt.Sprintf("%d patched (DIFF)", len(patchedList)))
		}
		if len(fullRewrittenList) > 0 {
			summaryParts = append(summaryParts, fmt.Sprintf("%d fully rewritten", len(fullRewrittenList)))
		}
		if len(createdList) > 0 {
			summaryParts = append(summaryParts, fmt.Sprintf("%d created", len(createdList)))
		}

		sendEvent(emit, domain.EventLog, "Preparing sandbox")

		sandbox, err := s.WS.PrepareSandbox(ctx)
		if err != nil {
			lastErrors = []string{err.Error()}
			if patchModeChanges || usePatchPrompt {
				if patchFixAttempts < maxPatchFixAttempts {
					patchRepairPending = true

					sendEvent(
						emit,
						domain.EventWarn,
						"Sandbox preparation failed. Patch will be regenerated before full-file fallback.",
					)
				} else {
					forceFull = true
				}
			}

			continue
		}

		if err := s.WS.ApplyChangesSmartWithPolicy(
			sandbox,
			changes,
			patchPolicy,
			s.Cfg.FuzzyMinConfidence,
		); err != nil {
			_ = os.RemoveAll(sandbox)
			lastErrors = []string{err.Error()}
			if patchModeChanges || usePatchPrompt {
				if patchFixAttempts < maxPatchFixAttempts {
					patchRepairPending = true
					sendEvent(
						emit,
						domain.EventWarn,
						"Patch rejected before validation. Will request a corrected patch.",
					)
					// Ранний fallback: после 2 неудач переходим к полному файлу.
					if patchFixAttempts >= 2 {
						sendEvent(emit, domain.EventWarn,
							"Patch failed twice. Switching to full-file mode.")
						forceFull = true
						patchRepairPending = false
					}
				} else {
					forceFull = true
				}
			}
			continue
		}

		s.Runner.DepsLog = func(msg string) {
			sendEvent(emit, domain.EventLog, msg)
		}
		emitEvent(emit, domain.Event{
			Type:      domain.EventLog,
			Message:   "Running go build",
			TaskStage: domain.TaskStageVerifying,
		})

		if err := s.Runner.Build(ctx, sandbox); err != nil {
			buildError := trim(err.Error(), 4000)

			lastErrors = []string{
				buildError,
			}

			if s.Cfg.AutoSearch &&
				isDependencyFetchError(buildError) {

				dependency := extractDependencyImportPath(
					buildError,
				)

				if dependency != "" {
					research, searchErr :=
						s.searchForDependency(
							ctx,
							sandbox,
							dependency,
							buildError,
							emit,
						)

					if searchErr != nil {
						sendEvent(
							emit,
							domain.EventWarn,
							fmt.Sprintf(
								"Dependency auto-search failed (non-fatal): %v",
								searchErr,
							),
						)
					} else if strings.TrimSpace(research) != "" {
						lastErrors = append(
							lastErrors,
							research,
						)
					}
				}
			}

			_ = os.RemoveAll(sandbox)

			if patchModeChanges || usePatchPrompt {
				if patchFixAttempts < maxPatchFixAttempts {
					patchFixAttempts++

					patchRepairPending = true

					sendEvent(
						emit,
						domain.EventWarn,
						fmt.Sprintf(
							"Build failed after patch. Requesting patch repair (%d/%d).",
							patchFixAttempts,
							maxPatchFixAttempts,
						),
					)
				} else {
					forceFull = true
				}
			}

			continue
		}

		if !opts.NoTests {
			sendEvent(emit, domain.EventLog, "Running go test")
			tests, err := s.Runner.Test(ctx, sandbox)
			result.Tests = tests
			if err != nil {
				_ = os.RemoveAll(sandbox)
				if ctx.Err() != nil {
					result.AddError(ctx.Err().Error())
					break
				}
				lastErrors = []string{formatTestFeedback(tests, err)}
				// ─── ИЗМЕНЕНО: патч применился, build прошёл, но тесты упали ──
				if patchModeChanges || usePatchPrompt {
					if patchFixAttempts < maxPatchFixAttempts {
						patchAppliedSuccessfully = true
						sendEvent(emit, domain.EventWarn,
							"Patch applied, build passed, but tests failed. Will request patch fix.")
					} else {
						forceFull = true
					}
				}
				continue
			}
			if tests.Failed > 0 {
				_ = os.RemoveAll(sandbox)
				lastErrors = []string{"tests failed", runner.FormatFeedback(tests)}
				// ─── ИЗМЕНЕНО: аналогично ──────────────────────────────
				if patchModeChanges || usePatchPrompt {
					if patchFixAttempts < maxPatchFixAttempts {
						patchAppliedSuccessfully = true
						sendEvent(emit, domain.EventWarn,
							"Patch applied, build passed, but tests failed. Will request patch fix.")
					} else {
						forceFull = true
					}
				}
				continue
			}
		}

		// ... далее без изменений (result.OutputFiles, DryRun, CopyToRootSafe и т.д.)
		result.OutputFiles = s.collectOutputFiles(sandbox, changes)
		if opts.DryRun {
			_ = os.RemoveAll(sandbox)
			result.Success = true
			result.FilesCreated = createdList
			result.FilesModified = modifiedList
			result.FilesPatched = patchedList
			result.FilesFullRewritten = fullRewrittenList
			for _, f := range patchedList {
				sendEvent(emit, domain.EventLog, "Dry-run DIFF patch: "+f)
			}
			for _, f := range fullRewrittenList {
				sendEvent(emit, domain.EventLog, "Dry-run full file rewrite: "+f)
			}
			for _, f := range createdList {
				sendEvent(emit, domain.EventLog, "Dry-run create new file: "+f)
			}
			if len(summaryParts) > 0 {
				result.Response = "Dry-run validated: " + strings.Join(summaryParts, ", ") + "."
			} else if patchModeChanges {
				result.Response = "Dry-run: patch changes were validated in sandbox but not applied."
			} else {
				result.Response = "Dry-run: changes were validated in sandbox but not applied."
			}
			return result
		}

		// ─── Diff preview перед применением ───────────────────────────
		if len(changes) > 0 {
			var previewLines []string
			for _, ch := range changes {
				if len(ch.Patches) > 0 {
					previewLines = append(previewLines,
						fmt.Sprintf("  Δ %s (%d patch block(s))", ch.Path, len(ch.Patches)))
				} else if s.fileExistsRoot(ch.Path) {
					previewLines = append(previewLines,
						fmt.Sprintf("  ≡ %s (full rewrite, %d bytes)", ch.Path, len(ch.Content)))
				} else {
					previewLines = append(previewLines,
						fmt.Sprintf("  + %s (new, %d bytes)", ch.Path, len(ch.Content)))
				}
			}
			sendEvent(emit, domain.EventLog,
				"Changes to apply:\n"+strings.Join(previewLines, "\n"))
		}

		sendEvent(emit, domain.EventLog, "Applying changes to project")
		if err := s.WS.CopyToRootSafe(sandbox, changes); err != nil {
			_ = os.RemoveAll(sandbox)
			lastErrors = []string{err.Error()}
			continue
		}
		s.WS.RefreshIndex()
		_ = os.RemoveAll(sandbox)
		result.Success = true
		result.FilesCreated = createdList
		result.FilesModified = modifiedList
		result.FilesPatched = patchedList
		result.FilesFullRewritten = fullRewrittenList
		for _, f := range patchedList {
			sendEvent(emit, domain.EventLog, "Applied DIFF patch: "+f)
		}
		for _, f := range fullRewrittenList {
			sendEvent(emit, domain.EventLog, "Applied full file rewrite: "+f)
		}
		for _, f := range createdList {
			sendEvent(emit, domain.EventLog, "Created new file: "+f)
		}
		if len(summaryParts) > 0 {
			result.Response = "Applied changes: " + strings.Join(summaryParts, ", ") + "."
		} else if patchModeChanges {
			result.Response = fmt.Sprintf("Applied %d patch/file change(s).", len(changes))
		} else {
			result.Response = fmt.Sprintf("Applied %d file(s).", len(changes))
		}
		if !opts.NoCommit && s.Cfg.AutoGitCommit {
			hash, err := s.commit(ctx, query, emit)
			if err != nil {
				result.AddWarning(fmt.Sprintf("git commit failed: %v", err))
			} else if hash != "" {
				result.GitCommit = hash
			}
		}
		result.PreTaskHead = preTaskHead
		s.lastPreTaskHead = preTaskHead
		if !opts.DryRun {
			result.CumulativeDiff = s.captureCumulativeDiff(ctx, preTaskHead)
		}
		return result
	}
	result.Success = false
	if len(lastErrors) == 0 {
		lastErrors = []string{"unknown error"}
	}
	result.Errors = append(result.Errors, lastErrors...)
	return result
}

// formatPatchContent форматирует патчи для передачи в промпт исправления.
func formatPatchContent(changes []domain.FileChange) string {
	var b strings.Builder
	for _, ch := range changes {
		if len(ch.Patches) == 0 {
			continue
		}
		b.WriteString("--- Patch: " + ch.Path + " ---\n")
		for _, p := range ch.Patches {
			if p.Symbol != "" {
				b.WriteString("--- Symbol: ")
				b.WriteString(p.Symbol)
				b.WriteString(" ---\n")
			}

			b.WriteString("<<<<<<< SEARCH\n")
			b.WriteString(p.Search)
			b.WriteString("\n=======\n")
			b.WriteString(p.Replace)
			b.WriteString("\n>>>>>>> REPLACE\n")
		}
	}
	return b.String()
}

func (s *Service) RunTests(ctx context.Context, emit func(domain.Event)) domain.Result {
	result := domain.Result{
		Mode: "test",
	}

	sendEvent(emit, domain.EventLog, "Preparing sandbox")

	sandbox, err := s.WS.PrepareSandbox(ctx)
	if err != nil {
		result.AddError(err.Error())
		return result
	}

	defer os.RemoveAll(sandbox)

	s.Runner.DepsLog = func(msg string) {
		sendEvent(emit, domain.EventLog, msg)
	}

	sendEvent(emit, domain.EventLog, "Running go build")

	if err := s.Runner.Build(ctx, sandbox); err != nil {
		result.AddError(err.Error())
		return result
	}

	emitEvent(emit, domain.Event{
		Type:      domain.EventLog,
		Message:   "Running tests",
		TaskStage: domain.TaskStageTesting,
	})

	tests, err := s.Runner.Test(ctx, sandbox)
	result.Tests = tests

	if err != nil {
		result.AddError(formatTestFeedback(tests, err))
		return result
	}

	if tests.Skipped {
		result.Success = true
		result.Response = "No Go test files found."
		return result
	}

	if tests.Failed > 0 {
		result.Success = false
		emitEvent(emit, domain.Event{
			Type:      domain.EventWarn,
			Message:   "Validation failed; repairing generated changes",
			TaskStage: domain.TaskStageRepairing,
		})
		result.Response = fmt.Sprintf(
			"Tests failed: %d passed, %d failed%s",
			tests.Passed,
			tests.Failed,
			coverageSuffix(tests),
		)
		result.AddError(runner.FormatFeedback(tests))
		return result
	}

	result.Success = true
	result.Response = fmt.Sprintf(
		"Tests passed: %d%s",
		tests.Passed,
		coverageSuffix(tests),
	)

	return result
}

func (s *Service) RunFile(ctx context.Context, file string, emit func(domain.Event)) domain.Result {
	result := domain.Result{
		Mode: "run",
	}

	emitEvent(emit, domain.Event{
		Type:      domain.EventLog,
		Message:   i18n.Localize("Running go program"),
		TaskStage: domain.TaskStageRun,
	})

	sendEvent(emit, domain.EventLog, "Preparing sandbox")

	sandbox, err := s.WS.PrepareSandbox(ctx)
	if err != nil {
		result.AddError(err.Error())
		return result
	}
	defer os.RemoveAll(sandbox)

	targetDir := sandbox

	if file != "" {
		safePath, err := security.SafeJoin(s.Cfg.WorkDir, file)
		if err != nil {
			result.AddError(err.Error())
			return result
		}

		rel, err := filepath.Rel(s.Cfg.WorkDir, safePath)
		if err != nil {
			result.AddError(err.Error())
			return result
		}

		targetDir = filepath.Join(sandbox, filepath.Dir(rel))
	}

	s.Runner.DepsLog = func(msg string) {
		sendEvent(emit, domain.EventLog, msg)
	}
	sendEvent(emit, domain.EventLog, "Running go run .")

	output, err := s.Runner.RunDir(ctx, targetDir)
	result.Response = trim(output, 20000)

	if err != nil {
		result.AddError(err.Error())
		return result
	}

	result.Success = true
	return result
}

func (s *Service) GitStatus(ctx context.Context, emit func(domain.Event)) domain.Result {
	result := domain.Result{Mode: "git"}

	if !s.Git.IsRepo(ctx) {
		result.Success = true
		result.Response = "Not a git repository."
		return result
	}

	out, err := s.Git.Status(ctx)
	if err != nil {
		result.AddError(err.Error())
		return result
	}

	if strings.TrimSpace(out) == "" {
		out = "Working tree clean."
	}

	result.Success = true
	result.Response = out
	return result
}

func (s *Service) GitDiff(ctx context.Context, emit func(domain.Event)) domain.Result {
	result := domain.Result{Mode: "git"}
	if !s.Git.IsRepo(ctx) {
		result.Success = true
		result.Response = "Not a git repository."
		return result
	}
	out, err := s.Git.Diff(ctx)
	if err != nil {
		result.AddError(err.Error())
		return result
	}
	// Если рабочая директория чистая, показываем изменения последнего коммита.
	if strings.TrimSpace(out) == "" {
		out, err = s.Git.DiffLast(ctx)
		if err != nil {
			out = "No diff."
		}
	}
	if strings.TrimSpace(out) == "" {
		out = "No diff."
	}
	result.Success = true
	result.Response = out
	return result
}

func (s *Service) GitCommit(ctx context.Context, emit func(domain.Event)) domain.Result {
	result := domain.Result{Mode: "git"}
	if err := s.Git.EnsureRepo(ctx, s.Cfg.GitAutoInit); err != nil {
		result.AddError(err.Error())
		return result
	}

	// Генерируем осмысленное сообщение на основе реального diff.
	sendEvent(emit, domain.EventLog, "Generating commit message from diff...")
	commitMessage := s.generateCommitMessage(ctx, "", emit)
	if commitMessage == "" {
		commitMessage = "gogitor: manual commit"
	}

	hash, err := s.Git.AutoCommit(ctx, commitMessage)
	if err != nil {
		result.AddError(err.Error())
		return result
	}
	result.Success = true
	if hash == "" {
		result.Response = "Nothing to commit."
	} else {
		result.Response = "Committed " + hash
		result.GitCommit = hash
	}
	return result
}

func (s *Service) GitInit(ctx context.Context, emit func(domain.Event)) domain.Result {
	result := domain.Result{Mode: "git"}

	if err := s.Git.EnsureRepo(ctx, true); err != nil {
		result.AddError(err.Error())
		return result
	}

	result.Success = true
	result.Response = "Git repository initialized."
	return result
}

func (s *Service) GitLog(ctx context.Context, emit func(domain.Event)) domain.Result {
	result := domain.Result{Mode: "git"}
	if !s.Git.IsRepo(ctx) {
		result.Success = true
		result.Response = "Not a git repository."
		return result
	}
	out, err := s.Git.LogHistory(ctx, 20)
	if err != nil {
		result.AddError(err.Error())
		return result
	}
	if strings.TrimSpace(out) == "" {
		out = "No commits yet."
	}
	result.Success = true
	result.Response = out
	return result
}

func (s *Service) GitCheckout(ctx context.Context, args []string, emit func(domain.Event)) domain.Result {
	result := domain.Result{Mode: "git"}
	if !s.Git.IsRepo(ctx) {
		result.Success = true
		result.Response = "Not a git repository."
		return result
	}
	// :git checkout -b <new-branch> — создать ветку и сразу переключиться на неё.
	if len(args) > 0 && args[0] == "-b" {
		if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
			result.AddError("usage: :git checkout -b <new-branch>")
			return result
		}
		name := strings.TrimSpace(args[1])
		out, err := s.Git.CheckoutNew(ctx, name)
		if err != nil {
			result.AddError(err.Error())
			return result
		}
		result.Success = true
		result.Response = "Created and switched to branch '" + name + "'.\n" + strings.TrimSpace(out)
		return result
	}
	hash := ""
	if len(args) > 0 {
		hash = strings.TrimSpace(args[0])
	}
	if hash == "" {
		// Хеш не указан — показываем список коммитов для выбора.
		logOut, err := s.Git.LogHistory(ctx, 10)
		if err != nil || strings.TrimSpace(logOut) == "" {
			result.AddError("usage: :git checkout <commit-hash>")
			return result
		}
		result.Success = true
		result.Response = "Specify a commit hash. Recent commits:\n" + logOut +
			"\nUsage: :git checkout <hash>"
		return result
	}
	sendEvent(emit, domain.EventWarn,
		"Checking out commit "+hash+" (detached HEAD). Use ':git checkout <branch>' to return.")
	out, err := s.Git.Checkout(ctx, hash)
	if err != nil {
		result.AddError(err.Error())
		return result
	}
	s.WS.RefreshIndex()
	result.Success = true
	result.Response = "Checked out " + hash + ".\n" + strings.TrimSpace(out)
	return result
}

func (s *Service) GitBranch(ctx context.Context, args []string, emit func(domain.Event)) domain.Result {
	result := domain.Result{Mode: "git"}
	if !s.Git.IsRepo(ctx) {
		result.Success = true
		result.Response = "Not a git repository."
		return result
	}

	// :git branch — список веток
	if len(args) == 0 {
		out, err := s.Git.BranchList(ctx)
		if err != nil {
			result.AddError(err.Error())
			return result
		}
		current, _ := s.Git.CurrentBranch(ctx)
		header := ""
		if current != "" {
			header = "Current branch: " + current + "\n\n"
		}
		if strings.TrimSpace(out) == "" {
			out = "No branches found."
		}
		result.Success = true
		result.Response = header + strings.TrimSpace(out)
		return result
	}

	// :git branch -d <name> или :git branch -D <name> — удаление
	if args[0] == "-d" || args[0] == "-D" {
		if len(args) < 2 {
			result.AddError("usage: :git branch -d <branch-name>")
			return result
		}
		name := args[1]
		force := args[0] == "-D"
		out, err := s.Git.BranchDelete(ctx, name, force)
		if err != nil {
			result.AddError(err.Error())
			return result
		}
		result.Success = true
		result.Response = "Deleted branch " + name + ".\n" + strings.TrimSpace(out)
		return result
	}

	// :git branch <name> — создание ветки
	name := args[0]
	out, err := s.Git.BranchCreate(ctx, name)
	if err != nil {
		result.AddError(err.Error())
		return result
	}
	result.Success = true
	result.Response = "Created branch '" + name + "'.\n" + strings.TrimSpace(out) +
		"\nUse ':git checkout " + name + "' to switch to it."
	return result
}

// GitRevert создаёт новый коммит, отменяющий указанный (или последний).
func (s *Service) GitRevert(ctx context.Context, args []string, emit func(domain.Event)) domain.Result {
	result := domain.Result{Mode: "git"}
	if !s.Git.IsRepo(ctx) {
		result.Success = true
		result.Response = "Not a git repository."
		return result
	}

	hash := "HEAD"
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		hash = strings.TrimSpace(args[0])
	}

	sendEvent(emit, domain.EventLog,
		fmt.Sprintf("Reverting commit %s...", hash))

	out, err := s.Git.Revert(ctx, hash)
	if err != nil {
		errMsg := err.Error()
		// Конфликт при revert — типичная ситуация
		if strings.Contains(errMsg, "CONFLICT") || strings.Contains(errMsg, "conflict") {
			result.AddError(
				"Revert conflict detected. Resolve conflicts manually, then run ':git commit'.")
			result.AddError(trim(errMsg, 2000))
			return result
		}
		result.AddError(errMsg)
		return result
	}

	s.WS.RefreshIndex()
	result.Success = true
	result.Response = fmt.Sprintf(
		"Reverted %s. A new revert commit was created.\n%s",
		hash, strings.TrimSpace(out))
	return result
}

// GitReset откатывает ветку до указанного коммита.
// Поддерживает флаг --hard для полного удаления изменений.
func (s *Service) GitReset(ctx context.Context, args []string, emit func(domain.Event)) domain.Result {
	result := domain.Result{Mode: "git"}
	if !s.Git.IsRepo(ctx) {
		result.Success = true
		result.Response = "Not a git repository."
		return result
	}

	// Парсим аргументы: [--hard] <hash>
	hard := false
	hash := ""
	for _, a := range args {
		trimmed := strings.TrimSpace(a)
		if trimmed == "--hard" {
			hard = true
			continue
		}
		if trimmed == "--soft" {
			// soft не реализуем отдельно; mixed достаточно
			continue
		}
		if hash == "" && trimmed != "" {
			hash = trimmed
		}
	}

	if hash == "" {
		// Хеш не указан — показываем последние коммиты
		logOut, err := s.Git.LogHistory(ctx, 10)
		if err != nil || strings.TrimSpace(logOut) == "" {
			result.AddError("usage: :git reset [--hard] <commit-hash>")
			return result
		}
		result.Success = true
		result.Response = "Specify a commit hash. Recent commits:\n" + logOut +
			"\nUsage: :git reset [--hard] <hash>"
		return result
	}

	// Проверяем наличие незакоммиченных изменений при --hard
	if hard {
		status, err := s.Git.Status(ctx)
		if err == nil && strings.TrimSpace(status) != "" {
			sendEvent(emit, domain.EventWarn,
				"WARNING: '--hard' will DISCARD all uncommitted changes:\n"+
					trim(status, 500))
		}
	}

	mode := "mixed (changes kept in working directory)"
	if hard {
		mode = "hard (all changes will be LOST)"
	}
	sendEvent(emit, domain.EventWarn,
		fmt.Sprintf("Resetting to %s (%s). This rewrites branch history.", hash, mode))

	out, err := s.Git.Reset(ctx, hash, hard)
	if err != nil {
		result.AddError(err.Error())
		return result
	}

	s.WS.RefreshIndex()
	result.Success = true
	result.Response = fmt.Sprintf(
		"Reset to %s (%s).\n%s", hash, mode, strings.TrimSpace(out))
	return result
}

func (s *Service) GitMerge(ctx context.Context, branch string, emit func(domain.Event)) domain.Result {
	result := domain.Result{Mode: "git"}
	if !s.Git.IsRepo(ctx) {
		result.Success = true
		result.Response = "Not a git repository."
		return result
	}
	branch = strings.TrimSpace(branch)
	if branch == "" {
		result.AddError("usage: :git merge <branch-name>")
		return result
	}
	current, _ := s.Git.CurrentBranch(ctx)
	sendEvent(emit, domain.EventLog,
		fmt.Sprintf("Merging '%s' into '%s'...", branch, current))
	out, err := s.Git.Merge(ctx, branch)
	if err != nil {
		result.AddError(err.Error())
		return result
	}
	s.WS.RefreshIndex()
	result.Success = true
	result.Response = fmt.Sprintf("Merged '%s' into '%s'.\n%s", branch, current, strings.TrimSpace(out))
	return result
}

func (s *Service) plan(ctx context.Context, query string, emit func(domain.Event)) []string {
	prompt := prompts.Plan(query)

	response, err := s.LLM.Send(ctx, prompt)
	if err != nil {
		return []string{query}
	}

	lines := strings.Split(response, "\n")
	var tasks []string

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "Subtask") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				task := strings.TrimSpace(parts[1])
				if task != "" {
					tasks = append(tasks, task)
				}
			}
		}
	}

	if len(tasks) == 0 {
		return []string{query}
	}

	if len(tasks) > 5 {
		tasks = tasks[:5]
	}

	return tasks
}

func (s *Service) commit(ctx context.Context, message string, emit func(domain.Event)) (string, error) {
	if err := s.Git.EnsureRepo(ctx, s.Cfg.GitAutoInit); err != nil {
		return "", err
	}

	// Пытаемся сгенерировать осмысленное сообщение на основе реального diff.
	sendEvent(emit, domain.EventLog, "Generating commit message from diff...")
	commitMessage := s.generateCommitMessage(ctx, message, emit)
	if commitMessage == "" {
		// Fallback: старое поведение.
		commitMessage = "gogitor: " + truncate(message, 70)
	}

	hash, err := s.Git.AutoCommit(ctx, commitMessage)
	if err != nil {
		return "", err
	}
	if hash != "" {
		sendEvent(emit, domain.EventLog, "Git commit created: "+hash)
	}
	return hash, nil
}

func (s *Service) buildCodeContext(query string, targetFiles []string) codeContextInfo {
	existingTargets := s.WS.ExistingFiles(targetFiles)
	hasExisting := len(existingTargets) > 0 || s.WS.HasGoFiles()
	if !hasExisting {
		return codeContextInfo{}
	}

	// Масштабируем лимиты от размера контекста модели
	maxFiles, maxBytes := s.contextLimits()

	context := s.WS.BuildSmartContext(query, targetFiles, maxFiles, maxBytes)
	return codeContextInfo{
		Context:         context,
		HasExisting:     true,
		ExistingTargets: existingTargets,
	}
}

func (s *Service) contextLimits() (
	maxFiles int,
	maxBytes int,
) {
	ctxTokens :=
		s.effectiveAgentContextTokens()

	if ctxTokens <= 0 {
		ctxTokens = 32768
	}

	switch {
	case ctxTokens <= 8192:
		maxFiles = 10

	case ctxTokens <= 32768:
		maxFiles = 18

	case ctxTokens <= 65536:
		maxFiles = 30

	case ctxTokens <= 131072:
		maxFiles = 45

	default:
		maxFiles = 70
	}

	const bytesPerToken = 4
	const projectContextShare = 0.70

	maxBytes = int(
		float64(
			ctxTokens*bytesPerToken,
		) * projectContextShare,
	)

	if maxBytes < 16*1024 {
		maxBytes = 16 * 1024
	}

	return maxFiles, maxBytes
}

func (s *Service) patchPolicyForOptions(
	opts Options,
) workspace.PatchPolicy {
	if opts.AgentDepth == AgentDepthDeep {
		return workspace.PatchPolicyStrict
	}

	caps := s.agentModelCapabilities()

	if caps.HasPatchPolicy {
		return caps.PatchPolicy
	}

	return workspace.PatchPolicyForModel(
		s.Cfg.Provider,
		s.Cfg.Model,
		s.Cfg.PatchPolicies,
	)
}

// reviewLimits возвращает лимиты для ревьюера/верификатора.
func (s *Service) reviewLimits() (maxTotal int, maxPerFile int) {
	ctxTokens :=
		s.effectiveAgentContextTokens()
	switch {
	case ctxTokens <= 8192:
		return 15000, 4000
	case ctxTokens <= 32768:
		return 30000, 8000
	case ctxTokens <= 65536:
		return 80000, 20000
	case ctxTokens <= 131072:
		return 150000, 40000
	default: // 256K+
		return 300000, 60000
	}
}

func (s *Service) isComplex(query string) bool {
	lower := strings.ToLower(query)

	keywords := []string{
		"раздели",
		"split",
		"рефактор",
		"refactor",
		"архитект",
		"architecture",
		"систем",
		"system",
		"многофайл",
		"multi-file",
		"много файлов",
		"проект",
		"project",
		"api",
		"сервер",
		"server",
	}

	if containsAny(lower, keywords) {
		return true
	}

	return len(strings.Fields(query)) > 14
}

func (s *Service) needsModify(query string) bool {
	lower := strings.ToLower(query)

	keywords := []string{
		"измени",
		"исправь",
		"обнови",
		"переделай",
		"перепиши",
		"рефактор",
		"рефакторинг",
		"раздели",
		"разбить",
		"разбей",
		"вынеси",
		"вынести",
		"перенеси",
		"перенести",
		"добавь",
		"улучши",
		"модифицир",
		"модификац",
		"модифик",
		"modify",
		"fix",
		"update",
		"change",
		"refactor",
		"rewrite",
		"split",
		"divide",
		"separate",
		"extract",
		"move",
		"add",
		"improve",
		"correct",
	}

	return containsAny(lower, keywords)
}

func (s *Service) isAnalysisOnlyTask(query string) bool {
	lower := strings.ToLower(query)

	analysisKeywords := []string{
		"analyze",
		"analyse",
		"analysis",
		"read",
		"inspect",
		"identify",
		"find",
		"extract",
		"explain",
		"understand",
		"determine",
		"review",

		"проанализируй",
		"анализ",
		"прочитай",
		"читать",
		"изучи",
		"изучить",
		"найди",
		"найти",
		"определи",
		"определить",
		"извлеки",
		"извлечь",
		"объясни",
		"объяснить",
		"пойми",
		"понять",
		"разберись",
		"разобраться",
	}

	modificationKeywords := []string{
		"create",
		"write",
		"add",
		"modify",
		"change",
		"fix",
		"update",
		"refactor",
		"rewrite",
		"generate",
		"implement",
		"delete",
		"remove",
		"move",
		"split",
		"separate",
		"correct",
		"improve",
		"apply",
		"save",
		"модифицир",
		"модификац",
		"модифик",
		"создай",
		"создать",
		"напиши",
		"писать",
		"добавь",
		"добавить",
		"измени",
		"изменить",
		"исправь",
		"исправить",
		"обнови",
		"обновить",
		"переделай",
		"переделать",
		"перепиши",
		"переписать",
		"сгенерируй",
		"сгенерировать",
		"реализуй",
		"реализовать",
		"удали",
		"удалить",
		"перемести",
		"переместить",
		"раздели",
		"разделить",
		"вынеси",
		"вынести",
		"примени",
		"применить",
		"сохрани",
		"сохранить",
		"сделай",
		"сделать",
	}

	if !containsAny(lower, analysisKeywords) {
		return false
	}

	if containsAny(lower, modificationKeywords) {
		return false
	}

	if s.needsModify(query) || s.isSplitOrRefactor(query) {
		return false
	}

	return true
}

func (s *Service) isSplitOrRefactor(query string) bool {
	lower := strings.ToLower(query)

	keywords := []string{
		"раздели",
		"разделить",
		"разделение",
		"разбей",
		"разбить",
		"вынеси",
		"вынести",
		"перенеси",
		"перенести",
		"рефактор",
		"рефакторинг",
		"реструктур",
		"split",
		"divide",
		"separate",
		"extract",
		"move",
		"refactor",
		"refactoring",
		"restructure",
	}

	return containsAny(lower, keywords)
}

func (s *Service) fileExistsRoot(path string) bool {
	full, err := security.SafeJoin(s.Cfg.WorkDir, path)
	if err != nil {
		return false
	}

	_, err = os.Stat(full)
	return err == nil
}

func (s *Service) historyString() string {
	if len(s.history) == 0 {
		return ""
	}

	// Масштабируем размер истории от контекста
	maxItems := 5
	queryLen := 500
	answerLen := 800

	ctxTokens := s.Cfg.EffectiveContextTokens()
	if ctxTokens > 65536 {
		maxItems = 10
		queryLen = 1500
		answerLen = 3000
	}
	if ctxTokens > 131072 {
		maxItems = 15
		queryLen = 3000
		answerLen = 6000
	}

	start := len(s.history) - maxItems
	if start < 0 {
		start = 0
	}

	var b strings.Builder
	for _, h := range s.history[start:] {
		b.WriteString("USER: ")
		b.WriteString(truncate(h.Query, queryLen))
		b.WriteString("\nASSISTANT: ")
		b.WriteString(truncate(h.Answer, answerLen))
		b.WriteString("\n")
	}
	return b.String()
}

func (s *Service) addHistory(query, answer string) {
	s.history = append(s.history, historyItem{
		Query:  query,
		Answer: answer,
	})

	if len(s.history) > 20 {
		s.history = s.history[len(s.history)-20:]
	}
}

var knownFileExtensions = []string{
	// Go
	".go", ".mod", ".sum",
	// Web
	".html", ".htm", ".css", ".js", ".ts", ".jsx", ".tsx",
	// Скрипты
	".sh", ".bash", ".zsh", ".fish", ".command",
	// Конфигурация
	".json", ".yaml", ".yml", ".toml", ".xml", ".env",
	".cfg", ".ini", ".conf", ".properties",
	// Данные / БД
	".sql", ".csv",
	// Документация / задачи
	".md", ".txt", ".rst",
	// Другие языки
	".py", ".rb", ".php", ".java",
	".c", ".cpp", ".h", ".hpp",
	// Схемы / API
	".proto", ".graphql", ".gql",
}

func extractTargetFiles(query string) []string {
	var files []string
	seen := map[string]bool{}
	for _, word := range strings.Fields(query) {
		word = strings.Trim(word, ".,;:()[]{}\"'`")
		if hasKnownFileExtension(word) {
			if !seen[word] {
				seen[word] = true
				files = append(files, word)
			}
		}
	}
	return files
}

func hasKnownFileExtension(word string) bool {
	lower := strings.ToLower(word)
	for _, ext := range knownFileExtensions {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

func defaultPath(query string, targetFiles []string) string {
	if len(targetFiles) > 0 {
		return targetFiles[0]
	}

	lower := strings.ToLower(query)
	if strings.Contains(lower, "html") {
		return "index.html"
	}

	return "main.go"
}

func containsAny(s string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(s, kw) {
			return true
		}
	}
	return false
}

func truncate(s string, max int) string {
	return textutil.LimitRunes(strings.TrimSpace(s), max, "...")
}

func trim(s string, max int) string {
	return textutil.LimitRunes(s, max, "...")
}

// ─── Отображение плана выполнения ────────────────────────────

// sendPlanBoard показывает пользователю план перед началом выполнения.
func sendPlanBoard(emit func(domain.Event), goal string, acceptance []string, items []string) {
	if emit == nil {
		return
	}
	emit(domain.Event{
		Type:    domain.EventPlan,
		Message: i18n.Localize(fmt.Sprintf("Execution plan (goal: %s)", goal)),
		Plan: &domain.PlanUpdate{
			Goal:       goal,
			Acceptance: acceptance,
			Items:      items,
		},
	})
}

// sendPlanStatus обновляет статус одного пункта плана.
func sendPlanStatus(emit func(domain.Event), index, total int, task string, st domain.PlanStatus, note string) {
	if emit == nil {
		return
	}
	msg := fmt.Sprintf("[%d/%d] %s", index, total, task)
	if strings.TrimSpace(note) != "" {
		msg += " — " + note
	}
	emit(domain.Event{
		Type:    domain.EventPlan,
		Message: msg,
		Plan: &domain.PlanUpdate{
			ItemIndex: index,
			Status:    st,
			Note:      note,
		},
	})
}

// sendPlanSummary отправляет итоговую сводку по плану.
func sendPlanSummary(emit func(domain.Event), st domain.PlanStatus, completed, total int) {
	if emit == nil {
		return
	}
	emit(domain.Event{
		Type:    domain.EventPlan,
		Message: i18n.Localize(fmt.Sprintf("Plan completed: %d/%d", completed, total)),
		Plan: &domain.PlanUpdate{
			Status: st,
		},
	})
}

func sendEvent(emit func(domain.Event), typ domain.EventType, msg string) {
	if emit == nil {
		return
	}

	emit(domain.Event{
		Type:    typ,
		Message: i18n.Localize(msg),
	})
}

func emitEvent(emit func(domain.Event), event domain.Event) {
	if emit == nil {
		return
	}

	emit(event)
}

func DetectLanguage() string {
	return string(i18n.Current())
}

func HelpText() string {
	if DetectLanguage() == "ru" {
		return helpTextRu()
	}
	return helpTextEn()
}

func helpTextEn() string {
	return `# Gogitor Commands

## General
- **:help** — Show help
- **:clear** — Clear in-memory conversation context
- **:save <file>** — Save last result to file (.md, .txt, .go, .json)
- **:reasoning** — Show current thinking mode state
- **:reasoning on/off** — Enable/disable thinking mode

## Code & Analysis
- **:code <task>** — Create or modify code
- **:fast <task>** — Quick single-pass code generation (no multi-agent pipeline)
- **:load <file>** — Load a task from a .txt/.md file and route it
- **:fix <error>** — Fix error from stack trace / terminal output
- **:ask <question>** — Chat mode
- **:analyze <task>** — Analyze code without modifying files
- **:search <query>** — Web search
- **:suggest** — Analyze project health: tech debt, missing tests, code smells

## Execution & Testing
- **:run [file]** — Run Go project or file directory in sandbox
- **:test** — Run tests in sandbox
- **:test lint** — Run golangci-lint and auto-fix issues via LLM
- **:vet** — Run go vet (fast, no LLM required)
- **:todo** — List TODO/FIXME/HACK markers in project files
- **:history** — Show recent task execution history
- **:task-diff** — Show cumulative diff of the last completed task

## Articles
- **:article <topic>** — Write a technical article (simple note)
- **:article --full <topic>** — Write a complex multi-section article

## Autonomy & Computer
- **:autonomy [on|off|status|run|clear]** — Autonomous engineering mode
- **:mutate [limit]** — Run mutation testing (deterministic, no LLM)
- **:autogen-tests [n]** — Auto-generate unit tests for untested functions
- **:computer <task>** — Execute system admin task (requires GOGITOR_COMPUTER_ENABLED=true)

---

## Agent
- **:agent <task>** — Run the full Agent Harness.
- **:agent deep <task>** — Run the strengthened Agent profile with:
    - task isolation
    - deterministic quality gates
    - session artifacts
    - final verification
- **:agent interview <task>** — Ask clarifying questions before running the Agent.
- **:agent reflect** — Analyze the latest Agent session and extract lessons.
- **:agent report** — Show the latest Agent report
- **:agent resume** — Resume the latest failed Agent session
- **:agent undo** — Revert the latest completed Agent commit
---

## Git & GitHub
- **:git status** — Show working tree status
- **:git diff** — Show diff (working dir vs HEAD)
- **:git diff-task** — Cumulative diff of the last task
- **:git commit** — Commit all changes
- **:git commit --split <f1,f2>** — Separate commits for specified files
- **:git init** — Initialize git repository
- **:git log** — Show commit history
- **:git checkout <h>** — Checkout commit or branch
- **:git checkout -b <n>** — Create and switch to new branch
- **:git branch** — List / create / delete branches
- **:git merge <n>** — Merge branch
- **:git revert [h]** — Revert commit (safe, creates new commit)
- **:git reset [--hard] <h>** — Reset to commit (--hard is destructive)
- **:git push [branch]** — Push to remote
- **:git pull [branch]** — Pull from remote
- **:git fetch** — Fetch from remote
- **:git clone <url>** — Clone repository
- **:git remote** — List / add / remove remotes
- **:git create <name> [--private] [--desc <text>]** — Create GitHub repository
- **:git pr** — Create Pull Request
- **:git issue** — Create Issue from failing tests
- **:git changelog** — Generate CHANGELOG.md
- **:git pr-comment <n> [text]** — Add PR comment

### Git Details
**:git commit**
Commits all changes. Sets user.name=Gogitor if missing.

**:git commit --split <f1,f2>**
Creates separate commits for specified files. Each gets an LLM-generated message. Remaining changes go into a general commit.

**:git checkout <h>**
Switch to commit or branch. Commits result in detached HEAD.

**:git revert [h]**
Safely undoes a commit by creating a new one.

**:git reset [--hard] <h>**
Rewrites history. --hard permanently discards uncommitted changes.

**:git pr**
Creates a PR from the current branch to the default branch. Requires GitHub token.

**:git issue**
Runs tests and creates a GitHub Issue if any fail.

**:git changelog**
Parses Conventional Commits and generates CHANGELOG.md.

---

## TUI Keys
- **F2** — Toggle mouse text selection mode
- **Tab** — Switch input/output focus
- **Alt+Enter** — New line in input
- **Ctrl+C** — Cancel task / quit
`
}

func helpTextRu() string {
	return `# Команды Gogitor

## Общие
- **:help** — Показать справку
- **:clear** — Очистить контекст разговора
- **:save <файл>** — Сохранить результат в файл (.md, .txt, .go, .json)
- **:reasoning** — Показать состояние режима размышления
- **:reasoning on/off** — Включить/выключить режим размышления

## Код и Анализ
- **:code <задача>** — Создать или изменить код
- **:fast <задача>** — Быстрая генерация кода без мультиагентного конвейера
- **:load <файл>** — Загрузить задачу из файла .txt/.md и передать роутеру
- **:fix <ошибка>** — Исправить ошибку по stack trace / выводу терминала
- **:ask <вопрос>** — Режим чата
- **:analyze <задача>** — Анализ кода без изменения файлов
- **:search <запрос>** — Поиск в интернете
- **:suggest** — Анализ состояния проекта (техдолг, тесты, запахи кода)

## Запуск и Тесты
- **:run [файл]** — Запуск Go-проекта или файла в песочнице
- **:test** — Запуск тестов в песочнице
- **:test lint** — Запуск golangci-lint и автоисправление через LLM
- **:vet** — Запуск go vet (быстро, без LLM)
- **:todo** — Вывод TODO/FIXME/HACK маркеров
- **:history** — История выполненных задач
- **:task-diff** — Накопительный diff последней задачи

## Статьи
- **:article <тема>** — Написать техническую статью (простая заметка)
- **:article --full <тема>** — Написать сложную многосекционную статью

## Автономность и Компьютер
- **:autonomy [on|off|status|run|clear]** — Автономный режим (фоновый мониторинг)
- **:mutate [limit]** — Мутационное тестирование (детерминированно)
- **:autogen-tests [n]** — Автогенерация тестов для нетестированных функций
- **:computer <задача>** — Управление компьютером (требует GOGITOR_COMPUTER_ENABLED=true)

---

## Agent
- **:agent <задача>** — Запустить полноценный Agent Harness.
- **:agent deep <задача>** — Запустить усиленный профиль Agent:
    - изоляция подзадач
    - quality gates
    - артефакты сессии
    - финальная проверка
- **:agent interview <задача>** — Сначала задать уточняющие вопросы.
- **:agent reflect** — Проанализировать последнюю сессию Agent и извлечь уроки.
- **:agent report** — Показать отчёт последней Agent-сессии
- **:agent resume** — Продолжить последнюю неуспешную Agent-сессию
- **:agent undo** — Отменить последний Git-коммит Agent

---

## Git и GitHub
- **:git status** — Статус рабочей директории
- **:git diff** — Разница (working dir vs HEAD)
- **:git diff-task** — Накопительный diff последней задачи
- **:git commit** — Закоммитить все изменения
- **:git commit --split <ф1,ф2>** — Раздельные коммиты для указанных файлов
- **:git init** — Инициализировать репозиторий
- **:git log** — История коммитов
- **:git checkout <h>** — Переключиться на коммит или ветку
- **:git checkout -b <n>** — Создать и переключиться на новую ветку
- **:git branch** — Список / создание / удаление веток
- **:git merge <n>** — Слить ветку
- **:git revert [h]** — Отменить коммит (безопасно, создаёт новый)
- **:git reset [--hard] <h>** — Откатить к коммиту (--hard удаляет изменения)
- **:git push [ветка]** — Отправить в remote
- **:git pull [ветка]** — Получить из remote
- **:git fetch** — Загрузить из remote
- **:git clone <url>** — Клонировать репозиторий
- **:git remote** — Список / добавление / удаление remote
- **:git create <name> [--private] [--desc <text>]** — Создать репозиторий на GitHub
- **:git pr** — Создать Pull Request
- **:git issue** — Создать Issue из падающих тестов
- **:git changelog** — Сгенерировать CHANGELOG.md
- **:git pr-comment <n> [текст]** — Добавить комментарий к PR

### Детали Git
**:git commit**
Коммитит все изменения. Устанавливает user.name=Gogitor, если не задано.

**:git commit --split <ф1,ф2>**
Создаёт отдельные коммиты для указанных файлов с LLM-генерацией сообщений. Остальные изменения попадают в общий коммит.

**:git checkout <h>**
Переключение на коммит или ветку. При переходе на коммит возникает detached HEAD.

**:git revert [h]**
Безопасно отменяет коммит, создавая новый.

**:git reset [--hard] <h>**
Переписывает историю. --hard безвозвратно удаляет незакоммиченные изменения.

**:git pr**
Создаёт PR из текущей ветки в default branch. Требует токен GitHub.

**:git issue**
Запускает тесты и создаёт Issue на GitHub, если есть упавшие.

**:git changelog**
Парсит Conventional Commits и генерирует CHANGELOG.md.

---

## Клавиши TUI
- **F2** — Режим выделения текста мышью
- **Tab** — Переключение фокуса ввод/вывод
- **Alt+Enter** — Новая строка в поле ввода
- **Ctrl+C** — Отмена задачи / выход
`
}

func hasPatches(changes []domain.FileChange) bool {
	for _, ch := range changes {
		if len(ch.Patches) > 0 {
			return true
		}
	}

	return false
}

func (s *Service) resolveIntent(ctx context.Context, query string, emit func(domain.Event), raw bool) domain.Intent {
	ctx = agent.WithRole(ctx, agent.RoleRouter)
	ctx = agent.WithPriority(ctx, agent.PriorityHigh)
	ctx = agent.WithPurpose(ctx, "detect user intent")

	if !s.Cfg.ReasoningRouter {
		ctx = llm.WithReasoningDisabled(ctx)
	}

	fallback := domain.Intent{
		Mode:   "chat",
		Task:   query,
		Reason: "fallback to chat",
	}

	query = strings.TrimSpace(query)
	if query == "" {
		return fallback
	}

	prompt := prompts.Intent(
		s.intentHistoryString(),
		query,
		s.projectSummary(),
	)

	response, err := s.LLM.Send(ctx, prompt)
	if err != nil {
		if ctx.Err() == nil {
			sendEvent(emit, domain.EventWarn, fmt.Sprintf("Intent detection failed: %v", err))
		}
		return fallback
	}

	intent, err := parseIntentResponse(response)
	if err != nil {
		sendEvent(emit, domain.EventWarn, fmt.Sprintf("Cannot parse intent response: %v", err))
		return fallback
	}

	intent.Mode = normalizeIntentMode(intent.Mode)
	intent.Reason = strings.TrimSpace(intent.Reason)

	if raw {
		intent.Task = query
	} else {
		intent.Task = strings.TrimSpace(intent.Task)
		if intent.Task == "" {
			intent.Task = query
		}
	}

	if (intent.Mode == "analyze" || intent.Mode == "chat") && s.looksLikeFileCodeTask(query) {
		intent.Mode = "code"
		if strings.TrimSpace(intent.Reason) == "" {
			intent.Reason = "task creates or modifies files"
		}
	}
	return intent
}

func parseIntentResponse(response string) (domain.Intent, error) {
	cleaned := codegen.CleanCode(response)

	// Извлекаем все валидные JSON-объекты из ответа
	candidates := extractAllJSONCandidates(cleaned)
	if len(candidates) == 0 {
		return domain.Intent{}, fmt.Errorf("no JSON object found")
	}

	// Стратегия 1: ищем объект с валидным полем mode.
	validModes := map[string]bool{
		"code": true, "analyze": true, "search": true,
		"run": true, "test": true, "git": true, "chat": true,
		"coding": true, "edit": true, "modify": true, "create": true,
		"fix": true, "refactor": true, "rewrite": true, "split": true,
		"analysis": true, "review": true, "explain": true,
		"web": true, "internet": true, "execute": true, "tests": true,
	}

	for _, candidate := range candidates {
		var intent domain.Intent
		if err := json.Unmarshal(candidate, &intent); err != nil {
			continue
		}
		modeLower := strings.ToLower(strings.TrimSpace(intent.Mode))
		if validModes[modeLower] {
			return intent, nil
		}
	}

	// Стратегия 2: если ни один кандидат не дал валидный mode.
	var intent domain.Intent
	if err := json.Unmarshal(candidates[0], &intent); err != nil {
		return domain.Intent{}, fmt.Errorf("invalid JSON: %w", err)
	}
	return intent, nil
}

func normalizeIntentMode(mode string) string {
	m := strings.ToLower(strings.TrimSpace(mode))

	switch m {
	case "code", "coding", "edit", "modify", "create", "refactor", "rewrite", "split":
		return "code"
	case "article", "write_article", "blog", "post", "статья", "заметка":
		return "article"
	case "fix":
		return "fix"

	case "analyze", "analysis", "review", "explain":
		return "analyze"

	case "search", "web", "internet":
		return "search"

	case "run", "execute":
		return "run"

	case "test", "tests":
		return "test"

	case "git":
		return "git"
	case "computer", "shell", "os", "terminal", "bash":
		return "computer"
	default:
		return "chat"
	}
}

func (s *Service) intentHistoryString() string {
	if len(s.history) == 0 {
		return ""
	}
	start := len(s.history) - 3
	if start < 0 {
		start = 0
	}
	const maxQueryRunes = 6000
	const maxAnswerRunes = 16000

	var b strings.Builder
	for _, h := range s.history[start:] {
		b.WriteString("USER: ")
		b.WriteString(truncate(h.Query, maxQueryRunes))
		b.WriteString("\nASSISTANT: ")
		b.WriteString(truncate(h.Answer, maxAnswerRunes))
		b.WriteString("\n")
	}
	return b.String()
}

func (s *Service) projectSummary() string {
	files := s.WS.GoFiles(10)
	var b strings.Builder
	fmt.Fprintf(&b, "project: %s\n", filepath.Base(s.Cfg.WorkDir))
	fmt.Fprintf(&b, "has_go_files: %v\n", len(files) > 0)
	if len(files) > 0 {
		b.WriteString("go_files: ")
		b.WriteString(strings.Join(files, ", "))
	}

	if idx := s.WS.ExistingIndex(); idx != nil && idx.Ready() {
		fmt.Fprintf(&b, "\nindexed_files: %d", idx.FileCount())
	}

	return strings.TrimSpace(b.String())
}

func effectiveTask(original, refined string, raw bool) string {
	if raw {
		return original
	}
	refined = strings.TrimSpace(refined)
	if refined == "" {
		return original
	}
	return refined
}

func firstTargetFile(candidates ...string) string {
	for _, candidate := range candidates {
		files := extractTargetFiles(candidate)
		if len(files) > 0 {
			return files[0]
		}
	}
	return ""
}

func normalizeGitSubcommand(s string) string {
	lower := strings.ToLower(strings.TrimSpace(s))
	if lower == "" {
		return ""
	}
	for _, word := range strings.Fields(lower) {
		switch word {
		case "status", "diff", "commit", "init", "log", "checkout", "branch", "merge",
			"push", "pull", "fetch", "clone", "remote", "revert", "reset":
			return word
		}
	}
	switch {
	case strings.Contains(lower, "revert"),
		strings.Contains(lower, "отмени коммит"),
		strings.Contains(lower, "откат коммита"):
		return "revert"
	case strings.Contains(lower, "reset"),
		strings.Contains(lower, "сброс"),
		strings.Contains(lower, "откат ветки"):
		return "reset"
	case strings.Contains(lower, "push"),
		strings.Contains(lower, "запуш"),
		strings.Contains(lower, "отправь"),
		strings.Contains(lower, "залей"):
		return "push"
	case strings.Contains(lower, "pull"),
		strings.Contains(lower, "стяни"),
		strings.Contains(lower, "подтяни"),
		strings.Contains(lower, "скачай"):
		return "pull"
	case strings.Contains(lower, "fetch"):
		return "fetch"
	case strings.Contains(lower, "clone"),
		strings.Contains(lower, "клонируй"),
		strings.Contains(lower, "склонируй"):
		return "clone"
	case strings.Contains(lower, "create"),
		strings.Contains(lower, "создай"),
		strings.Contains(lower, "создать"):
		return "create"
	case strings.Contains(lower, "remote"):
		return "remote"
	case strings.Contains(lower, "log"),
		strings.Contains(lower, "истори"),
		strings.Contains(lower, "коммиты"),
		strings.Contains(lower, "журнал"):
		return "log"
	case strings.Contains(lower, "checkout"),
		strings.Contains(lower, "вернуться"),
		strings.Contains(lower, "откат"),
		strings.Contains(lower, "переключ"):
		return "checkout"
	case strings.Contains(lower, "merge"),
		strings.Contains(lower, "слить"),
		strings.Contains(lower, "слияние"),
		strings.Contains(lower, "объедини"):
		return "merge"
	case strings.Contains(lower, "branch"),
		strings.Contains(lower, "ветк"),
		strings.Contains(lower, "ветвь"):
		return "branch"
	case strings.Contains(lower, "commit"),
		strings.Contains(lower, "коммит"),
		strings.Contains(lower, "закоммить"):
		return "commit"
	case strings.Contains(lower, "diff"),
		strings.Contains(lower, "разница"),
		strings.Contains(lower, "изменен"):
		return "diff"
	case strings.Contains(lower, "init"),
		strings.Contains(lower, "инициализ"):
		return "init"
	case strings.Contains(lower, "status"),
		strings.Contains(lower, "статус"):
		return "status"
	}
	return ""
}

func formatTestFeedback(tests domain.TestsStatus, err error) string {
	var b strings.Builder

	if err != nil {
		b.WriteString(trim(err.Error(), 2000))
		b.WriteByte('\n')
	}

	if fb := strings.TrimSpace(runner.FormatFeedback(tests)); fb != "" {
		b.WriteString(fb)
	}

	return strings.TrimSpace(b.String())
}

func coverageSuffix(tests domain.TestsStatus) string {
	if strings.TrimSpace(tests.CoverageOutput) != "" {
		return fmt.Sprintf(" (%s)", strings.TrimSpace(tests.CoverageOutput))
	}

	if tests.Coverage > 0 {
		return fmt.Sprintf(" (coverage: %.1f%%)", tests.Coverage)
	}

	return ""
}

func (s *Service) collectOutputFiles(sandbox string, changes []domain.FileChange) []domain.OutputFile {
	var out []domain.OutputFile
	seen := make(map[string]bool)

	for _, ch := range changes {
		if seen[ch.Path] {
			continue
		}
		seen[ch.Path] = true

		full, err := security.SafeJoin(sandbox, ch.Path)
		if err != nil {
			continue
		}

		data, err := os.ReadFile(full)
		if err != nil {
			continue
		}

		out = append(out, domain.OutputFile{
			Path:    ch.Path,
			Content: string(data),
		})
	}

	return out
}

func mergeOutputFiles(base, add []domain.OutputFile) []domain.OutputFile {
	idx := make(map[string]int, len(base))

	for i, f := range base {
		idx[f.Path] = i
	}

	for _, f := range add {
		if i, ok := idx[f.Path]; ok {
			base[i] = f
		} else {
			idx[f.Path] = len(base)
			base = append(base, f)
		}
	}

	return base
}

// ─── GitHub / remote git-операции ────────────────────────────────────

func (s *Service) GitPush(ctx context.Context, branch string, emit func(domain.Event)) domain.Result {
	result := domain.Result{Mode: "git"}
	if err := s.Git.EnsureRepo(ctx, s.Cfg.GitAutoInit); err != nil {
		result.AddError(sanitizeTokenFromError(err.Error(), s.Cfg.GitHubToken))
		return result
	}
	// Если указан GitHub URL в конфиге, убеждаемся что remote origin настроен.
	if s.Cfg.GitHubURL != "" {
		if err := s.Git.EnsureRemote(ctx, "origin", s.Cfg.GitHubURL); err != nil {
			result.AddError(fmt.Sprintf("cannot set remote origin: %v", err))
			return result
		}
	}
	// Проверяем, что remote существует.
	remotes, _ := s.Git.RemoteList(ctx)
	if strings.TrimSpace(remotes) == "" {
		result.AddError("no remote configured. Use ':git remote add <url>' or --github <url>")
		return result
	}
	sendEvent(emit, domain.EventLog, "Pushing to remote...")
	if strings.TrimSpace(s.Cfg.GitHubToken) != "" {
		tokenType := github.TokenType(s.Cfg.GitHubToken)

		sendEvent(
			emit,
			domain.EventLog,
			fmt.Sprintf("GitHub authentication: token loaded (%s), using HTTPS for git operations", tokenType),
		)
	} else {
		sendEvent(
			emit,
			domain.EventWarn,
			"GitHub authentication: token is empty",
		)
	}
	out, err := s.Git.WithAuthenticatedRemote(ctx, "origin", s.Cfg.GitHubToken, func() (string, error) {
		return s.Git.Push(ctx, "origin", branch, false)
	})

	if err != nil {
		result.AddError(sanitizeTokenFromError(err.Error(), s.Cfg.GitHubToken))
		return result
	}
	result.Success = true
	result.Response = "Push successful.\n" + strings.TrimSpace(out)
	return result
}

func (s *Service) GitPull(ctx context.Context, branch string, emit func(domain.Event)) domain.Result {
	result := domain.Result{Mode: "git"}
	if err := s.Git.EnsureRepo(ctx, s.Cfg.GitAutoInit); err != nil {
		result.AddError(sanitizeTokenFromError(err.Error(), s.Cfg.GitHubToken))
		return result
	}
	if s.Cfg.GitHubURL != "" {
		if err := s.Git.EnsureRemote(ctx, "origin", s.Cfg.GitHubURL); err != nil {
			result.AddError(fmt.Sprintf("cannot set remote origin: %v", err))
			return result
		}
	}
	remotes, _ := s.Git.RemoteList(ctx)
	if strings.TrimSpace(remotes) == "" {
		result.AddError("no remote configured. Use ':git remote add <url>' or --github <url>")
		return result
	}
	sendEvent(emit, domain.EventLog, "Pulling from remote...")

	out, err := s.Git.WithAuthenticatedRemote(ctx, "origin", s.Cfg.GitHubToken, func() (string, error) {
		return s.Git.Pull(ctx, "origin", branch)
	})
	if err != nil {
		result.AddError(sanitizeTokenFromError(err.Error(), s.Cfg.GitHubToken))
		return result
	}
	s.WS.RefreshIndex()
	result.Success = true
	result.Response = "Pull successful.\n" + strings.TrimSpace(out)
	return result
}

func (s *Service) GitFetch(ctx context.Context, emit func(domain.Event)) domain.Result {
	result := domain.Result{Mode: "git"}
	if !s.Git.IsRepo(ctx) {
		result.Success = true
		result.Response = "Not a git repository."
		return result
	}
	sendEvent(emit, domain.EventLog, "Fetching from remote...")

	out, err := s.Git.WithAuthenticatedRemote(ctx, "origin", s.Cfg.GitHubToken, func() (string, error) {
		return s.Git.Fetch(ctx, "origin")
	})
	if err != nil {
		result.AddError(sanitizeTokenFromError(err.Error(), s.Cfg.GitHubToken))
		return result
	}
	result.Success = true
	if strings.TrimSpace(out) == "" {
		out = "Already up to date."
	}
	result.Response = "Fetch complete.\n" + strings.TrimSpace(out)
	return result
}

func (s *Service) GitClone(ctx context.Context, repoURL string, emit func(domain.Event)) domain.Result {
	result := domain.Result{Mode: "git"}
	if repoURL == "" {
		repoURL = s.Cfg.GitHubURL
	}
	if repoURL == "" {
		result.AddError("usage: :git clone <url> or set --github <url>")
		return result
	}

	_, repoName, err := github.ParseRepoURL(repoURL)
	if err != nil {
		result.AddError(fmt.Sprintf("cannot parse repo URL: %v", err))
		return result
	}

	targetDir := filepath.Join(s.Cfg.WorkDir, repoName)
	if _, err := os.Stat(targetDir); err == nil {
		result.AddError(fmt.Sprintf("directory %s already exists", targetDir))
		return result
	}

	sendEvent(emit, domain.EventLog, fmt.Sprintf("Cloning %s ...", repoURL))
	if s.Cfg.GitHubToken != "" {
		tokenType := github.TokenType(s.Cfg.GitHubToken)
		sendEvent(emit, domain.EventLog, fmt.Sprintf("Using GitHub token (%s)", tokenType))
	} else {
		sendEvent(emit, domain.EventWarn, "No GitHub token configured. Private repos will fail.")
	}

	out, err := s.Git.WithCloneAuth(ctx, repoURL, s.Cfg.GitHubToken, func() (string, error) {
		return s.Git.Clone(ctx, repoURL, targetDir)
	})

	if err != nil {
		// Не показываем токен в ошибке
		safeErr := sanitizeTokenFromError(err.Error(), s.Cfg.GitHubToken)
		result.AddError(safeErr)
		return result
	}

	// Убираем токен из remote URL в склонированном репо.
	if s.Cfg.GitHubToken != "" {
		cloneGit := git.New(targetDir, s.Log)
		_, _ = cloneGit.RemoteSetURL(ctx, "origin", repoURL)
	}

	s.switchWorkDir(targetDir)
	sendEvent(emit, domain.EventLog, fmt.Sprintf("Switched working directory to %s", targetDir))

	result.Success = true
	result.Response = fmt.Sprintf("Cloned into %s\n%s", targetDir, strings.TrimSpace(out))
	return result
}

func (s *Service) switchWorkDir(newDir string) {
	s.Cfg.WorkDir = newDir
	s.Git = git.New(newDir, s.Log)

	if s.WS != nil {
		_ = s.WS.Close()
	}

	s.WS = workspace.New(newDir)
	s.Stats = loadLLMStats(newDir)
}

// sanitizeTokenFromError убирает токен из текста ошибки, чтобы не показать его в логах.
func sanitizeTokenFromError(msg, token string) string {
	if token == "" {
		return msg
	}
	return strings.ReplaceAll(msg, token, "***TOKEN***")
}

func (s *Service) GitRemote(ctx context.Context, args []string, emit func(domain.Event)) domain.Result {
	result := domain.Result{Mode: "git"}
	if !s.Git.IsRepo(ctx) {
		result.Success = true
		result.Response = "Not a git repository."
		return result
	}
	// :git remote — список
	if len(args) == 0 {
		out, err := s.Git.RemoteList(ctx)
		if err != nil {
			result.AddError(err.Error())
			return result
		}
		if strings.TrimSpace(out) == "" {
			out = "No remotes configured."
		}
		result.Success = true
		result.Response = strings.TrimSpace(out)
		return result
	}
	switch strings.ToLower(args[0]) {
	case "add":
		if len(args) < 3 {
			result.AddError("usage: :git remote add <name> <url>")
			return result
		}
		name, url := args[1], args[2]
		out, err := s.Git.RemoteAdd(ctx, name, url)
		if err != nil {
			result.AddError(err.Error())
			return result
		}
		result.Success = true
		result.Response = fmt.Sprintf("Remote '%s' added: %s\n%s", name, url, strings.TrimSpace(out))
		return result
	case "remove", "rm":
		if len(args) < 2 {
			result.AddError("usage: :git remote remove <name>")
			return result
		}
		name := args[1]
		out, err := s.Git.RemoteRemove(ctx, name)
		if err != nil {
			result.AddError(err.Error())
			return result
		}
		result.Success = true
		result.Response = fmt.Sprintf("Remote '%s' removed.\n%s", name, strings.TrimSpace(out))
		return result
	case "set-url":
		if len(args) < 3 {
			result.AddError("usage: :git remote set-url <name> <url>")
			return result
		}
		name, url := args[1], args[2]
		out, err := s.Git.RemoteSetURL(ctx, name, url)
		if err != nil {
			result.AddError(err.Error())
			return result
		}
		result.Success = true
		result.Response = fmt.Sprintf("Remote '%s' URL set to %s\n%s", name, url, strings.TrimSpace(out))
		return result
	default:
		result.AddError("usage: :git remote [add <name> <url> | remove <name> | set-url <name> <url>]")
		return result
	}
}

func (s *Service) GitCreate(ctx context.Context, args []string, emit func(domain.Event)) domain.Result {
	result := domain.Result{Mode: "git"}
	if s.Cfg.GitHubToken == "" {
		result.AddError("GitHub token is required. Use --key-github <token>")
		return result
	}
	if _, err := s.GitHub.ValidateToken(ctx); err != nil {
		result.AddError(fmt.Sprintf("GitHub token validation failed: %v", err))
		return result
	}

	// Парсим аргументы: :git create <name> [--private] [--desc <text>]
	name := ""
	private := false
	description := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--private", "-p":
			private = true
		case "--desc", "--description", "-d":
			if i+1 < len(args) {
				i++
				// Собираем ВСЕ следующие аргументы, пока не встретим новый флаг (--...)
				var descParts []string
				for i < len(args) && !strings.HasPrefix(args[i], "--") {
					descParts = append(descParts, args[i])
					i++
				}
				i-- // компенсируем i++ внешнего цикла
				description = strings.Join(descParts, " ")
			}
		default:
			if name == "" {
				name = args[i]
			}
		}
	}

	if name == "" {
		result.AddError("usage: :git create <repo-name> [--private] [--desc <description>]")
		return result
	}

	sendEvent(emit, domain.EventLog, fmt.Sprintf("Creating repository %q on GitHub...", name))
	repo, err := s.GitHub.CreateRepo(ctx, name, private, description)
	if err != nil {
		result.AddError(err.Error())
		return result
	}

	visibility := "public"
	if repo.Private {
		visibility = "private"
	}
	sendEvent(emit, domain.EventLog, fmt.Sprintf("Repository created: %s (%s)", repo.FullName, visibility))

	// Настраиваем remote origin в текущем проекте.
	if s.Git.IsRepo(ctx) {
		_ = s.Git.EnsureRemote(ctx, "origin", repo.CloneURL)
		sendEvent(emit, domain.EventLog, "Remote 'origin' set to "+repo.CloneURL)
	}

	result.Success = true
	result.Response = fmt.Sprintf(
		"Repository created: https://github.com/%s (%s)\nClone URL: %s",
		repo.FullName, visibility, repo.CloneURL,
	)
	return result
}

func dispatcherConfig(cfg *config.Config) agent.Config {
	timeout := time.Duration(cfg.LLMTimeout) * time.Second
	if timeout <= 0 {
		timeout = 3000 * time.Second
	}

	// Масштабируем бюджет от размера контекста модели
	sessionTokens := 2_000_000
	sessionDuration := 45 * time.Minute
	coderTokens := 1_500_000
	reviewerTokens := 300_000

	ctxTokens := cfg.EffectiveContextTokens()
	if ctxTokens > 65536 {
		sessionTokens = 8_000_000
		sessionDuration = 90 * time.Minute
		coderTokens = 6_000_000
		reviewerTokens = 1_000_000
	}
	if ctxTokens > 131072 {
		sessionTokens = 20_000_000
		sessionDuration = 120 * time.Minute
		coderTokens = 15_000_000
		reviewerTokens = 2_000_000
	}

	coderRequests := cfg.MaxIterations * 8
	if coderRequests < 24 {
		coderRequests = 24
	}

	return agent.Config{
		DefaultTimeout:     timeout,
		MaxSessionRequests: 120,
		MaxSessionTokens:   sessionTokens,
		MaxSessionDuration: sessionDuration,
		MaxQueue:           128,
		AgingPerSecond:     5,

		// ─── Retry ───────────────────────────────────────────
		MaxRetries:      2,               // 2 повторных попытки
		RetryBaseDelay:  1 * time.Second, // первая задержка 1s
		RetryMaxDelay:   8 * time.Second, // потолок 8s
		RetryMultiplier: 2.0,             // 1s → 2s → 4s

		RoleQuotas: map[agent.Role]agent.RoleQuota{
			agent.RoleRouter: {
				MaxRequests:   30,
				MaxTokens:     150_000,
				MaxDuration:   3 * time.Minute,
				PriorityBoost: agent.PriorityHigh,
			},
			agent.RolePlanner: {
				MaxRequests:   50,
				MaxTokens:     2_000_000,
				MaxDuration:   30 * time.Minute,
				PriorityBoost: agent.PriorityHigh,
			},
			agent.RoleCoder: {
				MaxRequests:   coderRequests,
				MaxTokens:     coderTokens,
				MaxDuration:   sessionDuration - 5*time.Minute,
				PriorityBoost: 0,
			},
			agent.RoleReviewer: {
				MaxRequests:   20,
				MaxTokens:     reviewerTokens,
				MaxDuration:   10 * time.Minute,
				PriorityBoost: agent.PriorityHigh,
			},
			agent.RoleTester: {
				MaxRequests:   20,
				MaxTokens:     reviewerTokens,
				MaxDuration:   10 * time.Minute,
				PriorityBoost: agent.PriorityNormal,
			},
			agent.RoleVerifier: {
				MaxRequests:   50,
				MaxTokens:     reviewerTokens,
				MaxDuration:   30 * time.Minute,
				PriorityBoost: agent.PriorityCritical,
			},
			agent.RoleSecurity: {
				MaxRequests:   10,
				MaxTokens:     500_000,
				MaxDuration:   5 * time.Minute,
				PriorityBoost: agent.PriorityHigh,
			},
			agent.RoleSearcher: {
				MaxRequests:   5,
				MaxTokens:     100_000,
				MaxDuration:   2 * time.Minute,
				PriorityBoost: agent.PriorityLow,
			},
			agent.RoleDocs: {
				MaxRequests:   10,
				MaxTokens:     300_000,
				MaxDuration:   5 * time.Minute,
				PriorityBoost: agent.PriorityLow,
			},
		},
		ReasoningEnabled: cfg.ReasoningEnabled,
	}
}

func (s *Service) emitDispatcherUsage(emit func(domain.Event)) {
	if s.Agents == nil {
		return
	}

	session, _ := s.Agents.Snapshot()

	sendEvent(
		emit,
		domain.EventLog,
		i18n.T(
			"LLM dispatcher usage: requests=%d estimated_tokens=%d duration=%s queue=%d",
			session.Requests,
			session.EstimatedTokens,
			session.Duration.Round(time.Millisecond),
			s.Agents.QueueLen(),
		),
	)
}

func (s *Service) reviewChanges(
	ctx context.Context,
	originalTask string,
	res domain.Result,
	emit func(domain.Event),
) {
	if len(res.FilesCreated) == 0 && len(res.FilesModified) == 0 {
		return
	}

	ctx = agent.WithRole(ctx, agent.RoleReviewer)
	ctx = agent.WithPriority(ctx, agent.PriorityHigh)
	ctx = agent.WithPurpose(ctx, "review generated changes")

	var b strings.Builder

	b.WriteString("You are a strict senior Go code reviewer.\n")
	b.WriteString("Review the following change summary and point out risks.\n\n")

	b.WriteString("ORIGINAL TASK:\n")
	b.WriteString(originalTask)
	b.WriteString("\n\n")

	if len(res.FilesCreated) > 0 {
		b.WriteString("CREATED FILES:\n")
		for _, f := range res.FilesCreated {
			b.WriteString("- " + f + "\n")
		}
		b.WriteString("\n")
	}

	if len(res.FilesModified) > 0 {
		b.WriteString("MODIFIED FILES:\n")
		for _, f := range res.FilesModified {
			b.WriteString("- " + f + "\n")
		}
		b.WriteString("\n")
	}

	if len(res.FilesPatched) > 0 {
		b.WriteString("PATCHED FILES:\n")
		for _, f := range res.FilesPatched {
			b.WriteString("- " + f + "\n")
		}
		b.WriteString("\n")
	}

	if len(res.FilesFullRewritten) > 0 {
		b.WriteString("FULLY REWRITTEN FILES:\n")
		for _, f := range res.FilesFullRewritten {
			b.WriteString("- " + f + "\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(`RULES:
1. Be concise.
2. Focus on regressions, missing edge cases, security risks, and architecture problems.
3. Do not rewrite code unless necessary.
4. Return markdown.
`)

	sendEvent(emit, domain.EventLog, "Reviewer agent: analyzing changes")

	response, err := s.LLM.Send(ctx, b.String())
	if err != nil {
		sendEvent(emit, domain.EventWarn, fmt.Sprintf("Reviewer agent failed: %v", err))
		return
	}

	sendEvent(emit, domain.EventLog, "Reviewer agent finished")
	sendEvent(emit, domain.EventWarn, "Reviewer:\n"+response)
}

func (s *Service) looksLikeFileCodeTask(query string) bool {
	lower := strings.ToLower(query)

	fileHint := strings.Contains(lower, ".sh") ||
		strings.Contains(lower, ".go") ||
		strings.Contains(lower, ".html") ||
		strings.Contains(lower, ".htm") ||
		strings.Contains(lower, "файл") ||
		strings.Contains(lower, "file") ||
		strings.Contains(lower, "скрипт") ||
		strings.Contains(lower, "script")

	if !fileHint {
		return false
	}

	codeHints := []string{
		"создай",
		"создать",
		"напиши",
		"писать",
		"сгенерируй",
		"сгенерировать",
		"реализуй",
		"реализовать",
		"добавь",
		"добавить",
		"измени",
		"изменить",
		"исправь",
		"исправить",
		"обнови",
		"обновить",
		"улучши",
		"улучшить",
		"модифицир",
		"модификац",
		"модифик",
		"create",
		"write",
		"generate",
		"implement",
		"add",
		"modify",
		"fix",
		"update",
		"improve",
	}

	return containsAny(lower, codeHints)
}

func (s *Service) RunLint(ctx context.Context, emit func(domain.Event)) domain.Result {
	result := domain.Result{Mode: "test"}
	emitEvent(emit, domain.Event{
		Type:      domain.EventLog,
		Message:   i18n.Localize("Running golangci-lint"),
		TaskStage: domain.TaskStageLint,
	})

	_ = s.Runner.EnsureLintConfig(ctx, s.Cfg.WorkDir)
	sendEvent(emit, domain.EventLog, "Preparing sandbox")
	sandbox, err := s.WS.PrepareSandbox(ctx)
	if err != nil {
		result.AddError(err.Error())
		return result
	}
	defer os.RemoveAll(sandbox)

	s.Runner.DepsLog = func(msg string) {
		sendEvent(emit, domain.EventLog, msg)
	}

	sendEvent(emit, domain.EventLog, "Running go build")
	if err := s.Runner.Build(ctx, sandbox); err != nil {
		result.AddError(err.Error())
		return result
	}

	sendEvent(emit, domain.EventLog, "Running golangci-lint")
	lintOutput, lintErr := s.Runner.Lint(ctx, sandbox)

	result.Lint.Run = true
	result.Lint.Output = trim(lintOutput, 20000)

	// golangci-lint не установлен
	if lintErr != nil && strings.Contains(lintErr.Error(), "golangci-lint is not installed") {
		result.AddError(lintErr.Error())
		return result
	}

	// Считаем РЕАЛЬНОЕ количество проблем из вывода
	issues := runner.CountLintIssues(lintOutput)
	result.Lint.Issues = issues

	// Если проблем нет — успех, НЕ отправляем в LLM
	if issues == 0 {
		result.Lint.Passed = true
		result.Success = true
		result.Response = "golangci-lint: no issues found."
		return result
	}

	// Есть проблемы — отправляем в LLM для исправления
	sendEvent(emit, domain.EventWarn,
		fmt.Sprintf("golangci-lint found %d issue(s), sending to LLM for fixing", issues))

	fixTask := buildLintFixTask(lintOutput)
	fixResult := s.ExecuteCode(ctx, fixTask, Options{}, emit)

	result.Success = fixResult.Success
	result.FilesCreated = fixResult.FilesCreated
	result.FilesModified = fixResult.FilesModified
	result.FilesPatched = fixResult.FilesPatched
	result.FilesFullRewritten = fixResult.FilesFullRewritten
	result.OutputFiles = fixResult.OutputFiles
	result.Tests = fixResult.Tests
	result.GitCommit = fixResult.GitCommit
	result.Iterations = fixResult.Iterations
	result.Errors = fixResult.Errors
	result.Warnings = fixResult.Warnings

	if fixResult.Success {
		result.Response = fmt.Sprintf(
			"golangci-lint issues fixed (%d issue(s) were found). %s",
			issues, fixResult.Response,
		)
	} else {
		result.Response = fmt.Sprintf(
			"golangci-lint found %d issue(s), but automatic fix failed.",
			issues,
		)
	}

	return result
}

func buildLintFixTask(lintOutput string) string {
	var b strings.Builder
	b.WriteString("Fix ALL golangci-lint issues listed below. ")
	b.WriteString("Do NOT change program behavior, do NOT add features. ")
	b.WriteString("Only fix the reported lint issues.\n\n")
	b.WriteString("GOLANGCI-LINT OUTPUT:\n")
	b.WriteString(trim(lintOutput, 12000))
	b.WriteString("\n\nRULES:\n")
	b.WriteString("1. Fix every reported issue.\n")
	b.WriteString("2. Preserve existing behavior.\n")
	b.WriteString("3. Do not add new dependencies unless required by the fix.\n")
	b.WriteString("4. The result must compile with go build and pass go test.\n")
	return b.String()
}

// generateCommitMessageForFile генерирует commit-сообщение для конкретного файла.
func (s *Service) generateCommitMessageForFile(
	ctx context.Context,
	file string,
	emit func(domain.Event),
) string {
	// Помечаем новые файлы как intent-to-add, чтобы они появились в diff.
	_ = s.Git.AddIntentToAll(ctx)
	defer func() {
		_ = s.Git.ResetAll(ctx)
	}()

	// Diff конкретного файла.
	diff, err := s.Git.DiffFile(ctx, file)
	if err != nil {
		diff = ""
	}

	// Если diff пуст (новый untracked файл), читаем содержимое.
	if strings.TrimSpace(diff) == "" {
		fullPath := filepath.Join(s.Cfg.WorkDir, file)
		if data, readErr := os.ReadFile(fullPath); readErr == nil {
			diff = fmt.Sprintf("new file: %s\n%s", file, string(data))
			if len(diff) > 6000 {
				diff = textutil.TruncateStringBytes(diff, 6000) + "\n... (truncated)"
			}
		}
	}

	if strings.TrimSpace(diff) == "" {
		return "chore: update " + file
	}

	// Ограничиваем размер.
	const maxDiffLen = 8000
	if len(diff) > maxDiffLen {
		diff = textutil.TruncateStringBytes(diff, maxDiffLen) + "\n... (diff truncated)"
	}

	// Используем тот же промпт CommitMessage, но с diff одного файла.
	fileStatus := file // git status --short для одного файла не нужен, передаём имя
	prompt := prompts.CommitMessage(diff, fileStatus, "")

	msgCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	msgCtx = agent.WithRole(msgCtx, agent.RoleCoder)
	msgCtx = agent.WithPriority(msgCtx, agent.PriorityNormal)
	msgCtx = agent.WithPurpose(msgCtx, "generate commit message for "+file)
	msgCtx = llm.WithReasoningDisabled(msgCtx)
	msg, err := s.LLM.Send(msgCtx, prompt)
	if err != nil {
		s.Log.Warn("commit message generation failed for file", "file", file, "err", err)
		return "chore: update " + file
	}

	msg = cleanCommitMessage(msg)
	if msg == "" {
		return "chore: update " + file
	}
	return msg
}

// GitCommitSplit выполняет раздельные коммиты для указанных файлов
// и общий коммит для всех остальных изменений.
func (s *Service) GitCommitSplit(
	ctx context.Context,
	targetFiles []string,
	emit func(domain.Event),
) domain.Result {
	result := domain.Result{Mode: "git"}

	if err := s.Git.EnsureRepo(ctx, s.Cfg.GitAutoInit); err != nil {
		result.AddError(err.Error())
		return result
	}
	// Если --split указан без файлов — показываем подсказку.
	if len(targetFiles) == 0 {
		allFiles, err := s.Git.StatusFiles(ctx)
		if err != nil {
			result.AddError(err.Error())
			return result
		}
		result.Success = true
		result.Response = "Specify files to split. Changed files:\n" +
			strings.Join(allFiles, "\n") +
			"\nUsage: :git commit --split file1.go,file2.go"
		return result
	}

	// Проверяем, что есть изменения.
	status, err := s.Git.Status(ctx)
	if err != nil {
		result.AddError(err.Error())
		return result
	}
	if strings.TrimSpace(status) == "" {
		result.Success = true
		result.Response = "Nothing to commit."
		return result
	}

	// Получаем список всех изменённых файлов.
	allFiles, err := s.Git.StatusFiles(ctx)
	if err != nil {
		result.AddError(err.Error())
		return result
	}

	// Разделяем: целевые файлы и остальные.
	targetSet := make(map[string]bool, len(targetFiles))
	for _, f := range targetFiles {
		targetSet[strings.TrimSpace(f)] = true
	}

	var splitFiles []string     // файлы для отдельных коммитов
	var remainingFiles []string // файлы для общего коммита
	for _, f := range allFiles {
		if targetSet[f] {
			splitFiles = append(splitFiles, f)
		} else {
			remainingFiles = append(remainingFiles, f)
		}
	}

	// Предупреждение, если указанный файл не найден среди изменений.
	for _, tf := range targetFiles {
		tf = strings.TrimSpace(tf)
		found := false
		for _, f := range allFiles {
			if f == tf {
				found = true
				break
			}
		}
		if !found {
			result.AddWarning(fmt.Sprintf("file '%s' has no changes, skipping", tf))
		}
	}

	if len(splitFiles) == 0 {
		result.AddError("none of the specified files have changes")
		return result
	}

	var commits []string
	var lastHash string

	// ─── Шаг 1: Отдельные коммиты для каждого целевого файла ───
	for _, file := range splitFiles {
		sendEvent(emit, domain.EventLog,
			fmt.Sprintf("Generating commit message for: %s", file))

		// Генерируем сообщение по diff конкретного файла.
		msg := s.generateCommitMessageForFile(ctx, file, emit)
		sendEvent(emit, domain.EventLog, fmt.Sprintf("Commit message: %s", msg))

		// Добавляем файл в индекс.
		if err := s.Git.AddFile(ctx, file); err != nil {
			result.AddWarning(fmt.Sprintf("cannot add %s: %v", file, err))
			continue
		}

		// Создаём коммит.
		if err := s.Git.CommitMessage(ctx, msg); err != nil {
			result.AddWarning(fmt.Sprintf("cannot commit %s: %v", file, err))
			continue
		}

		// Получаем хеш коммита.
		hash, _ := s.Git.HeadHash(ctx)
		if len(hash) > 7 {
			hash = hash[:7]
		}
		commits = append(commits, fmt.Sprintf("%s → %s", file, hash))
		sendEvent(emit, domain.EventLog,
			fmt.Sprintf("Committed %s (%s)", file, hash))
	}

	// ─── Шаг 2: Общий коммит для остальных файлов ───
	if len(remainingFiles) > 0 {
		sendEvent(emit, domain.EventLog,
			fmt.Sprintf("Generating general commit message for %d remaining file(s)...",
				len(remainingFiles)))
		generalMsg := s.generateCommitMessage(ctx, "", emit)
		if generalMsg == "" {
			generalMsg = "gogitor: update remaining files"
		}
		if err := s.Git.AddAll(ctx); err != nil {
			result.AddWarning(fmt.Sprintf("cannot add remaining files: %v", err))
		} else if err := s.Git.CommitMessage(ctx, generalMsg); err != nil {
			result.AddWarning(fmt.Sprintf("cannot commit remaining files: %v", err))
		} else {
			hash, _ := s.Git.HeadHash(ctx)
			if len(hash) > 7 {
				hash = hash[:7]
			}
			commits = append(commits, fmt.Sprintf("remaining (%d files) → %s",
				len(remainingFiles), hash))
			sendEvent(emit, domain.EventLog,
				fmt.Sprintf("Committed remaining %d file(s) (%s)",
					len(remainingFiles), hash))
		}
	}

	// ─── Результат ───
	s.WS.RefreshIndex()
	result.Success = true
	result.Response = fmt.Sprintf("Created %d commit(s):\n%s",
		len(commits), strings.Join(commits, "\n"))
	if lastHash != "" {
		result.GitCommit = sanitizeGitHash(lastHash)
	}

	return result
}

func sanitizeGitHash(hash string) string {
	var clean strings.Builder
	for _, r := range hash {
		if r < 128 {
			clean.WriteRune(r)
		}
	}
	return strings.TrimSpace(clean.String())
}

func (s *Service) generateCommitMessage(ctx context.Context, taskContext string, emit func(domain.Event)) string {
	_ = s.Git.AddIntentToAll(ctx)
	defer func() {
		_ = s.Git.ResetAll(ctx)
	}()

	// Получаем diff рабочих изменений (до git add).
	diff, err := s.Git.Diff(ctx)
	if err != nil {
		diff = ""
	}

	// Получаем статус файлов (включая новые untracked-файлы, которых нет в diff).
	status, err := s.Git.Status(ctx)
	if err != nil {
		status = ""
	}

	// Если изменений нет — генерировать нечего.
	if strings.TrimSpace(diff) == "" && strings.TrimSpace(status) == "" {
		return ""
	}

	// Ограничиваем размер diff, чтобы не превысить контекст модели.
	const maxDiffLen = 12000
	if len(diff) > maxDiffLen {
		diff = textutil.TruncateStringBytes(diff, maxDiffLen) + "\n... (diff truncated)"
	}

	prompt := prompts.CommitMessage(diff, status, taskContext)

	s.emitProgressStart(
		emit,
		i18n.T("Generating commit message from diff..."),
		agent.RoleCoder,
		"commit",
		prompt,
		0,
		0,
	)
	// Короткий таймаут: генерация сообщения не должна блокировать надолго.
	msgCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	msgCtx = agent.WithRole(msgCtx, agent.RoleCoder)
	msgCtx = agent.WithPriority(msgCtx, agent.PriorityNormal)
	msgCtx = agent.WithPurpose(msgCtx, "generate commit message")
	msgCtx = llm.WithReasoningDisabled(msgCtx)
	msg, err := s.LLM.Send(msgCtx, prompt)
	if err != nil {
		s.Log.Warn("commit message generation failed", "err", err)
		return ""
	}

	msg = cleanCommitMessage(msg)
	if msg == "" {
		return ""
	}

	sendEvent(emit, domain.EventLog, "Commit message: "+msg)
	return msg
}

func cleanCommitMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return ""
	}

	// Убираем возможные markdown-ограждения.
	if strings.HasPrefix(msg, "```") {
		lines := strings.Split(msg, "\n")
		var clean []string
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "```") {
				continue
			}
			clean = append(clean, line)
		}
		msg = strings.TrimSpace(strings.Join(clean, "\n"))
	}

	// Убираем кавычки, если LLM обернул в них ответ.
	if len(msg) >= 2 && (msg[0] == '"' || msg[0] == '\'') {
		msg = strings.Trim(msg, "\"'")
		msg = strings.TrimSpace(msg)
	}

	// Убираем возможные префиксы вроде "Commit message:".
	lower := strings.ToLower(msg)
	for _, prefix := range []string{"commit message:", "message:", "commit:"} {
		if strings.HasPrefix(lower, prefix) {
			msg = strings.TrimSpace(msg[len(prefix):])
			break
		}
	}

	// Валидация первой строки.
	lines := strings.Split(msg, "\n")
	firstLine := strings.TrimSpace(lines[0])
	if len(firstLine) < 5 || len(firstLine) > 200 {
		return ""
	}

	// Проверяем, что строка начинается с допустимого типа Conventional Commit.
	validTypes := []string{
		"feat", "fix", "refactor", "docs", "test",
		"chore", "perf", "ci", "style", "build",
		"update", "add", "remove", "merge", "revert", "wip",
		"wip", "add", "update", "remove", "delete",
		"change", "improve", "optimize", "enhance", "rename",
		"move", "init", "setup", "config", "release",
		"bump", "security", "hotfix", "deps", "lint",
		"format", "clean", "fixup", "temp", "save", "backup", "merge",
	}
	hasValidType := false
	for _, t := range validTypes {
		if strings.HasPrefix(firstLine, t+"(") || strings.HasPrefix(firstLine, t+":") {
			hasValidType = true
			break
		}
	}
	if !hasValidType {
		lines[0] = "chore: " + lines[0]
		return strings.Join(lines, "\n")
	}

	return msg
}

// captureHead возвращает HEAD до начала задачи.
func (s *Service) captureHead(ctx context.Context) string {
	if !s.Git.IsRepo(ctx) {
		return ""
	}
	head, _ := s.Git.HeadHash(ctx)
	return head
}

func (s *Service) captureCumulativeDiff(ctx context.Context, preTaskHead string) string {
	if !s.Git.IsRepo(ctx) {
		return ""
	}
	const maxDiffLen = 100000
	var diff string
	if preTaskHead != "" {
		d, err := s.Git.DiffRange(ctx, preTaskHead, "HEAD")
		if err == nil && strings.TrimSpace(d) != "" {
			diff = d
		}
	}
	if diff == "" {
		_ = s.Git.AddIntentToAll(ctx)
		d, _ := s.Git.Diff(ctx)
		_ = s.Git.ResetAll(ctx)
		diff = d
	}

	if len(diff) > maxDiffLen {
		diff = textutil.TruncateStringBytes(diff, maxDiffLen) + "\n... (diff truncated)"
	}

	return diff
}

// DecisionJournal показывает журнал принятых решений с анализом «долга».
func (s *Service) DecisionJournal(ctx context.Context, emit func(domain.Event)) domain.Result {
	result := domain.Result{Mode: "decisions"}
	emitEvent(emit, domain.Event{
		Type:      domain.EventLog,
		Message:   i18n.Localize("Reading decision journal"),
		TaskStage: domain.TaskStageAnalyze,
	})
	mem := loadAgentMemory(s.Cfg.WorkDir)
	journal := mem.journal()

	// Если журнал пуст — сообщаем.
	if len(journal.Entries) == 0 && len(journal.FailedApproaches) == 0 && len(mem.Decisions) == 0 {
		result.Success = true
		result.Response = "Decision journal is empty. Decisions are recorded automatically during multi-agent tasks."
		return result
	}

	// Формируем базовый хронологический вывод.
	var b strings.Builder

	b.WriteString("## 📋 Decision Journal\n\n")

	// Хронология решений.
	if len(journal.Entries) > 0 {
		b.WriteString("### Timeline\n\n")
		for _, e := range journal.Entries {
			icon := "●"
			if e.Temporary {
				icon = "◐"
			}
			fmt.Fprintf(&b, "%s **[%s]** #%d: %s\n", icon, e.Date, e.ID, e.Decision)
			if e.Temporary && e.Constraint != "" {
				fmt.Fprintf(&b, "  ⏳ Temporary — constraint: %s\n", e.Constraint)
			}
			if e.Context != "" {
				fmt.Fprintf(&b, "  📝 Context: %s\n", e.Context)
			}
			for _, alt := range e.Alternatives {
				fmt.Fprintf(&b, "  ✗ Rejected: %s", alt.Description)
				if alt.Reason != "" {
					fmt.Fprintf(&b, " (%s)", alt.Reason)
				}
				b.WriteByte('\n')
			}
			b.WriteByte('\n')
		}
	}

	// Старые строковые решения (обратная совместимость).
	if len(mem.Decisions) > 0 && len(journal.Entries) == 0 {
		b.WriteString("### Decisions (legacy)\n\n")
		for i, d := range mem.Decisions {
			fmt.Fprintf(&b, "%d. %s\n", i+1, d)
		}
		b.WriteByte('\n')
	}

	// Неудачные подходы.
	if len(journal.FailedApproaches) > 0 {
		b.WriteString("### ✗ Failed Approaches\n\n")
		for _, f := range journal.FailedApproaches {
			fmt.Fprintf(&b, "  ✗ %s\n", f)
		}
		b.WriteByte('\n')
	}

	// Запрашиваем LLM-анализ долга.
	journalText := mem.journalForPrompt(50)
	if strings.TrimSpace(journalText) != "" {
		sendEvent(emit, domain.EventLog, "Analyzing decision debt with LLM...")

		maxFiles, maxBytes := s.contextLimits()
		projectContext := s.WS.BuildSmartContext("", nil, maxFiles/4, maxBytes/4)

		prompt := prompts.AnalyzeDecisions(journalText, projectContext)
		ctx = agent.WithRole(ctx, agent.RoleDefault)
		ctx = agent.WithPriority(ctx, agent.PriorityNormal)
		ctx = agent.WithPurpose(ctx, "analyze decision journal")

		response, err := s.LLM.Send(ctx, prompt)
		if err == nil {
			var analysis struct {
				Summary string `json:"summary"`
				Debts   []struct {
					DecisionID   int    `json:"decision_id"`
					Decision     string `json:"decision"`
					OriginalDate string `json:"original_date"`
					Constraint   string `json:"constraint"`
					Suggestion   string `json:"suggestion"`
				} `json:"debts"`
				Patterns  []string `json:"patterns"`
				RiskNotes []string `json:"risk_notes"`
			}
			if parseErr := parseAgentJSON(response, &analysis); parseErr == nil {
				if analysis.Summary != "" {
					fmt.Fprintf(&b, "### 🔍 Analysis\n\n%s\n\n", analysis.Summary)
				}
				if len(analysis.Debts) > 0 {
					b.WriteString("### ⚠️ Decision Debt\n\n")
					for _, d := range analysis.Debts {
						fmt.Fprintf(&b, "**#%d** [%s] %s\n", d.DecisionID, d.OriginalDate, d.Decision)
						fmt.Fprintf(&b, "  Constraint was: %s\n", d.Constraint)
						fmt.Fprintf(&b, "  💡 %s\n\n", d.Suggestion)
					}
				}
				if len(analysis.Patterns) > 0 {
					b.WriteString("### Patterns\n\n")
					for _, p := range analysis.Patterns {
						fmt.Fprintf(&b, "  • %s\n", p)
					}
					b.WriteByte('\n')
				}
				if len(analysis.RiskNotes) > 0 {
					b.WriteString("### Risks\n\n")
					for _, r := range analysis.RiskNotes {
						fmt.Fprintf(&b, "  ⚠ %s\n", r)
					}
					b.WriteByte('\n')
				}
			}
		} else {
			sendEvent(emit, domain.EventWarn,
				fmt.Sprintf("LLM analysis failed: %v (showing raw journal)", err))
		}
	}

	result.Success = true
	result.Response = b.String()
	return result
}

func (s *Service) isRemoteLLM() bool {
	return !s.isLocalModelEndpoint()
}

// Suggest анализирует проект и предлагает улучшения.
func (s *Service) Suggest(ctx context.Context, emit func(domain.Event)) domain.Result {
	ctx = agent.WithStatusFunc(ctx, s.agentStatusEmitter(emit))
	emitEvent(emit, domain.Event{
		Type:      domain.EventAgent,
		Message:   i18n.Localize("current stage: suggest"),
		TaskStage: domain.TaskStageAnalyze,
	})
	sendEvent(emit, domain.EventLog, "Reading project files for health review")

	maxFiles, maxBytes := s.contextLimits()
	projectContext := s.WS.BuildSmartContext("health review tech debt tests", nil, maxFiles, maxBytes)

	mem := loadAgentMemory(s.Cfg.WorkDir)
	prompt := prompts.Suggest(projectContext, mem.summary(20))

	sendEvent(emit, domain.EventLog, "Sending suggest request to LLM")
	response, err := s.sendLLMStreaming(
		ctx,
		prompt,
		emit,
		agent.RoleDefault,
		agent.PriorityNormal,
		"suggest",
	)
	if err != nil {
		return domain.Result{
			Success: false,
			Mode:    "suggest",
			Errors:  []string{err.Error()},
		}
	}
	return domain.Result{
		Success:  true,
		Mode:     "suggest",
		Response: response,
	}
}

// RunVet выполняет go vet в песочнице.
func (s *Service) RunVet(ctx context.Context, emit func(domain.Event)) domain.Result {
	result := domain.Result{Mode: "test"}
	emitEvent(emit, domain.Event{
		Type:      domain.EventLog,
		Message:   i18n.Localize("Running go vet"),
		TaskStage: domain.TaskStageAnalyze,
	})
	sendEvent(emit, domain.EventLog, "Preparing sandbox")
	sandbox, err := s.WS.PrepareSandbox(ctx)
	if err != nil {
		result.AddError(err.Error())
		return result
	}
	defer os.RemoveAll(sandbox)

	s.Runner.DepsLog = func(msg string) {
		sendEvent(emit, domain.EventLog, msg)
	}
	sendEvent(emit, domain.EventLog, "Running go vet")
	output, err := s.Runner.Vet(ctx, sandbox)
	if err != nil {
		result.Success = false
		result.Response = output
		result.AddError("go vet found issues")
		return result
	}
	result.Success = true
	result.Response = "go vet: no issues found."
	return result
}

// ScanTODO возвращает найденные TODO/FIXME/HACK.
func (s *Service) ScanTODO(ctx context.Context, emit func(domain.Event)) domain.Result {
	emitEvent(emit, domain.Event{
		Type:      domain.EventLog,
		Message:   i18n.Localize("Scanning TODO markers"),
		TaskStage: domain.TaskStageAnalyze,
	})
	items := s.WS.ScanTODOs(50)
	if len(items) == 0 {
		return domain.Result{
			Success:  true,
			Mode:     "todo",
			Response: "No TODO/FIXME/HACK markers found. Project is clean.",
		}
	}
	return domain.Result{
		Success:  true,
		Mode:     "todo",
		Response: workspace.FormatTODOs(items),
	}
}

// ─── GitHub: Pull Request ────────────────────────────────────────────

// GitPR создаёт Pull Request из текущей ветки.
func (s *Service) GitPR(ctx context.Context, emit func(domain.Event)) domain.Result {
	result := domain.Result{Mode: "git"}

	if !s.Git.IsRepo(ctx) {
		result.Success = true
		result.Response = "Not a git repository."
		return result
	}
	if s.Cfg.GitHubToken == "" {
		result.AddError("GitHub token is required. Use --key-github <token>")
		return result
	}

	// Определяем owner/repo из remote origin.
	remoteURL, err := s.Git.RemoteURL(ctx, "origin")
	if err != nil {
		result.AddError("remote 'origin' not found; use ':git remote add origin <url>'")
		return result
	}
	owner, repo, err := github.ParseRepoURL(remoteURL)
	if err != nil {
		result.AddError(fmt.Sprintf("cannot parse remote URL: %v", err))
		return result
	}

	// Текущая ветка.
	headBranch, err := s.Git.CurrentBranch(ctx)
	if err != nil || headBranch == "" {
		result.AddError("cannot determine current branch")
		return result
	}
	if headBranch == "main" || headBranch == "master" {
		result.AddError("refusing to create PR from main/master; switch to a feature branch")
		return result
	}

	// Base branch из GitHub API.
	repoInfo, err := s.GitHub.RepoInfo(ctx, owner, repo)
	if err != nil {
		result.AddError(fmt.Sprintf("cannot get repo info: %v", err))
		return result
	}
	baseBranch := repoInfo.DefaultBr
	if baseBranch == "" {
		baseBranch = "main"
	}

	// Diff для генерации описания.
	sendEvent(emit, domain.EventLog, "Generating PR description from diff...")
	diff, _ := s.Git.DiffRange(ctx, baseBranch, headBranch)

	title, body := s.generatePRDescription(ctx, diff, headBranch, emit)

	sendEvent(emit, domain.EventLog,
		fmt.Sprintf("Creating PR: %s → %s ...", headBranch, baseBranch))
	pr, err := s.GitHub.CreatePullRequest(ctx, owner, repo, title, body, headBranch, baseBranch)
	if err != nil {
		result.AddError(err.Error())
		return result
	}

	result.Success = true
	result.Response = fmt.Sprintf(
		"Pull Request created: #%d\n%s\nTitle: %s",
		pr.Number, pr.HTMLURL, pr.Title,
	)
	return result
}

// generatePRDescription генерирует title и body PR через LLM с fallback.
func (s *Service) generatePRDescription(
	ctx context.Context,
	diff, branch string,
	emit func(domain.Event),
) (string, string) {
	fallbackTitle := fmt.Sprintf("feat: changes from %s", branch)
	fallbackBody := fmt.Sprintf("Automated PR from branch `%s`.", branch)

	if strings.TrimSpace(diff) == "" {
		return fallbackTitle, fallbackBody
	}

	prompt := prompts.PRDescription(diff, branch)
	prCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	prCtx = agent.WithRole(prCtx, agent.RoleDocs)
	prCtx = agent.WithPriority(prCtx, agent.PriorityNormal)
	prCtx = agent.WithPurpose(prCtx, "generate PR description")
	prCtx = llm.WithReasoningDisabled(prCtx)
	response, err := s.LLM.Send(prCtx, prompt)
	if err != nil {
		sendEvent(emit, domain.EventWarn,
			fmt.Sprintf("PR description LLM failed, using fallback: %v", err))
		return fallbackTitle, fallbackBody
	}

	var prDesc struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	if err := parseAgentJSON(response, &prDesc); err != nil {
		sendEvent(emit, domain.EventWarn,
			fmt.Sprintf("Cannot parse PR description JSON, using fallback: %v", err))
		return fallbackTitle, fallbackBody
	}
	if strings.TrimSpace(prDesc.Title) == "" {
		prDesc.Title = fallbackTitle
	}
	if strings.TrimSpace(prDesc.Body) == "" {
		prDesc.Body = fallbackBody
	}
	return prDesc.Title, prDesc.Body
}

// ─── GitHub: Issue из ошибок тестов ─────────────────────────────────

// GitIssue создаёт Issue из ошибок тестов.
func (s *Service) GitIssue(ctx context.Context, emit func(domain.Event)) domain.Result {
	result := domain.Result{Mode: "git"}

	if !s.Git.IsRepo(ctx) {
		result.Success = true
		result.Response = "Not a git repository."
		return result
	}
	if s.Cfg.GitHubToken == "" {
		result.AddError("GitHub token is required. Use --key-github <token>")
		return result
	}

	remoteURL, err := s.Git.RemoteURL(ctx, "origin")
	if err != nil {
		result.AddError("remote 'origin' not found")
		return result
	}
	owner, repo, err := github.ParseRepoURL(remoteURL)
	if err != nil {
		result.AddError(fmt.Sprintf("cannot parse remote URL: %v", err))
		return result
	}

	// Запускаем тесты для получения ошибок.
	sendEvent(emit, domain.EventLog, "Running tests to collect failures...")
	testResult := s.RunTests(ctx, emit)

	if testResult.Tests.Failed == 0 && len(testResult.Tests.Failures) == 0 {
		result.Success = true
		result.Response = "All tests passed — nothing to report as an issue."
		return result
	}

	// Формируем Issue.
	title := fmt.Sprintf("test: %d test(s) failing", testResult.Tests.Failed)
	var body strings.Builder
	body.WriteString("## Failing Tests\n\n")
	body.WriteString("Auto-generated by Gogitor.\n\n")
	for i, f := range testResult.Tests.Failures {
		if i >= 10 {
			fmt.Fprintf(&body, "... and %d more\n", len(testResult.Tests.Failures)-10)
			break
		}
		fmt.Fprintf(&body, "### `%s`\n", f.Test)
		if f.Function != "" {
			fmt.Fprintf(&body, "- **Function:** `%s`\n", f.Function)
		}
		if f.File != "" {
			fmt.Fprintf(&body, "- **Location:** `%s:%d`\n", f.File, f.Line)
		}
		if f.Message != "" {
			fmt.Fprintf(&body, "- **Message:**\n```\n%s\n```\n", truncate(f.Message, 500))
		}
		body.WriteString("\n")
	}
	if testResult.Tests.Output != "" {
		body.WriteString("## Raw Output (truncated)\n\n```\n")
		body.WriteString(truncate(testResult.Tests.Output, 2000))
		body.WriteString("\n```\n")
	}

	sendEvent(emit, domain.EventLog, "Creating issue on GitHub...")
	issue, err := s.GitHub.CreateIssue(ctx, owner, repo, title, body.String(), []string{"bug", "tests"})
	if err != nil {
		result.AddError(err.Error())
		return result
	}

	result.Success = true
	result.Response = fmt.Sprintf(
		"Issue created: #%d\n%s\nTitle: %s",
		issue.Number, issue.HTMLURL, issue.Title,
	)
	return result
}

// ─── GitHub: Changelog ───────────────────────────────────────────────

// GitChangelog генерирует CHANGELOG.md из истории коммитов.
func (s *Service) GitChangelog(ctx context.Context, emit func(domain.Event)) domain.Result {
	result := domain.Result{Mode: "git"}

	if !s.Git.IsRepo(ctx) {
		result.Success = true
		result.Response = "Not a git repository."
		return result
	}

	sendEvent(emit, domain.EventLog, "Reading commit history...")
	subjects, err := s.Git.LogSubjects(ctx, 200)
	if err != nil {
		result.AddError(err.Error())
		return result
	}
	if len(subjects) == 0 {
		result.Success = true
		result.Response = "No commits found."
		return result
	}

	changelog := buildChangelog(subjects)
	changelogPath := filepath.Join(s.Cfg.WorkDir, "CHANGELOG.md")
	if err := os.WriteFile(changelogPath, []byte(changelog), 0o644); err != nil {
		result.AddError(fmt.Sprintf("cannot write CHANGELOG.md: %v", err))
		return result
	}

	result.Success = true
	result.FilesCreated = []string{"CHANGELOG.md"}
	result.Response = fmt.Sprintf(
		"CHANGELOG.md generated from %d commits (%d categorized).",
		len(subjects), countCategorized(subjects),
	)
	return result
}

// ─── GitHub: Review comments ─────────────────────────────────────────

// GitPRComment отправляет комментарий к PR.
func (s *Service) GitPRComment(
	ctx context.Context,
	args []string,
	emit func(domain.Event),
) domain.Result {
	result := domain.Result{Mode: "git"}

	if !s.Git.IsRepo(ctx) {
		result.Success = true
		result.Response = "Not a git repository."
		return result
	}
	if s.Cfg.GitHubToken == "" {
		result.AddError("GitHub token is required. Use --key-github <token>")
		return result
	}
	if len(args) == 0 {
		result.AddError("usage: :git pr-comment <PR-number> [text]")
		return result
	}

	prNumber, err := strconv.Atoi(args[0])
	if err != nil || prNumber <= 0 {
		result.AddError("PR number must be a positive integer")
		return result
	}

	remoteURL, err := s.Git.RemoteURL(ctx, "origin")
	if err != nil {
		result.AddError("remote 'origin' not found")
		return result
	}
	owner, repo, err := github.ParseRepoURL(remoteURL)
	if err != nil {
		result.AddError(fmt.Sprintf("cannot parse remote URL: %v", err))
		return result
	}

	// Текст комментария: из аргументов или из последнего reviewer-результата.
	commentText := strings.TrimSpace(strings.Join(args[1:], " "))
	if commentText == "" {
		commentText = "Automated review comment from Gogitor."
	}

	sendEvent(emit, domain.EventLog,
		fmt.Sprintf("Adding comment to PR #%d...", prNumber))
	if err := s.GitHub.AddIssueComment(ctx, owner, repo, prNumber, commentText); err != nil {
		result.AddError(err.Error())
		return result
	}

	result.Success = true
	result.Response = fmt.Sprintf("Comment added to PR #%d.", prNumber)
	return result
}

// conventionalCommitRE парсит Conventional Commits: type(scope): description
var conventionalCommitRE = regexp.MustCompile(
	`^(\w+)(\(([^)]+)\))?:\s*(.+)$`,
)

type changelogCategory struct {
	Title    string
	Prefixes []string
}

var changelogCategories = []changelogCategory{
	{"🚀 Features", []string{"feat"}},
	{"🐛 Bug Fixes", []string{"fix", "hotfix"}},
	{"♻️ Refactoring", []string{"refactor"}},
	{"🧪 Tests", []string{"test"}},
	{"📚 Documentation", []string{"docs"}},
	{"⚡ Performance", []string{"perf"}},
	{"🔧 Chores", []string{"chore"}},
	{"🏗️ Build", []string{"build"}},
	{"🔁 CI", []string{"ci"}},
}

// buildChangelog формирует Markdown changelog из списка коммитов.
func buildChangelog(subjects []string) string {
	grouped := make(map[string][]string)
	var uncategorized []string

	for _, subject := range subjects {
		m := conventionalCommitRE.FindStringSubmatch(subject)
		if m == nil {
			uncategorized = append(uncategorized, subject)
			continue
		}
		commitType := strings.ToLower(m[1])
		scope := m[3]
		desc := m[4]

		entry := desc
		if scope != "" {
			entry = fmt.Sprintf("**%s:** %s", scope, desc)
		}

		placed := false
		for _, cat := range changelogCategories {
			for _, prefix := range cat.Prefixes {
				if commitType == prefix {
					grouped[cat.Title] = append(grouped[cat.Title], entry)
					placed = true
					break
				}
			}
			if placed {
				break
			}
		}
		if !placed {
			uncategorized = append(uncategorized, subject)
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Changelog\n\n")
	fmt.Fprintf(&b, "> Generated by Gogitor on %s\n\n", time.Now().Format("2006-01-02"))

	for _, cat := range changelogCategories {
		items, ok := grouped[cat.Title]
		if !ok || len(items) == 0 {
			continue
		}
		fmt.Fprintf(&b, "## %s\n\n", cat.Title)
		for _, item := range items {
			fmt.Fprintf(&b, "- %s\n", item)
		}
		b.WriteString("\n")
	}

	if len(uncategorized) > 0 {
		b.WriteString("## Other\n\n")
		for _, item := range uncategorized {
			fmt.Fprintf(&b, "- %s\n", item)
		}
		b.WriteString("\n")
	}

	return b.String()
}

// countCategorized считает коммиты, попавшие в категории.
func countCategorized(subjects []string) int {
	count := 0
	for _, subject := range subjects {
		if conventionalCommitRE.MatchString(subject) {
			count++
		}
	}
	return count
}

// parseCommitSplitArgs парсит аргументы :git commit --split file1,file2
// Возвращает список файлов и флаг наличия --split.
func ParseCommitSplitArgs(args []string) ([]string, bool) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--split" || arg == "--per-file" || arg == "--by-file" {
			// Следующий аргумент — список файлов через запятую.
			if i+1 < len(args) {
				filesStr := strings.TrimSpace(args[i+1])
				if filesStr != "" && !strings.HasPrefix(filesStr, "--") {
					var files []string
					for _, f := range strings.Split(filesStr, ",") {
						f = strings.TrimSpace(f)
						if f != "" {
							files = append(files, f)
						}
					}
					if len(files) > 0 {
						return files, true
					}
				}
			}
			// Флаг есть, но файлы не указаны — показываем подсказку.
			return nil, true
		}
		// Также поддерживаем формат --split=file1,file2
		for _, prefix := range []string{"--split=", "--per-file=", "--by-file="} {
			if strings.HasPrefix(arg, prefix) {
				filesStr := strings.TrimPrefix(arg, prefix)
				var files []string
				for _, f := range strings.Split(filesStr, ",") {
					f = strings.TrimSpace(f)
					if f != "" {
						files = append(files, f)
					}
				}
				if len(files) > 0 {
					return files, true
				}
				return nil, true
			}
		}
	}
	return nil, false
}

// ExecuteComputer выполняет задачу в режиме управления компьютером.
func (s *Service) ExecuteComputer(
	ctx context.Context,
	task string,
	emit func(domain.Event),
) domain.Result {
	ctx = agent.WithStatusFunc(ctx, s.agentStatusEmitter(emit))
	result := domain.Result{Mode: "computer"}

	if !s.Cfg.ComputerEnabled {
		result.AddError("computer mode is disabled")
		return result
	}

	sendEvent(emit, domain.EventLog, "Detecting OS...")
	osInfo := s.ComputerOS
	sendEvent(emit, domain.EventLog, fmt.Sprintf(
		"OS: %s %s %s | pkg: %s | shell: %s | sudo: %v",
		osInfo.OS, osInfo.Distro, osInfo.Version,
		osInfo.PkgManager, osInfo.Shell, osInfo.HasSudo,
	))

	// ─── Пре-валидация задачи: запрещённые команды ──────────
	taskAssessment := security.AssessCommand(task, s.Cfg.WorkDir)
	if taskAssessment.Risk == security.RiskForbidden {
		result.AddError(fmt.Sprintf(
			"task contains a FORBIDDEN command: %s", taskAssessment.Reason))
		return result
	}
	if security.ContainsCommandSubstitution(task) {
		result.AddError(
			"task contains command substitution ($(...), `...`), which is not allowed")
		return result
	}
	if !s.Cfg.ComputerAllowSudo {
		lowerTask := strings.ToLower(task)
		for _, kw := range []string{"sudo", "doas", "pkexec", "runuser"} {
			if strings.Contains(lowerTask, kw) {
				result.AddError(fmt.Sprintf(
					"%s is not allowed; use --allow-sudo to enable", kw))
				return result
			}
		}
	}
	// ─── Планирование ──────────────────────────────────────
	sendEvent(emit, domain.EventLog, "Generating execution plan...")
	mem := loadAgentMemory(s.Cfg.WorkDir)
	prompt := prompts.ComputerPlan(task, osInfo, mem.summary(20), s.Cfg.WorkDir)

	var plan domain.ComputerPlan
	err := s.sendAgentJSON(
		ctx,
		agent.RolePlanner,
		agent.PriorityHigh,
		"computer plan",
		prompt,
		&plan,
	)
	if err != nil {
		result.AddError(fmt.Sprintf("plan generation failed: %v", err))
		return result
	}
	if len(plan.Steps) == 0 {
		result.AddError("plan contains no steps")
		return result
	}
	if len(plan.Steps) > 10 {
		plan.Steps = plan.Steps[:10]
	}

	// Показываем план в TUI
	planItems := make([]string, len(plan.Steps))
	for i, st := range plan.Steps {
		planItems[i] = fmt.Sprintf("[%s] %s", st.Risk, st.Command)
	}
	sendPlanBoard(emit, plan.Goal, nil, planItems)

	// ─── Выполнение шагов ──────────────────────────────────
	var executedCommands []string
	var outputs []string

	for i, step := range plan.Steps {
		stepNum := i + 1
		sendPlanStatus(emit, stepNum, len(plan.Steps),
			step.Command, domain.PlanRunning, "")

		sendEvent(emit, domain.EventLog, fmt.Sprintf(
			"Step %d/%d: %s", stepNum, len(plan.Steps), step.Command,
		))

		// Детерминированная проверка безопасности (ПЕРВЫЙ уровень)
		assessment := security.AssessCommand(step.Command, s.Cfg.WorkDir)
		if assessment.Risk == security.RiskForbidden {
			sendPlanStatus(emit, stepNum, len(plan.Steps),
				step.Command, domain.PlanFailed, "FORBIDDEN")
			s.ComputerAudit.RecordBlocked(
				step.Command, "forbidden", assessment.Reason,
			)
			result.AddError(fmt.Sprintf(
				"step %d FORBIDDEN: %s (%s)",
				stepNum, step.Command, assessment.Reason,
			))
			result.Success = false
			return result
		}

		// Выполняем
		cmdResult, execErr := s.ComputerExecutor.Execute(ctx, step.Command)
		if execErr != nil {
			sendPlanStatus(emit, stepNum, len(plan.Steps),
				step.Command, domain.PlanFailed, truncate(execErr.Error(), 200))
			s.ComputerAudit.RecordBlocked(
				step.Command, assessment.Risk.String(), execErr.Error(),
			)
			result.AddError(fmt.Sprintf("step %d failed: %v", stepNum, execErr))
			result.Success = false
			return result
		}

		s.ComputerAudit.Record(cmdResult, assessment.Risk == security.RiskHigh)

		if cmdResult.ExitCode != 0 {
			sendPlanStatus(emit, stepNum, len(plan.Steps),
				step.Command, domain.PlanFailed,
				fmt.Sprintf("exit code %d", cmdResult.ExitCode))

			// Error Recovery
			sendEvent(emit, domain.EventLog,
				"Command failed, attempting error recovery...")
			recoveryPrompt := prompts.ComputerErrorRecovery(
				step.Command, cmdResult.Output, "",
				cmdResult.ExitCode, osInfo,
			)
			var recovery struct {
				Diagnosis      string `json:"diagnosis"`
				FixCommand     string `json:"fix_command"`
				Alternative    string `json:"alternative"`
				RequiresSearch bool   `json:"requires_search"`
				SearchQuery    string `json:"search_query"`
			}
			recErr := s.sendAgentJSON(
				ctx, agent.RoleCoder, agent.PriorityHigh,
				"computer error recovery", recoveryPrompt, &recovery,
			)

			if recErr == nil && recovery.FixCommand != "" {
				sendEvent(emit, domain.EventLog, fmt.Sprintf(
					"Recovery: %s → %s", recovery.Diagnosis, recovery.FixCommand,
				))
				// Проверяем fix-команду
				fixAssessment := security.AssessCommand(recovery.FixCommand, s.Cfg.WorkDir)
				if fixAssessment.Risk == security.RiskForbidden {
					sendEvent(emit, domain.EventWarn,
						fmt.Sprintf("Recovery command is FORBIDDEN: %s", recovery.FixCommand))
				} else if security.ContainsCommandSubstitution(recovery.FixCommand) {
					sendEvent(emit, domain.EventWarn,
						"Recovery command contains command substitution, skipping")
				}
				if !s.Cfg.ComputerAllowSudo && strings.Contains(recovery.FixCommand, "sudo") {
					sendEvent(emit, domain.EventWarn,
						"Recovery suggests sudo, but sudo is not allowed. "+
							"Use --allow-sudo flag to enable.")
				} else {
					fixResult, fixErr := s.ComputerExecutor.Execute(ctx, recovery.FixCommand)

					if fixErr == nil && fixResult.ExitCode == 0 {
						s.ComputerAudit.Record(fixResult, false)
						executedCommands = append(executedCommands, recovery.FixCommand)
						outputs = append(outputs, fixResult.Output)

						// Показываем вывод fix-команды
						if trimmed := strings.TrimSpace(fixResult.Output); trimmed != "" {
							const maxOutputRunes = 2000
							display := trimmed
							if len([]rune(display)) > maxOutputRunes {
								display = string([]rune(display)[:maxOutputRunes]) + "\n... (вывод обрезан)"
							}
							sendEvent(emit, domain.EventLog, fmt.Sprintf(
								"Output [%s] (recovery):\n%s", recovery.FixCommand, display,
							))
						}

						sendPlanStatus(emit, stepNum, len(plan.Steps),
							step.Command, domain.PlanWarn, "fixed after error")
						continue
					}

				}
			}

			result.AddError(fmt.Sprintf(
				"step %d failed with exit code %d", stepNum, cmdResult.ExitCode,
			))
			result.Success = false
			return result
		}

		executedCommands = append(executedCommands, step.Command)
		outputs = append(outputs, cmdResult.Output)

		// ─── Показываем вывод команды пользователю ──────────────
		if trimmed := strings.TrimSpace(cmdResult.Output); trimmed != "" {
			const maxOutputRunes = 2000
			display := trimmed
			if len([]rune(display)) > maxOutputRunes {
				display = string([]rune(display)[:maxOutputRunes]) + "\n... (вывод обрезан)"
			}
			sendEvent(emit, domain.EventLog, fmt.Sprintf(
				"Output [%s]:\n%s", step.Command, display,
			))
		}

		sendPlanStatus(emit, stepNum, len(plan.Steps),
			step.Command, domain.PlanDone, "")
	}

	// ─── Верификация ───────────────────────────────────────
	sendEvent(emit, domain.EventLog, "Verifying task completion...")
	verifyPrompt := prompts.ComputerResultCheck(task, executedCommands, outputs)
	var verification struct {
		Completed    bool     `json:"completed"`
		Verification string   `json:"verification"`
		Missing      []string `json:"missing"`
		SideEffects  []string `json:"side_effects"`
		Risks        []string `json:"risks"`
	}
	verErr := s.sendAgentJSON(
		ctx, agent.RoleVerifier, agent.PriorityCritical,
		"computer verification", verifyPrompt, &verification,
	)
	if verErr == nil {
		if !verification.Completed {
			result.AddWarning("verifier: task may be incomplete: " +
				strings.Join(verification.Missing, "; "))
		}
		if len(verification.SideEffects) > 0 {
			result.AddWarning("side effects: " +
				strings.Join(verification.SideEffects, "; "))
		}
		if len(verification.Risks) > 0 {
			result.AddWarning("risks: " +
				strings.Join(verification.Risks, "; "))
		}
	}
	sendPlanSummary(emit, domain.PlanDone, len(plan.Steps), len(plan.Steps))
	result.Success = true

	// Формируем итоговый ответ с историей и последними выводами
	var responseBuilder strings.Builder
	responseBuilder.WriteString(fmt.Sprintf(
		"Computer task completed: %d steps executed.\n\n",
		len(executedCommands),
	))
	responseBuilder.WriteString(s.ComputerAudit.FormatHistory(len(executedCommands)))

	// Добавляем последние выводы команд
	if len(outputs) > 0 {
		responseBuilder.WriteString("\n── Command Outputs ──\n")
		for i, out := range outputs {
			trimmed := strings.TrimSpace(out)
			if trimmed == "" {
				continue
			}
			cmdName := ""
			if i < len(executedCommands) {
				cmdName = executedCommands[i]
			}
			const maxOutputRunes = 3000
			display := trimmed
			if len([]rune(display)) > maxOutputRunes {
				display = string([]rune(display)[:maxOutputRunes]) + "\n... (вывод обрезан)"
			}
			responseBuilder.WriteString(fmt.Sprintf("\n$ %s\n%s\n", cmdName, display))
		}
	}

	result.Response = responseBuilder.String()
	return result
}

// ─── Autonomy Commands ───────────────────────────────────────────────

func (s *Service) handleAutonomyCommand(ctx context.Context, args []string, emit func(domain.Event)) domain.Result {
	if s.Autonomy == nil {
		return domain.Result{Success: false, Mode: "autonomy", Errors: []string{"autonomy controller not initialized"}}
	}
	if len(args) == 0 {
		return s.autonomyStatus(emit)
	}
	switch strings.ToLower(args[0]) {
	case "on":
		s.Autonomy.Monitor.Start()
		return domain.Result{Success: true, Mode: "autonomy", Response: "Autonomy monitor started."}
	case "off":
		s.Autonomy.Monitor.Stop()
		return domain.Result{Success: true, Mode: "autonomy", Response: "Autonomy monitor stopped."}
	case "status":
		return s.autonomyStatus(emit)
	case "run":
		return s.autonomyRun(ctx, emit)
	case "clear":
		s.Autonomy.Queue = autonomy.NewTaskQueue()
		return domain.Result{Success: true, Mode: "autonomy", Response: "Autonomy queue cleared."}
	default:
		return domain.Result{
			Success: false, Mode: "autonomy",
			Errors: []string{"usage: :autonomy [on|off|status|run|clear]"},
		}
	}
}

func (s *Service) autonomyStatus(emit func(domain.Event)) domain.Result {
	var b strings.Builder
	b.WriteString(s.Autonomy.Monitor.Status())
	b.WriteString("\n\n")
	b.WriteString(s.Autonomy.Queue.FormatPending())
	return domain.Result{Success: true, Mode: "autonomy", Response: b.String()}
}

// autonomyRun выполняет задачи из очереди, которые можно исправить автоматически.
// Режим "предложение → подтверждение": пользователь запускает эту команду вручную.
// Для каждой задачи вызывается executeSimple с узким промптом.
// Модель 20B+ получает одну конкретную задачу, а не "улучши проект".
func (s *Service) autonomyRun(ctx context.Context, emit func(domain.Event)) domain.Result {
	tasks := s.Autonomy.Queue.Pending()
	if len(tasks) == 0 {
		return domain.Result{Success: true, Mode: "autonomy", Response: "Autonomy queue is empty. Nothing to execute."}
	}
	var results []string
	appliedCount := 0
	for _, task := range tasks {
		var taskPrompt string
		switch task.Source {
		case "build_error":
			taskPrompt = "Fix the following Go build error. Make minimal changes.\n\nERROR:\n" + task.Detail
		case "vet_warning":
			taskPrompt = "Fix the following go vet issues. Make minimal changes.\n\nISSUES:\n" + task.Detail
		default:
			// Для остальных типов (TODO, покрытие) — только предложение
			results = append(results, fmt.Sprintf("[SUGGESTED] #%d [%s] %s", task.ID, task.Source, task.Title))
			s.Autonomy.Queue.SetStatus(task.ID, "dismissed")
			continue
		}
		sendEvent(emit, domain.EventLog, i18n.T("Executing autonomy task #%d: %s", task.ID, task.Title))
		res := s.ExecuteCode(ctx, taskPrompt, Options{}, emit)
		if res.Success {
			s.Autonomy.Queue.SetStatus(task.ID, "applied")
			results = append(results, fmt.Sprintf("[APPLIED] #%d [%s] %s", task.ID, task.Source, task.Title))
			appliedCount++
		} else {
			s.Autonomy.Queue.SetStatus(task.ID, "failed")
			results = append(results, fmt.Sprintf("[FAILED] #%d [%s] %s: %s", task.ID, task.Source, task.Title, strings.Join(res.Errors, "; ")))
		}
	}
	response := fmt.Sprintf("Autonomy run: %d task(s) processed, %d applied.\n\n%s", len(tasks), appliedCount, strings.Join(results, "\n"))
	return domain.Result{Success: true, Mode: "autonomy", Response: response}
}

func (s *Service) handleMutateCommand(ctx context.Context, args []string, emit func(domain.Event)) domain.Result {
	if s.Autonomy == nil {
		return domain.Result{Success: false, Mode: "mutate", Errors: []string{"autonomy controller not initialized"}}
	}
	limit := s.Cfg.AutonomyMutationLimit
	if len(args) > 0 {
		if n, err := strconv.Atoi(args[0]); err == nil && n > 0 {
			limit = n
		}
	}
	mutator := autonomy.NewMutator(s.WS, s.Runner, limit)
	sendEvent(emit, domain.EventLog, i18n.T("Generating mutations (deterministic, no LLM)..."))
	mutations := mutator.GenerateMutations()
	if len(mutations) == 0 {
		return domain.Result{Success: true, Mode: "mutate", Response: "No applicable mutations found."}
	}
	sendEvent(emit, domain.EventLog, i18n.T("Generated %d mutations. Running mutation tests...", len(mutations)))
	report := mutator.Run(ctx, mutations, emit)
	return domain.Result{Success: true, Mode: "mutate", Response: report.Format()}
}

func (s *Service) handleTestGenCommand(ctx context.Context, args []string, emit func(domain.Event)) domain.Result {
	if s.Autonomy == nil {
		return domain.Result{Success: false, Mode: "autogen-tests", Errors: []string{"autonomy controller not initialized"}}
	}
	maxFuncs := 5
	if len(args) > 0 {
		if n, err := strconv.Atoi(args[0]); err == nil && n > 0 {
			maxFuncs = n
		}
	}
	sendEvent(emit, domain.EventLog, i18n.T("Scanning for untested exported functions (AST, no LLM)..."))
	untested := s.Autonomy.TestGen.FindUntested(maxFuncs)
	if len(untested) == 0 {
		return domain.Result{Success: true, Mode: "autogen-tests", Response: "No untested exported functions found."}
	}
	sendEvent(emit, domain.EventLog, i18n.T("Found %d untested function(s). Generating tests...", len(untested)))
	var results []string
	for _, fn := range untested {
		name := fn.Name
		if fn.Receiver != "" {
			name = fn.Receiver + "." + fn.Name
		}
		sendEvent(emit, domain.EventLog, i18n.T("Generating test for %s (%s:%d)...", name, fn.File, fn.Line))
		testCode, err := s.Autonomy.TestGen.GenerateForFunc(ctx, fn)
		if err != nil {
			results = append(results, fmt.Sprintf("[ERROR] %s: %v", name, err))
			continue
		}
		testFile, err := s.Autonomy.TestGen.ApplyTest(ctx, fn, testCode, s.Runner, emit)
		if err != nil {
			results = append(results, fmt.Sprintf("[FAILED] %s: %v", name, err))
			continue
		}
		results = append(results, fmt.Sprintf("[CREATED] %s → %s", name, testFile))
	}
	response := fmt.Sprintf("## Test Generation Results\n\n%s", strings.Join(results, "\n"))
	return domain.Result{Success: true, Mode: "autogen-tests", Response: response}
}

// ─── Функции-обертки для тестирования ─────────────────────────────
// Эти функции позволяют тестировать логику без создания экземпляра Service

// isAnalysisOnlyTask проверяет, является ли задача только анализом
// (без изменения файлов). Обертка над методом Service для удобства тестирования.
func isAnalysisOnlyTask(query string) bool {
	lower := strings.ToLower(query)
	analysisKeywords := []string{
		"analyze", "analyse", "analysis", "read", "inspect", "identify",
		"find", "extract", "explain", "understand", "determine", "review",
		"проанализируй", "анализ", "прочитай", "читать", "изучи", "изучить",
		"найди", "найти", "определи", "определить", "извлеки", "извлечь",
		"объясни", "объяснить", "пойми", "понять", "разберись", "разобраться",
	}
	modificationKeywords := []string{
		"create", "write", "add", "modify", "change", "fix", "update",
		"refactor", "rewrite", "generate", "implement", "delete", "remove",
		"move", "split", "separate", "correct", "improve", "apply", "save",
		"модифицир", "модификац", "модифик", "создай", "создать", "напиши",
		"писать", "добавь", "добавить", "измени", "изменить", "исправь",
		"исправить", "обнови", "обновить", "переделай", "переделать",
		"перепиши", "переписать", "сгенерируй", "сгенерировать", "реализуй",
		"реализовать", "удали", "удалить", "перемести", "переместить",
		"раздели", "разделить", "вынеси", "вынести", "примени", "применить",
		"сохрани", "сохранить", "сделай", "сделать",
	}
	if !containsAny(lower, analysisKeywords) {
		return false
	}
	if containsAny(lower, modificationKeywords) {
		return false
	}
	return true
}

// isSplitOrRefactor проверяет, является ли задача разделением или рефакторингом.
// Обертка над методом Service для удобства тестирования.
func isSplitOrRefactor(query string) bool {
	lower := strings.ToLower(query)
	keywords := []string{
		"раздели", "разделить", "разделение", "разбей", "разбить",
		"вынеси", "вынести", "перенеси", "перенести", "рефактор",
		"рефакторинг", "реструктур", "split", "divide", "separate",
		"extract", "move", "refactor", "refactoring", "restructure",
	}
	return containsAny(lower, keywords)
}

// handleLoad читает задачу из файла и передаёт её в ProcessEvents,
// где роутер намерений определяет режим (код / анализ / поиск / fix и т.д.).
// Формат пути: <директория>/<файл> относительно корня проекта или
// абсолютный путь / путь с ~. Поддерживаются только .txt и .md.
func (s *Service) handleLoad(
	ctx context.Context,
	argString string,
	emit func(domain.Event),
) domain.Result {
	argString = strings.TrimSpace(argString)
	if argString == "" {
		return domain.Result{
			Success: false,
			Mode:    "load",
			Errors:  []string{"usage: :load <path/to/file.txt|file.md>"},
		}
	}

	path := expandHome(argString)

	// Относительный путь считаем относительно корня проекта.
	if !filepath.IsAbs(path) {
		path = filepath.Join(s.Cfg.WorkDir, path)
	}

	task, err := readTaskFile(path)
	if err != nil {
		return domain.Result{
			Success: false,
			Mode:    "load",
			Errors:  []string{err.Error()},
		}
	}

	sendEvent(emit, domain.EventLog,
		fmt.Sprintf("Task loaded from file: %s", filepath.Base(path)))

	// Задача уходит в тот же конвейер, что и обычный пользовательский
	// запрос: роутер намерений сам выберет режим (код, анализ, поиск,
	// fix, git и т.д.).
	s.RawTask = true
	defer func() { s.RawTask = false }()
	return s.ProcessEvents(ctx, task, emit)
}

// readTaskFile читает и валидирует файл задачи (.txt / .md).
// Повторяет логику из internal/ui/cli, чтобы TUI не зависел от пакета cli.
func readTaskFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("cannot read task file: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("task file path is a directory: %s", path)
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".txt" && ext != ".md" {
		return "", fmt.Errorf("task file must have .txt or .md extension: %s", path)
	}
	const maxTaskFileSize = 1 << 20 // 1 MB
	if info.Size() > maxTaskFileSize {
		return "", fmt.Errorf("task file is too large (%d bytes), max %d bytes",
			info.Size(), maxTaskFileSize)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("cannot read task file: %w", err)
	}
	task := string(data)
	task = strings.TrimPrefix(task, "\ufeff") // UTF-8 BOM
	task = strings.TrimSpace(task)
	if task == "" {
		return "", fmt.Errorf("task file is empty: %s", path)
	}
	return task, nil
}

// expandHome раскрывает префикс "~/..." в абсолютный путь.
// Дубликат из пакета cli (функция неэкспортирована и недоступна извне).
func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}
