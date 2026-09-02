package domain

import "time"

type EventType string

const (
	EventLog      EventType = "log"
	EventIntent   EventType = "intent"
	EventWarn     EventType = "warn"
	EventError    EventType = "error"
	EventDone     EventType = "done"
	EventAgent    EventType = "agent"
	EventPlan     EventType = "plan"
	EventToken    EventType = "token"
	EventProgress EventType = "progress"
	EventComputerConfirm EventType = "computer_confirm"
	EventComputerBlocked EventType = "computer_blocked"
)

// TaskStage описывает текущую фазу выполнения пользовательской задачи.
type TaskStage string

const (
	TaskStageIdle       TaskStage = "idle"
	TaskStagePlanning   TaskStage = "planning"
	TaskStageContext    TaskStage = "context"
	TaskStageCoding     TaskStage = "coding"
	TaskStageReview     TaskStage = "review"
	TaskStageVerifying  TaskStage = "verifying"
	TaskStageTesting    TaskStage = "testing"
	TaskStageRepairing  TaskStage = "repairing"
	TaskStageCompleted  TaskStage = "completed"
	TaskStageFailed     TaskStage = "failed"
	TaskStageCancelled  TaskStage = "cancelled"
	TaskStageExecuting TaskStage = "executing"
	TaskStageChat      TaskStage = "chat"
	TaskStageAnalyze   TaskStage = "analyze"
	TaskStageSearch    TaskStage = "search"
	TaskStageGit       TaskStage = "git"
	TaskStageRun       TaskStage = "run"
	TaskStageLint      TaskStage = "lint"
	TaskStageArticle   TaskStage = "article"
	TaskStageComputer  TaskStage = "computer"
)

func (s TaskStage) Symbol() string {
	switch s {
	case TaskStagePlanning:
		return "◐"
	case TaskStageContext:
		return "◌"
	case TaskStageCoding:
		return "▶"
	case TaskStageReview:
		return "◎"
	case TaskStageVerifying:
		return "◇"
	case TaskStageTesting:
		return "◆"
	case TaskStageRepairing:
		return "↻"
	case TaskStageCompleted:
		return "✓"
	case TaskStageFailed:
		return "✗"
	case TaskStageCancelled:
		return "⊘"
	case TaskStageExecuting:
		return "⚙"
	case TaskStageChat:
		return "◈"
	case TaskStageAnalyze:
		return "◉"
	case TaskStageSearch:
		return "⬢"
	case TaskStageGit:
		return "⎇"
	case TaskStageRun:
		return "▷"
	case TaskStageLint:
		return "▤"
	case TaskStageArticle:
		return "✎"
	case TaskStageComputer:
		return "⚙"
	default:
		return "○"
	}
}

type ProgressUpdate struct {
	Stage          string  `json:"stage,omitempty"`
	ItemIndex      int     `json:"item_index,omitempty"`
	TotalItems     int     `json:"total_items,omitempty"`
	ETASeconds     int     `json:"eta_seconds,omitempty"`
	ElapsedSeconds int     `json:"elapsed_seconds,omitempty"`
	Progress       float64 `json:"progress,omitempty"`
}

type AgentStatus struct {
	Role     string `json:"role,omitempty"`
	Purpose  string `json:"purpose,omitempty"`
	Requests int    `json:"requests,omitempty"`
	Tokens   int    `json:"tokens,omitempty"`
	Duration string `json:"duration,omitempty"`
	Queue    int    `json:"queue,omitempty"`
	Kind     string `json:"kind,omitempty"`
}

// ComputerStep — один шаг плана computer mode.
type ComputerStep struct {
	Command        string `json:"command"`
	Description    string `json:"description"`
	Risk           string `json:"risk"`
	ExpectedResult string `json:"expected_result"`
	Rollback       string `json:"rollback,omitempty"`
	Status         string `json:"status"` // pending|running|done|failed|blocked
	Output         string `json:"output,omitempty"`
	ExitCode       int    `json:"exit_code,omitempty"`
}

// ComputerPlan — план computer mode.
type ComputerPlan struct {
	Goal  string         `json:"goal"`
	Steps []ComputerStep `json:"steps"`
}

