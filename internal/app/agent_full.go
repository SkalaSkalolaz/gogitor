package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
	"path/filepath"

	"gogitor/internal/agent"
	"gogitor/internal/domain"
    "gogitor/internal/llm"
	"gogitor/internal/prompts"
	"gogitor/internal/runner"
	"gogitor/internal/security"
	"gogitor/internal/textutil"
)

// fullPlan — структурированный план от planner agent.
type fullPlan struct {
	Goal       string            `json:"goal"`
	Acceptance []string          `json:"acceptance"`
	Subtasks   []fullPlanSubtask `json:"subtasks"`
}

type fullPlanSubtask struct {
	Task           string   `json:"task"`
	Acceptance     []string `json:"acceptance"`
	NeedsSearch    bool     `json:"needs_search"`
	SaveResearchTo string   `json:"save_research_to,omitempty"`
	UsesResearch   []string `json:"uses_research,omitempty"`
}

type agentSubtaskState string

const (
	agentSubtaskRequired         agentSubtaskState = "required"
	agentSubtaskAlreadySatisfied agentSubtaskState = "already_satisfied"
	agentSubtaskUnknown          agentSubtaskState = "unknown"
)

type agentSubtaskAssessment struct {
	State  agentSubtaskState
	Reason string
}

const (
	maxAgentPlanContextBytes     = 96000
	maxPreviousSubtaskDeltaBytes = 24000
	maxAgentSubtaskAttempts      = 2
    maxSubtaskResearchFileBytes  = 24000
	maxSubtaskResearchTotalBytes = 36000
	maxSubtaskResearchRawBytes   = 16000
)

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

type agentTaskSymbolSpan struct {
	Name  string
	Start int
	End   int
}

func sourceFunctionNames(
	source string,
) map[string]bool {

	names := make(map[string]bool)

	re :=
		regexp.MustCompile(
			`(?m)^\s*func\s+(?:\([^)]*\)\s*)?([A-Za-z_][A-Za-z0-9_]*)\s*\(`,
		)

	matches :=
		re.FindAllStringSubmatch(source, -1)

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}

		name :=
			strings.TrimSpace(match[1])

		if name != "" {
			names[name] = true
		}
	}

	return names
}

func taskSymbolSpans(
	task string,
	names map[string]bool,
) []agentTaskSymbolSpan {

	var spans []agentTaskSymbolSpan
	seen := make(map[string]bool)

	for name := range names {
		if seen[name] {
			continue
		}

		re :=
			regexp.MustCompile(
				`\b` +
					regexp.QuoteMeta(name) +
					`\b`,
			)

		locs :=
			re.FindAllStringIndex(
				task,
				-1,
			)

		for _, loc := range locs {
			if len(loc) != 2 {
				continue
			}

			spans = append(
				spans,
				agentTaskSymbolSpan{
					Name:  name,
					Start: loc[0],
					End:   loc[1],
				},
			)
		}

		if len(locs) > 0 {
			seen[name] = true
		}
	}

	sort.Slice(
		spans,
		func(i, j int) bool {
			return spans[i].Start <
				spans[j].Start
		},
	)

	return spans
}

func isAtomicTaskJoiner(
	text string,
) bool {

	text =
		strings.ToLower(
			strings.TrimSpace(text),
		)

	text =
		strings.Trim(
			text,
			" \t\r\n,;+&",
		)

	switch text {
	case "":
		return true

	case "and":
		return true

	case "и":
		return true
	}

	return false
}

func splitCompoundAgentSubtask(
	sub fullPlanSubtask,
	source string,
) []fullPlanSubtask {

	task :=
		strings.TrimSpace(
			sub.Task,
		)

	if task == "" {
		return []fullPlanSubtask{sub}
	}

	names :=
		sourceFunctionNames(source)

	if len(names) < 2 {
		return []fullPlanSubtask{sub}
	}

	spans :=
		taskSymbolSpans(
			task,
			names,
		)

	if len(spans) < 2 {
		return []fullPlanSubtask{sub}
	}

	// Удаляем повторные упоминания одного и того же
	// символа из рассмотрения.
	var unique []agentTaskSymbolSpan
	seen := make(map[string]bool)

	for _, span := range spans {
		if seen[span.Name] {
			continue
		}

		seen[span.Name] = true
		unique = append(
			unique,
			span,
		)
	}

	if len(unique) < 2 {
		return []fullPlanSubtask{sub}
	}

	for i := 1; i < len(unique); i++ {
		gap :=
			task[unique[i-1].End:unique[i].Start]

		if !isAtomicTaskJoiner(gap) {
			return []fullPlanSubtask{sub}
		}
	}

	prefix :=
		strings.TrimSpace(
			task[:unique[0].Start],
		)

	suffix :=
		strings.TrimSpace(
			task[unique[len(unique)-1].End:],
		)


	out :=
		make(
			[]fullPlanSubtask,
			0,
			len(unique),
		)

	for idx, span := range unique {
		parts := make([]string, 0, 3)

		if prefix != "" {
			parts = append(parts, prefix)
		}

		parts = append(
			parts,
			span.Name,
		)

		if suffix != "" {
			parts = append(
				parts,
				suffix,
			)
		}

		child := fullPlanSubtask{
			Task: strings.Join(parts, " "),
			Acceptance: append(
				[]string(nil),
				sub.Acceptance...,
			),
			UsesResearch: append(
				[]string(nil),
				sub.UsesResearch...,
			),
		}

		if idx == 0 {
			child.NeedsSearch = sub.NeedsSearch
			child.SaveResearchTo = sub.SaveResearchTo
		}

		out = append(out, child)
	}


	return out
}

func (s *Service) enforceAtomicAgentPlan(
	plan *fullPlan,
	originalTask string,
	emit func(domain.Event),
) *fullPlan {

	if plan == nil ||
		len(plan.Subtasks) == 0 {
		return plan
	}

	source :=
		s.buildFreshAgentSubtaskContext(
			originalTask,
		)

	if strings.TrimSpace(source) == "" {
		return plan
	}

	maxSubtasks :=
		s.agentModelCapabilities().MaxSubtasks

	out :=
		make(
			[]fullPlanSubtask,
			0,
			len(plan.Subtasks),
		)

	for _, sub := range plan.Subtasks {

		parts :=
			splitCompoundAgentSubtask(
				sub,
				source,
			)

		if len(parts) <= 1 {
			out = append(out, sub)
			continue
		}

		// Не нарушаем configured max subtasks.
		if maxSubtasks > 0 &&
			len(out)+len(parts) > maxSubtasks {

			sendEvent(
				emit,
				domain.EventWarn,
				fmt.Sprintf(
					"Compound Agent subtask kept because splitting would exceed max subtasks: %s",
					sub.Task,
				),
			)

			out = append(out, sub)
			continue
		}

		sendEvent(
			emit,
			domain.EventLog,
			fmt.Sprintf(
				"Split compound Agent subtask into %d atomic subtasks: %s",
				len(parts),
				sub.Task,
			),
		)

		out = append(
			out,
			parts...,
		)
	}

	plan.Subtasks = out

	return plan
}

