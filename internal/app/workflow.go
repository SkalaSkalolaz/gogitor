package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gogitor/internal/llm"
    "gogitor/internal/i18n"
    "gogitor/internal/github"
	"gogitor/internal/agent"
	"gogitor/internal/domain"
	"gogitor/internal/prompts"
	"gogitor/internal/runner"
	"gogitor/internal/security"
)

// workflowPRDTask — атомарная задача workflow.
type workflowPRDTask struct {
	ID           int      `json:"id"`
	Title        string   `json:"title"`
	Instructions string   `json:"instructions"`
	Acceptance   []string `json:"acceptance,omitempty"`
	Status       string   `json:"status"`
	Commit       string   `json:"commit,omitempty"`
}

// workflowPRD — упрощённый PRD для workflow-lite.
type workflowPRD struct {
	Version int               `json:"version"`
	Goal    string            `json:"goal"`
	Tasks   []workflowPRDTask `json:"tasks"`
}

// workflowGateReport — отчёт quality gates для одной задачи workflow.
type workflowGateReport struct {
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
	Warnings      []string `json:"warnings,omitempty"`
	Errors        []string `json:"errors,omitempty"`
}

// executeWorkflowLite — workflow/harness режим второго этапа.
func (s *Service) executeWorkflowLite(
	ctx context.Context,
	query string,
	opts Options,
	emit func(domain.Event),
) domain.Result {
	final := domain.Result{
		Success: true,
		Mode:    "workflow",
		DryRun:  opts.DryRun,
	}

	// Dry-run в workflow-lite пока не имеет смысла, потому что задачи
	// должны применяться последовательно. Поэтому деградируем в simple.
	if opts.DryRun {
		sendEvent(
			emit,
			domain.EventWarn,
			"Workflow dry-run is not fully supported; falling back to simple execution",
		)
		return s.executeSimple(ctx, query, opts, emit)
	}

	preTaskHead := s.captureHead(ctx)

	stamp := time.Now().Format("20060102-150405")
	baseDir := s.workflowBaseDir(opts, emit)
	workflowDir := filepath.Join(baseDir, stamp)

	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		sendEvent(
			emit,
			domain.EventWarn,
			fmt.Sprintf(
				"Cannot create workflow dir %s: %v; falling back to .gogitor/workflow",
				workflowDir,
				err,
			),
		)

		workflowDir = filepath.Join(s.Cfg.WorkDir, ".gogitor", "workflow", stamp)
		if err2 := os.MkdirAll(workflowDir, 0o755); err2 != nil {
			sendEvent(
				emit,
				domain.EventWarn,
				fmt.Sprintf(
					"Cannot create fallback workflow dir %s: %v; falling back to simple execution",
					workflowDir,
					err2,
				),
			)
			return s.executeSimple(ctx, query, opts, emit)
		}
	}

	sendEvent(
		emit,
		domain.EventLog,
		fmt.Sprintf("Workflow artifacts dir: %s", workflowDir),
	)

	inboxPath := filepath.Join(workflowDir, "inbox.md")
	researchPath := filepath.Join(workflowDir, "research.md")
	planPath := filepath.Join(workflowDir, "plan.md")
	prdPath := filepath.Join(workflowDir, "prd.json")
	processPath := filepath.Join(workflowDir, "process.md")

	// ─── inbox.md ─────────────────────────────────────────────
	inbox := formatWorkflowInbox(query, opts.InterviewAnswers)
	if err := writeWorkflowFile(inboxPath, inbox); err != nil {
		sendEvent(emit, domain.EventWarn, fmt.Sprintf("Cannot write inbox.md: %v", err))
	}

	// ─── research.md ─────────────────────────────────────────
	maxFiles, maxBytes := s.contextLimits()
	targetFiles := extractTargetFiles(query)

	researchContext := s.WS.BuildSmartContext(
		query,
		targetFiles,
		maxFiles/4,
		maxBytes/4,
	)

	if strings.TrimSpace(researchContext) == "" {
		researchContext = "No additional project context selected."
	}

	research := formatWorkflowResearch(query, targetFiles, researchContext)
	if err := writeWorkflowFile(researchPath, research); err != nil {
		sendEvent(emit, domain.EventWarn, fmt.Sprintf("Cannot write research.md: %v", err))
	}

	// ─── plan.md / prd.json ──────────────────────────────────
	mem := loadAgentMemory(s.Cfg.WorkDir)
	plan := s.planFullOrFallback(ctx, query, "", mem, emit)
	plan = validateWorkflowPlan(plan, query)

	prd := buildWorkflowPRD(plan)
	if err := validateWorkflowPRD(prd); err != nil {
		sendEvent(
			emit,
			domain.EventWarn,
			fmt.Sprintf("PRD validation failed: %v; rebuilding fallback PRD", err),
		)

		plan = validateWorkflowPlan(&fullPlan{}, query)
		prd = buildWorkflowPRD(plan)
	}

	planMarkdown := formatWorkflowPlanMarkdown(query, plan)
	if err := writeWorkflowFile(planPath, planMarkdown); err != nil {
		sendEvent(emit, domain.EventWarn, fmt.Sprintf("Cannot write plan.md: %v", err))
	}

	if err := saveWorkflowPRD(prdPath, prd); err != nil {
		sendEvent(emit, domain.EventWarn, fmt.Sprintf("Cannot write prd.json: %v", err))
	}

	appendWorkflowProcess(processPath, fmt.Sprintf("Workflow started: %s", stamp))
	appendWorkflowProcess(processPath, "Goal: "+plan.Goal)
	appendWorkflowProcess(processPath, "Artifacts dir: "+workflowDir)

	// ─── План в TUI ──────────────────────────────────────────
	planItems := make([]string, 0, len(plan.Subtasks))
	for _, sub := range plan.Subtasks {
		planItems = append(planItems, sub.Task)
	}

	// ─── Итеративное планирование: вопросы по плану ──────────
	if s.Cfg.WorkflowAskUser {
		subtaskTexts := make([]string, len(plan.Subtasks))
		for i, st := range plan.Subtasks {
			subtaskTexts[i] = st.Task
		}
		reviewPrompt := prompts.WorkflowPlanReview(query, plan.Goal, subtaskTexts)
		var reviewResult struct {
			Questions []struct {
				ID       int    `json:"id"`
				Question string `json:"question"`
				Concern  string `json:"concern"`
			} `json:"questions"`
			Risks    []string `json:"risks"`
			Approved bool     `json:"approved"`
		}
		reviewErr := s.sendAgentJSON(
			ctx,
			agent.RolePlanner,
			agent.PriorityHigh,
			"workflow plan review",
			reviewPrompt,
			&reviewResult,
		)
		if reviewErr == nil && len(reviewResult.Questions) > 0 && !reviewResult.Approved {
			// Есть вопросы — показываем пользователю и ждём ответа.
			var questionsText strings.Builder
			if DetectLanguage() == "ru" {
				questionsText.WriteString("## 📋 Рецензирование плана\n\n")
				questionsText.WriteString("План сгенерирован. Перед выполнением у меня есть вопросы:\n\n")
			} else {
				questionsText.WriteString("## 📋 Plan Review\n\n")
				questionsText.WriteString("The plan has been generated. Before execution, I have some questions:\n\n")
			}
			for _, q := range reviewResult.Questions {
				fmt.Fprintf(&questionsText, "**Q%d:** %s\n", q.ID, q.Question)
				if q.Concern != "" {
					fmt.Fprintf(&questionsText, "  _(concern: %s)_\n", q.Concern)
				}
				questionsText.WriteString("\n")
			}
			if len(reviewResult.Risks) > 0 {
				if DetectLanguage() == "ru" {
					questionsText.WriteString("**Выявленные риски:**\n")
				} else {
					questionsText.WriteString("**Identified risks:**\n")
				}
				for _, r := range reviewResult.Risks {
					questionsText.WriteString("- ⚠ " + r + "\n")
				}
				questionsText.WriteString("\n")
			}
			questionsText.WriteString("---\n")
			if DetectLanguage() == "ru" {
				questionsText.WriteString(
					"Ответьте на вопросы, или введите `go`/`ok`/`skip` для продолжения с текущим планом.\n")
			} else {
				questionsText.WriteString(
					"Answer the questions, or type `go`/`ok`/`skip` to proceed with the current plan.\n")
			}

			// Сохраняем состояние для продолжения после ответа пользователя.
			s.pendingPlanReview = &PendingPlanReview{
				Task:        query,
				Plan:        plan,
				PRD:         prd,
				WorkflowDir: workflowDir,
				PlanPath:    planPath,
				PrdPath:     prdPath,
				ProcessPath: processPath,
				PreTaskHead: preTaskHead,
				Opts:        opts,
				CreatedAt:   time.Now(),
			}

			appendWorkflowProcess(processPath, "Plan review: awaiting user feedback")
			return domain.Result{
				Success:           true,
				Mode:              "workflow-plan-review",
				Response:          questionsText.String(),
				AwaitingSelection: true,
				RefinedTask:       query,
			}
		}
		// Если вопросов нет, план одобрен, или LLM не смог сгенерировать
		// рецензию — продолжаем с текущим планом.
		if reviewErr != nil {
			sendEvent(emit, domain.EventWarn,
				fmt.Sprintf("Plan review failed (non-fatal): %v", reviewErr))
		}
	}

	return s.executeWorkflowTasks(
		ctx, query, plan, prd, workflowDir,
		planPath, prdPath, processPath, preTaskHead,
		opts, &final, emit,
	)
}