// PlanStatus — статус пункта плана выполнения.
type PlanStatus string

const (
	PlanPending PlanStatus = "pending" // ○ ещё не начат
	PlanRunning PlanStatus = "running" // ▶ выполняется
	PlanDone    PlanStatus = "done"    // ✓ выполнен
	PlanWarn    PlanStatus = "warn"    // ⚠ выполнен с предупреждениями
	PlanFailed  PlanStatus = "failed"  // ✗ не выполнен
	PlanSkipped PlanStatus = "skipped" // ⊘ пропущен
)

// Symbol возвращает символ статуса для отображения.
func (s PlanStatus) Symbol() string {
	switch s {
	case PlanRunning:
		return "▶"
	case PlanDone:
		return "✓"
	case PlanWarn:
		return "⚠"
	case PlanFailed:
		return "✗"
	case PlanSkipped:
		return "⊘"
	default:
		return "○"
	}
}

type PlanUpdate struct {
	Goal           string     `json:"goal,omitempty"`
	Acceptance     []string   `json:"acceptance,omitempty"`
	Items          []string   `json:"items,omitempty"`
	ItemIndex      int        `json:"item_index,omitempty"` // 1-based
	Status         PlanStatus `json:"status,omitempty"`
	Note           string     `json:"note,omitempty"`
	Progress       float64    `json:"progress,omitempty"`
	ETASeconds     int        `json:"eta_seconds,omitempty"`
	ElapsedSeconds int        `json:"elapsed_seconds,omitempty"`
}

type Event struct {
	Type     EventType       `json:"type"`
	Message  string          `json:"message"`
	Result   *Result         `json:"result,omitempty"`
	Plan     *PlanUpdate     `json:"plan,omitempty"`
	Progress *ProgressUpdate `json:"progress,omitempty"`
	Agent    *AgentStatus    `json:"agent,omitempty"`
	TaskStage TaskStage `json:"task_stage,omitempty"`
}

type Intent struct {
	Mode   string `json:"mode"`
	Task   string `json:"task"`
	Reason string `json:"reason,omitempty"`
}

type Patch struct {
	Search  string `json:"search"`
	Replace string `json:"replace"`
	Symbol  string `json:"symbol,omitempty"`
}

type FileChange struct {
	Path      string  `json:"path"`
	Content   string  `json:"-"`
	Created   bool    `json:"created"`
	Patches   []Patch `json:"patches,omitempty"`
	PatchMode bool    `json:"patch_mode,omitempty"`
}

type TestFailure struct {
	Package  string `json:"package,omitempty"`
	Test     string `json:"test,omitempty"`
	Function string `json:"function,omitempty"`
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
	Message  string `json:"message,omitempty"`
}

type TestsStatus struct {
	Run            bool          `json:"run"`
	Skipped        bool          `json:"skipped"`
	Passed         int           `json:"passed"`
	Failed         int           `json:"failed"`
	Output         string        `json:"output,omitempty"`
	Coverage       float64       `json:"coverage,omitempty"`
	CoverageOutput string        `json:"coverage_output,omitempty"`
	Failures       []TestFailure `json:"failures,omitempty"`
}

type LintStatus struct {
	Run     bool   `json:"run"`
	Skipped bool   `json:"skipped"`
	Passed  bool   `json:"passed"`
	Issues  int    `json:"issues"`
	Output  string `json:"output,omitempty"`
}

type OutputFile struct {
	Path    string
	Content string
}

type QualityGateStatus struct {
	Build         bool    `json:"build"`
	Tests         bool    `json:"tests"`
	Vet           bool    `json:"vet"`
	Gofmt         bool    `json:"gofmt"`
	Lint          bool    `json:"lint"`
	LintInstalled bool    `json:"lint_installed"`
	LintIssues    int     `json:"lint_issues"`
	TestsPassed   int     `json:"tests_passed"`
	TestsFailed   int     `json:"tests_failed"`
	Coverage      float64 `json:"coverage,omitempty"`
	Passed        bool    `json:"passed"`
}


