package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"gogitor/internal/agent"
	"gogitor/internal/domain"
	"gogitor/internal/prompts"
)

// fullPlan — структурированный план от planner agent.
type fullPlan struct {
	Goal       string            `json:"goal"`
	Acceptance []string          `json:"acceptance"`
	Subtasks   []fullPlanSubtask `json:"subtasks"`
}

type fullPlanSubtask struct {
	Task       string   `json:"task"`
	Acceptance []string `json:"acceptance"`
	NeedsSearch bool     `json:"needs_search"`
}

// agentReview — результат работы reviewer agent.
// Используется как итоговая структура после гибкого парсинга.
type agentReview struct {
	Approved       bool     `json:"approved"`
	CriticalIssues []string `json:"critical_issues"`
	Suggestions    []string `json:"suggestions"`
}

// rawAgentReview — промежуточная структура для первичного парсинга.
// Поля CriticalIssues и Suggestions имеют тип []any, чтобы принять
// как строки, так и объекты (которые часто генерируют маленькие модели).
type rawAgentReview struct {
	Approved       bool `json:"approved"`
	CriticalIssues []any `json:"critical_issues"`
	Suggestions    []any `json:"suggestions"`
}

// convertRawReview конвертирует гибко распарсенный ответ в строгую структуру.
// Обрабатывает три варианта элементов массива:
//  1. string — используется как есть
//  2. map[string]any — извлекается текстовое поле (description, text, message, issue, suggestion)
//  3. любой другой тип — конвертируется через fmt.Sprintf
func convertRawReview(raw *rawAgentReview) agentReview {
	review := agentReview{
		Approved: raw.Approved,
	}
	review.CriticalIssues = convertAnySliceToStrings(raw.CriticalIssues)
	review.Suggestions = convertAnySliceToStrings(raw.Suggestions)
	return review
}

// convertAnySliceToStrings преобразует []any в []string,
// извлекая строковое представление из каждого элемента.
func convertAnySliceToStrings(items []any) []string {
	if len(items) == 0 {
		return nil
	}
	var result []string
	for _, item := range items {
		s := anyToString(item)
		if s != "" {
			result = append(result, s)
		}
	}
	return result
}

// anyToString извлекает строку из произвольного значения.
// Для объектов (map[string]any) ищет наиболее вероятные текстовые поля.
func anyToString(v any) string {
	switch val := v.(type) {
	case string:
		return strings.TrimSpace(val)
	case float64:
		return fmt.Sprintf("%v", val)
	case bool:
		return fmt.Sprintf("%v", val)
	case map[string]any:
		// Ищем наиболее вероятные текстовые поля в объекте
		textKeys := []string{
			"description", "text", "message", "issue",
			"suggestion", "comment", "note", "detail",
			"reason", "content", "body", "value",
		}
		for _, key := range textKeys {
			if field, ok := val[key]; ok {
				if s, ok := field.(string); ok && strings.TrimSpace(s) != "" {
					return strings.TrimSpace(s)
				}
			}
		}
		// Если ни одно поле не подошло, сериализуем объект в JSON-строку
		data, err := json.Marshal(val)
		if err != nil {
			return fmt.Sprintf("%v", val)
		}
		return string(data)
	case []any:
		// Массив внутри массива — берём первый элемент
		if len(val) > 0 {
			return anyToString(val[0])
		}
		return ""
	default:
		return fmt.Sprintf("%v", val)
	}
}

// agentVerification — результат работы verifier agent.
type agentVerification struct {
	Completed bool     `json:"completed"`
	Missing   []string `json:"missing"`
	Risks     []string `json:"risks"`
	FixTask   string   `json:"fix_task"`
}