// executeWorkflowTasks выполняет задачи плана последовательно.
// Вызывается из executeWorkflowLite или ContinueWorkflowPlanReview.
func (s *Service) executeWorkflowTasks(
	ctx context.Context,
	query string,
	plan *fullPlan,
	prd *workflowPRD,
	workflowDir string,
	planPath string,
	prdPath string,
	processPath string,
	preTaskHead string,
	opts Options,
	final *domain.Result,
	emit func(domain.Event),
) domain.Result {
	// ─── План в TUI ──────────────────────────────────────────
	planItems := make([]string, 0, len(plan.Subtasks))
	for _, sub := range plan.Subtasks {
		planItems = append(planItems, sub.Task)
	}
	sendPlanBoard(emit, plan.Goal, plan.Acceptance, planItems)

	// ─── Agent memory и профиль модели (для Reviewer/Verifier) ────
	mem := loadAgentMemory(s.Cfg.WorkDir)
	profile := s.modelProfile()
	isSmallModel := profile == modelProfileSmall

	// ─── Выполнение задач ────────────────────────────────────
	for i, sub := range plan.Subtasks {
		taskEmit := workflowTaskEmitter(emit, processPath, i+1, sub.Task)
		sendPlanStatus(
			taskEmit,
			i+1,
			len(plan.Subtasks),
			sub.Task,
			domain.PlanRunning,
			"",
		)
		appendWorkflowProcess(
			processPath,
			fmt.Sprintf("Task %d started: %s", i+1, sub.Task),
		)

		// ─── Автопоиск для подзадачи ─────────────────────────────
		searchContext := ""
		if sub.NeedsSearch && s.Cfg.AutoSearch && s.SafeSearch != nil {
			searchContent, searchErr := s.searchForSubtask(ctx, sub.Task, taskEmit)
			if searchErr == nil && searchContent != "" {
				searchContext = searchContent
			}
			// При ошибке поиска продолжаем без него (non-fatal).
		}

		taskPrompt := formatWorkflowTask(query, plan, sub, i, len(plan.Subtasks), searchContext)
		subOpts := opts
		subOpts.ProgressItem = i + 1
		subOpts.ProgressTotal = len(plan.Subtasks)
		subOpts.NoCommit = true

		res := s.executeSimple(ctx, taskPrompt, subOpts, taskEmit)
		final.Iterations += res.Iterations
		mergeWorkflowResult(final, res)

		if ctx.Err() != nil {
			prd.Tasks[i].Status = "failed"
			_ = saveWorkflowPRD(prdPath, prd)
			sendPlanStatus(taskEmit, i+1, len(plan.Subtasks), sub.Task,
				domain.PlanFailed, "context canceled")
			appendWorkflowProcess(processPath, fmt.Sprintf("Task %d canceled", i+1))
			final.Success = false
			final.AddError(ctx.Err().Error())
			return *final
		}

		if !res.Success {
			prd.Tasks[i].Status = "failed"
			_ = saveWorkflowPRD(prdPath, prd)
			note := truncate(strings.Join(res.Errors, "; "), 180)
			sendPlanStatus(taskEmit, i+1, len(plan.Subtasks), sub.Task,
				domain.PlanFailed, note)
			appendWorkflowProcess(processPath,
				fmt.Sprintf("Task %d failed: %s", i+1, strings.Join(res.Errors, "; ")))
			final.Success = false
			final.Errors = append(final.Errors, res.Errors...)
			final.AddWarning("previous workflow tasks may already have been committed")
			return *final
		}

		// ─── Reviewer (из Agent) ────────────────────────────────────
		if !isSmallModel {
			changedFiles := len(res.FilesCreated) + len(res.FilesModified) +
				len(res.FilesPatched) + len(res.FilesFullRewritten)
			if changedFiles > 0 {
				sendEvent(taskEmit, domain.EventAgent, "current stage: reviewer")
				review, reviewErr := s.runReviewer(
					ctx, query, sub.Task, res, mem, taskEmit,
				)
				if reviewErr == nil && !review.Approved && len(review.CriticalIssues) > 0 {
					issues := strings.Join(review.CriticalIssues, "; ")
					sendEvent(taskEmit, domain.EventWarn,
						"Reviewer found critical issues: "+issues)

					fixTask := buildReviewFixTask(sub.Task, review)
					sendEvent(taskEmit, domain.EventAgent,
						"current stage: coder (correction of reviewer comments)")
					fixRes := s.executeSimple(ctx, fixTask, subOpts, taskEmit)
					final.Iterations += fixRes.Iterations
					mergeWorkflowResult(final, fixRes)
					if !fixRes.Success {
						final.Warnings = append(final.Warnings,
							"reviewer fix failed: "+strings.Join(fixRes.Errors, "; "))
					}
				}
			}
		}

		// ─── Quality gates ───────────────────────────────────
		gate := s.runWorkflowQualityGates(ctx, taskEmit, i+1)
		gatePath := filepath.Join(
			workflowDir,
			fmt.Sprintf("gate-report-task-%02d.json", i+1),
		)
		if err := saveWorkflowGateReport(gatePath, &gate); err != nil {
			sendEvent(taskEmit, domain.EventWarn,
				fmt.Sprintf("Cannot write gate report: %v", err))
		}
		final.Warnings = append(final.Warnings, gate.Warnings...)
		appendWorkflowProcess(processPath, fmt.Sprintf(
			"Task %d gates: passed=%v build=%v tests=%v vet=%v gofmt=%v lint=%v issues=%d",
			i+1, gate.Passed, gate.Build, gate.Tests, gate.Vet,
			gate.Gofmt, gate.Lint, gate.LintIssues))

		if !gate.Passed {
			prd.Tasks[i].Status = "failed"
			_ = saveWorkflowPRD(prdPath, prd)
			sendPlanStatus(taskEmit, i+1, len(plan.Subtasks), sub.Task,
				domain.PlanFailed, "quality gates failed")
			appendWorkflowProcess(processPath, fmt.Sprintf(
				"Task %d failed quality gates: %s", i+1, strings.Join(gate.Errors, "; ")))
			final.Success = false
			final.Errors = append(final.Errors, gate.Errors...)
			final.AddWarning("changes from the failed task remain applied but uncommitted")
			return *final
		}

		// ─── Отдельный коммит задачи ─────────────────────────
		prd.Tasks[i].Status = "done"
		_ = saveWorkflowPRD(prdPath, prd)
		appendWorkflowProcess(processPath,
			fmt.Sprintf("Task %d done; quality gates passed", i+1))

		commitHash := ""
		if !opts.NoCommit && s.Cfg.AutoGitCommit {
			commitMessage := workflowCommitMessage(i+1, len(plan.Subtasks), sub.Task)
			hash, err := s.commitWorkflowTask(ctx, commitMessage, taskEmit)
			if err != nil {
				final.AddWarning(fmt.Sprintf("workflow task %d commit failed: %v", i+1, err))
			} else {
				commitHash = hash
			}
		}
		if commitHash != "" {
			prd.Tasks[i].Commit = commitHash
			_ = saveWorkflowPRD(prdPath, prd)
			appendWorkflowProcess(processPath,
				fmt.Sprintf("Task %d commit: %s", i+1, commitHash))
		}

		note := ""
		if commitHash != "" {
			note = "commit " + commitHash
		} else if !opts.NoCommit && s.Cfg.AutoGitCommit {
			note = "no commit created"
		} else {
			note = "commit disabled"
		}
		sendPlanStatus(taskEmit, i+1, len(plan.Subtasks), sub.Task,
			domain.PlanDone, note)
		appendWorkflowProcess(processPath, fmt.Sprintf(
			"Task %d completed: commit=%s created=%d modified=%d patched=%d rewritten=%d",
			i+1, commitHash, len(res.FilesCreated), len(res.FilesModified),
			len(res.FilesPatched), len(res.FilesFullRewritten)))
	}

	// ─── Verifier (из Agent) ────────────────────────────────────
	if !isSmallModel {
		sendEvent(emit, domain.EventAgent, "current stage: verifier")
		verification, verErr := s.runVerifier(
			ctx, query, plan, *final, mem, emit,
		)
		if verErr == nil && !verification.Completed {
			missing := strings.Join(verification.Missing, "; ")
			sendEvent(emit, domain.EventWarn,
				"Verifier: task not fully completed: "+missing)

			if strings.TrimSpace(verification.FixTask) != "" {
				sendEvent(emit, domain.EventAgent,
					"current stage: coder (fix verifier)")
				fixOpts := opts
				fixOpts.NoCommit = true
				fixRes := s.executeSimple(ctx, verification.FixTask, fixOpts, emit)
				final.Iterations += fixRes.Iterations
				mergeWorkflowResult(final, fixRes)
				if !fixRes.Success {
					final.Success = false
					final.Errors = append(final.Errors, fixRes.Errors...)
				}
			}
		}
	}

	// Сохраняем память агента (решения, уроки)
	_ = mem.save(s.Cfg.WorkDir)

	sendPlanSummary(emit, domain.PlanDone, len(plan.Subtasks), len(plan.Subtasks))
	if final.Success {
		final.Response = i18n.T("Workflow completed %d task(s).", len(plan.Subtasks))
		appendWorkflowProcess(processPath, "Workflow completed")
	}
	final.PreTaskHead = preTaskHead
	s.lastPreTaskHead = preTaskHead
	if !opts.DryRun {
		final.CumulativeDiff = s.captureCumulativeDiff(ctx, preTaskHead)
	}
	return *final
}