type Result struct {
	Success            bool              `json:"success"`
	Mode               string            `json:"mode"`
	Response           string            `json:"response,omitempty"`
	RefinedTask        string            `json:"refined_task,omitempty"`
	IntentReason       string            `json:"intent_reason,omitempty"`
	FilesCreated       []string          `json:"files_created,omitempty"`
	FilesModified      []string          `json:"files_modified,omitempty"`
	Errors             []string          `json:"errors,omitempty"`
	Warnings           []string          `json:"warnings,omitempty"`
	Iterations         int               `json:"iterations,omitempty"`
	Tests              TestsStatus       `json:"tests"`
	GitCommit          string            `json:"git_commit,omitempty"`
	DryRun             bool              `json:"dry_run,omitempty"`
	FilesPatched       []string          `json:"files_patched,omitempty"`
	FilesFullRewritten []string          `json:"files_full_rewritten,omitempty"`
	OutputFiles        []OutputFile      `json:"-"`
	Lint               LintStatus        `json:"lint"`
    QualityGates QualityGateStatus `json:"quality_gates,omitempty"`
	PreTaskHead        string            `json:"pre_task_head,omitempty"`
	CumulativeDiff     string            `json:"cumulative_diff,omitempty"`
	Comparison         *ComparisonResult `json:"comparison,omitempty"`
	AwaitingSelection  bool              `json:"awaiting_selection,omitempty"`
	SelectedApproach   string            `json:"selected_approach,omitempty"`
}

type TaskHistoryEntry struct {
	ID          int       `json:"id"`
	Time        time.Time `json:"time"`
	Query       string    `json:"query"`
	Mode        string    `json:"mode"`
	Success     bool      `json:"success"`
	Iterations  int       `json:"iterations"`
	Files       []string  `json:"files,omitempty"`
	AddedLines  int       `json:"added_lines,omitempty"`
	RemovedLines int      `json:"removed_lines,omitempty"`
	GitCommit   string    `json:"git_commit,omitempty"`
}

func (r *Result) AddError(msg string) {
	r.Errors = append(r.Errors, msg)
}

func (r *Result) AddWarning(msg string) {
	r.Warnings = append(r.Warnings, msg)
}

// Approach — один вариант реализации.
type Approach struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	Complexity    string `json:"complexity"`
	Performance   string `json:"performance"`
	Readability   string `json:"readability"`
	Dependencies  string `json:"dependencies"`
	Testability   string `json:"testability"`
	Justification string `json:"justification"`
	Tradeoffs     string `json:"tradeoffs"`
	Recommended   bool   `json:"recommended"`
}

// ComparisonResult — результат сравнительного анализа.
type ComparisonResult struct {
	Approaches     []Approach `json:"approaches"`
	RecommendedID  int        `json:"recommended_id"`
	Recommendation string     `json:"recommendation"`
}

// Alternative — отклонённая альтернатива.
type Alternative struct {
	Description string `json:"description"`
	Reason      string `json:"reason,omitempty"`
}

// DecisionEntry — одна запись в журнале решений.
type DecisionEntry struct {
	ID           int           `json:"id"`
	Date         string        `json:"date"`
	Decision     string        `json:"decision"`
	Context      string        `json:"context,omitempty"`
	Temporary    bool          `json:"temporary,omitempty"`
	Constraint   string        `json:"constraint,omitempty"`
	Alternatives []Alternative `json:"alternatives,omitempty"`
	Source       string        `json:"source,omitempty"`
}

// DecisionDebt — обнаруженный «долг»: временное решение,
// ограничение которого, возможно, снято.
type DecisionDebt struct {
	DecisionID   int    `json:"decision_id"`
	Decision     string `json:"decision"`
	OriginalDate string `json:"original_date"`
	Constraint   string `json:"constraint"`
	Suggestion   string `json:"suggestion"`
}

// DecisionJournal — полный журнал для отображения.
type DecisionJournal struct {
	Entries        []DecisionEntry `json:"entries"`
	FailedApproaches []string      `json:"failed_approaches,omitempty"`
	Debts          []DecisionDebt  `json:"debts,omitempty"`
	Summary        string          `json:"summary,omitempty"`
}

