package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"gogitor/internal/agent"
	"gogitor/internal/domain"
	"gogitor/internal/prompts"
	"gogitor/internal/runner"
	"gogitor/internal/security"
)

// fullPlan — структурированный план от planner agent.
type fullPlan struct {
	Goal       string            `json:"goal"`
	Acceptance []string          `json:"acceptance"`
	Subtasks   []fullPlanSubtask `json:"subtasks"`
}

type fullPlanSubtask struct {
	Task        string   `json:"task"`
	Acceptance  []string `json:"acceptance"`
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
	Approved       bool  `json:"approved"`
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
	depth := normalizeAgentDepth(string(opts.AgentDepth))

	if depth == AgentDepthAuto {
		depth = s.agentDepthForTask(query)
	}

	if depth != AgentDepthDeep {
		depth = AgentDepthNormal
	}

	opts.AgentDepth = depth

	deep := depth == AgentDepthDeep

	if deep {
		sendEvent(
			emit,
			domain.EventAgent,
			"Agent profile: deep",
		)
	} else {
		sendEvent(
			emit,
			domain.EventAgent,
			"Agent profile: normal",
		)

	}

	if s.Cfg.AutoSearch && s.isRemoteLLM() {
		sendEvent(emit, domain.EventWarn,
			"WARNING: auto-search is enabled with a REMOTE LLM provider. "+
				"Project code and search queries will be sent to external servers. "+
				"Use a local Ollama instance for sensitive projects.")
	}

	sendEvent(
		emit,
		domain.EventLog,
		"Full agent mode: planner + coder + reviewer + verifier",
	)
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
			errMsg := fmt.Sprintf("cannot create agent checkpoint: %v", err)
			sendEvent(emit, domain.EventError, errMsg)
			return domain.Result{
				Success: false, Mode: "agent", DryRun: opts.DryRun,
				Errors: []string{errMsg},
			}
		}
		defer checkpoint.cleanup()
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
	patched := map[string]bool{}
	fullRewritten := map[string]bool{}

	var session *agentSession

	session, err := s.startAgentSession(
		query,
		depth,
		preTaskHead,
		opts.InterviewAnswers,
	)

	state := &agentSessionState{
		Version:     1,
		Task:        query,
		Depth:       depth,
		StartedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		PreTaskHead: preTaskHead,
		Status:      "planning",
		ResumedFrom: opts.AgentResumeSource,
	}

	if session != nil {
		if err := saveAgentState(
			session,
			state,
		); err != nil {
			final.AddWarning(
				fmt.Sprintf(
					"agent state could not be saved: %v",
					err,
				),
			)
		}

		defer func() {
			if final.Success {
				state.Status = "completed"
			} else {
				state.Status = "failed"
			}

			state.GitCommit = final.GitCommit

			if err := saveAgentState(
				session,
				state,
			); err != nil {
				final.AddWarning(
					fmt.Sprintf(
						"agent final state could not be saved: %v",
						err,
					),
				)
			}
		}()
	}

	if err != nil {
		final.AddWarning(
			fmt.Sprintf(
				"agent session could not be created: %v",
				err,
			),
		)
	}

	if session != nil {
		defer func() {
			if err := saveAgentResult(
				session,
				&final,
			); err != nil {
				final.AddWarning(
					fmt.Sprintf(
						"agent result could not be saved: %v",
						err,
					),
				)
			}
		}()
	}

    addResultFiles := func(
    	res domain.Result,
    	createdSet map[string]bool,
    	modifiedSet map[string]bool,
    	patchedSet map[string]bool,
    	fullRewrittenSet map[string]bool,
    ) {
    	for _, f := range res.FilesCreated {
    		createdSet[f] = true
    	}
    
    	for _, f := range res.FilesModified {
    		modifiedSet[f] = true
    	}
    
    	for _, f := range res.FilesPatched {
    		modifiedSet[f] = true
    		patchedSet[f] = true
    	}
    
    	for _, f := range res.FilesFullRewritten {
    		modifiedSet[f] = true
    		fullRewrittenSet[f] = true
    	}
    }

    addFiles := func(res domain.Result) {
    	addResultFiles(
    		res,
    		created,
    		modified,
    		patched,
    		fullRewritten,
    	)
    
    	final.FilesCreated =
    		sortedKeys(created)
    
    	final.FilesModified =
    		sortedKeys(modified)
    
    	final.FilesPatched =
    		sortedKeys(patched)
    
    	final.FilesFullRewritten =
    		sortedKeys(fullRewritten)
    
    	final.OutputFiles =
    		mergeOutputFiles(
    			final.OutputFiles,
    			res.OutputFiles,
    		)
    }

	rollback := func(reason string) {
		if opts.DryRun || checkpoint == nil {
            state.CompletedSubtasks = 0
            state.CurrentSubtask = 0
            
            if session != nil {
            	if err := saveAgentState(
            		session,
            		state,
            	); err != nil {
            		final.AddWarning(
            			fmt.Sprintf(
            				"cannot save rollback state: %v",
            				err,
            			),
            		)
            	}
            }
			return
		}
		sendEvent(emit, domain.EventWarn, "Rollback: "+reason)
		err := s.rollbackAgentCheckpoint(checkpoint, sortedKeys(created), sortedKeys(modified))
		if err != nil {
			final.AddWarning(fmt.Sprintf("rollback failed: %v", err))
			return
		}

		final.AddWarning("changes were rolled back to pre-agent state")
	}

	// ─── Planning ────────────────────────────────────────────────
	var plan *fullPlan

	if opts.AgentResumePlan != nil {
		plan = opts.AgentResumePlan

		plan = validateAgentPlan(
			plan,
			query,
		)

		sendEvent(
			emit,
			domain.EventAgent,
			"resuming saved agent plan",
		)
	} else {
		plan = s.planFullOrFallback(
			ctx,
			query,
			approach,
			mem,
			emit,
		)
	}

	if session != nil {
		if err := saveAgentPlan(
			session,
			query,
			plan,
		); err != nil {
			final.AddWarning(
				fmt.Sprintf(
					"agent plan could not be saved: %v",
					err,
				),
			)
		}
	}
	sendEvent(emit, domain.EventAgent, fmt.Sprintf("planner completed: goal=%s", plan.Goal))
	s.emitAgentBudget(emit, "planning")

	mem.addDecision(fmt.Sprintf("plan goal: %s", plan.Goal))
	mem.addDecisionEntry(domain.DecisionEntry{Decision: fmt.Sprintf("Plan goal: %s", plan.Goal), Context: query, Source: "planner"})
	for i, st := range plan.Subtasks {
		mem.addDecision(fmt.Sprintf("planned subtask %d: %s", i+1, st.Task))
		mem.addDecisionSimple(fmt.Sprintf("Subtask %d: %s", i+1, st.Task), "planner")
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

	resumeFrom := opts.AgentResumeFrom

	if resumeFrom < 0 {
		resumeFrom = 0
	}

	if resumeFrom > len(plan.Subtasks) {
		resumeFrom = len(plan.Subtasks)
	}

	completedSubtasks := resumeFrom

	state.TotalSubtasks = len(plan.Subtasks)
	state.CompletedSubtasks = completedSubtasks
	state.CurrentSubtask = resumeFrom

	if session != nil {
		_ = saveAgentState(
			session,
			state,
		)
	}
	markPlan := func(index int, st domain.PlanStatus, note string) {
		if index >= 1 && index <= len(planStatuses) {
			planStatuses[index-1] = st
		}
		sendPlanStatus(emit, index, len(plan.Subtasks), planItems[index-1], st, note)
	}

	// ─── Subtask execution ───────────────────────────────────────
	for i, sub := range plan.Subtasks {
		if i < resumeFrom {
			planStatuses[i] =
				domain.PlanDone

			continue
		}

		state.CurrentSubtask = i + 1

		if session != nil {
			_ = saveAgentState(
				session,
				state,
			)
		}

		sendEvent(
			emit,
			domain.EventAgent,
			fmt.Sprintf(
				"current subtask %d/%d: %s",
				i+1,
				len(plan.Subtasks),
				sub.Task,
			),
		)

		markPlan(i+1, domain.PlanRunning, "")

        var subtaskCheckpoint *agentCheckpoint
        
        subtaskCreated := map[string]bool{}
        subtaskModified := map[string]bool{}
        subtaskPatched := map[string]bool{}
        subtaskFullRewritten := map[string]bool{}
        
        if !opts.DryRun {
        	subtaskCheckpoint, err =
        		s.createAgentCheckpoint(ctx)
        
        	if err != nil {
        		final.Success = false
        
        		final.AddError(
        			fmt.Sprintf(
        				"cannot create checkpoint for subtask %d: %v",
        				i+1,
        				err,
        			),
        		)
        
        		rollback(
        			"cannot create subtask checkpoint",
        		)
        
        		return final
        	}
        }

        rollbackSubtask := func(reason string) {
        	if opts.DryRun ||
        		subtaskCheckpoint == nil {
        		return
        	}
        
        	sendEvent(
        		emit,
        		domain.EventWarn,
        		"Subtask rollback: "+reason,
        	)
        
        	err := s.rollbackAgentCheckpoint(
        		subtaskCheckpoint,
        		sortedKeys(subtaskCreated),
        		sortedKeys(subtaskModified),
        	)
        
        	if err != nil {
        		final.AddWarning(
        			fmt.Sprintf(
        				"subtask rollback failed: %v",
        				err,
        			),
        		)
        	} else {
        		final.AddWarning(
        			"current subtask changes were rolled back; previous subtasks preserved",
        		)
        	}
        
        	subtaskCheckpoint.cleanup()
        	subtaskCheckpoint = nil
        }

		subOpts := opts
		subOpts.NoCommit = true
		subOpts.ProgressItem = i + 1
		subOpts.ProgressTotal = len(plan.Subtasks)

		taskForCoder := sub.Task
		searchContext := ""

		if sub.NeedsSearch &&
			s.Cfg.AutoSearch &&
			s.SafeSearch != nil {

			searchContext, err :=
				s.searchForSubtask(
					ctx,
					sub.Task,
					emit,
				)

			if err == nil &&
				searchContext != "" {
				sendEvent(
					emit,
					domain.EventLog,
					"Web search context added to subtask",
				)
			}
		}

		if deep {
			taskForCoder = formatAgentTask(
				query,
				plan,
				sub,
				i,
				len(plan.Subtasks),
				searchContext,
			)
		} else {
			if len(sub.Acceptance) > 0 {
				taskForCoder +=
					"\nAcceptance criteria:\n- " +
						strings.Join(
							sub.Acceptance,
							"\n- ",
						)
			}

			if searchContext != "" {
				taskForCoder +=
					"\n\n" +
						searchContext
			}
		}

		if approach != "" {
			taskForCoder +=
				"\n\nSELECTED IMPLEMENTATION APPROACH (follow this):\n" +
					approach
		}

		isAnalysis := s.isAnalysisOnlyTask(
			taskForCoder,
		)
		if !isAnalysis {
			sendEvent(emit, domain.EventAgent, "current stage: coder")
			sendEvent(emit, domain.EventAgent, "coder started")
		}

		subCtx := agent.WithRole(ctx, agent.RoleCoder)
		subCtx = agent.WithPriority(subCtx, agent.PriorityNormal)
		subCtx = agent.WithPurpose(subCtx, fmt.Sprintf("subtask %d/%d", i+1, len(plan.Subtasks)))
        recordSubtaskFiles := func(res domain.Result) {
        	addResultFiles(
        		res,
        		subtaskCreated,
        		subtaskModified,
        		subtaskPatched,
        		subtaskFullRewritten,
        	)
        
        	addFiles(res)
        }

		res := s.executeSimple(subCtx, taskForCoder, subOpts, emit)
		final.Iterations += res.Iterations
		recordSubtaskFiles(res)

		if !res.Success {
			markPlan(i+1, domain.PlanFailed, truncate(strings.Join(res.Errors, "; "), 200))
			final.Success = false
			final.Errors = append(final.Errors, res.Errors...)
            rollbackSubtask("subtask failed")
			return final
		}

		if isAnalysis {
			markPlan(i+1, domain.PlanDone, "")
			completedSubtasks = i + 1

			state.CompletedSubtasks =
				completedSubtasks

			state.CurrentSubtask =
				completedSubtasks

			if session != nil {
				if err := saveAgentState(
					session,
					state,
				); err != nil {
					final.AddWarning(
						fmt.Sprintf(
							"cannot save agent progress: %v",
							err,
						),
					)
				}
			}

			continue
		}

		// ─── Reviewer ─────────────────────────────────────────
		changedFiles := len(res.FilesCreated) + len(res.FilesModified) + len(res.FilesPatched) + len(res.FilesFullRewritten)
		itemStatus := domain.PlanDone
		itemNote := ""

		if changedFiles > 0 {
			sendEvent(emit, domain.EventAgent, "current stage: reviewer")
			review, err := s.runReviewer(ctx, query, sub.Task, res, mem, emit)

			if err != nil {
				if deep {
					markPlan(
						i+1,
						domain.PlanFailed,
						"reviewer unavailable",
					)

					final.Success = false

					final.AddError(
						fmt.Sprintf(
							"reviewer failed in deep mode: %v",
							err,
						),
					)
                    rollbackSubtask(
                    	"reviewer unavailable in deep mode",
                    )
					return final
				}

				itemStatus = domain.PlanWarn
				itemNote = "reviewer unavailable"

			} else if !review.Approved &&
				len(review.CriticalIssues) > 0 {

				issues := strings.Join(review.CriticalIssues, "; ")
				sendEvent(emit, domain.EventWarn, "Reviewer found critical issues: "+issues)
				fixTask := buildReviewFixTask(sub.Task, review)
				fixRes := s.executeSimple(agent.WithRole(ctx, agent.RoleCoder), fixTask, subOpts, emit)
				final.Iterations += fixRes.Iterations
				recordSubtaskFiles(fixRes)
				if !fixRes.Success {
					markPlan(i+1, domain.PlanFailed, "fix after reviewer failed")
					final.Success = false
					final.Errors = append(final.Errors, fixRes.Errors...)
                    rollbackSubtask(
                    	"reviewer fix failed",
                    )
					return final
				}
				itemStatus = domain.PlanWarn
				itemNote = "fixed after reviewer comments"
			} else if len(review.Suggestions) > 0 {
				itemStatus = domain.PlanWarn
				itemNote = fmt.Sprintf("reviewer suggestions: %d", len(review.Suggestions))
			}
		}
		if deep && !isAnalysis {
			sendEvent(
				emit,
				domain.EventAgent,
				"current stage: quality gates",
			)

			gate := s.runAgentDeepQualityGates(
				ctx,
				emit,
				i+1,
				opts.NoTests,
			)

			final.QualityGates =
				gate.toDomain()

			if session != nil {
				if err := saveAgentGateReport(
					session.gatePath(i+1),
					&gate,
				); err != nil {
					final.AddWarning(
						fmt.Sprintf(
							"cannot save gate report: %v",
							err,
						),
					)
				}
			}

			if session != nil {
				appendAgentProcess(
					session.ProcessPath,
					fmt.Sprintf(
						"Quality gate task %d: passed=%v",
						i+1,
						gate.Passed,
					),
				)
			}

			if !gate.Passed {
				markPlan(
					i+1,
					domain.PlanFailed,
					"quality gates failed",
				)

				final.Success = false
				final.Errors = append(
					final.Errors,
					gate.Errors...,
				)

                rollbackSubtask(
                	"agent deep quality gates failed",
                )
				return final
			}
		}

		markPlan(
			i+1,
			itemStatus,
			itemNote,
		)

        completedSubtasks = i + 1
        
        state.CompletedSubtasks =
        	completedSubtasks
        
        state.CurrentSubtask =
        	completedSubtasks
        
        if session != nil {
        	if err := saveAgentState(
        		session,
        		state,
        	); err != nil {
        		final.AddWarning(
        			fmt.Sprintf(
        				"cannot save agent progress: %v",
        				err,
        			),
        		)
        	}
        }
        
        if subtaskCheckpoint != nil {
        	subtaskCheckpoint.cleanup()
        	subtaskCheckpoint = nil
        }

	}

	// -----------------------------------------------------------
	// Deterministic verification.
	// Для Deep эта проверка уже входит в final quality gates.
	// Для Normal выполняем минимальные объективные проверки.
	// -----------------------------------------------------------

	if !deep {
		check := s.runAgentDeterministicChecks(
			ctx,
			final,
			opts.NoTests,
			emit,
		)

		final.Tests = check.Tests

		if !check.Passed() {
			final.Success = false
			final.Errors = append(
				final.Errors,
				check.Errors...,
			)

			rollback(
				"deterministic final verification failed",
			)

			return final
		}
	}

	// ─── Verifier & Finalization ───────────────────────────────
	sendEvent(
		emit,
		domain.EventAgent,
		"current stage: verifier",
	)

	verification, err := s.runVerifier(
		ctx,
		query,
		plan,
		final,
		mem,
		emit,
	)

	if err != nil {
		final.Success = false
		final.AddError(
			"verifier failed: " + err.Error(),
		)

		rollback("verifier failed")
		return final
	}

	// -----------------------------------------------------------
	// Если verifier обнаружил реальные пропуски,
	// исправляем их и ОБЯЗАТЕЛЬНО запускаем verifier повторно.
	// -----------------------------------------------------------

	if !verification.Completed {
		if strings.TrimSpace(
			verification.FixTask,
		) == "" {

			final.Success = false

			if len(verification.Missing) > 0 {
				final.AddError(
					"verifier: task is incomplete: " +
						strings.Join(
							verification.Missing,
							"; ",
						),
				)
			} else {
				final.AddError(
					"verifier: task is incomplete",
				)
			}

			rollback(
				"verifier reported incomplete task",
			)

			return final
		}

		sendEvent(
			emit,
			domain.EventAgent,
			"current stage: coder (verifier fix)",
		)

		fixOpts := opts
		fixOpts.NoCommit = true

		fixRes := s.executeSimple(
			agent.WithRole(
				ctx,
				agent.RoleCoder,
			),
			verification.FixTask,
			fixOpts,
			emit,
		)

		final.Iterations += fixRes.Iterations
		addFiles(fixRes)

		if !fixRes.Success {
			final.Success = false
			final.Errors = append(
				final.Errors,
				fixRes.Errors...,
			)

			rollback(
				"verifier fix failed",
			)

			return final
		}

		if !deep {
			check := s.runAgentDeterministicChecks(
				ctx,
				final,
				opts.NoTests,
				emit,
			)

			final.Tests = check.Tests

			if !check.Passed() {
				final.Success = false
				final.Errors = append(
					final.Errors,
					check.Errors...,
				)

				rollback(
					"deterministic verification after verifier fix failed",
				)

				return final
			}
		}

		// Повторная verification — ОБЯЗАТЕЛЬНА.
		verification, err =
			s.runVerifier(
				ctx,
				query,
				plan,
				final,
				mem,
				emit,
			)

		if err != nil {
			final.Success = false
			final.AddError(
				"verifier after fix failed: " +
					err.Error(),
			)

			rollback(
				"verifier after fix failed",
			)

			return final
		}

		if !verification.Completed {
			final.Success = false

			if len(verification.Missing) > 0 {
				final.AddError(
					"verifier: task remains incomplete: " +
						strings.Join(
							verification.Missing,
							"; ",
						),
				)
			} else {
				final.AddError(
					"verifier: task remains incomplete",
				)
			}

			for _, risk := range verification.Risks {
				final.AddWarning(
					"verifier risk: " + risk,
				)
			}

			rollback(
				"task remains incomplete after verifier fix",
			)

			return final
		}
	}

	// -----------------------------------------------------------
	// Финальные Deep quality gates.
	// Они выполняются ПОСЛЕ verifier,
	// потому что verifier fix мог изменить код.
	// -----------------------------------------------------------

	if deep {
		sendEvent(
			emit,
			domain.EventAgent,
			"current stage: final quality gates",
		)

		finalGate := s.runAgentDeepQualityGates(
			ctx,
			emit,
			0,
			opts.NoTests,
		)

		final.QualityGates =
			finalGate.toDomain()

		if session != nil {
			if err := saveAgentGateReport(
				session.FinalGatePath,
				&finalGate,
			); err != nil {
				final.AddWarning(
					fmt.Sprintf(
						"cannot save final gate report: %v",
						err,
					),
				)
			}
		}

		if !finalGate.Passed {
			final.Success = false
			final.Errors = append(
				final.Errors,
				finalGate.Errors...,
			)

			rollback(
				"final quality gates failed",
			)

			return final
		}
	}

	// -----------------------------------------------------------
	// Только теперь задача считается успешно выполненной.
	// -----------------------------------------------------------


	if !opts.DryRun &&
		!opts.NoCommit &&
		s.Cfg.AutoGitCommit {

		hash, err := s.commit(
			ctx,
			query,
			emit,
		)

		if err != nil {
			final.AddWarning(
				fmt.Sprintf(
					"git commit failed: %v",
					err,
				),
			)
		} else if hash != "" {
			final.GitCommit = hash

			if session != nil {
				appendAgentProcess(
					session.ProcessPath,
					"Final Git commit: "+hash,
				)
			}
		}
	}

    completedForReport := 0
    totalForReport := len(plan.Subtasks)
    
    if state != nil {
    	completedForReport =
    		state.CompletedSubtasks
    }
    
    final.Response = formatAgentTaskReport(
    	final,
    	depth,
    	completedForReport,
    	totalForReport,
    )

	final.PreTaskHead = preTaskHead
	s.lastPreTaskHead = preTaskHead
	if !opts.DryRun {
		final.CumulativeDiff = s.captureCumulativeDiff(ctx, preTaskHead)
	}
	_ = mem.save(s.Cfg.WorkDir)
	return final
}