func (s *Service) executeAgentFull(
	ctx context.Context,
	query string,
	approach string,
	opts Options,
	emit func(domain.Event),
) domain.Result {
    if s.Cfg.AutoSearch && s.isRemoteLLM() {
    	sendEvent(emit, domain.EventWarn,
    		"WARNING: auto-search is enabled with a REMOTE LLM provider. "+
    			"Project code and search queries will be sent to external servers. "+
    			"Use a local Ollama instance for sensitive projects.")
    }
	sendEvent(emit, domain.EventLog, "Full agent mode: planner + coder + reviewer + verifier")

	if approach != "" {
		sendEvent(emit, domain.EventAgent,
			"Using selected approach: "+truncate(approach, 200))
	}

	sendEvent(emit, domain.EventAgent, "orchestrator enabled")
    emitEvent(emit, domain.Event{
    	Type:      domain.EventAgent,
    	Message:   "current stage: planning",
    	TaskStage: domain.TaskStagePlanning,
    })
	sendEvent(emit, domain.EventAgent, "planner started")

	mem := loadAgentMemory(s.Cfg.WorkDir)

	if approach != "" {
		mem.addDecisionWithAlternatives(
			fmt.Sprintf("Selected implementation approach: %s", truncate(approach, 300)),
			query,
			nil,
			"user",
		)
	}

	var checkpoint *agentCheckpoint
	if !opts.DryRun {
		var err error
		checkpoint, err = s.createAgentCheckpoint(ctx)
		if err != nil {
			sendEvent(emit, domain.EventWarn, fmt.Sprintf("checkpoint failed: %v", err))
		} else {
			defer checkpoint.cleanup()
		}
	}

	defer s.WS.RefreshIndex()

	final := domain.Result{
		Success: true,
		Mode:    "agent",
		DryRun:  opts.DryRun,
	}
    preTaskHead := s.captureHead(ctx)

	created := map[string]bool{}
	modified := map[string]bool{}
	addFiles := func(res domain.Result) {
		for _, f := range res.FilesCreated {
			created[f] = true
		}
		for _, f := range res.FilesModified {
			modified[f] = true
		}
		for _, f := range res.FilesPatched {
			modified[f] = true
		}
		for _, f := range res.FilesFullRewritten {
			modified[f] = true
		}
		final.FilesCreated = sortedKeys(created)
		final.FilesModified = sortedKeys(modified)
		final.OutputFiles = mergeOutputFiles(final.OutputFiles, res.OutputFiles)
	}

	rollback := func(reason string) {
		if opts.DryRun || checkpoint == nil {
			return
		}
		sendEvent(emit, domain.EventWarn, "Rollback: "+reason)
		err := s.rollbackAgentCheckpoint(
			checkpoint,
			sortedKeys(created),
			sortedKeys(modified),
		)
		if err != nil {
			final.AddWarning(fmt.Sprintf("rollback failed: %v", err))
			return
		}
		final.AddWarning("changes were rolled back to pre-agent state")
	}

	// ─── Planning ────────────────────────────────────────────────
	plan := s.planFullOrFallback(ctx, query, approach, mem, emit)
	sendEvent(emit, domain.EventAgent, fmt.Sprintf("planner completed: goal=%s", plan.Goal))
	s.emitAgentBudget(emit, "planning")

	mem.addDecision(fmt.Sprintf("plan goal: %s", plan.Goal))
	mem.addDecisionEntry(domain.DecisionEntry{
		Decision: fmt.Sprintf("Plan goal: %s", plan.Goal),
		Context:  query,
		Source:   "planner",
	})
	for i, st := range plan.Subtasks {
		mem.addDecision(fmt.Sprintf("planned subtask %d: %s", i+1, st.Task))
		mem.addDecisionSimple(
			fmt.Sprintf("Subtask %d: %s", i+1, st.Task),
			"planner",
		)
	}
	for _, a := range plan.Acceptance {
		mem.addConvention(a)
	}
	_ = mem.save(s.Cfg.WorkDir)

	sendEvent(emit, domain.EventLog, fmt.Sprintf("Plan contains %d subtasks", len(plan.Subtasks)))

	planItems := make([]string, 0, len(plan.Subtasks))
	for _, st := range plan.Subtasks {
		planItems = append(planItems, st.Task)
	}
	sendPlanBoard(emit, plan.Goal, plan.Acceptance, planItems)

	planStatuses := make([]domain.PlanStatus, len(plan.Subtasks))
	markPlan := func(index int, st domain.PlanStatus, note string) {
		if index >= 1 && index <= len(planStatuses) {
			planStatuses[index-1] = st
		}
		sendPlanStatus(emit, index, len(plan.Subtasks), planItems[index-1], st, note)
	}

	var analysisNotes []string

	// ─── Subtask execution ───────────────────────────────────────

    for i, sub := range plan.Subtasks {
    	sendEvent(
    		emit,
    		domain.EventAgent,
    		fmt.Sprintf("current subtask %d/%d: %s", i+1, len(plan.Subtasks), sub.Task),
    	)
    
    	markPlan(i+1, domain.PlanRunning, "")
    
    	subOpts := opts
    	subOpts.NoCommit = true
    
    	subOpts.ProgressItem = i + 1
    	subOpts.ProgressTotal = len(plan.Subtasks)
    
    	taskForCoder := sub.Task
    
    	if len(sub.Acceptance) > 0 {
    		taskForCoder += "\nAcceptance criteria:\n- " + strings.Join(sub.Acceptance, "\n- ")
    	}
    
    	if approach != "" {
    		taskForCoder += "\n\nSELECTED IMPLEMENTATION APPROACH (follow this):\n" + approach
    	}
    
    	if len(analysisNotes) > 0 {
    		taskForCoder += "\n\nPrevious analysis notes:\n" + strings.Join(analysisNotes, "\n")
    	}

    	if sub.NeedsSearch && s.Cfg.AutoSearch && s.SafeSearch != nil {
    		searchContent, searchErr := s.searchForSubtask(ctx, sub.Task, emit)
    		if searchErr == nil && searchContent != "" {
    			taskForCoder += "\n\n" + searchContent
    			sendEvent(emit, domain.EventLog,
    				"Web search context added to subtask")
    		}
    		// При ошибке поиска продолжаем без него (non-fatal).
    	}

    
    	if s.Stats != nil && emit != nil {
    		eta := s.Stats.estimate(agent.RoleCoder, "code", taskForCoder)
    
    		emit(domain.Event{
    			Type:    domain.EventProgress,
    			Message: sub.Task,
    			Progress: &domain.ProgressUpdate{
    				Stage:      truncate(sub.Task, 80),
    				ItemIndex:  i + 1,
    				TotalItems: len(plan.Subtasks),
    				ETASeconds: int(eta.Seconds() + 0.5),
    			},
    		})
    	}
    
    	isAnalysis := s.isAnalysisOnlyTask(taskForCoder)
    
    	if !isAnalysis {
    		sendEvent(emit, domain.EventAgent, "current stage: coder")
    		sendEvent(emit, domain.EventAgent, "coder started")
    	}
    
    	subCtx := agent.WithRole(ctx, agent.RoleCoder)
    	subCtx = agent.WithPriority(subCtx, agent.PriorityNormal)
    	subCtx = agent.WithPurpose(subCtx, fmt.Sprintf("subtask %d/%d", i+1, len(plan.Subtasks)))
    
    	res := s.executeSimple(subCtx, taskForCoder, subOpts, emit)
    
		final.Iterations += res.Iterations
		addFiles(res)

		if !res.Success {
			markPlan(i+1, domain.PlanFailed, truncate(strings.Join(res.Errors, "; "), 200))
			final.Success = false
			final.Errors = append(final.Errors, res.Errors...)
			mem.addFailed(fmt.Sprintf("subtask %d failed: %s", i+1, strings.Join(res.Errors, " | ")))
    		mem.addDecisionEntry(domain.DecisionEntry{
    			Decision:  fmt.Sprintf("Subtask %d FAILED: %s", i+1, sub.Task),
    			Context:   strings.Join(res.Errors, "; "),
    			Temporary: false,
    			Source:    "coder",
    		})
			_ = mem.save(s.Cfg.WorkDir)
			rollback("subtask failed")
			s.emitDispatcherUsage(emit)
			return final
		}

		if isAnalysis {
			if strings.TrimSpace(res.Response) != "" {
				analysisNotes = append(analysisNotes, fmt.Sprintf(
					"Subtask %d/%d (%s):\n%s",
					i+1,
					len(plan.Subtasks),
					sub.Task,
					truncate(res.Response, 3000),
				))
			}
			mem.addDecision(fmt.Sprintf("analysis subtask %d completed: %s", i+1, truncate(res.Response, 300)))
			_ = mem.save(s.Cfg.WorkDir)
			markPlan(i+1, domain.PlanDone, "")
			s.emitAgentBudget(emit, fmt.Sprintf("subtask %d", i+1))
			continue
		}

		// ─── Reviewer ─────────────────────────────────────────
		changedFiles := len(res.FilesCreated) +
			len(res.FilesModified) +
			len(res.FilesPatched) +
			len(res.FilesFullRewritten)

		itemStatus := domain.PlanDone
		itemNote := ""

		if changedFiles > 0 {
			sendEvent(emit, domain.EventAgent, "current stage: reviewer")
			sendEvent(emit, domain.EventAgent, "reviewer started")
		} else {
			sendEvent(emit, domain.EventAgent, "current stage: reviewer (skipped, no changes)")
		}

		review, err := s.runReviewer(ctx, query, sub.Task, res, mem, emit)
		if err != nil {
			sendEvent(emit, domain.EventWarn, fmt.Sprintf("reviewer failed: %v", err))
			itemStatus = domain.PlanWarn
			itemNote = "reviewer unavailable"
		} else if !review.Approved && len(review.CriticalIssues) > 0 {
			issues := strings.Join(review.CriticalIssues, "; ")
			sendEvent(emit, domain.EventWarn, "Reviewer found critical issues: "+issues)
			mem.addFailed(fmt.Sprintf("reviewer rejected subtask %d: %s", i+1, issues))
			mem.addDecisionEntry(domain.DecisionEntry{
				Decision: fmt.Sprintf("Reviewer rejected subtask %d, fix applied", i+1),
				Context:  issues,
				Source:   "reviewer",
			})
			fixTask := buildReviewFixTask(sub.Task, review)

			sendEvent(emit, domain.EventAgent, "current stage: coder (correction of reviewer comments)")
			sendEvent(emit, domain.EventAgent, "coder started")

			fixCtx := agent.WithRole(ctx, agent.RoleCoder)
			fixCtx = agent.WithPriority(fixCtx, agent.PriorityHigh)
			fixCtx = agent.WithPurpose(fixCtx, fmt.Sprintf("fix reviewer issues %d/%d", i+1, len(plan.Subtasks)))

			fixRes := s.executeSimple(fixCtx, fixTask, subOpts, emit)
			final.Iterations += fixRes.Iterations
			addFiles(fixRes)

			if !fixRes.Success {
				markPlan(i+1, domain.PlanFailed, "fix after reviewer failed")
				final.Success = false
				final.Errors = append(final.Errors, fixRes.Errors...)
				mem.addFailed("reviewer fix failed: " + strings.Join(fixRes.Errors, " | "))
				_ = mem.save(s.Cfg.WorkDir)
				rollback("reviewer fix failed")
				s.emitDispatcherUsage(emit)
				return final
			}

			itemStatus = domain.PlanWarn
			itemNote = "fixed after reviewer comments (build+tests passed)"
		} else if len(review.Suggestions) > 0 {
			itemStatus = domain.PlanWarn
			itemNote = fmt.Sprintf("reviewer suggestions: %d", len(review.Suggestions))
		}

		markPlan(i+1, itemStatus, itemNote)
		s.emitAgentBudget(emit, fmt.Sprintf("subtask %d", i+1))
	}

	// ─── Plan summary ──────────────────────────────────────────
	warnedItems := 0
	for _, st := range planStatuses {
		if st == domain.PlanWarn {
			warnedItems++
		}
	}
	summaryStatus := domain.PlanDone
	if warnedItems > 0 {
		summaryStatus = domain.PlanWarn
	}
	sendPlanSummary(emit, summaryStatus, len(plan.Subtasks), len(plan.Subtasks))
	s.emitAgentBudget(emit, "subtasks")

	// ─── Verifier ────────────────────────────────────────────────
    emitEvent(emit, domain.Event{
    	Type:      domain.EventAgent,
    	Message:   "current stage: verification",
    	TaskStage: domain.TaskStageVerifying,
    })
	sendEvent(emit, domain.EventAgent, "current stage: verifier")
	sendEvent(emit, domain.EventAgent, "verifier started")

	verification, err := s.runVerifier(ctx, query, plan, final, mem, emit)
	if err != nil {
		sendEvent(emit, domain.EventWarn, fmt.Sprintf("verifier failed: %v", err))
	} else {
		if len(verification.Risks) > 0 {
			final.AddWarning("verifier risks: " + strings.Join(verification.Risks, "; "))
		}
		if !verification.Completed {
			missing := strings.Join(verification.Missing, "; ")
			sendEvent(emit, domain.EventWarn, "Verifier: task not fully completed: "+missing)

			if strings.TrimSpace(verification.FixTask) != "" {
				sendEvent(emit, domain.EventAgent, "current stage: coder (fix verifier)")
				sendEvent(emit, domain.EventAgent, "coder started")

				fixCtx := agent.WithRole(ctx, agent.RoleCoder)
				fixCtx = agent.WithPriority(fixCtx, agent.PriorityCritical)
				fixCtx = agent.WithPurpose(fixCtx, "verifier fix")

				fixOpts := opts
				fixOpts.NoCommit = true

                emitEvent(emit, domain.Event{
                	Type:      domain.EventAgent,
                	Message:   "current stage: repairing",
                	TaskStage: domain.TaskStageRepairing,
                })
				fixRes := s.executeSimple(fixCtx, verification.FixTask, fixOpts, emit)
				final.Iterations += fixRes.Iterations
				addFiles(fixRes)

				if fixRes.Success {
					verification2, err2 := s.runVerifier(ctx, query, plan, final, mem, emit)
					if err2 == nil && verification2.Completed {
						final.Success = true
					} else {
						final.Success = false
						final.AddError("verifier did not confirm completion after fix")
					}
				} else {
					final.Success = false
					final.Errors = append(final.Errors, fixRes.Errors...)
				}
			} else {
				final.Success = false
				final.AddError("verifier reported incomplete task and provided no fix task")
			}
		}
	}
	s.emitAgentBudget(emit, "verification")

	// ─── Finalization ────────────────────────────────────────────
	if final.Success {
		final.Response = fmt.Sprintf("Full agent completed %d subtasks.", len(plan.Subtasks))
		if !opts.DryRun && !opts.NoCommit && s.Cfg.AutoGitCommit {
			hash, err := s.commit(ctx, query, emit)
			if err != nil {
				final.AddWarning(fmt.Sprintf("git commit failed: %v", err))
			} else {
				final.GitCommit = hash
			}
		}
	} else {
		final.AddWarning("agent finished unsuccessfully; changes were not automatically rolled back")
	}

	final.PreTaskHead = preTaskHead
	s.lastPreTaskHead = preTaskHead
	if !opts.DryRun {
		final.CumulativeDiff = s.captureCumulativeDiff(ctx, preTaskHead)
	}
	_ = mem.save(s.Cfg.WorkDir)
	s.emitDispatcherUsage(emit)
	return final
}

