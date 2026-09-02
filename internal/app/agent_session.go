package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gogitor/internal/agent"
	"gogitor/internal/domain"
	"gogitor/internal/llm"
	"gogitor/internal/prompts"
	"gogitor/internal/runner"
	"gogitor/internal/textutil"
)

type agentSession struct {
	Dir           string
	InboxPath     string
	ResearchPath  string
	PlanPath      string
	PlanJSONPath  string
	ProcessPath   string
	ResultPath    string
	FinalGatePath string
}

func (r agentGateReport) toDomain() domain.QualityGateStatus {
	return domain.QualityGateStatus{
		Build:         r.Build,
		Tests:         r.Tests,
		Vet:           r.Vet,
		Gofmt:         r.Gofmt,
		Lint:          r.Lint,
		LintInstalled: r.LintInstalled,
		LintIssues:    r.LintIssues,
		TestsPassed:   r.TestsPassed,
		TestsFailed:   r.TestsFailed,
		Coverage:      r.Coverage,
		Passed:        r.Passed,
	}
}

func (s *agentSession) gatePath(index int) string {
	if index <= 0 {
		return filepath.Join(s.Dir, "gate-final.json")
	}
	return filepath.Join(s.Dir, fmt.Sprintf("gate-task-%02d.json", index))
}

func (s *Service) startAgentSession(task string, depth AgentDepth, preTaskHead string, answers []prompts.AgentInterviewAnswer) (*agentSession, error) {
	stamp := time.Now().Format("20060102-150405")
	dir := filepath.Join(s.Cfg.WorkDir, ".gogitor", "agent", stamp)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	session := &agentSession{
		Dir:           dir,
		InboxPath:     filepath.Join(dir, "inbox.md"),
		ResearchPath:  filepath.Join(dir, "research.md"),
		PlanPath:      filepath.Join(dir, "plan.md"),
		PlanJSONPath:  filepath.Join(dir, "plan.json"),
		ProcessPath:   filepath.Join(dir, "process.md"),
		ResultPath:    filepath.Join(dir, "result.json"),
		FinalGatePath: filepath.Join(dir, "gate-final.json"),
	}

	inbox := formatAgentInbox(task, depth, answers)

	if err := os.WriteFile(
		session.InboxPath,
		[]byte(inbox),
		0o644,
	); err != nil {
		return nil, fmt.Errorf(
			"write agent inbox: %w",
			err,
		)
	}

	research := "# Agent Research\n\n" +
		s.projectSummary() +
		"\n"

	if err := os.WriteFile(
		session.ResearchPath,
		[]byte(research),
		0o644,
	); err != nil {
		return nil, fmt.Errorf(
			"write agent research: %w",
			err,
		)
	}

	appendAgentProcess(session.ProcessPath, "Agent session started: "+stamp)
	appendAgentProcess(session.ProcessPath, "Depth: "+string(depth))
	if preTaskHead != "" {
		appendAgentProcess(session.ProcessPath, "Pre-task HEAD: "+preTaskHead)
	}
	return session, nil
}

func formatAgentInbox(task string, depth AgentDepth, answers []prompts.AgentInterviewAnswer) string {
	var b strings.Builder
	b.WriteString("# Agent Inbox\n")
	b.WriteString("## Original request\n")
	b.WriteString(strings.TrimSpace(task) + "\n")
	b.WriteString("## Depth\n" + string(depth) + "\n")
	if len(answers) > 0 {
		b.WriteString("## Interview Q&A\n")
		for _, a := range answers {
			fmt.Fprintf(&b, "**Q%d:** %s\n**A%d:** %s\n", a.ID, a.Question, a.ID, a.Answer)
		}
	}
	return b.String()
}

func appendAgentProcess(path, line string) {
	if strings.TrimSpace(line) == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "[%s] %s\n", time.Now().Format("2006-01-02 15:04:05"), line)
}

