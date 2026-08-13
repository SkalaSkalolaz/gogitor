package prompts

import (
	"fmt"
	"strings"
)

// ExecutionStrategy просит LLM выбрать режим выполнения задачи.
// Используется только для внешних/достаточно сильных моделей и только
// когда детерминированные правила считают задачу средней или сложной.
func ExecutionStrategy(task, projectSummary, modelProfile string, complexityScore int) string {
	var b strings.Builder

	b.WriteString(`You are the execution strategy router for Gogitor, an AI coding assistant for Go.
Decide which execution mode should be used for the user task.

Return ONLY valid compact JSON.
Do not return markdown.
Do not return explanations outside JSON.

JSON schema:
{
  "execution_mode": "fast|agent|workflow",
  "confidence": 0,
  "complexity": "low|medium|high",
  "risk": "low|medium|high",
  "reason": "short reason",
  "ask_user": false
}

Mode rules:
1. fast: simple, local, low-risk changes, usually one file or one small function.
2. agent: multi-step changes that need planning and review but do not require full workflow artifacts.
3. workflow: complex architectural changes, many files, unclear requirements, refactoring, or tasks needing traceability.

Additional rules:
1. Do not choose workflow for trivial tasks.
2. Do not choose fast if the task likely changes architecture or many files.
3. If unsure, set ask_user=true and choose the safer mode.
4. Keep the reason short.
`)

	b.WriteString("\nMODEL PROFILE:\n")
	b.WriteString(modelProfile)
	b.WriteString("\n")

	fmt.Fprintf(&b, "COMPLEXITY SCORE: %d\n", complexityScore)

	if strings.TrimSpace(projectSummary) != "" {
		b.WriteString("\nPROJECT SUMMARY:\n")
		b.WriteString(projectSummary)
		b.WriteString("\n")
	}

	b.WriteString("\nUSER TASK:\n")
	b.WriteString(task)
	b.WriteString("\n")

	return b.String()
}