// planFullOrFallback пытается получить структурированный JSON-план.
func (s *Service) planFullOrFallback(
	ctx context.Context,
	query string,
    approach string,
	mem *agentMemory,
	emit func(domain.Event),
) *fullPlan {
	var prompt string
	if approach != "" {
		prompt = prompts.PlanFullWithApproach(query, approach, mem.summary(30))
	} else {
		prompt = prompts.PlanFull(query, mem.summary(30))
	}

	var plan fullPlan
	err := s.sendAgentJSON(
		ctx,
		agent.RolePlanner,
		agent.PriorityHigh,
		"create full plan",
		prompt,
		&plan,
	)

	if err != nil {
		sendEvent(emit, domain.EventWarn, fmt.Sprintf("structured plan failed, fallback to legacy plan: %v", err))
		planCtx := agent.WithRole(ctx, agent.RolePlanner)
		planCtx = agent.WithPriority(planCtx, agent.PriorityHigh)
		planCtx = agent.WithPurpose(planCtx, "fallback plan")
		oldPlan := s.plan(planCtx, query, emit)
		for _, task := range oldPlan {
			plan.Subtasks = append(plan.Subtasks, fullPlanSubtask{
				Task: task,
			})
		}
	}

	var clean []fullPlanSubtask
	for _, st := range plan.Subtasks {
		if strings.TrimSpace(st.Task) != "" {
			clean = append(clean, st)
		}
	}
	clean = sanitizePlanSubtasks(clean, emit)
	if len(clean) == 0 {
		clean = append(clean, fullPlanSubtask{
			Task: query,
		})
	}
	if len(clean) > 5 {
		clean = clean[:5]
	}
	plan.Subtasks = clean
	if strings.TrimSpace(plan.Goal) == "" {
		plan.Goal = query
	}
	return &plan
}


