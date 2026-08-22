package prompts

import (
	"fmt"
	"strings"

	"gogitor/internal/textutil"
)

// WorkflowInterviewQuestions просит LLM сгенерировать уточняющие вопросы
// перед выполнением workflow-задачи.
func WorkflowInterviewQuestions(task, projectSummary string) string {
	var b strings.Builder
	b.WriteString(`You are a requirements analyst for a Go project workflow.
Before executing the task, generate 3-5 clarifying questions that will help
produce a better implementation plan.

Return ONLY valid compact JSON. No markdown. No explanations outside JSON.
JSON schema:
{
  "questions": [
    {
      "id": 1,
      "question": "the clarifying question",
      "why": "why this question matters",
      "default": "reasonable default answer if user skips"
    }
  ],
  "assumptions": ["assumption made if user provides no answers"]
}

RULES:
1. Questions must be specific to THIS task, not generic.
2. Focus on: scope boundaries, error handling strategy, API contracts,
   naming conventions, backward compatibility, performance requirements.
3. Maximum 5 questions.
4. Each question must have a sensible default.
5. Write in the same language as the TASK.
`)
	b.WriteString("\nTASK:\n")
	b.WriteString(task)
	b.WriteString("\n")
	if strings.TrimSpace(projectSummary) != "" {
		b.WriteString("\nPROJECT SUMMARY:\n")
		b.WriteString(projectSummary)
		b.WriteString("\n")
	}
	return b.String()
}

// WorkflowInterviewSummary формирует итоговый уточнённый контекст
// на основе ответов пользователя.
func WorkflowInterviewSummary(task string, answers []WorkflowAnswer) string {
	var b strings.Builder
	b.WriteString(`You are a requirements analyst. Based on the task and the user's answers
to clarifying questions, produce a REFINED TASK SPECIFICATION.

Return ONLY the refined task text. No markdown fences. No JSON.
The refined task must be concrete enough for a code generation agent.

RULES:
1. Incorporate all answers.
2. Make scope boundaries explicit.
3. State what NOT to do if the user specified exclusions.
4. Keep it under 500 words.
5. Write in the same language as the original task.

ORIGINAL TASK:
`)
	b.WriteString(task)
	b.WriteString("\n\nQ&A:\n")
	for _, a := range answers {
		fmt.Fprintf(&b, "Q%d: %s\nA%d: %s\n\n", a.ID, a.Question, a.ID, a.Answer)
	}
	return b.String()
}

