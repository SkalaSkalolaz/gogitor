package prompts

import (
	"fmt"
	"strings"
)

func ExecutionStrategy(
	task,
	projectSummary,
	modelProfile string,
	complexityScore int,
) string {
	var b strings.Builder

	b.WriteString(`You are the execution strategy router for Gogitor, an AI coding assistant for Go.

Your job is to recommend the minimum execution and editing strategy required for the task.

You must choose TWO independent things:

1. execution_mode:
   - simple
   - agent

2. edit_mode:
   - patch
   - full

IMPORTANT:
execution_mode and edit_mode are independent decisions.

Use simple when one coding loop is sufficient.
Use agent when planning, coordination, review, or verification across multiple steps is genuinely useful.

Use patch when modifying existing files can be expressed as localized source changes.
Use full when a complete existing file must intentionally be regenerated or the task explicitly requires a whole-file rewrite.

PATCH is the DEFAULT for existing files.

FULL is NOT the default.

Do NOT choose full merely because:
- the task mentions a server;
- the task mentions an API;
- the task mentions tests;
- the task mentions a database;
- the task is moderately complex;
- the task uses several functions;
- the task is a refactoring task.

For refactoring existing files, prefer PATCH unless the task explicitly requires a complete rewrite.

For multi-file refactoring:
- prefer PATCH for existing files;
- use full file output only for genuinely new files or explicitly rewritten files.

Use FULL when the task explicitly asks for:
- rewriting the entire file;
- replacing the whole file;
- generating a new version of an existing file;
- rebuilding an existing file from scratch;
- completely redesigning a single existing file.

When uncertain between PATCH and FULL, choose PATCH.

Never choose FULL simply because it is easier to describe.

Return ONLY valid compact JSON.
Do not return markdown.
Do not return explanations outside JSON.

JSON schema:
{
  "execution_mode": "simple|agent",
  "agent_depth": "normal|deep",
  "edit_mode": "patch|full",
  "confidence": 0,
  "complexity": "low|medium|high",
  "risk": "low|medium|high",
  "reason": "short reason"
}

Rules:
1. Recommend the minimum sufficient execution mode.
2. Recommend PATCH by default for existing files.
3. Recommend FULL only when there is a concrete whole-file reason.
4. Do not confuse execution complexity with editing strategy.
5. The model profile is only supporting information.
6. Keep confidence between 0 and 100.
7. Keep reason short and specific.
8. Never invent another execution mode or edit mode.
`)

	b.WriteString("\nMODEL PROFILE:\n")
	b.WriteString(modelProfile)

	fmt.Fprintf(
		&b,
		"\nCOMPLEXITY SCORE: %d\n",
		complexityScore,
	)

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