// workflowBaseDir определяет базовый каталог для артефактов workflow.
func (s *Service) workflowBaseDir(opts Options, emit func(domain.Event)) string {
	desired := strings.TrimSpace(opts.WorkflowDir)

	if desired == "" {
		desired = strings.TrimSpace(s.Cfg.WorkflowDir)
	}

	if desired == "" {
		desired = filepath.Join(".gogitor", "workflow")
	}

	full, err := security.SafeJoin(s.Cfg.WorkDir, desired)
	if err != nil {
		sendEvent(
			emit,
			domain.EventWarn,
			fmt.Sprintf(
				"Invalid workflow dir %s: %v; using .gogitor/workflow",
				desired,
				err,
			),
		)

		return filepath.Join(s.Cfg.WorkDir, ".gogitor", "workflow")
	}

	return full
}

// validateWorkflowPlan дополнительно очищает и ограничивает план.
func validateWorkflowPlan(plan *fullPlan, originalTask string) *fullPlan {
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

		if len(clean) >= 5 {
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

// validateWorkflowPRD проверяет PRD перед выполнением.
func validateWorkflowPRD(prd *workflowPRD) error {
	if prd == nil {
		return fmt.Errorf("prd is nil")
	}

	if strings.TrimSpace(prd.Goal) == "" {
		return fmt.Errorf("empty prd goal")
	}

	if len(prd.Tasks) == 0 {
		return fmt.Errorf("prd has no tasks")
	}

	if len(prd.Tasks) > 5 {
		return fmt.Errorf("prd has too many tasks: %d", len(prd.Tasks))
	}

	for i := range prd.Tasks {
		if strings.TrimSpace(prd.Tasks[i].Title) == "" {
			return fmt.Errorf("task %d has empty title", i+1)
		}

		if strings.TrimSpace(prd.Tasks[i].Instructions) == "" {
			return fmt.Errorf("task %d has empty instructions", i+1)
		}

		if isRuntimeOnlySubtask(prd.Tasks[i].Instructions) {
			return fmt.Errorf("task %d is runtime-only", i+1)
		}

		if strings.TrimSpace(prd.Tasks[i].Status) == "" {
			prd.Tasks[i].Status = "pending"
		}
	}

	return nil
}

// buildWorkflowPRD создаёт PRD из плана.
func buildWorkflowPRD(plan *fullPlan) *workflowPRD {
	prd := &workflowPRD{
		Version: 1,
		Goal:    plan.Goal,
	}

	for i, sub := range plan.Subtasks {
		prd.Tasks = append(prd.Tasks, workflowPRDTask{
			ID:           i + 1,
			Title:        sub.Task,
			Instructions: sub.Task,
			Acceptance:   sub.Acceptance,
			Status:       "pending",
		})
	}

	return prd
}

// saveWorkflowPRD сохраняет PRD в JSON.
func saveWorkflowPRD(path string, prd *workflowPRD) error {
	data, err := json.MarshalIndent(prd, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o644)
}

// saveWorkflowGateReport сохраняет отчёт quality gates.
func saveWorkflowGateReport(path string, report *workflowGateReport) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o644)
}

// runWorkflowQualityGates выполняет quality gates в sandbox.
func (s *Service) runWorkflowQualityGates(
	ctx context.Context,
	emit func(domain.Event),
	taskIndex int,
) workflowGateReport {
	report := workflowGateReport{
		TaskIndex: taskIndex,
		Gofmt:     true,
		Lint:      true,
	}

	sandbox, err := s.WS.PrepareSandbox(ctx)
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("cannot prepare sandbox: %v", err))
		return report
	}
	defer os.RemoveAll(sandbox)

	sendEvent(emit, domain.EventLog, "Running workflow quality gates")

	if ctx.Err() != nil {
		report.Errors = append(report.Errors, ctx.Err().Error())
		return report
	}

	// ─── Адаптивность для малых моделей ────────────────────────
	profile := s.modelProfile()
	isSmallModel := profile == modelProfileSmall
	isMediumModel := profile == modelProfileMedium

	// ─── go build ────────────────────────────────────────────
	if err := s.Runner.Build(ctx, sandbox); err != nil {
		report.Errors = append(report.Errors, "go build failed: "+trim(err.Error(), 2000))
		return report
	}

	report.Build = true

	// ─── go test ─────────────────────────────────────────────
	tests, err := s.Runner.Test(ctx, sandbox)

	report.TestsPassed = tests.Passed
	report.TestsFailed = tests.Failed
	report.Coverage = tests.Coverage

	if err != nil {
		report.Errors = append(report.Errors, "go test failed: "+trim(err.Error(), 2000))
	}

	if tests.Failed > 0 {
		report.Errors = append(report.Errors, runner.FormatFeedback(tests))
	}

	if err == nil && tests.Failed == 0 {
		report.Tests = true
	}

	if tests.Skipped {
		report.Tests = true
		report.Warnings = append(report.Warnings, "tests skipped: no test files")
	}

	// ─── go vet ──────────────────────────────────────────────
	vetOut, err := s.Runner.Vet(ctx, sandbox)
	if err != nil {
		if isSmallModel || isMediumModel {
			// Для моделей ≤31B vet — предупреждение, не блокировка.
			report.Vet = true
			report.Warnings = append(report.Warnings,
				"go vet found issues (non-blocking for small/medium model): "+trim(vetOut, 1000))
		} else {
			report.Errors = append(report.Errors, "go vet failed: "+trim(vetOut, 2000))
		}
	} else {
		report.Vet = true
	}

	// ─── gofmt check ─────────────────────────────────────────
	if isSmallModel {
		report.Warnings = append(report.Warnings, "gofmt gate skipped (small model profile)")
	} else if _, err := exec.LookPath("gofmt"); err != nil {
		report.Warnings = append(report.Warnings, "gofmt not installed; gofmt gate skipped")
	} else {
		out, err := runWorkflowCommand(ctx, sandbox, "gofmt", "-l", ".")
		if err != nil {
			report.Warnings = append(report.Warnings, "gofmt check failed: "+trim(err.Error(), 500))
		} else {
			out = strings.TrimSpace(out)
			if out != "" {
				report.Gofmt = false
				report.Warnings = append(
					report.Warnings,
					"gofmt found unformatted files: "+trim(out, 1000),
				)
			}
		}
	}

	// ─── golangci-lint ───────────────────────────────────────
	if isSmallModel {
		report.LintInstalled = false
		report.Warnings = append(report.Warnings, "golangci-lint gate skipped (small model profile)")
	} else if _, err := exec.LookPath("golangci-lint"); err != nil {
		report.LintInstalled = false
		report.Warnings = append(report.Warnings, "golangci-lint not installed; lint gate skipped")
	} else {
		report.LintInstalled = true
		lintOut, err := s.Runner.Lint(ctx, sandbox)
		issues := runner.CountLintIssues(lintOut)
		report.LintIssues = issues
		if err != nil && issues > 0 {
			report.Lint = false
			report.Warnings = append(
				report.Warnings,
				fmt.Sprintf("golangci-lint found %d issue(s)", issues),
			)
		} else if err != nil {
			report.Warnings = append(
				report.Warnings,
				"golangci-lint failed: "+trim(err.Error(), 500),
			)
		}
	}

	report.Passed = len(report.Errors) == 0 && report.Build && report.Tests && report.Vet

	for _, w := range report.Warnings {
		sendEvent(emit, domain.EventWarn, w)
	}

	for _, e := range report.Errors {
		sendEvent(emit, domain.EventError, e)
	}

	return report
}

