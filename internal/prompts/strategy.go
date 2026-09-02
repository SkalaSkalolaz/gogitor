package prompts

import (
	"fmt"
	"strings"
)

func ExecutionStrategy(task, projectSummary, modelProfile string, complexityScore int) string {
	var b strings.Builder
	b.WriteString(`You are the execution strategy router for Gogitor, an AI coding assistant for Go.

Decide whether the task should use:
- simple — one coding loop for small, local, low-risk work
- agent — Planner/Coder/Reviewer/Verifier for multi-step work

If agent is selected, also choose:
- normal — standard agent pipeline
- deep — stronger harness with task isolation, deterministic quality gates, persistent session artifacts and final verification.

Return ONLY valid compact JSON.
Do not return markdown.
Do not return explanations outside JSON.

JSON schema:
{
  "execution_mode": "simple|agent",
  "agent_depth": "normal|deep",
  "confidence": 0,
  "complexity": "low|medium|high",
  "risk": "low|medium|high",
  "reason": "short reason"
}

Rules:
1. Use simple only for genuinely small and low-risk tasks.
2. Use agent for multi-step, multi-file, refactoring, architectural, testing or high-risk changes.
3. Use deep when the task is architectural, large, ambiguous, touches many files, or has substantial regression risk.
4. Never invent another execution mode.
5. Keep the reason short.
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