type agentGateReport struct {
	TaskIndex     int      `json:"task_index"`
	Build         bool     `json:"build"`
	Tests         bool     `json:"tests"`
	Vet           bool     `json:"vet"`
	Gofmt         bool     `json:"gofmt"`
	Lint          bool     `json:"lint"`
	LintInstalled bool     `json:"lint_installed"`
	LintIssues    int      `json:"lint_issues"`
	TestsPassed   int      `json:"tests_passed"`
	TestsFailed   int      `json:"tests_failed"`
	Coverage      float64  `json:"coverage,omitempty"`
	Passed        bool     `json:"passed"`
	Errors        []string `json:"errors,omitempty"`
	Warnings      []string `json:"warnings,omitempty"`
}

func (s *Service) runAgentDeepQualityGates(
	ctx context.Context,
	emit func(domain.Event),
	taskIndex int,
	skipTests bool,
) agentGateReport {
	report := agentGateReport{
		TaskIndex: taskIndex,
		Lint:      true,
	}

	sandbox, err := s.WS.PrepareSandbox(ctx)
	if err != nil {
		report.Errors = append(
			report.Errors,
			fmt.Sprintf(
				"cannot prepare sandbox: %v",
				err,
			),
		)
		return report
	}
	defer os.RemoveAll(sandbox)

	sendEvent(
		emit,
		domain.EventLog,
		"Running Agent Deep quality gates",
	)

	if err := s.Runner.PrepareGoModule(
		ctx,
		sandbox,
	); err != nil {
		report.Errors = append(
			report.Errors,
			fmt.Sprintf(
				"go module preparation failed: %v",
				err,
			),
		)
		return report
	}

	s.Runner.ResolveDeps(
		ctx,
		sandbox,
	)

	// --------------------------------------------------
	// 1. gofmt — НЕ форматируем, только проверяем.
	// --------------------------------------------------

	if _, err := exec.LookPath("gofmt"); err != nil {
		report.Errors = append(
			report.Errors,
			"gofmt is not installed",
		)
	} else {
		out, err := runAgentGateCommand(
			ctx,
			sandbox,
			"gofmt",
			"-l",
			".",
		)

		if err != nil {
			report.Errors = append(
				report.Errors,
				fmt.Sprintf(
					"gofmt check failed: %v",
					err,
				),
			)
		} else if strings.TrimSpace(out) != "" {
			report.Errors = append(
				report.Errors,
				"unformatted files: "+
					trim(out, 2000),
			)
		} else {
			report.Gofmt = true
		}
	}

	// --------------------------------------------------
	// 2. go build — без Runner.Build(), чтобы build
	//    не форматировал проект перед проверкой.
	// --------------------------------------------------

	buildOut, err := runAgentGateCommand(
		ctx,
		sandbox,
		"go",
		"build",
		"-o",
		os.DevNull,
		"./...",
	)

	if err != nil {
		report.Errors = append(
			report.Errors,
			"build failed: "+
				trim(buildOut, 3000),
		)
	} else {
		report.Build = true
	}

	// --------------------------------------------------
	// 3. Tests
	// --------------------------------------------------

	if skipTests {
		report.Tests = true
		report.Warnings = append(
			report.Warnings,
			"tests skipped by user",
		)
	} else {
		tests, testErr :=
			s.Runner.Test(
				ctx,
				sandbox,
			)

		report.TestsPassed = tests.Passed
		report.TestsFailed = tests.Failed
		report.Coverage = tests.Coverage

		if tests.Skipped {
			report.Tests = true
		} else if testErr != nil {
			report.Errors = append(
				report.Errors,
				"test execution failed: "+
					testErr.Error(),
			)
		} else if tests.Failed > 0 {
			report.Errors = append(
				report.Errors,
				runner.FormatFeedback(tests),
			)
		} else {
			report.Tests = true
		}
	}

	// --------------------------------------------------
	// 4. go vet
	// --------------------------------------------------

	vetOut, err := s.Runner.Vet(
		ctx,
		sandbox,
	)

	if err != nil {
		report.Errors = append(
			report.Errors,
			"vet failed: "+
				trim(vetOut, 3000),
		)
	} else {
		report.Vet = true
	}

	// --------------------------------------------------
	// 5. golangci-lint
	// --------------------------------------------------

	if _, err := exec.LookPath(
		"golangci-lint",
	); err != nil {
		report.LintInstalled = false
		report.Warnings = append(
			report.Warnings,
			"golangci-lint is not installed; lint gate skipped",
		)
	} else {
		report.LintInstalled = true

		lintOut, lintErr :=
			s.Runner.Lint(
				ctx,
				sandbox,
			)

		issues :=
			runner.CountLintIssues(lintOut)

		report.LintIssues = issues

		if lintErr != nil {
			report.Lint = false

			report.Errors = append(
				report.Errors,
				"lint failed: "+
					trim(lintOut, 3000),
			)
		} else if issues > 0 {
			report.Lint = false

			report.Errors = append(
				report.Errors,
				fmt.Sprintf(
					"lint found %d issue(s)",
					issues,
				),
			)
		} else {
			report.Lint = true
		}
	}

	report.Passed =
		len(report.Errors) == 0 &&
			report.Build &&
			report.Tests &&
			report.Vet &&
			report.Gofmt &&
			(!report.LintInstalled ||
				report.Lint)

	for _, warning := range report.Warnings {
		sendEvent(
			emit,
			domain.EventWarn,
			warning,
		)
	}

	for _, failure := range report.Errors {
		sendEvent(
			emit,
			domain.EventError,
			failure,
		)
	}

	return report
}