func (s *Service) runReviewer(
    ctx context.Context,
    originalTask string,
    subtask string,
    res domain.Result,
    mem *agentMemory,
    emit func(domain.Event),
) (agentReview, error) {
    changed := len(res.FilesCreated) +
        len(res.FilesModified) +
        len(res.FilesPatched) +
        len(res.FilesFullRewritten)
    if changed == 0 {
        return agentReview{Approved: true}, nil
    }

    maxTotal, maxPerFile := s.reviewLimits()
    summary := agentChangeSummaryWithLimits(res, maxTotal, maxPerFile)

    prompt := prompts.ReviewChanges(originalTask, subtask, summary, mem.summary(20))

    var review agentReview
    err := s.sendAgentJSON(
        ctx,
        agent.RoleReviewer,
        agent.PriorityHigh,
        "review changes",
        prompt,
        &review,
    )
    if err != nil {
        return agentReview{Approved: true}, err
    }
    if !review.Approved && len(review.CriticalIssues) == 0 {
        review.Approved = true
    }
    if len(review.Suggestions) > 0 {
        sendEvent(emit, domain.EventLog, "Reviewer suggestions: "+strings.Join(review.Suggestions, "; "))
    }
    return review, nil
}

// sendAgentReviewFlexible отправляет запрос к LLM и парсит ответ
// через промежуточную структуру rawAgentReview, которая принимает
// как строки, так и объекты в массивах.
func (s *Service) sendAgentReviewFlexible(
	ctx context.Context,
	prompt string,
) (agentReview, error) {
	ctx = agent.WithRole(ctx, agent.RoleReviewer)
	ctx = agent.WithPriority(ctx, agent.PriorityHigh)
	ctx = agent.WithPurpose(ctx, "review changes")

	response, err := s.LLM.Send(ctx, prompt)
	if err != nil {
		return agentReview{}, fmt.Errorf("llm send failed: %w", err)
	}

	// Сначала пробуем строгий парсинг в agentReview
	var strict agentReview
	if err := parseAgentJSON(response, &strict); err == nil {
		return strict, nil
	}

	// Если строгий парсинг не удался, пробуем гибкий через rawAgentReview
	var raw rawAgentReview
	if err := parseAgentJSON(response, &raw); err != nil {
		return agentReview{}, fmt.Errorf("flexible parse failed: %w", err)
	}

	review := convertRawReview(&raw)
	return review, nil
}