func hashAgentContext(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func sortedUniqueStrings(
	values []string,
) []string {
	if len(values) == 0 {
		return nil
	}

	set := make(
		map[string]bool,
		len(values),
	)

	for _, value := range values {
		value = strings.TrimSpace(value)

		if value == "" {
			continue
		}

		set[value] = true
	}

	return sortedKeys(set)
}

func (s *Service) buildFreshAgentSubtaskContext(
	task string,
) string {
	targetFiles := extractTargetFiles(task)

	cc := s.buildCodeContext(
		task,
		targetFiles,
	)

	return strings.TrimSpace(cc.Context)
}

func extractAgentTaskIdentifiers(task string) []string {
	var result []string
	seen := make(map[string]bool)

	for _, raw := range strings.Fields(task) {
		token := strings.Trim(
			raw,
			"`\"'(),.:;[]{}<>",
		)

		if len(token) < 3 {
			continue
		}

		first := token[0]
		if first < 'A' || first > 'Z' {
			continue
		}

		if seen[token] {
			continue
		}

		seen[token] = true
		result = append(result, token)

		if len(result) >= 12 {
			break
		}
	}

	return result
}

func sourceHasAgentField(
	source string,
	name string,
) bool {
	for _, line := range strings.Split(source, "\n") {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, name+" ") ||
			strings.HasPrefix(trimmed, name+"\t") ||
			strings.HasPrefix(trimmed, name+"`") {
			return true
		}
	}

	return false
}

func sourceHasAgentMethod(
	source string,
	name string,
) bool {
	return strings.Contains(
		source,
		"func "+name+"(",
	) ||
		strings.Contains(
			source,
			") "+name+"(",
		)
}

func sourceHasAgentTicker(
	source string,
) bool {
	return strings.Contains(
		source,
		"time.NewTicker(",
	)
}

func (s *Service) assessAgentSubtask(
	sub fullPlanSubtask,
) agentSubtaskAssessment {
	task := strings.TrimSpace(sub.Task)
	if task == "" {
		return agentSubtaskAssessment{
			State:  agentSubtaskUnknown,
			Reason: "empty subtask",
		}
	}

	source := s.buildFreshAgentSubtaskContext(task)
	if source == "" {
		return agentSubtaskAssessment{
			State:  agentSubtaskUnknown,
			Reason: "current project source is unavailable",
		}
	}

	lower := strings.ToLower(task)

	// ------------------------------------------------------------
	// 1. Добавление файла
	// ------------------------------------------------------------
	if containsAny(
		lower,
		[]string{
			"create file",
			"add file",
			"создай файл",
			"создать файл",
			"добавь файл",
			"добавить файл",
		},
	) {
		targetFiles := extractTargetFiles(task)

		if len(targetFiles) > 0 &&
			len(s.WS.ExistingFiles(targetFiles)) ==
				len(targetFiles) {

			return agentSubtaskAssessment{
				State:  agentSubtaskAlreadySatisfied,
				Reason: "requested file already exists",
			}
		}
	}

	// ------------------------------------------------------------
	// 2. Добавление поля
	// ------------------------------------------------------------
	if containsAny(
		lower,
		[]string{
			"add field",
			"field to",
			"добавь поле",
			"добавить поле",
			"добавить свойство",
		},
	) {
		for _, id := range extractAgentTaskIdentifiers(task) {
			if sourceHasAgentField(source, id) {
				return agentSubtaskAssessment{
					State: agentSubtaskAlreadySatisfied,
					Reason: fmt.Sprintf(
						"field %s already exists in current source",
						id,
					),
				}
			}
		}
	}

	// ------------------------------------------------------------
	// 3. Добавление метода/функции
	// ------------------------------------------------------------
	if containsAny(
		lower,
		[]string{
			"add method",
			"add function",
			"добавь метод",
			"добавить метод",
			"добавь функцию",
			"добавить функцию",
		},
	) {
		for _, id := range extractAgentTaskIdentifiers(task) {
			if sourceHasAgentMethod(source, id) {
				return agentSubtaskAssessment{
					State: agentSubtaskAlreadySatisfied,
					Reason: fmt.Sprintf(
						"method or function %s already exists",
						id,
					),
				}
			}
		}
	}

	// ------------------------------------------------------------
	// 4. Ticker / background cleanup
	// ------------------------------------------------------------
	if containsAny(
		lower,
		[]string{
			"ticker",
			"newticker",
			"таймер",
			"горутин",
			"фоновой очист",
		},
	) {
		if sourceHasAgentTicker(source) {
			return agentSubtaskAssessment{
				State:  agentSubtaskAlreadySatisfied,
				Reason: "time.NewTicker already exists in current source",
			}
		}
	}

	// ------------------------------------------------------------
	// 5. Endpoint / route
	// ------------------------------------------------------------
	if containsAny(
		lower,
		[]string{
			"endpoint",
			"route",
			"маршрут",
			"эндпоинт",
		},
	) {
		targetFiles := extractTargetFiles(task)
		if len(targetFiles) > 0 &&
			len(s.WS.ExistingFiles(targetFiles)) ==
				len(targetFiles) {
		}

		if strings.Contains(
			source,
			"DELETE",
		) &&
			strings.Contains(
				source,
				"/paste/",
			) {

			return agentSubtaskAssessment{
				State:  agentSubtaskAlreadySatisfied,
				Reason: "DELETE /paste route already appears in current source",
			}
		}
	}

	return agentSubtaskAssessment{
		State:  agentSubtaskRequired,
		Reason: "no deterministic evidence that the subtask is already satisfied",
	}
}