func runAgentGateCommand(ctx context.Context, dir, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func saveAgentGateReport(path string, report *agentGateReport) error {
	data, _ := json.MarshalIndent(report, "", "  ")
	return os.WriteFile(path, data, 0o644)
}

func (s *Service) findLatestAgentSessionDir() (string, error) {
	baseDir := filepath.Join(s.Cfg.WorkDir, ".gogitor", "agent")
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return "", err
	}
	var latest string
	for _, e := range entries {
		if e.IsDir() && e.Name() > latest {
			latest = e.Name()
		}
	}
	if latest == "" {
		return "", fmt.Errorf("no agent sessions found")
	}
	return filepath.Join(baseDir, latest), nil
}

// Перенесенные функции валидации плана из workflow.go
func validateAgentPlan(plan *fullPlan, originalTask string) *fullPlan {
	if plan == nil {
		plan = &fullPlan{}
	}

	plan.Goal = strings.TrimSpace(plan.Goal)
	if plan.Goal == "" {
		plan.Goal = truncate(originalTask, 200)
	}

	var cleanAcceptance []string
	seenAcceptance := make(map[string]bool)

	for _, a := range plan.Acceptance {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}

		a = truncate(a, 220)
		key := strings.ToLower(a)

		if seenAcceptance[key] {
			continue
		}

		seenAcceptance[key] = true
		cleanAcceptance = append(cleanAcceptance, a)

		if len(cleanAcceptance) >= 10 {
			break
		}
	}

	plan.Acceptance = cleanAcceptance

	var clean []fullPlanSubtask
	seen := make(map[string]bool)

	for _, st := range plan.Subtasks {
		task := strings.TrimSpace(st.Task)
		if task == "" {
			continue
		}

		if isRuntimeOnlySubtask(task) {
			continue
		}

		key := strings.ToLower(task)
		if seen[key] {
			continue
		}

		seen[key] = true

		if len(task) > 800 {
			task = truncate(task, 800)
		}

		var acceptance []string
		seenAcc := make(map[string]bool)

		for _, a := range st.Acceptance {
			a = strings.TrimSpace(a)
			if a == "" {
				continue
			}

			a = truncate(a, 200)
			accKey := strings.ToLower(a)

			if seenAcc[accKey] {
				continue
			}

			seenAcc[accKey] = true
			acceptance = append(acceptance, a)

			if len(acceptance) >= 8 {
				break
			}
		}

		clean = append(clean, fullPlanSubtask{
			Task:        task,
			Acceptance:  acceptance,
			NeedsSearch: st.NeedsSearch,
		})

		if len(clean) >= 7 {
			break
		}
	}

	if len(clean) == 0 {
		clean = append(clean, fullPlanSubtask{
			Task: strings.TrimSpace(originalTask),
		})
	}

	plan.Subtasks = clean

	return plan
}