// runWorkflowCommand выполняет короткую вспомогательную команду для gates.
func runWorkflowCommand(ctx context.Context, dir, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir

	out, err := cmd.CombinedOutput()
	output := string(out)

	if ctx.Err() != nil {
		return output, ctx.Err()
	}

	return output, err
}

// commitWorkflowTask создаёт отдельный коммит для задачи workflow.
func (s *Service) commitWorkflowTask(
	ctx context.Context,
	message string,
	emit func(domain.Event),
) (string, error) {
	if err := s.Git.EnsureRepo(ctx, s.Cfg.GitAutoInit); err != nil {
		return "", err
	}

	hash, err := s.Git.AutoCommit(ctx, message)
	if err != nil {
		return "", err
	}

	if hash != "" {
		sendEvent(emit, domain.EventLog, "Git commit created: "+hash)
	}

	return hash, nil
}

// workflowCommitMessage формирует сообщение коммита для задачи workflow.
func workflowCommitMessage(index, total int, title string) string {
	clean := strings.TrimSpace(title)
	clean = strings.ReplaceAll(clean, "", " ")
	clean = strings.ReplaceAll(clean, "", " ")
	clean = truncate(clean, 50)

	return fmt.Sprintf("chore(workflow): task %d/%d %s", index, total, clean)
}

// workflowTaskEmitter оборачивает emit и пишет часть событий в process.md.
func workflowTaskEmitter(
	base func(domain.Event),
	processPath string,
	taskIndex int,
	taskTitle string,
) func(domain.Event) {
	return func(e domain.Event) {
		if base != nil {
			base(e)
		}

		switch e.Type {
		case domain.EventLog, domain.EventWarn, domain.EventError, domain.EventIntent:
			appendWorkflowProcess(
				processPath,
				fmt.Sprintf(
					"task %d (%s): [%s] %s",
					taskIndex,
					truncate(taskTitle, 80),
					e.Type,
					e.Message,
				),
			)

		case domain.EventPlan:
			if e.Plan != nil && e.Plan.Status != "" {
				appendWorkflowProcess(
					processPath,
					fmt.Sprintf(
						"task %d (%s): plan %s %s",
						taskIndex,
						truncate(taskTitle, 80),
						e.Plan.Status,
						e.Message,
					),
				)
			}
		}
	}
}


func formatWorkflowInbox(task string, answers []prompts.WorkflowAnswer) string {
	var b strings.Builder
	b.WriteString("# Task Inbox\n")
	b.WriteString("## Original request\n")
	b.WriteString(strings.TrimSpace(task))
	b.WriteString("\n")
	if len(answers) > 0 {
		b.WriteString("## Interview Q&A\n")
		for _, a := range answers {
			fmt.Fprintf(&b, "**Q%d:** %s\n", a.ID, a.Question)
			fmt.Fprintf(&b, "**A%d:** %s\n\n", a.ID, a.Answer)
		}
	}
	b.WriteString("## Clarified goal\n")
	b.WriteString("Automatically generated from the original request.\n")
	return b.String()
}

func formatWorkflowResearch(task string, targetFiles []string, researchContext string) string {
	var b strings.Builder

	b.WriteString("# Research\n\n")
	b.WriteString("## Task\n\n")
	b.WriteString(strings.TrimSpace(task))
	b.WriteString("\n\n")

	if len(targetFiles) > 0 {
		b.WriteString("## Mentioned files\n\n")

		for _, f := range targetFiles {
			b.WriteString("- " + f + "\n")
		}

		b.WriteString("\n")
	}

	b.WriteString("## Project context\n\n")
	b.WriteString(truncate(researchContext, 20000))
	b.WriteString("\n")

	return b.String()
}

func formatWorkflowPlanMarkdown(originalTask string, plan *fullPlan) string {
	var b strings.Builder

	b.WriteString("# Implementation Plan\n\n")

	b.WriteString("## Original task\n\n")
	b.WriteString(strings.TrimSpace(originalTask))
	b.WriteString("\n\n")

	b.WriteString("## Goal\n\n")
	b.WriteString(plan.Goal)
	b.WriteString("\n\n")

	if len(plan.Acceptance) > 0 {
		b.WriteString("## Acceptance criteria\n\n")

		for _, a := range plan.Acceptance {
			b.WriteString("- " + a + "\n")
		}

		b.WriteString("\n")
	}

	b.WriteString("## Tasks\n\n")

	for i, sub := range plan.Subtasks {
		fmt.Fprintf(&b, "%d. %s\n", i+1, sub.Task)

		if len(sub.Acceptance) > 0 {
			b.WriteString("   Acceptance:\n")

			for _, a := range sub.Acceptance {
				b.WriteString("   - " + a + "\n")
			}
		}

		b.WriteString("\n")
	}

	return b.String()
}

