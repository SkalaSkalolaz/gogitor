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
	StatePath     string
	FinalGatePath string
}

type agentSessionState struct {
	Version           int        `json:"version"`
	Task              string     `json:"task"`
	Depth             AgentDepth `json:"depth"`
	StartedAt         time.Time  `json:"started_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	PreTaskHead       string     `json:"pre_task_head,omitempty"`
	CurrentSubtask    int        `json:"current_subtask"`
	CompletedSubtasks int        `json:"completed_subtasks"`
	TotalSubtasks     int        `json:"total_subtasks"`
	Status            string     `json:"status"`
	GitCommit         string     `json:"git_commit,omitempty"`
	UndoCommit        string     `json:"undo_commit,omitempty"`
	ResumedFrom       string     `json:"resumed_from,omitempty"`
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
		StatePath:     filepath.Join(dir, "state.json"),
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

func saveAgentState(
	session *agentSession,
	state *agentSessionState,
) error {
	if session == nil || state == nil {
		return nil
	}

	state.UpdatedAt = time.Now()

	data, err := json.MarshalIndent(
		state,
		"",
		"  ",
	)
	if err != nil {
		return err
	}

	tmp := session.StatePath + ".tmp"

	if err := os.WriteFile(
		tmp,
		data,
		0o644,
	); err != nil {
		return err
	}

	return os.Rename(
		tmp,
		session.StatePath,
	)
}

func loadAgentState(
	dir string,
) (*agentSessionState, error) {
	path := filepath.Join(
		dir,
		"state.json",
	)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var state agentSessionState

	if err := json.Unmarshal(
		data,
		&state,
	); err != nil {
		return nil, err
	}

	return &state, nil
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
changedFiles []string,
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

    if len(changedFiles) > 0 && !hasGoFiles(changedFiles) {
    report.Lint = true
    report.LintInstalled = true
    report.Warnings = append(
    report.Warnings,
    "lint gate skipped: only non-Go files were changed",
    )
    } else if _, err := exec.LookPath("golangci-lint"); err != nil {
    report.LintInstalled = false
    report.Warnings = append(
    report.Warnings,
    "golangci-lint is not installed; lint gate skipped",
    )
    } else {
    report.LintInstalled = true
    // Собираем базовую линию: линт ДО изменений подзадачи
    baselineIssues := s.getLintBaseline(ctx, sandbox)
    // Линт ПОСЛЕ изменений
    newIssues, lintOut, lintErr :=
    s.Runner.LintWithBaseline(ctx, sandbox, baselineIssues)
    report.LintIssues = len(newIssues)
    if lintErr != nil && len(newIssues) > 0 {
    report.Lint = false
    report.Errors = append(
    report.Errors,
    fmt.Sprintf("lint failed (%d NEW issues): %s", len(newIssues), trim(lintOut, 3000)),
    )
    } else if lintErr != nil && len(newIssues) == 0 {
    // Ошибка линта, но все проблемы предсуществующие
    report.Lint = true
    report.Warnings = append(
    report.Warnings,
    fmt.Sprintf("lint returned pre-existing issues only; %d total, 0 new", len(baselineIssues)),
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

func (s *Service) findLatestResumableAgentSession() (
	string,
	*agentSessionState,
	error,
) {
	baseDir := filepath.Join(
		s.Cfg.WorkDir,
		".gogitor",
		"agent",
	)

	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return "", nil, err
	}

	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]

		if !entry.IsDir() {
			continue
		}

		dir := filepath.Join(
			baseDir,
			entry.Name(),
		)

		state, err :=
			loadAgentState(dir)

		if err != nil {
			continue
		}

		switch state.Status {
		case "failed", "running":
			return dir, state, nil
		}
	}

	return "",
		nil,
		fmt.Errorf(
			"no resumable agent session found",
		)
}

func loadAgentPlan(
	dir string,
) (*fullPlan, error) {
	data, err := os.ReadFile(
		filepath.Join(
			dir,
			"plan.json",
		),
	)

	if err != nil {
		return nil, err
	}

	var plan fullPlan

	if err := json.Unmarshal(
		data,
		&plan,
	); err != nil {
		return nil, err
	}

	return &plan, nil
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

	prompt = s.appendProjectInstructions(prompt)
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

	prompt = s.appendProjectInstructions(prompt)
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

func (s *Service) ExecuteAgentResume(
	ctx context.Context,
	emit func(domain.Event),
) domain.Result {
	dir, state, err :=
		s.findLatestResumableAgentSession()

	if err != nil {
		return domain.Result{
			Success: false,
			Mode:    "agent-resume",
			Errors: []string{
				err.Error(),
			},
		}
	}

    plan, err :=
    loadAgentPlan(dir)
    if err != nil {
    return domain.Result{
    Success: false,
    Mode:    "agent-resume",
    Errors: []string{
    fmt.Sprintf(
    "cannot load saved agent plan: %v",
    err,
    ),
    },
    }
    }
    // Проверяем, не вызвана ли ошибка предсуществующими проблемами проекта.
    // Если последняя ошибка — только предсуществующие линт-проблемы,
    // предлагаем пользователю их исправить перед повтором.
    if state.Status == "failed" {
    lastGateData, gateErr := os.ReadFile(
    filepath.Join(dir, fmt.Sprintf("gate-task-%02d.json", state.CurrentSubtask)),
    )
    if gateErr == nil {
    var lastGate agentGateReport
    if json.Unmarshal(lastGateData, &lastGate) == nil {
    hasPreexistingOnly := true
    for _, e := range lastGate.Errors {
    if strings.Contains(e, "NEW") {
    hasPreexistingOnly = false
    break
    }
    }
    if hasPreexistingOnly && len(lastGate.Errors) > 0 {
    sendEvent(
    emit,
    domain.EventWarn,
    "Previous failure was caused by pre-existing project issues. "+
    "Consider fixing them first: ':fix' or ':test lint'. "+
    "Proceeding with resume anyway.",
    )
    }
    }
    }
    }

	if state.CompletedSubtasks >= len(plan.Subtasks) {
		return domain.Result{
			Success: false,
			Mode:    "agent-resume",
			Errors: []string{
				"agent session has no unfinished subtasks",
			},
		}
	}

	sendEvent(
		emit,
		domain.EventAgent,
		fmt.Sprintf(
			"Resuming Agent session: %s",
			filepath.Base(dir),
		),
	)

	result := s.executeAgentFull(
		ctx,
		state.Task,
		"",
		Options{
			Mode:              "agent",
			AgentDepth:        state.Depth,
			AgentResumePlan:   plan,
			AgentResumeFrom:   state.CompletedSubtasks,
			AgentResumeSource: dir,
		},
		emit,
	)

	result.Mode = "agent"
	return result
}

func (s *Service) findLatestUndoableAgentSession() (
	string,
	*agentSessionState,
	error,
) {
	baseDir := filepath.Join(
		s.Cfg.WorkDir,
		".gogitor",
		"agent",
	)

	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return "", nil, err
	}

	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]

		if !entry.IsDir() {
			continue
		}

		dir := filepath.Join(
			baseDir,
			entry.Name(),
		)

		state, err :=
			loadAgentState(dir)

		if err != nil {
			continue
		}

		if state.Status != "completed" {
			continue
		}

		if strings.TrimSpace(
			state.GitCommit,
		) == "" {
			continue
		}

		return dir, state, nil
	}

	return "",
		nil,
		fmt.Errorf(
			"no completed Agent session with Git commit found",
		)
}

func formatAgentTaskReport(
	result domain.Result,
	depth AgentDepth,
	completedSubtasks int,
	totalSubtasks int,
) string {
	var b strings.Builder

	if result.Success {
		b.WriteString("AGENT COMPLETED\n")
	} else {
		b.WriteString("AGENT FAILED\n")
	}

	b.WriteString("\n")

	fmt.Fprintf(
		&b,
		"Profile: %s\n",
		depth,
	)

    fmt.Fprintf(
    	&b,
    	"Subtasks: %d/%d\n",
    	completedSubtasks,
    	totalSubtasks,
    )
	if result.Iterations > 0 {
		fmt.Fprintf(
			&b,
			"Iterations: %d\n",
			result.Iterations,
		)
	}

	if len(result.FilesCreated) > 0 ||
		len(result.FilesModified) > 0 ||
		len(result.FilesPatched) > 0 ||
		len(result.FilesFullRewritten) > 0 {

		b.WriteString("\nFILES\n")

		if len(result.FilesCreated) > 0 {
			fmt.Fprintf(
				&b,
				"Created: %d\n",
				len(result.FilesCreated),
			)
		}

		if len(result.FilesModified) > 0 {
			fmt.Fprintf(
				&b,
				"Modified: %d\n",
				len(result.FilesModified),
			)
		}

		if len(result.FilesPatched) > 0 {
			fmt.Fprintf(
				&b,
				"Patched (DIFF): %d\n",
				len(result.FilesPatched),
			)
		}

		if len(result.FilesFullRewritten) > 0 {
			fmt.Fprintf(
				&b,
				"Full rewritten: %d\n",
				len(result.FilesFullRewritten),
			)
		}
	}

	b.WriteString("\nTESTS\n")

	if result.Tests.Skipped {
		b.WriteString("Skipped\n")
	} else {
		fmt.Fprintf(
			&b,
			"Passed: %d\n",
			result.Tests.Passed,
		)

		fmt.Fprintf(
			&b,
			"Failed: %d\n",
			result.Tests.Failed,
		)

		if result.Tests.Coverage > 0 {
			fmt.Fprintf(
				&b,
				"Coverage: %.1f%%\n",
				result.Tests.Coverage,
			)
		}
	}

	if depth == AgentDepthDeep {
		g := result.QualityGates

		b.WriteString("\nQUALITY GATES\n")

		fmt.Fprintf(
			&b,
			"Build: %s\n",
			reportMark(g.Build),
		)

		fmt.Fprintf(
			&b,
			"Tests: %s\n",
			reportMark(g.Tests),
		)

		fmt.Fprintf(
			&b,
			"Vet: %s\n",
			reportMark(g.Vet),
		)

		fmt.Fprintf(
			&b,
			"Gofmt: %s\n",
			reportMark(g.Gofmt),
		)

		if g.LintInstalled {
			fmt.Fprintf(
				&b,
				"Lint: %s (%d issues)\n",
				reportMark(g.Lint),
				g.LintIssues,
			)
		} else {
			b.WriteString(
				"Lint: skipped (not installed)\n",
			)
		}
	}

	b.WriteString("\nVERIFICATION\n")

	if result.Success {
		b.WriteString("Final status: PASS\n")
	} else {
		b.WriteString("Final status: FAIL\n")
	}

	if result.GitCommit != "" {
		fmt.Fprintf(
			&b,
			"Git commit: %s\n",
			result.GitCommit,
		)
	}

	if len(result.Warnings) > 0 {
		b.WriteString("\nWARNINGS\n")

		for _, warning := range result.Warnings {
			fmt.Fprintf(
				&b,
				"- %s\n",
				warning,
			)
		}
	}

	if len(result.Errors) > 0 {
		b.WriteString("\nERRORS\n")

		for _, failure := range result.Errors {
			fmt.Fprintf(
				&b,
				"- %s\n",
				failure,
			)
		}
	}

	return strings.TrimSpace(
		b.String(),
	)
}

func reportMark(ok bool) string {
	if ok {
		return "PASS"
	}

	return "FAIL"
}

// getLintBaseline возвращает проблемы линта до начала подзадачи.
// Вызывается на копии песочницы ДО применения изменений.
func (s *Service) getLintBaseline(ctx context.Context, sandbox string) []runner.LintIssue {
baselineSandbox, err := s.WS.PrepareSandbox(ctx)
if err != nil {
return nil
}
defer os.RemoveAll(baselineSandbox)
if _, err := exec.LookPath("golangci-lint"); err != nil {
return nil
}
lintOut, _ := s.Runner.Lint(ctx, baselineSandbox)
return runner.ParseLintOutput(lintOut)
}

// hasGoFiles проверяет, есть ли в списке изменённых файлов .go файлы.
func hasGoFiles(files []string) bool {
	for _, f := range files {
		if strings.HasSuffix(strings.TrimSpace(f), ".go") {
			return true
		}
	}
	return false
}