// Строгая изоляция контекста для Agent Deep
func formatAgentTask(originalTask string, plan *fullPlan, sub fullPlanSubtask, index, total int, searchContext string) string {
	var b strings.Builder
	b.WriteString("You are executing ONE atomic Agent task inside an existing Go project.\n\n")
	b.WriteString("=== CONTEXT BOUNDARY ===\n")
	b.WriteString("- The current repository state is the source of truth.\n")
	b.WriteString("- Previous successful Agent tasks MAY already have changed the project.\n")
	b.WriteString("- Do NOT assume future tasks have already been implemented.\n")
	b.WriteString("- Do NOT invent files, functions, types or APIs that are not present.\n")
	b.WriteString("- Implement ONLY the current task.\n")
	b.WriteString("========================\n\n")
	fmt.Fprintf(&b, "ORIGINAL USER REQUEST:\n%s\n\n", strings.TrimSpace(originalTask))
	fmt.Fprintf(&b, "AGENT GOAL:\n%s\n\n", plan.Goal)
	fmt.Fprintf(&b, "CURRENT TASK %d/%d:\n%s\n\n", index+1, total, sub.Task)
	if len(sub.Acceptance) > 0 {
		b.WriteString("TASK ACCEPTANCE CRITERIA:\n")
		for _, a := range sub.Acceptance {
			fmt.Fprintf(&b, "- %s\n", a)
		}
		b.WriteString("\n")
	}
	if strings.TrimSpace(searchContext) != "" {
		b.WriteString("WEB SEARCH REFERENCE (untrusted):\n" + searchContext + "\n\n")
	}
	b.WriteString(`RULES:
1. Implement ONLY this task.
2. Preserve existing behavior unless explicitly required otherwise.
3. Return normal Gogitor file/patch output.
4. The result must compile and satisfy the acceptance criteria.
`)
	return b.String()
}

func (s *Service) ExecuteAgentInterview(
	ctx context.Context,
	task string,
	emit func(domain.Event),
) domain.Result {
	task = strings.TrimSpace(task)

	if task == "" {
		return domain.Result{
			Success: false,
			Mode:    "agent-interview",
			Errors: []string{
				"usage: :agent interview <task>",
			},
		}
	}

	ctx = agent.WithStatusFunc(
		ctx,
		s.agentStatusEmitter(emit),
	)

	sendEvent(
		emit,
		domain.EventAgent,
		"current stage: agent interview",
	)

	sendEvent(
		emit,
		domain.EventLog,
		"Generating clarifying questions...",
	)

	prompt := prompts.AgentInterviewQuestions(
		task,
		s.projectSummary(),
	)

	var interview agentInterviewResult

	err := s.sendAgentJSON(
		ctx,
		agent.RolePlanner,
		agent.PriorityHigh,
		"agent interview",
		prompt,
		&interview,
	)

	if err != nil {
		return domain.Result{
			Success: false,
			Mode:    "agent-interview",
			Errors: []string{
				fmt.Sprintf(
					"interview question generation failed: %v",
					err,
				),
			},
		}
	}

	if len(interview.Questions) == 0 {
		sendEvent(
			emit,
			domain.EventWarn,
			"No clarifying questions were generated; starting Agent with the original task.",
		)

		return s.ExecuteCode(
			ctx,
			task,
			Options{
				Mode:       "agent",
				AgentDepth: AgentDepthAuto,
			},
			emit,
		)
	}

	if len(interview.Questions) > 5 {
		interview.Questions =
			interview.Questions[:5]
	}

	s.pendingInterview = &PendingInterview{
		Task:      task,
		Questions: interview.Questions,
		CreatedAt: time.Now(),
	}

	var b strings.Builder

	b.WriteString(
		"## Agent interview\n\n",
	)

	b.WriteString(
		"Answer each question as `N: answer`.\n",
	)
	b.WriteString(
		"Use `skip` or `go` to accept all defaults.\n\n",
	)

	for _, q := range interview.Questions {
		fmt.Fprintf(
			&b,
			"### %d. %s\n",
			q.ID,
			q.Question,
		)

		if strings.TrimSpace(q.Why) != "" {
			fmt.Fprintf(
				&b,
				"Why: %s\n",
				q.Why,
			)
		}

		if strings.TrimSpace(q.Default) != "" {
			fmt.Fprintf(
				&b,
				"Default: %s\n",
				q.Default,
			)
		}

		b.WriteString("\n")
	}

	return domain.Result{
		Success:           true,
		Mode:              "agent-interview",
		AwaitingSelection: true,
		RefinedTask:       task,
		Response:          b.String(),
	}
}