func formatWorkflowTask(
	originalTask string,
	plan *fullPlan,
	sub fullPlanSubtask,
	index int,
	total int,
	searchContext string,
) string {
	var b strings.Builder

	// ── Явная инструкция изоляции для малых моделей (20B–31B) ──
	// Малые модели склонны «додумывать» контекст предыдущих задач
	// и ссылаться на код, который ещё не существует или уже устарел.
	b.WriteString("You are executing ONE atomic workflow task inside an existing Go project.\n\n")
	b.WriteString("=== CONTEXT ISOLATION (MANDATORY) ===\n")
	b.WriteString("- You are executing ONE independent task.\n")
	b.WriteString("- You have NO knowledge of other tasks in this workflow.\n")
	b.WriteString("- Do NOT assume any other subtask was completed.\n")
	b.WriteString("- Do NOT reference code from previous or future tasks.\n")
	b.WriteString("- Work ONLY with files that exist in the project right now.\n")
	b.WriteString("- If a file does not exist yet, do NOT reference or import it.\n")
	b.WriteString("- Treat this task as if it is the FIRST and ONLY task.\n")
	b.WriteString("=====================================\n\n")

	b.WriteString("ORIGINAL USER REQUEST:\n")
	b.WriteString(strings.TrimSpace(originalTask))
	b.WriteString("\n\n")

	b.WriteString("WORKFLOW GOAL:\n")
	b.WriteString(plan.Goal)
	b.WriteString("\n\n")

	if len(plan.Acceptance) > 0 {
		b.WriteString("GLOBAL ACCEPTANCE CRITERIA:\n")
		for _, a := range plan.Acceptance {
			b.WriteString("- ")
			b.WriteString(a)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	fmt.Fprintf(&b, "CURRENT TASK %d/%d:\n", index+1, total)
	b.WriteString(sub.Task)
	b.WriteString("\n")

	if len(sub.Acceptance) > 0 {
		b.WriteString("TASK ACCEPTANCE CRITERIA:\n")
		for _, a := range sub.Acceptance {
			b.WriteString("- ")
			b.WriteString(a)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if strings.TrimSpace(searchContext) != "" {
		b.WriteString("WEB SEARCH REFERENCE (untrusted, use only for API signatures and syntax):\n")
		b.WriteString(searchContext)
		b.WriteString("\n")
	}

	b.WriteString(`RULES:
1. Implement ONLY the current task.
2. Do not solve unrelated plan items.
3. Preserve existing behavior unless the task explicitly requires changing it.
4. Return changes in normal Gogitor file/patch format.
5. The result must compile and pass tests.
6. Work independently — only modify what THIS task requires.
7. Do NOT invent or assume code from other tasks in the plan.
8. If a file mentioned in the plan does not exist yet, create it as part of THIS task (if required) or skip it (if it belongs to another task).
`)
	return b.String()
}

func writeWorkflowFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	return os.WriteFile(path, []byte(content), 0o644)
}

func appendWorkflowProcess(path, line string) {
	if strings.TrimSpace(line) == "" {
		return
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	fmt.Fprintf(f, "[%s] %s", timestamp, line)
}

// parseWorkflowLessons извлекает уроки из ответа LLM.
// Ожидаемый формат: каждая строка начинается с "LESSON:".
func parseWorkflowLessons(response string) []string {
	var lessons []string
	lines := strings.Split(response, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "LESSON:") {
			lesson := strings.TrimSpace(strings.TrimPrefix(line, "LESSON:"))
			if lesson != "" && len(lessons) < 5 {
				lessons = append(lessons, lesson)
			}
		}
	}
	return lessons
}

func mergeWorkflowResult(final *domain.Result, res domain.Result) {
	final.FilesCreated = appendUniqueStrings(final.FilesCreated, res.FilesCreated...)
	final.FilesModified = appendUniqueStrings(final.FilesModified, res.FilesModified...)
	final.FilesPatched = appendUniqueStrings(final.FilesPatched, res.FilesPatched...)
	final.FilesFullRewritten = appendUniqueStrings(final.FilesFullRewritten, res.FilesFullRewritten...)

	final.OutputFiles = mergeOutputFiles(final.OutputFiles, res.OutputFiles)

	final.Tests = res.Tests
	final.Lint = res.Lint

	if res.GitCommit != "" {
		final.GitCommit = res.GitCommit
	}

	final.Warnings = append(final.Warnings, res.Warnings...)
}

func appendUniqueStrings(base []string, items ...string) []string {
	seen := make(map[string]bool, len(base)+len(items))

	for _, v := range base {
		seen[v] = true
	}

	for _, v := range items {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}

		seen[v] = true
		base = append(base, v)
	}

	return base
}

// ─── Workflow Interview ─────────────────────────────────────────────────

// interviewQuestion — один вопрос из LLM.
type interviewQuestion struct {
	ID       int    `json:"id"`
	Question string `json:"question"`
	Why      string `json:"why"`
	Default  string `json:"default"`
}

// interviewResult — структура ответа LLM на запрос вопросов.
type interviewResult struct {
	Questions   []interviewQuestion `json:"questions"`
	Assumptions []string            `json:"assumptions"`
}

// ExecuteWorkflowInterview проводит интерактивное интервью перед workflow.
// В TUI: вопросы показываются последовательно, пользователь отвечает.
// В CLI: все вопросы выводятся сразу, ответы читаются из stdin.
func (s *Service) ExecuteWorkflowInterview(
	ctx context.Context,
	task string,
	emit func(domain.Event),
) domain.Result {
	ctx = agent.WithStatusFunc(ctx, s.agentStatusEmitter(emit))
	sendEvent(emit, domain.EventAgent, "current stage: workflow interview")
	sendEvent(emit, domain.EventLog, "Generating clarifying questions...")

	// 1. Генерируем вопросы через LLM.
	prompt := prompts.WorkflowInterviewQuestions(task, s.projectSummary())
	var questions interviewResult
	err := s.sendAgentJSON(
		ctx,
		agent.RolePlanner,
		agent.PriorityHigh,
		"workflow interview questions",
		prompt,
		&questions,
	)
	if err != nil {
		sendEvent(emit, domain.EventWarn,
			fmt.Sprintf("Interview question generation failed: %v; proceeding without interview", err))
		// Fallback: просто запускаем workflow без интервью.
		return s.ExecuteCode(ctx, task, Options{Mode: "workflow"}, emit)
	}

	if len(questions.Questions) == 0 {
		sendEvent(emit, domain.EventLog, "No clarifying questions needed.")
		return s.ExecuteCode(ctx, task, Options{Mode: "workflow"}, emit)
	}

	// 2. Формируем текст с вопросами для пользователя.
	var questionsText strings.Builder
	if DetectLanguage() == "ru" {
		questionsText.WriteString("## 📋 Интервью Workflow\n\n")
		questionsText.WriteString(fmt.Sprintf(
			"Перед выполнением задачи у меня %d уточняющий(х) вопрос(ов):\n\n",
			len(questions.Questions)))
		for _, q := range questions.Questions {
			fmt.Fprintf(&questionsText, "**В%d:** %s\n", q.ID, q.Question)
			if q.Why != "" {
				fmt.Fprintf(&questionsText, "  _(зачем: %s)_\n", q.Why)
			}
			if q.Default != "" {
				fmt.Fprintf(&questionsText, "  _по умолчанию: %s_\n", q.Default)
			}
			questionsText.WriteString("\n")
		}
		questionsText.WriteString("---\n")
		questionsText.WriteString(
			"Ответьте на каждый вопрос (или введите `skip` для значений по умолчанию, " +
				"`go` для продолжения со значениями по умолчанию):\n")
	} else {
		questionsText.WriteString("## 📋 Workflow Interview\n\n")
		questionsText.WriteString(fmt.Sprintf(
			"Before executing the task, I have %d clarifying question(s):\n\n",
			len(questions.Questions)))
		for _, q := range questions.Questions {
			fmt.Fprintf(&questionsText, "**Q%d:** %s\n", q.ID, q.Question)
			if q.Why != "" {
				fmt.Fprintf(&questionsText, "  _(why: %s)_\n", q.Why)
			}
			if q.Default != "" {
				fmt.Fprintf(&questionsText, "  _default: %s_\n", q.Default)
			}
			questionsText.WriteString("\n")
		}
		questionsText.WriteString("---\n")
		questionsText.WriteString(
			"Answer each question (or type `skip` to use defaults, " +
				"`go` to proceed with all defaults):\n")
	}

	// 3. В TUI/CLI показываем вопросы и ждём ответов.
	// Для данного этапа: возвращаем результат с AwaitingSelection=true,
	// чтобы TUI показал вопросы и ждал следующего ввода.
	sendEvent(emit, domain.EventLog, "Interview questions ready.")

    // Сохраняем pending interview для обработки следующего ввода.
    s.pendingInterview = &PendingInterview{
        Task:      task,
        CreatedAt: time.Now(),
    }
    
    return domain.Result{
        Success:           true,
        Mode:              "workflow-interview",
        Response:          questionsText.String(),
        AwaitingSelection: true,
        RefinedTask: task,
    }
}

// ContinueWorkflowInterview обрабатывает ответы пользователя на вопросы интервью
// и запускает workflow с уточнённой задачей.
func (s *Service) ContinueWorkflowInterview(
	ctx context.Context,
	originalTask string,
	answersText string,
	emit func(domain.Event),
) domain.Result {
	ctx = agent.WithStatusFunc(ctx, s.agentStatusEmitter(emit))
	sendEvent(emit, domain.EventLog, "Processing interview answers...")
	
	answers := parseInterviewAnswers(answersText)
	
	// НОВОЕ: Если пользователь ввел "skip" или "go", пропускаем LLM-уточнение
	if len(answers) == 0 {
		sendEvent(emit, domain.EventLog, "Interview skipped by user, using original task with defaults.")
		return s.ExecuteCode(ctx, originalTask, Options{
			Mode:             "workflow",
			InterviewAnswers: answers, // передаем пустой слайс, в inbox.md не будет секции Q&A
		}, emit)
	}

	// Формируем уточнённую задачу через LLM (только если были реальные ответы).
	prompt := prompts.WorkflowInterviewSummary(originalTask, answers)
	response, err := s.LLM.Send(ctx, prompt)
	refinedTask := originalTask
	if err == nil && strings.TrimSpace(response) != "" {
		refinedTask = strings.TrimSpace(response)
		sendEvent(emit, domain.EventLog, "Task refined based on interview answers.")
	} else {
		sendEvent(emit, domain.EventWarn,
			fmt.Sprintf("Interview refinement failed: %v; using original task", err))
	}

	return s.ExecuteCode(ctx, refinedTask, Options{
		Mode:             "workflow",
		InterviewAnswers: answers,
	}, emit)
}
// parseInterviewAnswers парсит ответы пользователя на вопросы интервью.
func parseInterviewAnswers(text string) []prompts.WorkflowAnswer {
	text = strings.TrimSpace(text)
	lower := strings.ToLower(text)
	
	// Команды пропуска интервью (пользователь принял дефолты)
	skipWords := []string{"skip", "go", "пропустить", "дальше", "далее", "нет", "no", "default", "по умолчанию"}
	for _, w := range skipWords {
		if lower == w {
			return nil // Возвращаем пустой слайс — интервью пропущено
		}
	}

	var answers []prompts.WorkflowAnswer
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Формат "N: ответ" или "N. ответ"
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			parts = strings.SplitN(line, ".", 2)
		}
		if len(parts) == 2 {
			idStr := strings.TrimSpace(parts[0])
			if id, err := strconv.Atoi(idStr); err == nil && id > 0 {
				answers = append(answers, prompts.WorkflowAnswer{
					ID:       id,
					Question: fmt.Sprintf("Question %d", id),
					Answer:   strings.TrimSpace(parts[1]),
				})
				continue
			}
		}
		// Свободный текст — добавляем как общий ответ.
		answers = append(answers, prompts.WorkflowAnswer{
			ID:       len(answers) + 1,
			Question: "General",
			Answer:   line,
		})
	}
	return answers
}