func saveAgentResult(
	session *agentSession,
	result *domain.Result,
) error {
	if session == nil || result == nil {
		return nil
	}

	data, err := json.MarshalIndent(
		result,
		"",
		"  ",
	)
	if err != nil {
		return err
	}

	return os.WriteFile(
		session.ResultPath,
		data,
		0o644,
	)
}

func saveAgentPlan(
	session *agentSession,
	query string,
	plan *fullPlan,
) error {
	if session == nil || plan == nil {
		return nil
	}

	data, err := json.MarshalIndent(
		plan,
		"",
		"  ",
	)
	if err != nil {
		return err
	}

	if err := os.WriteFile(
		session.PlanJSONPath,
		data,
		0o644,
	); err != nil {
		return err
	}

	var b strings.Builder

	b.WriteString("# Agent Implementation Plan\n\n")
	b.WriteString("## Original task\n\n")
	b.WriteString(strings.TrimSpace(query))
	b.WriteString("\n\n")

	b.WriteString("## Goal\n\n")
	b.WriteString(plan.Goal)
	b.WriteString("\n\n")

	if len(plan.Acceptance) > 0 {
		b.WriteString("## Acceptance criteria\n\n")
		for _, item := range plan.Acceptance {
			fmt.Fprintf(
				&b,
				"- %s\n",
				item,
			)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Subtasks\n\n")

	for i, sub := range plan.Subtasks {
		fmt.Fprintf(
			&b,
			"%d. %s\n",
			i+1,
			sub.Task,
		)

		for _, acceptance := range sub.Acceptance {
			fmt.Fprintf(
				&b,
				"   - %s\n",
				acceptance,
			)
		}
	}

	return os.WriteFile(
		session.PlanPath,
		[]byte(b.String()),
		0o644,
	)
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
		prompt = prompts.PlanFullWithApproach(
			query,
			approach,
			mem.summary(30),
		)
	} else {
		prompt = prompts.PlanFull(
			query,
			mem.summary(30),
		)
	}

	prompt = s.appendProjectInstructions(prompt)

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
	if len(clean) > 7 {
		clean = clean[:7]
	}
    plan.Subtasks = clean
    
    validated := validateAgentPlan(
    	&plan,
    	query,
    )
    
    return s.limitAgentPlan(validated)
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

	prompt := prompts.ReviewChanges(
		originalTask,
		subtask,
		summary,
		mem.summary(20),
	)

	prompt = s.appendProjectInstructions(prompt)
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
		return agentReview{}, err
	}

	if !review.Approved && len(review.CriticalIssues) == 0 {
		review.Approved = true
	}
	if len(review.Suggestions) > 0 {
		sendEvent(emit, domain.EventLog, "Reviewer suggestions: "+strings.Join(review.Suggestions, "; "))
	}
	return review, nil
}