func (s *Service) ContinueAgentInterview(
	ctx context.Context,
	originalTask string,
	questions []agentInterviewQuestion,
	answersText string,
	emit func(domain.Event),
) domain.Result {
	ctx = agent.WithStatusFunc(
		ctx,
		s.agentStatusEmitter(emit),
	)

	sendEvent(
		emit,
		domain.EventLog,
		"Processing interview answers...",
	)

	answers := parseAgentInterviewAnswers(
		questions,
		answersText,
	)

	prompt := prompts.AgentInterviewSummary(
		originalTask,
		answers,
	)

	refinedTask := originalTask

	response, err := s.LLM.Send(
		ctx,
		prompt,
	)

	if err == nil &&
		strings.TrimSpace(response) != "" {
		refinedTask = strings.TrimSpace(response)

		sendEvent(
			emit,
			domain.EventLog,
			"Task refined based on interview answers.",
		)
	} else if err != nil {
		sendEvent(
			emit,
			domain.EventWarn,
			fmt.Sprintf(
				"Interview refinement failed: %v; using original task",
				err,
			),
		)
	}

	depth := s.agentDepthForTask(
		refinedTask,
	)

	return s.ExecuteCode(
		ctx,
		refinedTask,
		Options{
			Mode:             "agent",
			AgentDepth:       depth,
			InterviewAnswers: answers,
		},
		emit,
	)
}

func parseAgentInterviewAnswers(
	questions []agentInterviewQuestion,
	text string,
) []prompts.AgentInterviewAnswer {
	text = strings.TrimSpace(text)

	useDefaults := false

	switch strings.ToLower(text) {
	case "":
		useDefaults = true

	case "skip",
		"go",
		"default",
		"по умолчанию",
		"пропустить",
		"далее",
		"дальше":
		useDefaults = true
	}

	result := make(
		[]prompts.AgentInterviewAnswer,
		0,
		len(questions),
	)

	if useDefaults {
		for _, q := range questions {
			result = append(
				result,
				prompts.AgentInterviewAnswer{
					ID:       q.ID,
					Question: q.Question,
					Answer:   q.Default,
				},
			)
		}
		return result
	}

	answersByID := make(
		map[int]string,
		len(questions),
	)

	for _, line := range strings.Split(
		text,
		"\n",
	) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		re := regexp.MustCompile(
			`^\s*(\d+)\s*(?::|-|\))\s*(.+)$`,
		)

		m := re.FindStringSubmatch(line)
		if len(m) != 3 {
			continue
		}

		id, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}

		answersByID[id] =
			strings.TrimSpace(m[2])
	}

	for _, q := range questions {
		answer := answersByID[q.ID]

		if answer == "" {
			answer = q.Default
		}

		result = append(
			result,
			prompts.AgentInterviewAnswer{
				ID:       q.ID,
				Question: q.Question,
				Answer:   answer,
			},
		)
	}

	return result
}