func (s *Service) runVerifier(
    ctx context.Context,
    originalTask string,
    plan *fullPlan,
    final domain.Result,
    mem *agentMemory,
    emit func(domain.Event),
) (agentVerification, error) {
    maxTotal, maxPerFile := s.reviewLimits()
    summary := agentVerificationSummaryWithLimits(final, maxTotal, maxPerFile)
    
    if plan != nil && len(plan.Acceptance) > 0 {
        summary += "\nacceptance criteria:\n- " + strings.Join(plan.Acceptance, "\n- ")
    }
    
    task := truncate(originalTask, 4000)
    prompt := prompts.VerifyCompletion(task, summary, mem.summary(20))

	var verification agentVerification
	err := s.sendAgentJSON(
		ctx,
		agent.RoleVerifier,
		agent.PriorityCritical,
		"verify completion",
		prompt,
		&verification,
	)
	if err != nil {
		return agentVerification{Completed: true}, err
	}
	sanitizeVerification(&verification)
	return verification, nil
}

func sanitizeVerification(v *agentVerification) {
	if v == nil || v.Completed {
		return
	}
	var blocking []string
	for _, item := range v.Missing {
		if isRuntimeOnlyVerificationItem(item) {
			v.Risks = append(v.Risks, "ignored runtime-only requirement: "+item)
			continue
		}
		blocking = append(blocking, item)
	}
	v.Missing = blocking
	if len(blocking) == 0 {
		v.Completed = true
		v.FixTask = ""
		return
	}
	if v.FixTask != "" && isRuntimeOnlyVerificationItem(v.FixTask) {
		v.FixTask = "Fix only file/content issues: " + strings.Join(blocking, "; ")
	}
}