func (s *Service) limitAgentPlan(
	plan *fullPlan,
) *fullPlan {
	if plan == nil {
		return plan
	}

	max := s.agentModelCapabilities().MaxSubtasks

	if max <= 0 ||
		len(plan.Subtasks) <= max {
		return plan
	}

	plan.Subtasks =
		plan.Subtasks[:max]

	return plan
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

	summary := agentChangeSummaryWithLimits(
		final,
		maxTotal,
		maxPerFile,
	)
	if plan != nil && len(plan.Acceptance) > 0 {
		summary += "\nacceptance criteria:\n- " + strings.Join(plan.Acceptance, "\n- ")
	}

	task := truncate(originalTask, 4000)
	prompt := prompts.VerifyCompletion(
		task,
		summary,
		mem.summary(20),
	)

	prompt = s.appendProjectInstructions(prompt)
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
		return agentVerification{Completed: false}, fmt.Errorf(
			"verifier request failed: %w",
			err,
		)
	}
	sanitizeVerification(&verification)
	return verification, nil
}

type agentDeterministicCheck struct {
	FilesOK bool
	BuildOK bool
	TestsOK bool
	Tests   domain.TestsStatus
	Errors  []string
}

func (c agentDeterministicCheck) Passed() bool {
	return c.FilesOK &&
		c.BuildOK &&
		c.TestsOK &&
		len(c.Errors) == 0
}