// ─── Workflow Reflect ───────────────────────────────────────────────────

// ExecuteWorkflowReflect анализирует артефакты последнего workflow
// и генерирует ретроспективу.
func (s *Service) ExecuteWorkflowReflect(
	ctx context.Context,
	emit func(domain.Event),
) domain.Result {
	ctx = agent.WithStatusFunc(ctx, s.agentStatusEmitter(emit))
	sendEvent(emit, domain.EventAgent, "current stage: workflow reflect")

	// 1. Находим последнюю директорию workflow.
	workflowDir, err := s.findLatestWorkflowDir()
	if err != nil {
		return domain.Result{
			Success: false,
			Mode:    "workflow-reflect",
			Errors:  []string{fmt.Sprintf("no workflow artifacts found: %v", err)},
		}
	}
	sendEvent(emit, domain.EventLog, fmt.Sprintf("Analyzing workflow artifacts: %s", workflowDir))

	// 2. Читаем артефакты.
	goal := ""
	processLog := ""
	prdJSON := ""
	var gateReports []string

	// inbox.md → цель
	if data, err := os.ReadFile(filepath.Join(workflowDir, "inbox.md")); err == nil {
		goal = string(data)
	}
	// process.md → лог
	if data, err := os.ReadFile(filepath.Join(workflowDir, "process.md")); err == nil {
		processLog = string(data)
	}
	// prd.json → задачи
	if data, err := os.ReadFile(filepath.Join(workflowDir, "prd.json")); err == nil {
		prdJSON = string(data)
	}
	// gate-report-*.json → отчёты
	entries, _ := os.ReadDir(workflowDir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "gate-report-") && strings.HasSuffix(e.Name(), ".json") {
			if data, err := os.ReadFile(filepath.Join(workflowDir, e.Name())); err == nil {
				gateReports = append(gateReports, string(data))
			}
		}
	}
	gateReportsStr := strings.Join(gateReports, "\n---\n")

	// 3. Получаем cumulative diff из git (если есть).
	cumulativeDiff := ""
	if s.Git.IsRepo(ctx) {
		if head, err := s.Git.HeadHash(ctx); err == nil {
			diff, _ := s.Git.DiffRange(ctx, head+"~5", head)
			cumulativeDiff = diff
		}
	}

	// 4. Генерируем рефлексию через LLM.
	sendEvent(emit, domain.EventLog, "Generating workflow reflection...")

	var response string
	var llmErr error

	// Для малых моделей используем упрощённый промпт.
	if s.modelProfile() == modelProfileSmall {
		prompt := prompts.WorkflowReflectQuick(goal, processLog)
		response, llmErr = s.sendLLMStreaming(
			ctx, prompt, emit,
			agent.RoleDefault, agent.PriorityNormal, "workflow_reflect",
		)
	} else {
		prompt := prompts.WorkflowReflection(goal, processLog, prdJSON, gateReportsStr, cumulativeDiff)
		response, llmErr = s.sendLLMStreaming(
			ctx, prompt, emit,
			agent.RoleDefault, agent.PriorityNormal, "workflow_reflect",
		)
	}

	if llmErr != nil {
		// Fallback: показываем сырые данные.
		sendEvent(emit, domain.EventWarn,
			fmt.Sprintf("LLM reflection failed: %v; showing raw artifacts", llmErr))
		var fallback strings.Builder
		if DetectLanguage() == "ru" {
			fallback.WriteString("## Сводка артефактов Workflow\n\n")
			fallback.WriteString(fmt.Sprintf("**Директория:** `%s`\n\n", workflowDir))
			if goal != "" {
				fallback.WriteString("### Цель (inbox.md)\n```\n" + truncate(goal, 2000) + "\n```\n\n")
			}
			if processLog != "" {
				fallback.WriteString("### Журнал процесса\n```\n" + truncate(processLog, 3000) + "\n```\n\n")
			}
			if prdJSON != "" {
				fallback.WriteString("### PRD\n```json\n" + truncate(prdJSON, 2000) + "\n```\n")
			}
		} else {
			fallback.WriteString("## Workflow Artifacts Summary\n\n")
			fallback.WriteString(fmt.Sprintf("**Directory:** `%s`\n\n", workflowDir))
			if goal != "" {
				fallback.WriteString("### Goal (inbox.md)\n```\n" + truncate(goal, 2000) + "\n```\n\n")
			}
			if processLog != "" {
				fallback.WriteString("### Process Log\n```\n" + truncate(processLog, 3000) + "\n```\n\n")
			}
			if prdJSON != "" {
				fallback.WriteString("### PRD\n```json\n" + truncate(prdJSON, 2000) + "\n```\n")
			}
		}
		response = fallback.String()
	}

	// 5. Сохраняем рефлексию в файл.
	reflectionPath := filepath.Join(workflowDir, "reflection.md")
	reflectionTitle := "# Workflow Reflection"
	if DetectLanguage() == "ru" {
		reflectionTitle = "# Ретроспектива Workflow"
	}
	reflectionContent := fmt.Sprintf("%s\n\n_Generated: %s_\n\n%s\n",
		reflectionTitle, time.Now().Format("2006-01-02 15:04:05"), response)

	if err := os.WriteFile(reflectionPath, []byte(reflectionContent), 0o644); err != nil {
		sendEvent(emit, domain.EventWarn, fmt.Sprintf("Cannot save reflection.md: %v", err))
	} else {
		sendEvent(emit, domain.EventLog, fmt.Sprintf("Reflection saved: %s", reflectionPath))
	}

	// ─── Извлечение уроков для будущих воркфлоу ──────────────
	// Для small-моделей пропускаем: они не способны надёжно
	// извлекать структурированные уроки из текста.
	if s.modelProfile() != modelProfileSmall && llmErr == nil {
		sendEvent(emit, domain.EventLog, "Extracting lessons from reflection...")

        lessonsPrompt := prompts.WorkflowExtractLessons(response)
        lessonsCtx := llm.WithReasoningDisabled(ctx)
        lessonsResponse, lessonsErr := s.LLM.Send(lessonsCtx, lessonsPrompt)
		if lessonsErr == nil {
			lessons := parseWorkflowLessons(lessonsResponse)
			if len(lessons) > 0 {
				mem := loadAgentMemory(s.Cfg.WorkDir)
				for _, lesson := range lessons {
					mem.addLesson(lesson)
				}
				if err := mem.save(s.Cfg.WorkDir); err != nil {
					sendEvent(emit, domain.EventWarn,
						fmt.Sprintf("Cannot save lessons to agent memory: %v", err))
				} else {
					sendEvent(emit, domain.EventLog,
						fmt.Sprintf("Extracted %d lesson(s) for future workflows.", len(lessons)))
				}
			}
		} else {
			sendEvent(emit, domain.EventWarn,
				fmt.Sprintf("Lesson extraction failed (non-fatal): %v", lessonsErr))
		}
	}

	return domain.Result{
		Success:  true,
		Mode:     "workflow-reflect",
		Response: response,
	}
}