func isRuntimeOnlyVerificationItem(s string) bool {
	lower := strings.ToLower(s)
	fileBlocking := []string{
		"file was not created",
		"missing file",
		"no file",
		"wrong file name",
		"file is missing",
		"create file",
		"add file",
		"modify file",
		"файл не создан",
		"файл отсутствует",
		"нет файла",
		"неверное имя файла",
		"создать файл",
		"добавить файл",
		"изменить файл",
	}
	if containsAny(lower, fileBlocking) {
		return false
	}
	runtimeOnly := []string{
		"execute", "executed", "execution", "run", "running", "runtime",
		"terminal output", "output was verified", "send request", "sent request",
		"http response", "response was", "start server", "server started",
		"manual", "manually", "chmod", "executable", "permission", "permissions",
		"curl", "wget",
		"запуск", "запустить", "выполнен", "выполнить", "вывод", "терминал",
		"отправлен", "отправить", "запрос", "исполняемым", "права",
	}
	return containsAny(lower, runtimeOnly)
}

func buildReviewFixTask(subtask string, review agentReview) string {
	var b strings.Builder
	b.WriteString("Fix ONLY the following critical issues in the previous subtask result.\n")
	b.WriteString("Do NOT add new features, do NOT refactor, do NOT change anything else.\n")
	b.WriteString("Original subtask:\n")
	b.WriteString(subtask)
	b.WriteString("\n")
	b.WriteString("Critical issues to fix (fix ONLY these, nothing else):\n")
	for _, issue := range review.CriticalIssues {
		b.WriteString("- ")
		b.WriteString(issue)
		b.WriteByte('\n')
	}
	b.WriteString(`
RULES:
1. Fix ONLY the listed critical issues.
2. Do NOT change any other code.
3. Do NOT add new files unless absolutely required by the fix.
4. Do NOT refactor or improve code beyond the fix.
5. The result must compile with go build.
6. Return changes in normal Gogitor format.
`)
	return strings.TrimSpace(b.String())
}