func (s *Service) ExecuteAgentReflect(
	ctx context.Context,
	emit func(domain.Event),
) domain.Result {
	ctx = agent.WithStatusFunc(
		ctx,
		s.agentStatusEmitter(emit),
	)

	sessionDir, err :=
		s.findLatestAgentSessionDir()

	if err != nil {
		return domain.Result{
			Success: false,
			Mode:    "agent-reflect",
			Errors: []string{
				fmt.Sprintf(
					"no agent session found: %v",
					err,
				),
			},
		}
	}

	sendEvent(
		emit,
		domain.EventAgent,
		"current stage: agent reflect",
	)

	sendEvent(
		emit,
		domain.EventLog,
		fmt.Sprintf(
			"Analyzing agent artifacts: %s",
			sessionDir,
		),
	)

	inbox := readFileOrEmpty(
		filepath.Join(
			sessionDir,
			"inbox.md",
		),
	)

	plan := readFileOrEmpty(
		filepath.Join(
			sessionDir,
			"plan.json",
		),
	)

	processLog := readFileOrEmpty(
		filepath.Join(
			sessionDir,
			"process.md",
		),
	)

	resultJSON := readFileOrEmpty(
		filepath.Join(
			sessionDir,
			"result.json",
		),
	)

	gateReports := readAgentGateReports(
		sessionDir,
	)

	cumulativeDiff := ""

	if resultJSON != "" {
		var meta struct {
			PreTaskHead string `json:"pre_task_head"`
		}

		if err := json.Unmarshal(
			[]byte(resultJSON),
			&meta,
		); err == nil &&
			meta.PreTaskHead != "" &&
			s.Git.IsRepo(ctx) {

			cumulativeDiff, _ =
				s.Git.DiffRange(
					ctx,
					meta.PreTaskHead,
					"HEAD",
				)
		}
	}

	goal := extractAgentGoal(inbox)

	var prompt string

	if s.modelProfile() ==
		modelProfileSmall {

		prompt = prompts.AgentReflectQuick(
			goal,
			processLog,
		)
	} else {
		prompt = prompts.AgentReflection(
			goal,
			processLog,
			plan,
			gateReports,
			cumulativeDiff,
		)
	}

	response, llmErr :=
		s.sendLLMStreaming(
			ctx,
			prompt,
			emit,
			agent.RoleDefault,
			agent.PriorityNormal,
			"agent_reflect",
		)

	if llmErr != nil {
		sendEvent(
			emit,
			domain.EventWarn,
			fmt.Sprintf(
				"LLM reflection failed: %v; showing raw artifacts",
				llmErr,
			),
		)

		response =
			buildAgentReflectionFallback(
				sessionDir,
				goal,
				processLog,
				plan,
			)
	}

	reflectionPath :=
		filepath.Join(
			sessionDir,
			"reflection.md",
		)

	title := "# Agent Reflection"

	if DetectLanguage() == "ru" {
		title = "# Ретроспектива Agent"
	}

	content := fmt.Sprintf(
		"%s\n\n_Generated: %s_\n\n%s\n",
		title,
		time.Now().Format(
			"2006-01-02 15:04:05",
		),
		response,
	)

	if err := os.WriteFile(
		reflectionPath,
		[]byte(content),
		0o644,
	); err != nil {
		return domain.Result{
			Success:  true,
			Mode:     "agent-reflect",
			Response: response,
			Warnings: []string{
				fmt.Sprintf(
					"cannot save reflection.md: %v",
					err,
				),
			},
		}
	}

	// Lessons извлекаем только для не-малых моделей.
	if s.modelProfile() != modelProfileSmall &&
		llmErr == nil {

		lessonsPrompt :=
			prompts.AgentExtractLessons(
				response,
			)

		lessonsResponse, lessonsErr :=
			s.LLM.Send(
				llm.WithReasoningDisabled(ctx),
				lessonsPrompt,
			)

		if lessonsErr == nil {
			lessons :=
				parseAgentLessons(
					lessonsResponse,
				)

			if len(lessons) > 0 {
				mem :=
					loadAgentMemory(
						s.Cfg.WorkDir,
					)

				for _, lesson := range lessons {
					mem.addLesson(
						lesson,
					)
				}

				if err := mem.save(
					s.Cfg.WorkDir,
				); err != nil {
					sendEvent(
						emit,
						domain.EventWarn,
						fmt.Sprintf(
							"Cannot save lessons to agent memory: %v",
							err,
						),
					)
				}
			}
		}
	}

	return domain.Result{
		Success:  true,
		Mode:     "agent-reflect",
		Response: response,
	}
}