// findLatestWorkflowDir находит последнюю директорию workflow.
func (s *Service) findLatestWorkflowDir() (string, error) {
	baseDir := filepath.Join(s.Cfg.WorkDir, ".gogitor", "workflow")

	// Также проверяем кастомный workflow dir из конфига.
	if s.Cfg.WorkflowDir != "" {
		if full, err := security.SafeJoin(s.Cfg.WorkDir, s.Cfg.WorkflowDir); err == nil {
			if info, err := os.Stat(full); err == nil && info.IsDir() {
				baseDir = full
			}
		}
	}

	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return "", fmt.Errorf("workflow directory not found: %s", baseDir)
	}

	// Ищем последнюю по имени (формат: 20060102-150405).
	var latest string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			if e.Name() > latest {
				latest = e.Name()
			}
		}
	}
	if latest == "" {
		return "", fmt.Errorf("no workflow sessions found in %s", baseDir)
	}
	return filepath.Join(baseDir, latest), nil
}

// ─── Workflow PR (MR Flow) ────────────────────────────────────────────

// ExecuteWorkflowPR создаёт ветку и Pull Request для последнего workflow.
// Описание PR формируется из артефактов: plan.md, prd.json, process.md,
// gate-report-*.json.
func (s *Service) ExecuteWorkflowPR(
	ctx context.Context,
	emit func(domain.Event),
) domain.Result {
	result := domain.Result{Mode: "workflow-pr"}

	// ── 1. Проверяем git-репозиторий ──────────────────────────
	if !s.Git.IsRepo(ctx) {
		result.AddError("not a git repository; run :git init first")
		return result
	}

	// ── 2. Проверяем GitHub-токен ─────────────────────────────
	if s.Cfg.GitHubToken == "" {
		result.AddError("GitHub token is required. Use --key-github <token>")
		return result
	}

	// ── 3. Находим артефакты workflow ─────────────────────────
	workflowDir, err := s.findLatestWorkflowDir()
	if err != nil {
		result.AddError(fmt.Sprintf("no workflow artifacts found: %v", err))
		return result
	}
	sendEvent(emit, domain.EventLog,
		fmt.Sprintf("Workflow artifacts: %s", workflowDir))

	// ── 4. Читаем артефакты ───────────────────────────────────
	artifacts := s.readWorkflowArtifacts(workflowDir)
	if artifacts.Goal == "" {
		result.AddError("workflow goal not found in inbox.md")
		return result
	}

	// ── 5. Формируем описание PR ──────────────────────────────
	prBody := s.buildWorkflowPRBody(artifacts)
	prTitle := s.buildWorkflowPRTitle(artifacts.Goal)

	// ── 6. Определяем owner/repo ──────────────────────────────
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

	// ── 7. Определяем base branch ─────────────────────────────
	repoInfo, err := s.GitHub.RepoInfo(ctx, owner, repo)
	if err != nil {
		result.AddError(fmt.Sprintf("cannot get repo info: %v", err))
		return result
	}
	baseBranch := repoInfo.DefaultBr
	if baseBranch == "" {
		baseBranch = "main"
	}

	// ── 8. Создаём/используем ветку ───────────────────────────
	currentBranch, err := s.Git.CurrentBranch(ctx)
	if err != nil || currentBranch == "" {
		result.AddError("cannot determine current branch")
		return result
	}

	slug := workflowBranchSlug(artifacts.Goal)
	branchName := "workflow/" + slug

	if currentBranch == baseBranch {
		// На base-ветке — создаём новую workflow-ветку.
		sendEvent(emit, domain.EventLog,
			fmt.Sprintf("Creating branch '%s' from '%s'...", branchName, currentBranch))
		_, err = s.Git.CheckoutNew(ctx, branchName)
		if err != nil {
			result.AddError(fmt.Sprintf("cannot create branch: %v", err))
			return result
		}
		currentBranch = branchName
	} else if strings.HasPrefix(currentBranch, "workflow/") {
		// Уже на workflow-ветке — используем её.
		branchName = currentBranch
		sendEvent(emit, domain.EventLog,
			fmt.Sprintf("Using existing branch '%s'", branchName))
	} else {
		// На другой feature-ветке — используем её как есть.
		branchName = currentBranch
		sendEvent(emit, domain.EventLog,
			fmt.Sprintf("Using current branch '%s'", branchName))
	}

	// ── 9. Пушим ветку ────────────────────────────────────────
	sendEvent(emit, domain.EventLog,
		fmt.Sprintf("Pushing branch '%s' to origin...", branchName))
	_, err = s.Git.WithAuthenticatedRemote(ctx, "origin", s.Cfg.GitHubToken, func() (string, error) {
		return s.Git.Push(ctx, "origin", branchName, false)
	})
	if err != nil {
		result.AddError(sanitizeTokenFromError(
			fmt.Sprintf("push failed: %v", err), s.Cfg.GitHubToken))
		return result
	}

	// ── 10. Создаём PR ────────────────────────────────────────
	sendEvent(emit, domain.EventLog,
		fmt.Sprintf("Creating PR: %s → %s ...", branchName, baseBranch))
	pr, err := s.GitHub.CreatePullRequest(
		ctx, owner, repo, prTitle, prBody, branchName, baseBranch,
	)
	if err != nil {
		result.AddError(err.Error())
		return result
	}
	result.Success = true
	if DetectLanguage() == "ru" {
		result.Response = fmt.Sprintf(
			"Pull Request создан: #%d\n%s\nЗаголовок: %s\nВетка: %s → %s",
			pr.Number, pr.HTMLURL, pr.Title, branchName, baseBranch,
		)
	} else {
		result.Response = fmt.Sprintf(
			"Pull Request created: #%d\n%s\nTitle: %s\nBranch: %s → %s",
			pr.Number, pr.HTMLURL, pr.Title, branchName, baseBranch,
		)
	}
	return result
}

// workflowArtifacts — прочитанные артефакты workflow.
type workflowArtifacts struct {
	Goal        string
	PlanMD      string
	PRD         *workflowPRD
	ProcessLog  string
	GateReports []workflowGateReport
	Reflection  string
}

// readWorkflowArtifacts читает все артефакты из директории workflow.
func (s *Service) readWorkflowArtifacts(dir string) *workflowArtifacts {
	a := &workflowArtifacts{}

	// inbox.md → цель
	if data, err := os.ReadFile(filepath.Join(dir, "inbox.md")); err == nil {
		a.Goal = extractGoalFromInbox(string(data))
	}

	// plan.md
	if data, err := os.ReadFile(filepath.Join(dir, "plan.md")); err == nil {
		a.PlanMD = string(data)
	}

	// prd.json
	if data, err := os.ReadFile(filepath.Join(dir, "prd.json")); err == nil {
		var prd workflowPRD
		if json.Unmarshal(data, &prd) == nil {
			a.PRD = &prd
		}
	}

	// process.md
	if data, err := os.ReadFile(filepath.Join(dir, "process.md")); err == nil {
		a.ProcessLog = string(data)
	}

	// gate-report-task-*.json
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "gate-report-task-") &&
			strings.HasSuffix(e.Name(), ".json") {
			if data, err := os.ReadFile(filepath.Join(dir, e.Name())); err == nil {
				var report workflowGateReport
				if json.Unmarshal(data, &report) == nil {
					a.GateReports = append(a.GateReports, report)
				}
			}
		}
	}

	// reflection.md (если есть)
	if data, err := os.ReadFile(filepath.Join(dir, "reflection.md")); err == nil {
		a.Reflection = string(data)
	}

	return a
}

// extractGoalFromInbox извлекает цель из inbox.md.
func extractGoalFromInbox(content string) string {
	lines := strings.Split(content, "\n")
	inGoal := false
	var goal []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## Original request") ||
			strings.HasPrefix(trimmed, "## Clarified goal") {
			inGoal = true
			continue
		}
		if inGoal && strings.HasPrefix(trimmed, "## ") {
			break
		}
		if inGoal && trimmed != "" {
			goal = append(goal, trimmed)
		}
	}
	return strings.Join(goal, " ")
}