func agentVerificationSummaryWithLimits(res domain.Result, maxTotal, maxPerFile int) string {
    var b strings.Builder
    if len(res.OutputFiles) > 0 {
        b.WriteString("changed file snippets:\n")
        total := 0
        for _, f := range res.OutputFiles {
            if total >= maxTotal {
                break
            }
            header := "--- File: " + f.Path + " ---\n"
            content := truncate(f.Content, maxPerFile)
            b.WriteString(header)
            b.WriteString(content)
            b.WriteByte('\n')
            total += len(header) + len(content) + 1
        }
    }
    return strings.TrimSpace(b.String())
}

func agentVerificationSummary(res domain.Result) string {
    return agentVerificationSummaryWithLimits(res, 30000, 6000)
}

// agentChangeSummary формирует сводку изменений для ревьюера.
// Лимиты передаются извне для масштабирования от размера модели.
func agentChangeSummaryWithLimits(res domain.Result, maxTotal, maxPerFile int) string {

    var b strings.Builder

    if strings.TrimSpace(res.Response) != "" {
        b.WriteString("result: ")
        b.WriteString(truncate(res.Response, 3000))
        b.WriteByte('\n')
    }

    if len(res.FilesCreated) > 0 {
        b.WriteString("created files: ")
        b.WriteString(strings.Join(res.FilesCreated, ", "))
        b.WriteByte('\n')
    }
    if len(res.FilesModified) > 0 {
        b.WriteString("modified files: ")
        b.WriteString(strings.Join(res.FilesModified, ", "))
        b.WriteByte('\n')
    }
    if len(res.FilesPatched) > 0 {
        b.WriteString("patched files: ")
        b.WriteString(strings.Join(res.FilesPatched, ", "))
        b.WriteByte('\n')
    }
    if len(res.FilesFullRewritten) > 0 {
        b.WriteString("fully rewritten files: ")
        b.WriteString(strings.Join(res.FilesFullRewritten, ", "))
        b.WriteByte('\n')
    }

    if res.Tests.Run {
        if res.Tests.Failed == 0 {
            fmt.Fprintf(&b, "BUILD: PASSED | TESTS: ALL PASSED (passed=%d, coverage=%.1f%%)\n",
                res.Tests.Passed, res.Tests.Coverage)
        } else {
            fmt.Fprintf(&b, "BUILD: PASSED | TESTS: FAILED (passed=%d, failed=%d)\n",
                res.Tests.Passed, res.Tests.Failed)
        }
    } else if res.Tests.Skipped {
        b.WriteString("BUILD: PASSED | TESTS: skipped (no test files)\n")
    } else {
        b.WriteString("BUILD: PASSED\n")
    }

    if len(res.Warnings) > 0 {
        b.WriteString("warnings:\n")
        for _, w := range res.Warnings {
            b.WriteString("- ")
            b.WriteString(w)
            b.WriteByte('\n')
        }
    }
    if len(res.Errors) > 0 {
        b.WriteString("errors:\n")
        for _, e := range res.Errors {
            b.WriteString("- ")
            b.WriteString(e)
            b.WriteByte('\n')
        }
    }

    if exe := executableScriptNames(
        res.FilesCreated, res.FilesModified,
        res.FilesPatched, res.FilesFullRewritten,
    ); len(exe) > 0 {
        b.WriteString("executable script files: ")
        b.WriteString(strings.Join(exe, ", "))
        b.WriteByte('\n')
    }

    if len(res.OutputFiles) > 0 {
        b.WriteString("changed file snippets:\n")
        total := 0
        for _, f := range res.OutputFiles {
            if total >= maxTotal {
                break
            }
            header := "--- File: " + f.Path + " ---\n"
            content := truncate(f.Content, maxPerFile)
            b.WriteString(header)
            b.WriteString(content)
            b.WriteByte('\n')
            total += len(header) + len(content) + 1
        }
    }

    return strings.TrimSpace(b.String())
}


