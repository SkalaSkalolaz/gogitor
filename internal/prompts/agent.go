package prompts

import (
	"strings"
)

// PlanFull просит planner agent вернуть структурированный план в JSON.

func PlanFull(task, memory string) string {
	var b strings.Builder
	b.WriteString(`You are a software planning agent for a Go project.
Create an execution plan.
Return ONLY valid compact JSON.
Do not return markdown.
Do not return explanations outside JSON.
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
1. Maximum 7 subtasks.
2. Each subtask must be independently executable by a code generation agent.
3. Prefer small, file-only, verifiable steps.
3a. GRANULARITY: Each subtask must contain exactly ONE operation.
    If a task mentions multiple operations joined by "and", ",", or "+",
    you MUST split them into separate subtasks.
3b. If a subtask would modify more than 50 lines of code or touch
    more than 2 functions, split it into smaller subtasks.
4. Include practical acceptance criteria.
5. Do not invent unrelated features.
6. Do not create separate analysis-only subtasks. If analysis is needed, include it in the first coding subtask as context.
7. Prefer subtasks that create or modify files.
8. Write "goal", subtask text, and acceptance criteria in the same language as the TASK.
9. Use the exact file names, paths, endpoints, ports, and identifiers mentioned in the TASK. Do not replace them with example names.
10. If the TASK does not specify a file name, choose a short descriptive name appropriate for the task.
11. The coder agent can ONLY create or modify text files. It cannot run shell commands, cannot change file permissions, cannot start servers, cannot send HTTP requests, and cannot verify runtime behavior.
12. Do NOT create subtasks such as: chmod +x, make executable, run, execute, start, launch, curl, wget, send request, go run, manual test. Shell scripts created by Gogitor are automatically executable.
13. If the task is "analyze code and create a helper script", prefer ONE subtask: create the helper script using context from the code.
14. Set "needs_search" to true ONLY when the subtask requires looking up documentation for an unfamiliar third-party Go package, a specific API signature you are not sure about, or a library version. Do NOT set it for standard library tasks, simple file creation, or refactoring. Default is false.
BAD SUBTASKS:
- chmod +x <script_name>.sh
- run <script_name>.sh and check output
- start server and send request
GOOD SUBTASK:
- Create <script_name>.sh with #!/usr/bin/env bash and a curl command, using the file name from the TASK.
`)
	if strings.TrimSpace(memory) != "" {
		b.WriteString("\nPROJECT MEMORY:\n")
		b.WriteString(memory)
		b.WriteString("\n")
	}
	b.WriteString("\nTASK:\n")
	b.WriteString(task)
	b.WriteString("\n")
	return b.String()
}

func ReviewChanges(originalTask, subtask, changeSummary, memory string) string {
	var b strings.Builder
	b.WriteString(`You are a senior Go reviewer agent.
Review the changes produced by another agent.
Return ONLY valid compact JSON.
Do not return markdown.
Do not return explanations outside JSON.

MANDATORY JSON schema (follow EXACTLY):
{
"approved": true,
"critical_issues": ["string describing a critical issue"],
"suggestions": ["string describing a non-blocking suggestion"]
}

CRITICAL FORMAT RULES:
- "critical_issues" MUST be a JSON array of PLAIN STRINGS. Example: ["Missing error check on line 42"]
- "suggestions" MUST be a JSON array of PLAIN STRINGS. Example: ["Consider adding context timeout"]
- Do NOT use objects, nested JSON, key-value pairs, or arrays of objects inside these fields.
- Each element must be a single human-readable string, not a JSON object.
- If there are no issues, use empty arrays: []

CORRECT EXAMPLE:
{"approved":true,"critical_issues":[],"suggestions":["Add mutex for concurrent map access","Consider using context.WithTimeout for HTTP calls"]}

INCORRECT EXAMPLES (NEVER DO THIS):
{"approved":true,"critical_issues":[],"suggestions":[{"text":"Add mutex","severity":"low"}]}
{"approved":true,"critical_issues":[{"issue":"nil pointer","file":"main.go"}],"suggestions":[]}

RULES:
1. Set approved=false ONLY if there is a clear, concrete critical issue that would break compilation, cause a runtime panic, or introduce a security vulnerability.
2. Critical issues are strictly limited to:
- syntax or compile errors visible in the provided code;
- obvious nil-pointer dereference or race condition;
- security vulnerabilities (SQL injection, path traversal, hardcoded secrets);
- broken package structure (wrong package name, circular imports).
3. The code has ALREADY passed go build and go test successfully. Do not question compilation or test results.
4. Do NOT reject for:
- style preferences;
- missing comments or documentation;
- naming conventions;
- "could be improved" suggestions;
- missing error handling in non-critical paths;
- theoretical performance concerns;
- missing features not explicitly required by the subtask.
5. Do NOT mark "missing required part" as critical unless you can point to a specific file or function that is explicitly required by the subtask text and is clearly absent from the change summary. However, if the subtask explicitly says "Create <filename>" or "Создать <filename>", and that file is absent from the CHANGE SUMMARY, this IS a critical issue and you MUST set approved=false.
6. If you are unsure whether something is critical, approve and add it as a suggestion instead.
7. Suggestions may contain non-blocking improvements but must never block approval.
8. When in doubt, approve.
9. Keep each suggestion string under 200 characters.
10. Maximum 5 suggestions.
`)
	if strings.TrimSpace(memory) != "" {
		b.WriteString("\nPROJECT MEMORY:\n")
		b.WriteString(memory)
		b.WriteString("\n")
	}
	b.WriteString("\nORIGINAL TASK:\n")
	b.WriteString(originalTask)
	b.WriteString("\n")
	b.WriteString("CURRENT SUBTASK:\n")
	b.WriteString(subtask)
	b.WriteString("\n")
	b.WriteString("CHANGE SUMMARY:\n")
	b.WriteString(changeSummary)
	b.WriteString("\n")
	return b.String()
}