// WorkflowAnswer — один ответ в интервью.
type WorkflowAnswer struct {
	ID       int    `json:"id"`
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// WorkflowReflection просит LLM проанализировать артефакты workflow
// и сформировать ретроспективу.
func WorkflowReflection(
	goal string,
	processLog string,
	prdJSON string,
	gateReports string,
	cumulativeDiff string,
) string {
	var b strings.Builder
	b.WriteString(`You are a senior engineering lead performing a workflow retrospective.
Analyze the workflow execution artifacts below and produce a reflection report.

Return GitHub Flavored Markdown. Do NOT return --- File: blocks.

STRUCTURE (use exactly these headings):
## ✅ What Went Well
## ⚠️ What Could Be Improved
## 📊 Metrics Summary
## 💡 Recommendations for Future Workflows
## 🏁 Final Verdict

RULES:
1. Be specific — reference actual tasks, timings, and gate results.
2. Do not invent metrics that are not in the data.
3. If everything went perfectly, say so briefly.
4. Maximum 15 items total across all sections.
5. Write in the same language as the GOAL.
`)
	b.WriteString("\nWORKFLOW GOAL:\n")
	b.WriteString(goal)
	b.WriteString("\n")

	if strings.TrimSpace(prdJSON) != "" {
		b.WriteString("\nPRD (task definitions):\n")
		b.WriteString(textutil.TruncateStringBytes(prdJSON, 4000))
		b.WriteString("\n")
	}
	if strings.TrimSpace(processLog) != "" {
		b.WriteString("\nPROCESS LOG:\n")
		b.WriteString(textutil.TruncateStringBytes(processLog, 8000))
		b.WriteString("\n")
	}
	if strings.TrimSpace(gateReports) != "" {
		b.WriteString("\nQUALITY GATE REPORTS:\n")
		b.WriteString(textutil.TruncateStringBytes(gateReports, 4000))
		b.WriteString("\n")
	}
	if strings.TrimSpace(cumulativeDiff) != "" {
		b.WriteString("\nCUMULATIVE DIFF (truncated):\n")
		b.WriteString(textutil.TruncateStringBytes(cumulativeDiff, 3000))
		b.WriteString("\n")
	}
	return b.String()
}

// WorkflowReflectQuick — упрощённый промпт для малых моделей.
func WorkflowReflectQuick(goal, processLog string) string {
	var b strings.Builder
	b.WriteString(`You are an engineering lead. Briefly review this workflow execution log
and give 3-5 bullet points: what worked, what didn't, what to improve.
Write in the same language as the goal. No markdown headings needed.

GOAL: `)
	b.WriteString(goal)
	b.WriteString("\n\nLOG:\n")
	b.WriteString(textutil.TruncateStringBytes(processLog, 6000))
	b.WriteString("\n")
	return b.String()
}

// WorkflowPlanReview просит LLM проанализировать план и задать
// уточняющие вопросы перед началом выполнения.
func WorkflowPlanReview(task string, planGoal string, subtasks []string) string {
	var b strings.Builder
	b.WriteString(`You are a senior software architect reviewing an implementation plan.
Analyze the plan below and identify potential issues, risks, or ambiguities.
Return ONLY valid compact JSON. No markdown. No explanations outside JSON.
JSON schema:
{
  "questions": [
    {
      "id": 1,
      "question": "clarifying question about the plan",
      "concern": "what could go wrong if not clarified"
    }
  ],
  "risks": ["identified risk"],
  "approved": true
}
RULES:
1. Maximum 3 questions.
2. Focus on: architecture decisions, API contracts, error handling strategy,
   naming conventions, backward compatibility, data flow.
3. If the plan is clear, complete, and low-risk, set approved=true
   and return empty questions array.
4. Do NOT ask questions that are already answered in the task description.
5. Write in the same language as the TASK.
`)
	b.WriteString("\nORIGINAL TASK:\n")
	b.WriteString(task)
	b.WriteString("\n\nPLAN:\n")
	b.WriteString("Goal: " + planGoal + "\n")
	for i, st := range subtasks {
		fmt.Fprintf(&b, "Subtask %d: %s\n", i+1, st)
	}
	b.WriteString("\n")
	return b.String()
}

// WorkflowPlanRefine корректирует план на основе обратной связи пользователя.
func WorkflowPlanRefine(task, currentPlanJSON, userFeedback string) string {
	var b strings.Builder
	b.WriteString(`You are a software planning agent for a Go project.
Revise the execution plan based on user feedback.
Return ONLY valid compact JSON. No markdown. No explanations outside JSON.
JSON schema:
{
  "goal": "short goal",
  "acceptance": ["acceptance criterion"],
  "subtasks": [
    {
      "task": "concrete subtask",
      "acceptance": ["subtask acceptance criterion"],
      "needs_search": false
    }
  ]
}
RULES:
1. Incorporate ALL user feedback into the revised plan.
2. Maximum 7 subtasks.
2a. Each subtask must contain exactly ONE operation.
3. Each subtask must be independently executable by a code generation agent.
4. Preserve subtasks that the user did not object to.
5. Write in the same language as the TASK.
6. Do not invent unrelated features.
`)
	b.WriteString("\nORIGINAL TASK:\n")
	b.WriteString(task)
	b.WriteString("\n\nCURRENT PLAN (JSON):\n")
	b.WriteString(textutil.TruncateStringBytes(currentPlanJSON, 4000))
	b.WriteString("\n\nUSER FEEDBACK:\n")
	b.WriteString(userFeedback)
	b.WriteString("\n")
	return b.String()
}

func WorkflowExtractLessons(reflection string) string {
	var b strings.Builder
	b.WriteString(`You are an engineering lead extracting actionable lessons from a workflow retrospective.
Return ONLY the lessons, one per line, each prefixed with "LESSON:".
Maximum 5 lessons.
Each lesson must be a concrete, actionable rule for future workflows.
Do NOT return markdown. Do NOT return JSON. Do NOT return explanations.
EXAMPLE:
LESSON: Always run go vet before committing to catch unused variables early.
LESSON: Split tasks into smaller units when a single task touches more than 3 files.
LESSON: Include error handling in acceptance criteria for every subtask.
REFLECTION:
`)
	b.WriteString(textutil.TruncateStringBytes(reflection, 6000))
	b.WriteString("\n")
	return b.String()
}