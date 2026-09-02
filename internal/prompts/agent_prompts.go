package prompts

import (
	"fmt"
	"strings"

	"gogitor/internal/textutil"
)

// AgentInterviewAnswer — один ответ в интервью.
type AgentInterviewAnswer struct {
	ID       int    `json:"id"`
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

func AgentInterviewQuestions(task, projectSummary string) string {
	var b strings.Builder
	b.WriteString(`You are a requirements analyst for a Go project agent.
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

func AgentInterviewSummary(task string, answers []AgentInterviewAnswer) string {
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

func AgentReflection(
	goal string,
	processLog string,
	prdJSON string,
	gateReports string,
	cumulativeDiff string,
) string {
	var b strings.Builder
	b.WriteString(`You are a senior engineering lead performing a agent retrospective.
Analyze the agent execution artifacts below and produce a reflection report.

Return GitHub Flavored Markdown. Do NOT return --- File: blocks.

STRUCTURE (use exactly these headings):
## ✅ What Went Well
## ⚠️ What Could Be Improved
## 📊 Metrics Summary
## 💡 Recommendations for Future Agents
## 🏁 Final Verdict

RULES:
1. Be specific — reference actual tasks, timings, and gate results.
2. Do not invent metrics that are not in the data.
3. If everything went perfectly, say so briefly.
4. Maximum 15 items total across all sections.
5. Write in the same language as the GOAL.
`)
	b.WriteString("\nAGENT GOAL:\n")
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

func AgentReflectQuick(goal, processLog string) string {
	var b strings.Builder
	b.WriteString(`You are an engineering lead. Briefly review this agent execution log
and give 3-5 bullet points: what worked, what didn't, what to improve.
Write in the same language as the goal. No markdown headings needed.

GOAL: `)
	b.WriteString(goal)
	b.WriteString("\n\nLOG:\n")
	b.WriteString(textutil.TruncateStringBytes(processLog, 6000))
	b.WriteString("\n")
	return b.String()
}

func AgentExtractLessons(reflection string) string {
	var b strings.Builder
	b.WriteString(`You are an engineering lead extracting actionable lessons from a agent retrospective.
Return ONLY the lessons, one per line, each prefixed with "LESSON:".
Maximum 5 lessons.
Each lesson must be a concrete, actionable rule for future agents.
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