func buildAgentReflectionFallback(
	sessionDir string,
	goal string,
	processLog string,
	plan string,
) string {
	var b strings.Builder

	b.WriteString("## Agent Reflection Fallback\n\n")

	if strings.TrimSpace(goal) != "" {
		b.WriteString("### Goal\n\n")
		b.WriteString(strings.TrimSpace(goal))
		b.WriteString("\n\n")
	}

	if strings.TrimSpace(plan) != "" {
		b.WriteString("### Plan\n\n")
		b.WriteString(
			textutil.TruncateStringBytes(
				plan,
				5000,
			),
		)
		b.WriteString("\n\n")
	}

	if strings.TrimSpace(processLog) != "" {
		b.WriteString("### Execution Log\n\n")
		b.WriteString(
			textutil.TruncateStringBytes(
				processLog,
				7000,
			),
		)
		b.WriteString("\n\n")
	}

	b.WriteString("### Session\n\n")
	b.WriteString(sessionDir)
	b.WriteString("\n\n")

	b.WriteString(
		"LLM reflection was unavailable. " +
			"The report above contains the raw Agent artifacts " +
			"available for manual review.",
	)

	return b.String()
}

func readFileOrEmpty(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

func readAgentGateReports(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}

	var names []string

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if !strings.HasPrefix(
			entry.Name(),
			"gate-",
		) {
			continue
		}

		if !strings.HasSuffix(
			entry.Name(),
			".json",
		) {
			continue
		}

		names = append(names, entry.Name())
	}

	sort.Strings(names)

	var b strings.Builder

	for _, name := range names {
		data, err := os.ReadFile(
			filepath.Join(dir, name),
		)
		if err != nil {
			continue
		}

		fmt.Fprintf(
			&b,
			"%s:\n%s\n---\n",
			name,
			string(data),
		)
	}

	return b.String()
}

func extractAgentGoal(content string) string {
	lines := strings.Split(
		content,
		"\n",
	)

	for i, line := range lines {
		if strings.TrimSpace(line) ==
			"## Original request" {

			for _, next := range lines[i+1:] {
				next = strings.TrimSpace(next)

				if next == "" {
					continue
				}

				if strings.HasPrefix(
					next,
					"## ",
				) {
					return ""
				}

				return next
			}
		}
	}

	return ""
}

func parseAgentLessons(
	response string,
) []string {
	var lessons []string

	for _, line := range strings.Split(
		response,
		"\n",
	) {

		line = strings.TrimSpace(line)

		if !strings.HasPrefix(
			line,
			"LESSON:",
		) {
			continue
		}

		lesson := strings.TrimSpace(
			strings.TrimPrefix(
				line,
				"LESSON:",
			),
		)

		if lesson != "" {
			lessons = append(
				lessons,
				lesson,
			)
		}

		if len(lessons) >= 5 {
			break
		}
	}

	return lessons
}