// Обратная совместимость: старая сигнатура с дефолтными лимитами
func agentChangeSummary(res domain.Result) string {
    return agentChangeSummaryWithLimits(res, 30000, 8000)
}

func sanitizePlanSubtasks(subtasks []fullPlanSubtask, emit func(domain.Event)) []fullPlanSubtask {
	var out []fullPlanSubtask
	for _, st := range subtasks {
		if isRuntimeOnlySubtask(st.Task) {
			sendEvent(emit, domain.EventWarn, "Skipped non-file subtask: "+st.Task)
			continue
		}
		out = append(out, st)
	}
	return out
}

func isRuntimeOnlySubtask(task string) bool {
	lower := strings.ToLower(task)
	fileChange := []string{
		"создай", "создать", "напиши", "писать", "сгенерируй", "сгенерировать",
		"реализуй", "реализовать", "измени", "изменить", "обнови", "обновить",
		"исправь", "исправить", "вынеси", "вынести", "перенеси", "перенести",
		"раздели", "разделить", "добавь файл", "добавить файл",
		"create", "write", "generate", "implement", "modify", "update",
		"fix", "refactor", "extract", "move", "split",
	}
	hasFileObject := strings.Contains(lower, ".sh") ||
		strings.Contains(lower, ".go") ||
		strings.Contains(lower, "файл") ||
		strings.Contains(lower, "file") ||
		strings.Contains(lower, "скрипт") ||
		strings.Contains(lower, "script")
	permissionKeywords := []string{
		"chmod", "исполняемым", "исполняемый", "executable", "права", "permission",
		"режим доступа", "file mode",
	}
	runtimeKeywords := []string{
		"запусти", "запустить", "запуск", "выполни", "выполнить", "run", "execute",
		"start", "launch", "curl", "wget", "отправь запрос", "отправить запрос",
		"отправляет запрос", "send request", "send http", "http request", "go run",
		"вручную", "manual", "проверь вручную", "test manually", "verify by running",
	}
	if containsAny(lower, permissionKeywords) {
		if hasFileObject && containsAny(lower, fileChange) {
			return false
		}
		return true
	}
	if containsAny(lower, runtimeKeywords) {
		if hasFileObject && containsAny(lower, fileChange) {
			return false
		}
		return true
	}
	return false
}

func executableScriptNames(lists ...[]string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, list := range lists {
		for _, p := range list {
			if p == "" || seen[p] {
				continue
			}
			lower := strings.ToLower(p)
			if strings.HasSuffix(lower, ".sh") ||
				strings.HasSuffix(lower, ".bash") ||
				strings.HasSuffix(lower, ".zsh") ||
				strings.HasSuffix(lower, ".fish") ||
				strings.HasSuffix(lower, ".command") {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	return out
}