func ReviewChangesStrict(originalTask, subtask string) string {
	var b strings.Builder
	b.WriteString(`You are a code reviewer. Return ONLY this exact JSON structure. No other text.
{"approved":true,"critical_issues":[],"suggestions":[]}
Rules:
- approved: boolean (true or false)
- critical_issues: array of strings (empty [] if none)
- suggestions: array of strings (empty [] if none)
- Each array element MUST be a plain string like "Add error handling"
- NEVER use objects inside arrays
- When in doubt, set approved=true and empty arrays
ORIGINAL TASK: `)
	b.WriteString(originalTask)
	b.WriteString("\nSUBTASK:\n")
	b.WriteString(subtask)
	b.WriteString("\n")
	return b.String()
}

// VerifyCompletion просит verifier agent проверить, что исходная задача выполнена.
func VerifyCompletion(originalTask, resultSummary, memory string) string {
	var b strings.Builder

	b.WriteString(`You are a verification agent.
Decide whether the original task was completed.

Return ONLY valid compact JSON.
Do not return markdown.
Do not return explanations outside JSON.

JSON schema:
{
  "completed": true,
  "missing": [],
  "risks": [],
  "fix_task": ""
}

RULES:
1. completed=true if the original task is satisfied by static file changes shown in RESULT SUMMARY.
2. Gogitor can ONLY create or modify text files. It cannot run scripts, send HTTP requests, start servers, or verify terminal output. Do not mark runtime execution or output verification as missing.
3. Shell scripts (.sh, .bash, .zsh, .fish, .command) are automatically executable in Gogitor. Do not mark chmod, executable bit, or permission verification as missing or as a risk.
4. PROJECT MEMORY is background information only. Do not treat memory as acceptance criteria and do not add requirements that are not explicitly present in ORIGINAL TASK.
5. missing must contain only concrete file/content requirements that can be fixed by creating/modifying files.
6. risks may contain non-blocking technical risks. Risks alone must not make completed=false.
7. fix_task must be empty if completed=true.
8. If completed=false, fix_task must be a concrete file-only corrective task.
9. If the task asks to create a helper script and RESULT SUMMARY shows that the script file was created or modified, completed=true even if the script was not executed.
10. Do not require comparison with source files that are not present in RESULT SUMMARY. If the changed script contains a plausible endpoint consistent with the task, consider it sufficient.
`)

    if strings.TrimSpace(memory) != "" {
    	b.WriteString("\nPROJECT MEMORY (informational only, not requirements):\n")
    	b.WriteString(memory)
    	b.WriteString("\n")
    }

	b.WriteString("\nORIGINAL TASK:\n")
	b.WriteString(originalTask)
	b.WriteString("\n\n")

	b.WriteString("RESULT SUMMARY:\n")
	b.WriteString(resultSummary)
	b.WriteString("\n")

	return b.String()
}

// PlanFullWithApproach — планирование с учётом выбранного подхода.
func PlanFullWithApproach(task, approach, memory string) string {
	var b strings.Builder
	b.WriteString(`You are a software planning agent for a Go project.
Create an execution plan.
Return ONLY valid compact JSON.
Do not return markdown.
Do not return explanations outside JSON.
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
1. Maximum 5 subtasks.
2. Each subtask must be independently executable by a code generation agent.
3. Prefer small, file-only, verifiable steps.
4. Include practical acceptance criteria.
5. Do not invent unrelated features.
6. Do not create separate analysis-only subtasks.
7. Prefer subtasks that create or modify files.
8. Write "goal", subtask text, and acceptance criteria in the same language as the TASK.
9. Use the exact file names, paths, endpoints, ports, and identifiers mentioned in the TASK.
10. The coder agent can ONLY create or modify text files.
11. Do NOT create subtasks such as: chmod +x, run, execute, start, curl, wget.
12. The plan MUST follow the SELECTED IMPLEMENTATION APPROACH below. Do not deviate from it.
13. Set "needs_search" to true ONLY when the subtask requires looking up documentation for an unfamiliar third-party Go package, a specific API signature you are not sure about, or a library version. Do NOT set it for standard library tasks, simple file creation, or refactoring. Default is false.

`)
	if strings.TrimSpace(approach) != "" {
		b.WriteString("\nSELECTED IMPLEMENTATION APPROACH (MANDATORY — follow this exactly):\n")
		b.WriteString(approach)
		b.WriteString("\n")
	}
	if strings.TrimSpace(memory) != "" {
		b.WriteString("\nPROJECT MEMORY:\n")
		b.WriteString(memory)
		b.WriteString("\n")
	}
	b.WriteString("\nTASK:\n")
	b.WriteString(task)
	b.WriteString("\n")
	return b.String()
}