// buildWorkflowPRTitle формирует заголовок PR.
func (s *Service) buildWorkflowPRTitle(goal string) string {
	clean := strings.TrimSpace(goal)
	if len(clean) > 72 {
		clean = clean[:69] + "..."
	}
	return "feat(workflow): " + clean
}

// buildWorkflowPRBody формирует тело PR из артефактов workflow.
func (s *Service) buildWorkflowPRBody(a *workflowArtifacts) string {
	var b strings.Builder

	b.WriteString("## 📋 Workflow Execution Summary\n\n")

	// ── Goal ──────────────────────────────────────────────────
	if a.Goal != "" {
		b.WriteString("### 🎯 Goal\n")
		b.WriteString(a.Goal)
		b.WriteString("\n\n")
	}

	// ── Plan ──────────────────────────────────────────────────
	if strings.TrimSpace(a.PlanMD) != "" {
		b.WriteString("### 📝 Implementation Plan\n")
		planContent := a.PlanMD
		if len(planContent) > 3000 {
			planContent = planContent[:3000] + "\n... (truncated)"
		}
		b.WriteString(planContent)
		b.WriteString("\n\n")
	}

	// ── Tasks (PRD) ───────────────────────────────────────────
	if a.PRD != nil && len(a.PRD.Tasks) > 0 {
		b.WriteString("### ✅ Tasks\n")
		b.WriteString("| # | Task | Status | Commit |\n")
		b.WriteString("|---|------|--------|--------|\n")
		for _, t := range a.PRD.Tasks {
			status := "⏳ pending"
			switch t.Status {
			case "done":
				status = "✅ done"
			case "failed":
				status = "❌ failed"
			case "running":
				status = "▶ running"
			}
			commit := t.Commit
			if commit == "" {
				commit = "—"
			}
			title := t.Title
			if len(title) > 60 {
				title = title[:57] + "..."
			}
			fmt.Fprintf(&b, "| %d | %s | %s | `%s` |\n",
				t.ID, title, status, commit)
		}
		b.WriteString("\n")
	}

	// ── Quality Gates ─────────────────────────────────────────
	if len(a.GateReports) > 0 {
		b.WriteString("### 🚦 Quality Gates\n")
		b.WriteString("| Task | Build | Tests | Vet | Gofmt | Lint | Passed |\n")
		b.WriteString("|------|-------|-------|-----|-------|------|--------|\n")
		for _, g := range a.GateReports {
			passed := "✅"
			if !g.Passed {
				passed = "❌"
			}
			fmt.Fprintf(&b, "| %d | %s | %s | %s | %s | %s | %s |\n",
				g.TaskIndex,
				boolIcon(g.Build),
				boolIcon(g.Tests),
				boolIcon(g.Vet),
				boolIcon(g.Gofmt),
				boolIcon(g.Lint),
				passed,
			)
		}
		b.WriteString("\n")

		// Предупреждения из gates
		var allWarnings []string
		for _, g := range a.GateReports {
			allWarnings = append(allWarnings, g.Warnings...)
		}
		if len(allWarnings) > 0 {
			b.WriteString("**Warnings:**\n")
			for _, w := range allWarnings {
				b.WriteString("- ⚠ " + w + "\n")
			}
			b.WriteString("\n")
		}
	}

	// ── Process Log (последние 30 строк) ──────────────────────
	if strings.TrimSpace(a.ProcessLog) != "" {
		b.WriteString("### 📜 Process Log (last 30 lines)\n")
		b.WriteString("```\n")
		lines := strings.Split(strings.TrimSpace(a.ProcessLog), "\n")
		start := 0
		if len(lines) > 30 {
			start = len(lines) - 30
			b.WriteString("... (earlier entries omitted)\n")
		}
		for _, line := range lines[start:] {
			b.WriteString(line + "\n")
		}
		b.WriteString("```\n\n")
	}

	// ── Reflection ────────────────────────────────────────────
	if strings.TrimSpace(a.Reflection) != "" {
		b.WriteString("### 🔍 Reflection\n")
		reflection := a.Reflection
		if len(reflection) > 2000 {
			reflection = reflection[:2000] + "\n... (truncated)"
		}
		b.WriteString(reflection)
		b.WriteString("\n\n")
	}

	b.WriteString("---\n")
	b.WriteString("_Generated by Gogitor workflow_\n")

	return b.String()
}

// boolIcon возвращает ✅ или ❌.
func boolIcon(v bool) string {
	if v {
		return "✅"
	}
	return "❌"
}

// workflowBranchSlug формирует slug для имени ветки из цели задачи.
func workflowBranchSlug(goal string) string {
	slug := strings.ToLower(strings.TrimSpace(goal))
	// Убираем всё кроме букв, цифр, пробелов и дефисов.
	var b strings.Builder
	for _, r := range slug {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == ' ' {
			b.WriteRune(r)
		}
	}
	slug = strings.TrimSpace(b.String())
	// Заменяем пробелы на дефисы.
	slug = strings.ReplaceAll(slug, " ", "-")
	// Убираем повторяющиеся дефисы.
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	slug = strings.Trim(slug, "-")
	// Ограничиваем длину.
	if len(slug) > 40 {
		slug = slug[:40]
		slug = strings.TrimRight(slug, "-")
	}
	if slug == "" {
		slug = time.Now().Format("20060102-150405")
	}
	return slug
}

// ContinueWorkflowPlanReview обрабатывает ответ пользователя на вопросы
// по плану и продолжает выполнение workflow.
func (s *Service) ContinueWorkflowPlanReview(
	ctx context.Context,
	userResponse string,
	emit func(domain.Event),
) domain.Result {
	review := s.pendingPlanReview
	s.pendingPlanReview = nil

	ctx = agent.WithStatusFunc(ctx, s.agentStatusEmitter(emit))
	sendEvent(emit, domain.EventLog, "Processing plan review feedback...")

	plan := review.Plan
	prd := review.PRD

	// Проверяем, не является ли ответ командой пропуска.
	lower := strings.ToLower(strings.TrimSpace(userResponse))
	skipWords := []string{
		"go", "ok", "skip", "пропустить", "дальше", "далее",
		"да", "yes", "продолжить", "continue",
	}
	isSkip := false
	for _, w := range skipWords {
		if lower == w {
			isSkip = true
			break
		}
	}

	if !isSkip {
		// Корректируем план на основе ответа пользователя.
		sendEvent(emit, domain.EventLog, "Refining plan based on user feedback...")

		planJSON, _ := json.MarshalIndent(plan, "", "  ")
		refinePrompt := prompts.WorkflowPlanRefine(
			review.Task,
			string(planJSON),
			userResponse,
		)
		var newPlan fullPlan
		err := s.sendAgentJSON(
			ctx,
			agent.RolePlanner,
			agent.PriorityHigh,
			"workflow plan refine",
			refinePrompt,
			&newPlan,
		)
		if err == nil && len(newPlan.Subtasks) > 0 {
			plan = validateWorkflowPlan(&newPlan, review.Task)
			prd = buildWorkflowPRD(plan)

			// Перезаписываем plan.md и prd.json.
			planMarkdown := formatWorkflowPlanMarkdown(review.Task, plan)
			if err := writeWorkflowFile(review.PlanPath, planMarkdown); err != nil {
				sendEvent(emit, domain.EventWarn,
					fmt.Sprintf("Cannot rewrite plan.md: %v", err))
			}
			if err := saveWorkflowPRD(review.PrdPath, prd); err != nil {
				sendEvent(emit, domain.EventWarn,
					fmt.Sprintf("Cannot rewrite prd.json: %v", err))
			}
			appendWorkflowProcess(review.ProcessPath,
				"Plan refined based on user feedback")
			sendEvent(emit, domain.EventLog, "Plan refined based on feedback.")
		} else {
			sendEvent(emit, domain.EventWarn,
				fmt.Sprintf("Plan refinement failed: %v; using original plan", err))
		}
	} else {
		appendWorkflowProcess(review.ProcessPath,
			"Plan review: user approved current plan")
	}

	// Продолжаем выполнение задач.
	final := domain.Result{
		Success: true,
		Mode:    "workflow",
		DryRun:  review.Opts.DryRun,
	}
	return s.executeWorkflowTasks(
		ctx, review.Task, plan, prd, review.WorkflowDir,
		review.PlanPath, review.PrdPath, review.ProcessPath,
		review.PreTaskHead, review.Opts, &final, emit,
	)
}