func (s *Service) runAgentDeterministicChecks(
	ctx context.Context,
	result domain.Result,
	noTests bool,
	emit func(domain.Event),
) agentDeterministicCheck {
	check := agentDeterministicCheck{
		FilesOK: true,
	}

	sandbox, err :=
		s.WS.PrepareSandbox(ctx)

	if err != nil {
		check.Errors = append(
			check.Errors,
			fmt.Sprintf(
				"cannot prepare verification sandbox: %v",
				err,
			),
		)
		return check
	}

	defer os.RemoveAll(sandbox)

	// ------------------------------------------------------------
	// 1. Проверяем, что все заявленные изменённые файлы существуют.
	// ------------------------------------------------------------

	files := make(
		map[string]bool,
	)

	for _, path := range result.FilesCreated {
		files[path] = true
	}

	for _, path := range result.FilesModified {
		files[path] = true
	}

	for _, path := range result.FilesPatched {
		files[path] = true
	}

	for _, path := range result.FilesFullRewritten {
		files[path] = true
	}

	for path := range files {
		full, err :=
			security.SafeJoin(
				sandbox,
				path,
			)

		if err != nil {
			check.FilesOK = false
			check.Errors = append(
				check.Errors,
				fmt.Sprintf(
					"invalid changed path %s: %v",
					path,
					err,
				),
			)
			continue
		}

		info, err :=
			os.Stat(full)

		if err != nil {
			check.FilesOK = false
			check.Errors = append(
				check.Errors,
				fmt.Sprintf(
					"changed file is missing: %s",
					path,
				),
			)
			continue
		}

		if info.IsDir() {
			check.FilesOK = false
			check.Errors = append(
				check.Errors,
				fmt.Sprintf(
					"changed path is a directory: %s",
					path,
				),
			)
		}
	}

	if !check.FilesOK {
		return check
	}

	// ------------------------------------------------------------
	// 2. Build.
	// ------------------------------------------------------------

	emitEvent(
		emit,
		domain.Event{
			Type:      domain.EventLog,
			Message:   "Running deterministic final build check",
			TaskStage: domain.TaskStageVerifying,
		},
	)

	if err := s.Runner.Build(
		ctx,
		sandbox,
	); err != nil {
		check.Errors = append(
			check.Errors,
			"final build check failed: "+
				trim(err.Error(), 4000),
		)
		return check
	}

	check.BuildOK = true

	// ------------------------------------------------------------
	// 3. Tests.
	// ------------------------------------------------------------

	if noTests {
		check.TestsOK = true
		check.Tests = domain.TestsStatus{
			Skipped: true,
		}
		return check
	}

	emitEvent(
		emit,
		domain.Event{
			Type:      domain.EventLog,
			Message:   "Running deterministic final test check",
			TaskStage: domain.TaskStageVerifying,
		},
	)

	tests, testErr :=
		s.Runner.Test(
			ctx,
			sandbox,
		)

	check.Tests = tests

	if testErr != nil {
		check.Errors = append(
			check.Errors,
			"final test check failed: "+
				trim(testErr.Error(), 4000),
		)
		return check
	}

	if tests.Failed > 0 {
		check.Errors = append(
			check.Errors,
			runner.FormatFeedback(tests),
		)
		return check
	}

	check.TestsOK = true
	return check
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

	// Разделяем проблемы на файловые и прочие.
	var fileIssues []string
	var otherIssues []string
	for _, issue := range review.CriticalIssues {
		lower := strings.ToLower(issue)
		if strings.Contains(lower, "absent") ||
			strings.Contains(lower, "missing") ||
			strings.Contains(lower, "not created") ||
			strings.Contains(lower, "required file") {
			fileIssues = append(fileIssues, issue)
		} else {
			otherIssues = append(otherIssues, issue)
		}
	}

	if len(fileIssues) > 0 {
		b.WriteString("\n⚠ CRITICAL: REQUIRED FILES ARE MISSING\n")
		b.WriteString("You MUST create these files:\n")
		for _, issue := range fileIssues {
			b.WriteString("- " + issue + "\n")
		}
		b.WriteString("\n")
	}

	if len(otherIssues) > 0 {
		b.WriteString("Critical issues to fix (fix ONLY these, nothing else):\n")
		for _, issue := range otherIssues {
			b.WriteString("- " + issue + "\n")
		}
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