func isRecoverableAgentSubtaskFailure(
	errors []string,
) bool {
	text := strings.ToLower(
		strings.Join(errors, "\n"),
	)

	recoverableMarkers := []string{
		"llm did not return a valid search/replace patch",
		"expected format: --- patch:",
		"patch repair required",
		"no_op_patch",
		"symbol_not_found",
		"strict_symbol_required",
		"search block not found",
		"preflight",
		"stale source",
		"source mismatch",
		"duplicate patch",
		"patch parse",
	}

	for _, marker := range recoverableMarkers {
		if strings.Contains(text, marker) {
			return true
		}
	}

	return false
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
type agentVerificationCheck struct {
	Requirement string   `json:"requirement"`
	Satisfied   bool     `json:"satisfied"`
	Evidence    []string `json:"evidence"`
}

type agentVerification struct {
	Completed bool                     `json:"completed"`
	Missing   []string                 `json:"missing"`
	Risks     []string                 `json:"risks"`
	FixTask   string                   `json:"fix_task"`
	Checks    []agentVerificationCheck `json:"checks"`
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

	var acceptanceBaseline agentAcceptanceBaseline

    if !opts.DryRun &&
    		checkpoint != nil &&
    		!opts.AgentResumeVerificationOnly {
    
    		baseline, baselineErr :=
    			captureAgentAcceptanceBaseline(
    				checkpoint.Dir,
    			)

		if baselineErr != nil {
			final.AddWarning(
				fmt.Sprintf(
					"deterministic acceptance baseline unavailable: %v",
					baselineErr,
				),
			)
		} else {
			acceptanceBaseline =
				baseline
		}
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
			switch {
			case final.Success:
				state.Status = "completed"
			case state.Status == "verification_failed":
				// оставляем как есть: сессия должна оставаться
				// возобновляемой через :agent resume
			default:
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

    	final.ReviewerSuggestions = nil

		final.AddWarning("changes were rolled back to pre-agent state")
	}

	markVerificationFailed := func(reason string) {
		state.Status = "verification_failed"

		if session != nil {
			if err := saveAgentState(session, state); err != nil {
				final.AddWarning(
					fmt.Sprintf(
						"cannot save verification_failed state: %v",
						err,
					),
				)
			}
		}

		final.AddWarning(
			"verification failed (" + reason + "); " +
				"working tree preserved; run ':agent resume' to retry verification",
		)
	}

	// ─── Planning ────────────────────────────────────────────────
	var plan *fullPlan

	if opts.AgentResumePlan != nil {
		plan = opts.AgentResumePlan

		plan = validateAgentPlan(
			plan,
			query,
		)

    	plan =
    		s.enforceAtomicAgentPlan(
    			plan,
    			query,
    			emit,
    		)
    
    	plan =
    		validateAgentPlan(
    			plan,
    			query,
    		)
    
    	warnOnFileBoundAcceptance(query, plan, emit)   // NEW
    
    	plan =
    		s.limitAgentPlan(
    			plan,
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

	if opts.AgentResumePlan == nil {
		plan = s.validateAgentPlanAgainstSource(
			ctx,
			query,
			plan,
			emit,
		)
	}

	plan =
		s.enforceAtomicAgentPlan(
			plan,
			query,
			emit,
		)

	plan =
		validateAgentPlan(
			plan,
			query,
		)

	plan =
		s.limitAgentPlan(
			plan,
		)
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
    }
	markPlan := func(index int, st domain.PlanStatus, note string) {
		if index >= 1 && index <= len(planStatuses) {
			planStatuses[index-1] = st
		}
		sendPlanStatus(emit, index, len(plan.Subtasks), planItems[index-1], st, note)
	}

	// ─── Subtask execution ───────────────────────────────────────
	for i, sub := range plan.Subtasks {
		if opts.AgentResumeVerificationOnly ||
			i < resumeFrom {

			planStatuses[i] = domain.PlanDone
			continue
		}
		state.CurrentSubtask = i + 1

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

		// Снимок длины suggestions на момент старта подзадачи.
		// При откате этой подзадачи всё, что было записано после
		// этого момента, становится невалидным.
		suggestionsBeforeSubtask :=
			len(final.ReviewerSuggestions)

		rollbackSubtask := func(reason string) {
			if opts.DryRun || checkpoint == nil {
				return
			}
			sendEvent(
				emit,
				domain.EventWarn,
				"Subtask rollback: "+reason,
			)
			// checkpoint — скользящий: он уже содержит состояние
			// после последней успешной подзадачи. Откат к нему
			// убирает только изменения текущей подзадачи.
			err := s.rollbackAgentCheckpoint(
				checkpoint,
				nil, // created — не используется (совместимость)
				nil, // modified — не используется (совместимость)
			)
			if err != nil {
				final.AddWarning(
					fmt.Sprintf(
						"subtask rollback failed: %v",
						err,
					),
				)
				return
			}

			// Отбрасываем suggestions текущей подзадачи —
			// её код больше не существует.
			if len(final.ReviewerSuggestions) >
				suggestionsBeforeSubtask {

				final.ReviewerSuggestions =
					final.ReviewerSuggestions[:suggestionsBeforeSubtask]
			}

			final.AddWarning(
				"current subtask changes were rolled back; " +
					"previous subtasks preserved",
			)
		}

		subOpts := opts
		subOpts.NoCommit = true
		subOpts.ProgressItem = i + 1
		subOpts.ProgressTotal = len(plan.Subtasks)

		// ------------------------------------------------------------
		// CURRENT SUBTASK STATE
		// ------------------------------------------------------------

		assessment := s.assessAgentSubtask(sub)

		if assessment.State == agentSubtaskAlreadySatisfied {
			markPlan(
				i+1,
				domain.PlanDone,
				"already satisfied in current project state: "+assessment.Reason,
			)

			sendEvent(
				emit,
				domain.EventLog,
				fmt.Sprintf(
					"Skipping already satisfied subtask %d/%d: %s",
					i+1,
					len(plan.Subtasks),
					assessment.Reason,
				),
			)

			completedSubtasks = i + 1
			state.CompletedSubtasks = completedSubtasks
			state.CurrentSubtask = completedSubtasks

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
            }
			continue
		}

		// ------------------------------------------------------------
		// FRESH SOURCE SNAPSHOT FOR THIS SUBTASK
		// ------------------------------------------------------------

		freshSubtaskContext :=
			s.buildFreshAgentSubtaskContext(sub.Task)

		if freshSubtaskContext != "" {
			subOpts.AgentProjectContext =
				freshSubtaskContext

			state.CurrentContextHash =
				hashAgentContext(freshSubtaskContext)

			sendEvent(
				emit,
				domain.EventLog,
				fmt.Sprintf(
					"Fresh subtask source context: %d bytes",
					len(freshSubtaskContext),
				),
			)
		} else {
			sendEvent(
				emit,
				domain.EventWarn,
				"Fresh subtask source context is empty; executeSimple will use fallback context",
			)
		}

		// ------------------------------------------------------------
		// Дополнительный delta предыдущей успешной подзадачи.
		// Полный текущий source всё равно передаётся отдельно.
		// ------------------------------------------------------------

		taskForCoder := sub.Task

		if strings.TrimSpace(state.LastSubtaskDelta) != "" {
			taskForCoder +=
				"\n\n=== PREVIOUS SUBTASK CHANGE SUMMARY ===\n" +
					textutil.TruncateStringBytes(
						state.LastSubtaskDelta,
						maxPreviousSubtaskDeltaBytes,
					) +
					"\n=== END PREVIOUS SUBTASK CHANGE SUMMARY ==="
		}


		// ------------------------------------------------------------
		// RESEARCH (search + summarize + optional file save)
		// ------------------------------------------------------------

		if strings.TrimSpace(sub.SaveResearchTo) != "" &&
			!sub.NeedsSearch {

			sendEvent(
				emit,
				domain.EventWarn,
				fmt.Sprintf(
					"Subtask %d has save_research_to but needs_search=false; research file will not be created",
					i+1,
				),
			)
		}

		researchContext := ""

		if sub.NeedsSearch &&
			s.Cfg.AutoSearch &&
			s.SafeSearch != nil {

			var researchErr error

			researchContext, researchErr =
				s.searchAndSummarizeForSubtask(
					ctx,
					sub.Task,
					emit,
				)

			if researchErr != nil {
				sendEvent(
					emit,
					domain.EventWarn,
					fmt.Sprintf(
						"Subtask research failed (non-fatal): %v",
						researchErr,
					),
				)
				researchContext = ""
			} else if researchContext != "" {

				sendEvent(
					emit,
					domain.EventLog,
					"Auto-search: summarized research ready for subtask",
				)

				if strings.TrimSpace(sub.SaveResearchTo) != "" {
					savedPath, saveErr :=
						s.saveSubtaskResearch(
							sub.SaveResearchTo,
							i+1,
							sub.Task,
							researchContext,
						)

					if saveErr != nil {
						sendEvent(
							emit,
							domain.EventWarn,
							fmt.Sprintf(
								"Cannot persist research to %s: %v",
								sub.SaveResearchTo,
								saveErr,
							),
						)
					} else {
						sendEvent(
							emit,
							domain.EventLog,
							"Research saved to: "+savedPath,
						)
					}
				}
			}
		}

		if deep {
			taskForCoder = formatAgentTask(
				query,
				plan,
				sub,
				i,
				len(plan.Subtasks),
				researchContext,
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

			if researchContext != "" {
				taskForCoder +=
					"\n\n" +
						researchContext
			}
		}

		// Подставляем research, найденный на предыдущих
		// подзадачах, если планировщик его запросил.
		if len(sub.UsesResearch) > 0 {

			if missing := s.missingResearchFiles(
				sub.UsesResearch,
			); len(missing) > 0 {

				sendEvent(
					emit,
					domain.EventWarn,
					fmt.Sprintf(
						"Subtask %d references research files that do not exist: %s",
						i+1,
						strings.Join(missing, ", "),
					),
				)
			}

			if priorResearch := s.loadSubtaskResearch(
				sub.UsesResearch,
			); priorResearch != "" {

				taskForCoder +=
					"\n\n" +
						priorResearch

				sendEvent(
					emit,
					domain.EventLog,
					fmt.Sprintf(
						"Injected research from %d file(s)",
						len(sub.UsesResearch),
					),
				)
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

		if researchContext != "" {
			taskForCoder +=
				"\n\n" +
					"IMPORTANT: Web research has already been performed and summarized for this subtask. " +
					"Do not perform additional automatic research unless a new dependency " +
					"or explicit new external-information need is discovered during validation."
		}

		var res domain.Result
		var previousAttemptRepairContext string

		for attempt := 1; attempt <= maxAgentSubtaskAttempts; attempt++ {
			if attempt > 1 {
				sendEvent(
					emit,
					domain.EventWarn,
					fmt.Sprintf(
						"Retrying Agent subtask %d/%d with refreshed source context (attempt %d/%d)",
						i+1,
						len(plan.Subtasks),
						attempt,
						maxAgentSubtaskAttempts,
					),
				)

				freshSubtaskContext =
					s.buildFreshAgentSubtaskContext(sub.Task)

				if freshSubtaskContext != "" {
					subOpts.AgentProjectContext =
						freshSubtaskContext

					state.CurrentContextHash =
						hashAgentContext(freshSubtaskContext)

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
                    }
				}
			}

			attemptTask :=
				taskForCoder

			if strings.TrimSpace(
				previousAttemptRepairContext,
			) != "" {

				attemptTask +=
					"\n\n=== PREVIOUS SUBTASK REPAIR CONTEXT ===\n" +
						textutil.TruncateStringBytes(
							previousAttemptRepairContext,
							maxPreviousSubtaskDeltaBytes,
						) +
						"\n=== END PREVIOUS SUBTASK REPAIR CONTEXT ==="
			}

			res = s.executeCoderPass(
				subCtx,
				attemptTask,
				subOpts,
				emit,
			)

			final.Iterations += res.Iterations

			if res.Success {
				break
			}

			if strings.TrimSpace(
				res.PatchRepairContext,
			) != "" {

				previousAttemptRepairContext =
					res.PatchRepairContext
			} else {
				previousAttemptRepairContext =
					strings.Join(
						res.Errors,
						"\n",
					)
			}

			if attempt >= maxAgentSubtaskAttempts ||
				!isRecoverableAgentSubtaskFailure(res.Errors) {

				break
			}

			rollbackSubtask(
				"recoverable subtask failure before retry",
			)
		}

		if res.Success && strings.TrimSpace(sub.SaveResearchTo) != "" {
			savedPath, didSave, saveErr :=
				s.persistResearchFallback(
					sub.SaveResearchTo,
					i+1,
					sub.Task,
					res,
				)

			if saveErr != nil {
				sendEvent(
					emit,
					domain.EventWarn,
					fmt.Sprintf(
						"Cannot persist research fallback to %s: %v",
						sub.SaveResearchTo,
						saveErr,
					),
				)
			} else if didSave {
				res.FilesCreated = appendUniqueString(
					res.FilesCreated,
					savedPath,
				)

				sendEvent(
					emit,
					domain.EventLog,
					"Research fallback saved to: "+savedPath,
				)
			}
		}
		// ─── End research fallback ─────────────────────────────
		addFiles(res)

		if !res.Success {
			markPlan(
				i+1,
				domain.PlanFailed,
				truncate(
					strings.Join(res.Errors, "; "),
					200,
				),
			)

			final.Success = false
			final.Errors = append(
				final.Errors,
				res.Errors...,
			)

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

				issues :=
					strings.Join(
						review.CriticalIssues,
						"; ",
					)

				sendEvent(
					emit,
					domain.EventWarn,
					"Reviewer found critical issues: "+issues,
				)

				fixTask :=
					buildReviewFixTask(
						sub.Task,
						review,
					)

				fixRes :=
					s.executeCoderPass(
						agent.WithRole(
							ctx,
							agent.RoleCoder,
						),
						fixTask,
						subOpts,
						emit,
					)

				final.Iterations +=
					fixRes.Iterations

				addFiles(fixRes)

				if !fixRes.Success {
					markPlan(
						i+1,
						domain.PlanFailed,
						"fix after reviewer failed",
					)

					final.Success = false

					final.Errors =
						append(
							final.Errors,
							fixRes.Errors...,
						)

					rollbackSubtask(
						"reviewer fix failed",
					)

					return final
				}

				// Критическая правка считается принятой
				// только после повторной проверки reviewer.
				reviewedResult :=
					mergeAgentResults(
						res,
						fixRes,
					)

				reviewAfterFix, reviewErr :=
					s.runReviewer(
						ctx,
						query,
						sub.Task,
						reviewedResult,
						mem,
						emit,
					)

				if reviewErr != nil {
					markPlan(
						i+1,
						domain.PlanFailed,
						"reviewer unavailable after critical fix",
					)

					final.Success = false

					final.AddError(
						fmt.Sprintf(
							"reviewer failed after critical fix: %v",
							reviewErr,
						),
					)

					rollbackSubtask(
						"reviewer unavailable after critical fix",
					)

					return final
				}

				if !reviewAfterFix.Approved &&
					len(reviewAfterFix.CriticalIssues) > 0 {

					remainingIssues :=
						strings.Join(
							reviewAfterFix.CriticalIssues,
							"; ",
						)

					markPlan(
						i+1,
						domain.PlanFailed,
						"critical reviewer issues remain after fix",
					)

					final.Success = false

					final.AddError(
						"critical reviewer issues remain after fix: " +
							remainingIssues,
					)

					rollbackSubtask(
						"critical reviewer issues remain",
					)

					return final
				}

				res = reviewedResult

				itemStatus =
					domain.PlanDone

				itemNote =
					"critical reviewer issues fixed and re-reviewed"

    			if len(reviewAfterFix.Suggestions) > 0 {
    					appendReviewerSuggestions(
    						&final.ReviewerSuggestions,
    						reviewAfterFix,
    						i+1,
    						len(plan.Subtasks),
    					)
    					itemNote = fmt.Sprintf(
    						"critical reviewer issues fixed; %d suggestion(s) recorded",
    						len(reviewAfterFix.Suggestions),
    					)
    				}

			} else if len(review.Suggestions) > 0 {
				itemStatus = domain.PlanWarn
				itemNote = fmt.Sprintf("reviewer suggestions: %d", len(review.Suggestions))

				appendReviewerSuggestions(
					&final.ReviewerSuggestions,
					review,
					i+1,
					len(plan.Subtasks),
				)
			}

		}
		if deep && !isAnalysis {
			sendEvent(
				emit,
				domain.EventAgent,
				"current stage: quality gates",
			)
			// Собираем список изменённых файлов из результата подзадачи.
			changedFiles := make([]string, 0,
				len(res.FilesCreated)+len(res.FilesModified)+
					len(res.FilesPatched)+len(res.FilesFullRewritten))
			changedFiles = append(changedFiles, res.FilesCreated...)
			changedFiles = append(changedFiles, res.FilesModified...)
			changedFiles = append(changedFiles, res.FilesPatched...)
			changedFiles = append(changedFiles, res.FilesFullRewritten...)
			gate := s.runAgentDeepQualityGates(
				ctx,
				emit,
				i+1,
				opts.NoTests,
				changedFiles,
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
				// Проверяем, все ли ошибки — предсуществующие
				allPreexisting := true
				for _, e := range gate.Errors {
					if !strings.Contains(e, "NEW") && !strings.Contains(e, "new issue") {
						continue
					}
					allPreexisting = false
					break
				}
				if allPreexisting && len(gate.Errors) > 0 {
					// Все проблемы предсуществующие — предупреждаем, но не откатываем
					sendEvent(
						emit,
						domain.EventWarn,
						"Quality gates report issues in EXISTING code that were not introduced by this subtask. "+
							"Consider running ':fix' or ':agent' to address them separately.",
					)
					itemStatus = domain.PlanWarn
					itemNote = "pre-existing lint issues; subtask changes are valid"
				} else {
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

		}

		// ─── Скользящий чекпоинт ──────────────────────────
		// Все проверки пройдены. Фиксируем текущее состояние
		// проекта как новую безопасную точку.
		if !opts.DryRun && checkpoint != nil {
			if cpErr := s.updateAgentCheckpoint(
				ctx,
				checkpoint,
			); cpErr != nil {
				final.AddWarning(
					fmt.Sprintf(
						"checkpoint update failed after subtask %d: %v",
						i+1,
						cpErr,
					),
				)
			}
		}

		state.LastSubtask = i + 1

		state.LastSubtaskFiles = append(
			[]string{},
			res.FilesCreated...,
		)

		state.LastSubtaskFiles = append(
			state.LastSubtaskFiles,
			res.FilesModified...,
		)

		state.LastSubtaskFiles = append(
			state.LastSubtaskFiles,
			res.FilesPatched...,
		)

		state.LastSubtaskFiles = append(
			state.LastSubtaskFiles,
			res.FilesFullRewritten...,
		)

		// Удаляем дубликаты через уже существующий helper.
		state.LastSubtaskFiles =
			sortedUniqueStrings(
				state.LastSubtaskFiles,
			)

		state.LastSubtaskDelta =
			buildAgentSubtaskDelta(res)

		// Git commit is intentionally deferred until the complete agent run
		// has passed final verification. This keeps one user task = one commit.

		// ─── Статус и сохранение состояния ────────────────
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
		// Подзадачный чекпоинт больше не создаётся —
		// очистка не требуется.

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

	acceptance :=
		validateAgentAcceptance(
			s.Cfg.WorkDir,
			query,
			plan,
			acceptanceBaseline,
			final,
		)

	for _, warning := range acceptance.Warnings {

		final.AddWarning(
			"acceptance: " + warning,
		)
	}

	acceptanceSummary :=
		acceptance.String()

	verification, err :=
		s.runVerifier(
			ctx,
			query,
			plan,
			final,
			mem,
			acceptanceSummary,
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

	if !acceptance.Passed {
		verification.Completed = false

		for _, item := range acceptance.Blocking {

			verification.Missing =
				appendUniqueString(
					verification.Missing,
					item,
				)
		}

		if strings.TrimSpace(
			verification.FixTask,
		) == "" {

			verification.FixTask =
				acceptance.FixTask
		}

		if strings.TrimSpace(
			verification.FixTask,
		) == "" {

			verification.FixTask =
				"Fix the deterministic acceptance failures using file/content changes only: " +
					strings.Join(
						acceptance.Blocking,
						"; ",
					)
		}
	}

	if !acceptance.Passed {
		verification.Completed = false

		for _, item := range acceptance.Blocking {

			verification.Missing =
				appendUniqueString(
					verification.Missing,
					item,
				)
		}

		if strings.TrimSpace(
			verification.FixTask,
		) == "" {

			verification.FixTask =
				acceptance.FixTask
		} else if acceptance.FixTask != "" {
			verification.FixTask +=
				"\n\nAdditional deterministic acceptance failures:\n" +
					acceptance.FixTask
		}
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

			markVerificationFailed(
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

		fixRes := s.executeCoderPass(
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

			markVerificationFailed(
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
		acceptanceAfterFix :=
			validateAgentAcceptance(
				s.Cfg.WorkDir,
				query,
				plan,
				acceptanceBaseline,
				final,
			)

		for _, warning := range acceptanceAfterFix.Warnings {

			final.AddWarning(
				"acceptance after verifier fix: " +
					warning,
			)
		}

		verification, err =
			s.runVerifier(
				ctx,
				query,
				plan,
				final,
				mem,
				acceptanceAfterFix.String(),
				emit,
			)

		if !acceptanceAfterFix.Passed {
			verification.Completed = false

			for _, item := range acceptanceAfterFix.Blocking {

				verification.Missing =
					appendUniqueString(
						verification.Missing,
						item,
					)
			}

			if strings.TrimSpace(
				verification.FixTask,
			) == "" {
				verification.FixTask =
					acceptanceAfterFix.FixTask
			}
		}

		acceptanceAfterFix =
			validateAgentAcceptance(
				s.Cfg.WorkDir,
				query,
				plan,
				acceptanceBaseline,
				final,
			)

		for _, warning := range acceptanceAfterFix.Warnings {

			final.AddWarning(
				"acceptance after verifier fix: " +
					warning,
			)
		}

		acceptanceAfterFixSummary :=
			acceptanceAfterFix.String()

		if !acceptanceAfterFix.Passed {
			verification.Completed = false

			for _, item := range acceptanceAfterFix.Blocking {

				verification.Missing =
					appendUniqueString(
						verification.Missing,
						item,
					)
			}

			if strings.TrimSpace(
				verification.FixTask,
			) == "" {

				verification.FixTask =
					acceptanceAfterFix.FixTask
			}
		}

		verification, err =
			s.runVerifier(
				ctx,
				query,
				plan,
				final,
				mem,
				acceptanceAfterFixSummary,
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

			markVerificationFailed(
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

		finalChangedFiles := make([]string, 0)
		for _, f := range final.FilesCreated {
			finalChangedFiles = append(finalChangedFiles, f)
		}
		for _, f := range final.FilesModified {
			finalChangedFiles = append(finalChangedFiles, f)
		}
		for _, f := range final.FilesPatched {
			finalChangedFiles = append(finalChangedFiles, f)
		}
		finalGate := s.runAgentDeepQualityGates(
			ctx,
			emit,
			0,
			opts.NoTests,
			finalChangedFiles,
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

    if err := mem.save(s.Cfg.WorkDir); err != nil {
        final.AddWarning(
            fmt.Sprintf(
                "agent memory could not be saved: %v",
                err,
            ),
        )
    }

	return final
}

func buildAgentSubtaskDelta(
	res domain.Result,
) string {
	var b strings.Builder

	b.WriteString("Changed files:\n")

	seen := make(map[string]bool)

	appendFiles := func(label string, files []string) {
		for _, path := range files {
			path = strings.TrimSpace(path)
			if path == "" || seen[path] {
				continue
			}

			seen[path] = true

			b.WriteString("- ")
			b.WriteString(label)
			b.WriteString(": ")
			b.WriteString(path)
			b.WriteByte('\n')
		}
	}

	appendFiles("created", res.FilesCreated)
	appendFiles("modified", res.FilesModified)
	appendFiles("patched", res.FilesPatched)
	appendFiles("full-rewritten", res.FilesFullRewritten)

	if strings.TrimSpace(res.Response) != "" {
		b.WriteString("\nResult summary:\n")
		b.WriteString(
			textutil.TruncateStringBytes(
				res.Response,
				8000,
			),
		)
	}

	result := strings.TrimSpace(b.String())

	if len(result) > maxPreviousSubtaskDeltaBytes {
		result =
			textutil.TruncateStringBytes(
				result,
				maxPreviousSubtaskDeltaBytes,
			) +
				"\n... delta truncated ..."
	}

	return result
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

	planContext := s.buildFreshAgentSubtaskContext(query)

	if planContext != "" {
		if len(planContext) > maxAgentPlanContextBytes {
			planContext =
				textutil.TruncateStringBytes(
					planContext,
					maxAgentPlanContextBytes,
				) +
					"\n... current project source truncated for planning ..."
		}

		prompt +=
			"\n\n" +
				"=== CURRENT PROJECT SOURCE OF TRUTH ===\n" +
				planContext +
				"\n=== END CURRENT PROJECT SOURCE ===\n"
	}

	prompt = s.appendProjectInstructions(prompt)

	var plan fullPlan

	ctx = llm.WithSystemPrompt(ctx, prompts.SystemPlanner)
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

func (s *Service) validateAgentPlanAgainstSource(
	ctx context.Context,
	originalTask string,
	plan *fullPlan,
	emit func(domain.Event),
) *fullPlan {
	if plan == nil {
		return nil
	}

	// Однопунктный план не требует дополнительного LLM-вызова:
	// экономим время и токены.
	if len(plan.Subtasks) <= 1 {
		return plan
	}

	projectContext := s.buildFreshAgentSubtaskContext(originalTask)
	if projectContext == "" {
		return plan
	}

	if len(projectContext) > maxAgentPlanContextBytes {
		projectContext =
			textutil.TruncateStringBytes(
				projectContext,
				maxAgentPlanContextBytes,
			) +
				"\n... current project source truncated ..."
	}

	data, err := json.Marshal(plan)
	if err != nil {
		sendEvent(
			emit,
			domain.EventWarn,
			fmt.Sprintf(
				"agent plan validation skipped: cannot marshal plan: %v",
				err,
			),
		)
		return plan
	}

	prompt := prompts.ValidateAgentPlan(
		originalTask,
		projectContext,
		string(data),
	)

	prompt = s.appendProjectInstructions(prompt)

	var validated fullPlan

	err = s.sendAgentJSON(
		ctx,
		agent.RolePlanner,
		agent.PriorityHigh,
		"validate agent plan",
		prompt,
		&validated,
	)
	if err != nil {
		// Это защитный слой, поэтому failure-open.
		// Если validator недоступен, оригинальный план сохраняется.
		sendEvent(
			emit,
			domain.EventWarn,
			fmt.Sprintf(
				"agent plan validation failed; keeping original plan: %v",
				err,
			),
		)
		return plan
	}

	validated = *validateAgentPlan(
		&validated,
		originalTask,
	)

	if len(validated.Subtasks) == 0 &&
		len(plan.Subtasks) > 0 {

		sendEvent(
			emit,
			domain.EventWarn,
			"agent plan validator returned empty plan; keeping original plan",
		)

		return plan
	}

	sendEvent(
		emit,
		domain.EventLog,
		fmt.Sprintf(
			"agent plan validated against current source: %d subtasks",
			len(validated.Subtasks),
		),
	)

	return &validated
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

	summary :=
		agentChangeSummaryWithLimits(
			res,
			maxTotal,
			maxPerFile,
		)

	maxFiles, maxBytes :=
		s.contextLimits()

	if maxBytes > maxTotal {
		maxBytes = maxTotal
	}

	changedFiles := make(
		[]string,
		0,
		len(res.FilesCreated)+
			len(res.FilesModified)+
			len(res.FilesPatched)+
			len(res.FilesFullRewritten),
	)

	changedFiles = append(
		changedFiles,
		res.FilesCreated...,
	)

	changedFiles = append(
		changedFiles,
		res.FilesModified...,
	)

	changedFiles = append(
		changedFiles,
		res.FilesPatched...,
	)

	changedFiles = append(
		changedFiles,
		res.FilesFullRewritten...,
	)

	currentSource :=
		s.WS.BuildSmartContext(
			originalTask,
			changedFiles,
			maxFiles,
			maxBytes,
		)

	prompt :=
		prompts.ReviewChangesWithSource(
			originalTask,
			subtask,
			summary,
			currentSource,
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
	acceptanceSummary string,
	emit func(domain.Event),
) (agentVerification, error) {
	maxTotal, maxPerFile := s.reviewLimits()

	summary := agentChangeSummaryWithLimits(
		final,
		maxTotal,
		maxPerFile,
	)

	maxFiles, maxBytes :=
		s.contextLimits()

	if maxBytes > maxTotal {
		maxBytes = maxTotal
	}

	changedFiles := make(
		[]string,
		0,
		len(final.FilesCreated)+
			len(final.FilesModified)+
			len(final.FilesPatched)+
			len(final.FilesFullRewritten),
	)

	changedFiles = append(
		changedFiles,
		final.FilesCreated...,
	)

	changedFiles = append(
		changedFiles,
		final.FilesModified...,
	)

	changedFiles = append(
		changedFiles,
		final.FilesPatched...,
	)

	changedFiles = append(
		changedFiles,
		final.FilesFullRewritten...,
	)

	currentSource :=
		s.WS.BuildSmartContext(
			originalTask,
			changedFiles,
			maxFiles,
			maxBytes,
		)

	if plan != nil && len(plan.Acceptance) > 0 {
		summary += "\nacceptance criteria:\n- " + strings.Join(plan.Acceptance, "\n- ")
	}

	task := truncate(originalTask, 4000)
	prompt :=
		prompts.VerifyCompletionWithSource(
			task,
			summary,
			currentSource,
			acceptanceSummary,
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

	if requiresStructuredAgentVerification(
		originalTask,
	) &&
		len(verification.Checks) == 0 {

		verification.Completed = false

		verification.Missing =
			appendUniqueString(
				verification.Missing,
				"verifier did not provide required acceptance checks and source evidence",
			)

		if strings.TrimSpace(
			verification.FixTask,
		) == "" {

			verification.FixTask =
				"Re-check the original task against the current project source and fix every unmet explicit requirement using file changes only."
		}
	}
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

	var failedChecks []string

	for _, check := range v.Checks {
		requirement :=
			strings.TrimSpace(
				check.Requirement,
			)

		if requirement == "" {
			continue
		}

		if check.Satisfied {
			continue
		}

		failedChecks =
			append(
				failedChecks,
				requirement,
			)
	}

	if len(failedChecks) > 0 {
		v.Completed = false

		for _, item := range failedChecks {
			v.Missing =
				appendUniqueString(
					v.Missing,
					item,
				)
		}

		if strings.TrimSpace(
			v.FixTask,
		) == "" {

			v.FixTask =
				"Fix the unmet requirements listed by the verifier using file/content changes only: " +
					strings.Join(
						failedChecks,
						"; ",
					)
		}
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

// appendReviewerSuggestions добавляет suggestions от ревьюера в общий
// список Result.ReviewerSuggestions, снабжая каждую пометкой подзадачи.
func appendReviewerSuggestions(
	dst *[]string,
	review agentReview,
	subtaskIndex, subtaskTotal int,
) {
	for _, sug := range review.Suggestions {
		sug = strings.TrimSpace(sug)
		if sug == "" {
			continue
		}
		*dst = append(*dst,
			fmt.Sprintf("[%d/%d] %s", subtaskIndex, subtaskTotal, sug),
		)
	}
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

func mergeAgentResults(
	base,
	extra domain.Result,
) domain.Result {
	merged := base

	merged.Success = extra.Success

	if strings.TrimSpace(extra.Response) != "" {
		merged.Response =
			extra.Response
	}

	merged.FilesCreated =
		sortedUniqueStrings(
			append(
				append(
					[]string{},
					base.FilesCreated...,
				),
				extra.FilesCreated...,
			),
		)

	merged.FilesModified =
		sortedUniqueStrings(
			append(
				append(
					[]string{},
					base.FilesModified...,
				),
				extra.FilesModified...,
			),
		)

	merged.FilesPatched =
		sortedUniqueStrings(
			append(
				append(
					[]string{},
					base.FilesPatched...,
				),
				extra.FilesPatched...,
			),
		)

	merged.FilesFullRewritten =
		sortedUniqueStrings(
			append(
				append(
					[]string{},
					base.FilesFullRewritten...,
				),
				extra.FilesFullRewritten...,
			),
		)

	merged.OutputFiles =
		mergeOutputFiles(
			base.OutputFiles,
			extra.OutputFiles,
		)

	merged.Errors =
		append(
			append(
				[]string{},
				base.Errors...,
			),
			extra.Errors...,
		)

	merged.Warnings =
		append(
			append(
				[]string{},
				base.Warnings...,
			),
			extra.Warnings...,
		)

	if extra.Tests.Run ||
		extra.Tests.Skipped {

		merged.Tests =
			extra.Tests
	}

	return merged
}

func appendUniqueString(
	values []string,
	value string,
) []string {
	value = strings.TrimSpace(value)

	if value == "" {
		return values
	}

	for _, existing := range values {
		if existing == value {
			return values
		}
	}

	return append(
		values,
		value,
	)
}

// searchAndSummarizeForSubtask выполняет веб-поиск и
// дополнительно обрабатывает сырой результат через LLM,
// чтобы получить компактную выжимку фактов под конкретную
// задачу подзадачи.
func (s *Service) searchAndSummarizeForSubtask(
	ctx context.Context,
	task string,
	emit func(domain.Event),
) (string, error) {
	raw, err := s.searchForSubtask(ctx, task, emit)
	if err != nil {
		return "", err
	}

	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}

	summarized, err := s.summarizeSubtaskResearch(
		ctx,
		task,
		raw,
		emit,
	)
	if err != nil {
		return "", err
	}

	return summarized, nil
}

// summarizeSubtaskResearch превращает сырой веб-текст
// в компактную выжимку фактов, релевантных задаче.
func (s *Service) summarizeSubtaskResearch(
	ctx context.Context,
	task string,
	rawResearch string,
	emit func(domain.Event),
) (string, error) {
	rawResearch = strings.TrimSpace(rawResearch)
	if rawResearch == "" {
		return "", nil
	}

	if len(rawResearch) > maxSubtaskResearchRawBytes {
		rawResearch = textutil.TruncateStringBytes(
			rawResearch,
			maxSubtaskResearchRawBytes,
		) +
			"\n... raw research truncated before summarization ..."
	}

	sendEvent(
		emit,
		domain.EventLog,
		"Auto-search: summarizing research for subtask...",
	)

	sumCtx := agent.WithRole(ctx, agent.RolePlanner)
	sumCtx = agent.WithPriority(sumCtx, agent.PriorityNormal)
	sumCtx = agent.WithPurpose(sumCtx, "summarize subtask research")

	prompt := prompts.SubtaskResearchSummary(task, rawResearch)

	summarized, err := s.LLM.Send(sumCtx, prompt)
	if err != nil {
		return "", fmt.Errorf(
			"summarize research: %w",
			err,
		)
	}

	summarized = strings.TrimSpace(summarized)

	if summarized == "" ||
		strings.HasPrefix(summarized, "NO_RELEVANT_FACTS") {

		sendEvent(
			emit,
			domain.EventWarn,
			"Auto-search: no relevant facts extracted from research",
		)

		return "", nil
	}

	if len(summarized) > maxSubtaskResearchFileBytes {
		summarized = textutil.TruncateStringBytes(
			summarized,
			maxSubtaskResearchFileBytes,
		) +
			"\n\n... research summary truncated ..."
	}

	return summarized, nil
}

// saveSubtaskResearch сохраняет суммаризированный research
// на диск по пути, который указал планировщик через
// save_research_to.
func (s *Service) saveSubtaskResearch(
	relPath string,
	subtaskIndex int,
	task string,
	research string,
) (string, error) {
	relPath = strings.TrimSpace(relPath)
	if relPath == "" {
		return "", fmt.Errorf("empty research path")
	}

	switch strings.ToLower(filepath.Ext(relPath)) {
	case ".md", ".txt", ".json":
	default:
		return "", fmt.Errorf(
			"research path must have .md, .txt, or .json extension: %s",
			relPath,
		)
	}

	research = strings.TrimSpace(research)
	if research == "" {
		return "", fmt.Errorf("empty research content")
	}

	full, err := security.SafeJoin(s.Cfg.WorkDir, relPath)
	if err != nil {
		return "", fmt.Errorf(
			"invalid research path %q: %w",
			relPath,
			err,
		)
	}

	if err := os.MkdirAll(
		filepath.Dir(full),
		0o755,
	); err != nil {
		return "", fmt.Errorf(
			"create research directory: %w",
			err,
		)
	}

	var b strings.Builder

	fmt.Fprintf(
		&b,
		"# Research for subtask %d\n\n",
		subtaskIndex,
	)
	fmt.Fprintf(
		&b,
		"_Source task:_ %s\n\n",
		strings.TrimSpace(task),
	)
	fmt.Fprintf(
		&b,
		"_Saved:_ %s\n\n",
		time.Now().Format(time.RFC3339),
	)
	b.WriteString("## Extracted facts\n\n")
	b.WriteString(research)
	b.WriteString("\n")

	body := b.String()

	if len(body) > maxSubtaskResearchFileBytes {
		body = textutil.TruncateStringBytes(
			body,
			maxSubtaskResearchFileBytes,
		) +
			"\n\n... research file truncated ..."
	}

	if err := os.WriteFile(
		full,
		[]byte(body),
		0o644,
	); err != nil {
		return "", fmt.Errorf(
			"write research file %q: %w",
			relPath,
			err,
		)
	}

	return filepath.ToSlash(relPath), nil
}

// loadSubtaskResearch читает research-файлы, указанные
// в поле uses_research, и формирует один блок для промпта.
func (s *Service) loadSubtaskResearch(
	paths []string,
) string {
	if len(paths) == 0 {
		return ""
	}

	var blocks []string
	total := 0

	for _, relPath := range paths {
		relPath = strings.TrimSpace(relPath)
		if relPath == "" {
			continue
		}

		switch strings.ToLower(filepath.Ext(relPath)) {
		case ".md", ".txt", ".json":
		default:
			continue
		}

		full, err := security.SafeJoin(
			s.Cfg.WorkDir,
			relPath,
		)
		if err != nil {
			continue
		}

		info, err := os.Stat(full)
		if err != nil || info.IsDir() {
			continue
		}

		data, err := os.ReadFile(full)
		if err != nil {
			continue
		}

		remaining :=
			maxSubtaskResearchTotalBytes - total

		if remaining <= 0 {
			break
		}

		content := string(data)

		if len(content) > remaining {
			content = textutil.TruncateStringBytes(
				content,
				remaining,
			)
		}

		blocks = append(
			blocks,
			fmt.Sprintf(
				"--- Research file: %s ---\n%s",
				relPath,
				content,
			),
		)

		total += len(content)
	}

	if len(blocks) == 0 {
		return ""
	}

	return "=== RESEARCH FROM PREVIOUS SUBTASKS ===\n" +
		strings.Join(blocks, "\n\n") +
		"\n=== END RESEARCH FROM PREVIOUS SUBTASKS ==="
}

// missingResearchFiles возвращает список путей из uses_research,
// которые не существуют в проекте. 
func (s *Service) missingResearchFiles(
	paths []string,
) []string {
	if len(paths) == 0 {
		return nil
	}

	var missing []string

	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		full, err := security.SafeJoin(
			s.Cfg.WorkDir,
			p,
		)
		if err != nil {
			missing = append(missing, p)
			continue
		}

		info, err := os.Stat(full)
		if err != nil || info.IsDir() {
			missing = append(missing, p)
		}
	}

	return missing
}

// sanitizeUsesResearch приводит список ссылок на research
// к безопасному виду: дедупликация, отбрасывание пустых
// и некорректных путей.
func sanitizeUsesResearch(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}

	seen := make(map[string]bool, len(paths))
	out := make([]string, 0, len(paths))

	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}

		switch strings.ToLower(filepath.Ext(path)) {
		case ".md", ".txt", ".json":
		default:
			continue
		}

		if seen[path] {
			continue
		}

		seen[path] = true
		out = append(out, path)
	}

	if len(out) == 0 {
		return nil
	}

	return out
}

// persistResearchFallback сохраняет осмысленный текстовый ответ подзадачи
// в файл, указанный в save_research_to, если research-путь этот файл
// не создал.
func (s *Service) persistResearchFallback(
	relPath string,
	subtaskIndex int,
	task string,
	res domain.Result,
) (string, bool, error) {
	relPath = strings.TrimSpace(relPath)
	if relPath == "" {
		return "", false, nil
	}

	full, err := security.SafeJoin(s.Cfg.WorkDir, relPath)
	if err != nil {
		return "", false, fmt.Errorf(
			"invalid research path %q: %w",
			relPath,
			err,
		)
	}

	if _, statErr := os.Stat(full); statErr == nil {
		return relPath, false, nil
	}

	if !res.Success {
		return "", false, nil
	}

	content := strings.TrimSpace(res.Response)
	if !looksLikeResearchText(content) {
		return "", false, nil
	}

	savedPath, saveErr := s.saveSubtaskResearch(
		relPath,
		subtaskIndex,
		task,
		content,
	)
	if saveErr != nil {
		return "", false, saveErr
	}

	return savedPath, true, nil
}

// looksLikeResearchText определяет, является ли текстовый ответ
// достаточно содержательным, чтобы сохранить его как research-файл.
func looksLikeResearchText(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}

	if len(text) < 200 {
		return false
	}

	lower := strings.ToLower(text)

	if strings.HasPrefix(lower, "agent completed") ||
		strings.HasPrefix(lower, "agent failed") ||
		strings.HasPrefix(lower, "applied changes:") ||
		strings.HasPrefix(lower, "applied ") {

		return false
	}

	markers := []string{
		"##", "###",
		"\n- ", "\n* ", "\n1.", "\n2.",
		"```",
		"http://", "https://",
		"| ",
	}

	for _, marker := range markers {
		if strings.Contains(text, marker) {
			return true
		}
	}

	if len(text) >= 800 {
		return true
	}

	return false
}