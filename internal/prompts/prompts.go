package prompts

import (
	"strings"
	"fmt"

    "gogitor/internal/domain"
	"gogitor/internal/textutil"
)

func CodeCreate(task, projectContext string) string {
	var b strings.Builder
    b.WriteString(`You are a senior Go engineer.
    
STRICT RULES:
- Output ONLY valid Go code
- NO explanations
- NO markdown
- NO comments like "TODO"
- MUST compile with "go build"
- MUST include all required imports
- If modifying code: keep existing APIs unchanged unless necessary

OUTPUT FORMAT:
<full file content>

TASK:
`)


	b.WriteString(task)
	b.WriteString("\n")

	if strings.TrimSpace(projectContext) != "" {
		b.WriteString("PROJECT CONTEXT:\n")
		b.WriteString(projectContext)
		b.WriteString("\n")
	}

	b.WriteString(`RULES:
1. Return ONLY file blocks.
2. Use this exact format:
--- File: path/to/file.go ---
<full file content>
3. If multiple files are needed, repeat the format.
4. Do not include explanations.
5. Code must compile with standard Go tooling.
6. Prefer standard library only unless the task explicitly requires external packages.
7. Do not use placeholders.
`)

    if strings.Contains(task, "fix") || strings.Contains(task, "error") {
        b.WriteString(`
MODE: PATCH
- Modify ONLY broken parts
- Do NOT rewrite entire file
`)
    } else {
        b.WriteString(`
MODE: FULL
- Return complete file
`)
    }

    b.WriteString(`
EXAMPLE:

Input:
Fix function that returns wrong sum

Output:
package main

func sum(a, b int) int {
    return a + b
}
`)
	return b.String()
}

// CodeCreateInExistingProject используется, когда проект уже существует,
// но задача выглядит как создание новых файлов.
func CodeCreateInExistingProject(task, projectContext string) string {
	var b strings.Builder
	b.WriteString(`You are a senior Go engineer working inside an existing project.

TASK:
`)
	b.WriteString(task)
	b.WriteString("\n")

	b.WriteString("EXISTING PROJECT FILES:\n")
	b.WriteString(projectContext)
	b.WriteString("\n")

	b.WriteString(`RULES:
1. The files above are the current project source of truth.
2. Do NOT invent unrelated code.
3. Do NOT rewrite existing logic unless the task explicitly requires it.
4. If new files are needed, create them in a way that is compatible with the existing project.
5. If existing files must be changed, return their complete updated content.
6. Preserve existing package names, imports, function signatures, and behavior where possible.
7. Return ONLY file blocks.
8. Use this exact format:
--- File: path/to/file.go ---
<full file content>
9. Do not include explanations.
10. Do not use placeholders.
`)

	return b.String()
}

// CodeModify используется для изменения, рефакторинга, разделения кода,
// исправления ошибок и любых операций с существующим проектом.
func CodeModify(task, projectContext string) string {
	var b strings.Builder
	b.WriteString(`You are a senior Go engineer modifying an existing project.

TASK:
`)
	b.WriteString(task)
	b.WriteString("\n")

	b.WriteString("EXISTING PROJECT FILES:\n")
	b.WriteString(projectContext)
	b.WriteString("\n")

	b.WriteString(`CRITICAL RULES:
1. The existing project files above are the source of truth.
2. Do NOT rewrite code arbitrarily.
3. Do NOT invent new behavior unless the task explicitly asks for it.
4. Preserve existing logic, names, types, constants, imports, and behavior unless they must change.
5. If the task says "split", "divide", "extract", "move", or "refactor", you must move the existing code, not create unrelated code.
6. If code is moved to a new file, remove or adjust the moved code in the original file so the project still compiles.
7. Return complete file content for every file that must be created or changed.
8. Do not return unchanged files unless they are required for compilation after the change.
9. All Go files in the same directory must use compatible package names.
10. The final code must compile with standard Go tooling.
11. Return ONLY file blocks.
12. Use this exact format:
--- File: path/to/file.go ---
<full file content>
13. Do not include explanations.
14. Do not use placeholders.

EXAMPLE FOR A SPLIT TASK:
If the task is:
"Split the program into main.go and utils.go"
You must:
- Keep the existing program behavior.
- Move helper functions/types to utils.go if appropriate.
- Keep main() in main.go if it was there.
- Make sure both files compile together.
- Use the same package name if both files are in the same directory.
`)

	return b.String()
}

func CodeFix(task, filesContext, errors string) string {
	var b strings.Builder
	b.WriteString(`You are a senior Go engineer fixing an existing project.

ORIGINAL TASK:
`)
	b.WriteString(task)
	b.WriteString("\n")

	b.WriteString("CURRENT FILES:\n")
	b.WriteString(filesContext)
	b.WriteString("\n")

	b.WriteString("ERRORS:\n")
	b.WriteString(errors)
	b.WriteString("\n")

	b.WriteString(`RULES:
1. Fix all reported errors.
2. Preserve the original task intent.
3. Preserve existing behavior unless it is the cause of the error.
4. Do not rewrite the project arbitrarily.
5. Return ONLY corrected file blocks.
6. Use this exact format:
--- File: path/to/file.go ---
<full corrected file content>
7. Do not include explanations.
8. Do not use placeholders.
9. The final code must compile.
`)

	return b.String()
}

func Analyze(task, projectContext string) string {
	var b strings.Builder
	b.WriteString(`You are a senior Go engineer. Analyze the code and answer the user's request.

USER REQUEST:
`)
	b.WriteString(task)
	b.WriteString("\n")

	if strings.TrimSpace(projectContext) != "" {
		b.WriteString("PROJECT FILES:\n")
		b.WriteString(projectContext)
		b.WriteString("\n")
	}

	b.WriteString(`RULES:
1. Explain clearly and practically.
2. Point out bugs, risks, and improvements.
3. If useful, show short code examples in fenced code blocks with language tags.
4. Do not modify files.
5. Format the answer in GitHub Flavored Markdown.
6. Do not return --- File: blocks or placeholders.
7. Answer in the user's language if obvious.
`)

	return b.String()
}

// AnalyzeImage формирует промпт для анализа изображения.
func AnalyzeImage(task, projectContext string) string {
	var b strings.Builder
	b.WriteString(`You are a senior Go engineer with vision capabilities. Analyze the provided image and answer the user's request.
USER REQUEST:
`)
	b.WriteString(task)
	b.WriteString("\n")
	if strings.TrimSpace(projectContext) != "" {
		b.WriteString("PROJECT FILES:\n")
		b.WriteString(projectContext)
		b.WriteString("\n")
	}
	b.WriteString(`RULES:
1. Describe what you see in the image in detail.
2. If the image contains code, diagrams, error messages, or UI — analyze them.
3. If the image shows an architecture or design — explain it and suggest improvements.
4. Point out bugs, risks, and improvements if applicable.
5. Format the answer in GitHub Flavored Markdown.
6. Do not return --- File: blocks or placeholders.
7. Answer in the user's language if obvious.
`)
	return b.String()
}

func Chat(history, query string) string {
	var b strings.Builder
	b.WriteString("You are Gogitor, a helpful assistant for Go developers.\n")

	if strings.TrimSpace(history) != "" {
		b.WriteString("Conversation history:\n")
		b.WriteString(history)
		b.WriteString("\n")
	}

	b.WriteString("User: ")
	b.WriteString(query)
	b.WriteString("\n")

	b.WriteString(`RULES:
1. Answer in GitHub Flavored Markdown.
2. Be concise and practical.
3. If the question is about Go, prefer Go idioms and standard library.
4. Use fenced code blocks with language tags for code.
5. Use lists and headings only when they improve readability.
6. Do not modify files and do not return --- File: blocks.
7. Answer in the user's language if obvious.
`)

	return b.String()
}

func SearchQuery(query string) string {
	var b strings.Builder
	b.WriteString(`Create a concise web search query for the following user question.
Return ONLY the search query, no explanations.

USER QUESTION:
`)
	b.WriteString(query)
	b.WriteString("\n")

	return b.String()
}

func SearchAnswer(query, searchContent string) string {
	var b strings.Builder
	b.WriteString(`You are a helpful assistant with web search results.

USER QUESTION:
`)
	b.WriteString(query)
	b.WriteString("\n")

	b.WriteString("WEB SEARCH CONTENT:\n")
	b.WriteString(searchContent)
	b.WriteString("\n")

	b.WriteString(`RULES:
1. Answer using the web search content when relevant.
2. If sources conflict, say so.
3. If the content is insufficient, say what is missing.
4. Do not invent facts.
5. Format the answer in GitHub Flavored Markdown.
6. Use fenced code blocks when showing code.
7. If you mention sources, use a markdown list.
8. Do not return --- File: blocks.
`)

	return b.String()
}

func Plan(task string) string {
	var b strings.Builder
	b.WriteString(`You are a software planning agent. Break the task into 2-5 concrete subtasks.

TASK:
`)
	b.WriteString(task)
	b.WriteString("\n")

	b.WriteString(`RULES:
1. Return only lines in this format:
Subtask 1: ...
Subtask 2: ...
2. Maximum 5 subtasks.
3. Each subtask must be independently executable by a code generation agent.
4. No explanations.
`)

	return b.String()
}

func CodeModifyDiff(task, projectContext string) string {
	// Совместимый wrapper для старых вызовов.
	return CodeModifyDiffForModel(
		task,
		projectContext,
		"balanced",
	)
}

func CodeModifyDiffForModel(
	task,
	projectContext,
	patchPolicy string,
) string {
	var b strings.Builder

	b.WriteString(`You are a senior Go engineer modifying an existing project using minimal patches.

TASK:
`)
	b.WriteString(task)
	b.WriteString(`

EXISTING PROJECT FILES:
`)
	b.WriteString(projectContext)
	b.WriteString(`

CRITICAL RULES:
1. The existing project files above are the source of truth.
2. Do NOT rewrite code arbitrarily.
3. Do NOT invent new behavior unless the task explicitly asks for it.
4. Preserve existing logic, names, types, constants, imports, and behavior unless they must change.
5. Return ONLY changes required by the task.
6. Do not return unchanged files.
7. Do not include explanations.
8. Do not use placeholders.
9. If a new file is required, return it as a full file.
10. If an existing file is modified, prefer a minimal patch.

PATCH FORMAT:
--- Patch: path/to/file.go ---
<<<<<<< SEARCH
exact existing code
=======
new replacement code
>>>>>>> REPLACE

OPTIONAL SYMBOL ANCHOR:
--- Symbol: FunctionName ---
or:
--- Symbol: ReceiverType.MethodName ---

The Symbol line is optional.
If you know the exact function or method being changed, include it.
Do not invent a Symbol that does not exist in the supplied source.

RULES FOR SEARCH:
- SEARCH must come VERBATIM from the existing source.
- Do not invent missing source code.
- Do not modify indentation inside SEARCH.
- Do not add or remove spaces/tabs.
- Keep SEARCH focused on the smallest safe logical block.
- Do not include unrelated code.
- Use one logical modification per SEARCH/REPLACE block.
- If you need multiple changes in the same file, use multiple SEARCH/REPLACE blocks.

RULES FOR REPLACE:
- REPLACE must contain valid Go code.
- Use the same indentation style as the original file.
- If the file uses tabs, use tabs. If spaces, use spaces.
- Do not add trailing whitespace.
- Do not include SEARCH/REPLACE markers inside REPLACE.
`)

	// НОВОЕ: Примеры формата
	b.WriteString(`
EXAMPLES:

=== Example 1: Simple one-line change ===

Input task: "Change the timeout from 30 to 60 seconds"
Existing code contains:
    timeout := 30 * time.Second

Correct patch:
--- Patch: internal/config/config.go ---
<<<<<<< SEARCH
	timeout := 30 * time.Second
=======
	timeout := 60 * time.Second
>>>>>>> REPLACE

=== Example 2: Multi-line function modification ===

Input task: "Add error handling to the ParseConfig function"
Existing code contains:
    func ParseConfig(path string) *Config {
        data := os.ReadFile(path)
        return json.Unmarshal(data)
    }

Correct patch:
--- Patch: internal/config/config.go ---
--- Symbol: ParseConfig ---
<<<<<<< SEARCH
func ParseConfig(path string) *Config {
	data := os.ReadFile(path)
	return json.Unmarshal(data)
}
=======
func ParseConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}
>>>>>>> REPLACE

=== Example 3: Adding a new function ===

Input task: "Add a Validate method to the Config struct"
Existing code contains a Config struct but no Validate method.

Correct patch (insert after existing code):
--- Patch: internal/config/config.go ---
--- Symbol: Config ---
<<<<<<< SEARCH
type Config struct {
	Timeout int
	Port    int
}
=======
type Config struct {
	Timeout int
	Port    int
}

func (c *Config) Validate() error {
	if c.Timeout <= 0 {
		return fmt.Errorf("timeout must be positive, got %d", c.Timeout)
	}
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("port must be 1-65535, got %d", c.Port)
	}
	return nil
}
>>>>>>> REPLACE

=== Example 4: Adding imports ===

Input task: "Use fmt.Errorf instead of errors.New"
Existing code contains:
    import (
        "errors"
    )
    ...
    return errors.New("config invalid")

Correct patch:
--- Patch: internal/config/config.go ---
<<<<<<< SEARCH
import (
	"errors"
)
=======
import (
	"fmt"
)
>>>>>>> REPLACE

--- Patch: internal/config/config.go ---
<<<<<<< SEARCH
	return errors.New("config invalid")
=======
	return fmt.Errorf("config invalid: %v", err)
>>>>>>> REPLACE

=== Example 5: Changing a test ===

Input task: "Fix the test to expect the new error message"
Existing test contains:
    func TestValidate(t *testing.T) {
        err := cfg.Validate()
        if err == nil {
            t.Fatal("expected error")
        }
    }

Correct patch:
--- Patch: internal/config/config_test.go ---
--- Symbol: TestValidate ---
<<<<<<< SEARCH
func TestValidate(t *testing.T) {
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error")
	}
}
=======
func TestValidate(t *testing.T) {
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "timeout must be positive") {
		t.Fatalf("unexpected error: %v", err)
	}
}
>>>>>>> REPLACE

=== Example 6: New file (full file, not patch) ===

Input task: "Create a new file internal/config/validate.go with the Validate function"

Correct output:
--- File: internal/config/validate.go ---
package config

import "fmt"

func (c *Config) Validate() error {
	if c.Timeout <= 0 {
		return fmt.Errorf("timeout must be positive, got %d", c.Timeout)
	}
	return nil
}
`)

	// НОВОЕ: Примеры типичных ошибок
	b.WriteString(`
COMMON MISTAKES TO AVOID:

MISTAKE 1: SEARCH does not match the original file.
WRONG: SEARCH contains "return nil" but the file has "return 0".
CORRECT: Copy the exact text from EXISTING PROJECT FILES.

MISTAKE 2: SEARCH has different indentation.
WRONG: SEARCH uses 4 spaces but the file uses tabs.
CORRECT: Match the exact whitespace of the original.

MISTAKE 3: SEARCH is too large (contains entire file).
WRONG: SEARCH contains 50+ lines of unrelated code.
CORRECT: SEARCH contains only the 2-20 lines that change.

MISTAKE 4: SEARCH is too small (ambiguous).
WRONG: SEARCH is just "}" — this matches many places.
CORRECT: Include enough surrounding context to be unique.

MISTAKE 5: Inventing code in SEARCH.
WRONG: SEARCH contains code that does not exist in the file.
CORRECT: SEARCH must be a verbatim copy from EXISTING PROJECT FILES.

MISTAKE 6: Multiple changes in one SEARCH/REPLACE.
WRONG: One SEARCH/REPLACE changes imports AND a function.
CORRECT: Use separate SEARCH/REPLACE blocks for each change.

MISTAKE 7: SEARCH block is a single line like "return".
WRONG: SEARCH is just "return" — too ambiguous.
CORRECT: Include the full function or at least 2-3 unique lines.
`)

	switch strings.ToLower(strings.TrimSpace(patchPolicy)) {
	case "strict":
		b.WriteString(`
PATCH POLICY: STRICT
- Target 8B-14B class models.
- Prefer EXACT or whitespace-tolerant matches.
- Keep SEARCH short, normally 2-12 lines.
- Do NOT attempt to guess a location.
- If the location is uncertain, produce a smaller patch or do not patch.
- Never use fuzzy matching. The SEARCH must match exactly.
- Prefer including the full function signature in SEARCH for uniqueness.

Example for strict policy:
--- Patch: main.go ---
--- Symbol: main ---
<<<<<<< SEARCH
func main() {
	fmt.Println("hello")
}
=======
func main() {
	fmt.Println("world")
}
>>>>>>> REPLACE
`)
	case "advanced":
		b.WriteString(`
PATCH POLICY: ADVANCED
- Target strong local or cloud models.
- Prefer providing a Symbol anchor whenever possible.
- SEARCH may contain larger context when necessary.
- Make the patch precise and localized.
- Never modify unrelated symbols.
- You may use larger SEARCH blocks (up to 40 lines) if needed for clarity.
- Prefer Symbol anchors for all method-level changes.

Example for advanced policy with Symbol:
--- Patch: internal/server/server.go ---
--- Symbol: Server.HandleHealth ---
<<<<<<< SEARCH
func (s *Server) HandleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}
=======
func (s *Server) HandleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
    w.Write([]byte("{'status':'healthy'}"))
}
>>>>>>> REPLACE
`)
	default:
		b.WriteString(`
PATCH POLICY: BALANCED
- Target approximately 15B-32B models.
- Prefer EXACT or normalized matching first.
- Use a Symbol anchor when the modification is inside a known function or method.
- Keep SEARCH reasonably small, normally 2-24 lines.
- Never include unrelated functions.
- If two similar code blocks exist, use Symbol to disambiguate.

Example for balanced policy:
--- Patch: internal/handler/handler.go ---
--- Symbol: Handler.ServeHTTP ---
<<<<<<< SEARCH
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.handleGet(w, r)
}
=======
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.handleGet(w, r)
	case http.MethodPost:
		h.handlePost(w, r)
	default:
		http.Error(w, "not allowed", http.StatusMethodNotAllowed)
	}
}
>>>>>>> REPLACE
`)
	}

	b.WriteString(`
IF THE CHANGE CANNOT BE EXPRESSED SAFELY AS A SMALL PATCH:
--- File: path/to/file.go ---
<complete updated file>

The final code must compile with standard Go tooling.

FINAL CHECKLIST (verify before output):
[ ] Every SEARCH block is copied VERBATIM from EXISTING PROJECT FILES.
[ ] Every SEARCH block is unique in the file (or has a Symbol anchor).
[ ] REPLACE blocks use the same indentation as the original file.
[ ] No SEARCH/REPLACE markers appear inside SEARCH or REPLACE content.
[ ] Each SEARCH/REPLACE block performs exactly one logical change.
[ ] All modified files will still compile together.
[ ] New files use --- File: format, not --- Patch: format.
`)
	return b.String()
}


func Intent(history, query, projectSummary string) string {
	var b strings.Builder
	b.WriteString(`You are the intent router for Gogitor, an AI coding assistant for Go.
Your job is to decide which Gogitor mode should handle the user request.
Return ONLY valid compact JSON.
Do not return markdown.
Do not return explanations outside JSON.
JSON schema:
{"mode":"code|analyze|search|chat|run|test|git|fix|article|computer","task":"refined task","reason":"short reason"}
Mode rules:
- code: create, modify, refactor, split, extract, move, add features, write program/code, generate files.
- fix: user pastes a panic, stack trace, build error output, test failure, or runtime error and wants it diagnosed and fixed. If the input contains "panic:", "goroutine", "runtime error", ".go:" with line numbers, or "--- FAIL", choose fix.
- analyze: explain code, review code, compare approaches, find bugs, answer questions about existing code without modifying files.
- If the request asks to analyze code AND then create/modify/write/save/generate any file or script, choose code, not analyze.
- search: needs fresh web information, latest versions, news, documentation lookup, internet search.
- run: user asks to run or execute a Go program.
- test: user asks to run tests, lint code, or check code quality with golangci-lint.
- git: user asks for git status, diff, commit, init, log, checkout, branch, merge, push, pull, fetch, clone, remote.
- computer: user asks to execute shell commands, manage files/directories, check system status, install packages, or interact with the OS (e.g., cat, ls, grep, mkdir, df, top, apt, history).
- chat: general conversation, advice, questions not requiring file changes, tests, run, git, or web search.
- article: write an article, blog post, story, news piece, tutorial, essay,documentation text, or any creative/informational text. NOT code generation.
`)

	if strings.TrimSpace(history) != "" {
		b.WriteString("\nDIALOG HISTORY:\n")
		b.WriteString(history)
		b.WriteString("\n")
	}

	if strings.TrimSpace(projectSummary) != "" {
		b.WriteString("\nPROJECT SUMMARY:\n")
		b.WriteString(projectSummary)
		b.WriteString("\n")
	}

	b.WriteString("\nUSER REQUEST:\n")
	b.WriteString(query)
	b.WriteString("\n")

	return b.String()
}

func CommitMessage(diff, fileStatus, taskContext string) string {
	var b strings.Builder
	b.WriteString(`You are a Git commit message generator for a Go project.
Generate a commit message following Conventional Commits format.

FORMAT: type(scope): description

RULES:
1. type MUST be one of: feat, fix, refactor, docs, test, chore, perf, ci, build
2. scope is the main package or module affected (e.g., auth, db, api, runner, workspace). Omit scope if changes span many unrelated modules.
3. description: imperative mood, lowercase first letter, no trailing period, max 72 characters
4. If changes are complex, add a short body after a blank line (max 3 lines)
5. Write in English
6. Return ONLY the commit message text. No markdown fences, no quotes, no explanations.
7. Base the message strictly on the actual diff provided, not on assumptions.

EXAMPLES:
feat(auth): add JWT token validation to middleware
fix(runner): handle nil pointer when test output is empty
refactor(workspace): extract patch application into separate method
test(index): add coverage for BM25 ranking with empty corpus
chore: update golangci-lint configuration

`)
	if strings.TrimSpace(taskContext) != "" {
		b.WriteString("USER TASK (context only, the diff is the source of truth):\n")
		if len(taskContext) > 500 {
			b.WriteString(textutil.LimitRunes(taskContext, 500, "..."))
			b.WriteString("...")
		} else {
			b.WriteString(taskContext)
		}
		b.WriteString("\n\n")
	}
	if strings.TrimSpace(fileStatus) != "" {
		b.WriteString("CHANGED FILES (git status --short):\n")
		if len(fileStatus) > 2000 {
			b.WriteString(textutil.LimitRunes(fileStatus, 2000, "..."))
			b.WriteString("...")
		} else {
			b.WriteString(fileStatus)
		}
		b.WriteString("\n\n")
	}
	b.WriteString("GIT DIFF:\n")
	b.WriteString(diff)
	b.WriteString("\n")
	return b.String()
}

func CompareApproaches(task, projectContext string) string {
	var b strings.Builder
	b.WriteString(`You are a senior Go architect performing a comparative analysis.

Analyze the task and propose 2-3 FUNDAMENTALLY DIFFERENT implementation approaches.

Return ONLY valid compact JSON.
Do not return markdown.
Do not return explanations outside JSON.

JSON schema:
{
  "approaches": [
    {
      "id": 1,
      "name": "Short approach name (3-5 words)",
      "description": "1-2 sentence description of the approach",
      "complexity": "low|medium|high — brief explanation",
      "performance": "excellent|good|adequate|poor — brief explanation",
      "readability": "excellent|good|adequate|poor — brief explanation",
      "dependencies": "stdlib only | N external: list them",
      "testability": "excellent|good|adequate|difficult — brief explanation",
      "justification": "Why this approach is viable and when to choose it",
      "tradeoffs": "Main trade-offs of this approach"
    }
  ],
  "recommended_id": 1,
  "recommendation": "Which approach is recommended and why, including key trade-offs"
}

RULES:
1. Propose 2-3 approaches that are FUNDAMENTALLY different in architecture or technique.
2. Each approach must be realistic and implementable in Go.
3. Be honest about trade-offs. No approach is perfect.
4. The recommendation must consider the specific task context.
5. Write "name", "description", "justification", "tradeoffs", and "recommendation" in the same language as the TASK.
6. Criteria values (complexity, performance, readability, testability) should start with a rating word, then a dash and brief explanation in the task language.
7. Use concrete Go patterns, standard library features, and well-known libraries.
8. Do not invent unrelated features.
`)
	if strings.TrimSpace(projectContext) != "" {
		b.WriteString("\nPROJECT CONTEXT:\n")
		b.WriteString(projectContext)
		b.WriteString("\n")
	}
	b.WriteString("\nTASK:\n")
	b.WriteString(task)
	b.WriteString("\n")
	return b.String()
}

// AnalyzeDecisions просит LLM проанализировать журнал решений
// и найти «долг» — временные решения, которые стоит пересмотреть.
func AnalyzeDecisions(journalText, projectContext string) string {
	var b strings.Builder
	b.WriteString(`You are a senior Go architect reviewing a project's decision history.
You are given the DECISION JOURNAL of a project and its current state.

Your job:
1. Summarize the key decisions chronologically (2-4 sentences).
2. Identify "decision debt": temporary decisions whose original constraint may no longer apply.
3. For each debt item, explain WHY it might be time to revisit.
4. Note any patterns (e.g. repeatedly choosing quick fixes over proper solutions).

Return ONLY valid compact JSON.
Do not return markdown.
Do not return explanations outside JSON.

JSON schema:
{
  "summary": "2-4 sentence overview of the decision history",
  "debts": [
    {
      "decision_id": 1,
      "decision": "the temporary decision text",
      "original_date": "date from journal",
      "constraint": "the original constraint that forced this decision",
      "suggestion": "why the constraint may be lifted and what to do now"
    }
  ],
  "patterns": ["observed pattern in decision-making"],
  "risk_notes": ["any risks from accumulated debt"]
}

RULES:
1. Only flag debts where the constraint is plausibly resolved.
2. If there are no debts, return an empty debts array.
3. Be specific — reference actual decisions from the journal.
4. Write in the same language as the journal entries.
5. Maximum 5 debts, maximum 3 patterns, maximum 3 risk_notes.
`)
	b.WriteString("\nDECISION JOURNAL:\n")
	b.WriteString(journalText)
	b.WriteString("\n")
	if strings.TrimSpace(projectContext) != "" {
		b.WriteString("\nCURRENT PROJECT CONTEXT:\n")
		b.WriteString(projectContext)
		b.WriteString("\n")
	}
	return b.String()
}

// ApproachSelection просит LLM определить, выбирает ли пользователь подход,
// модифицирует его, или задаёт совершенно новую задачу.
func ApproachSelection(approaches []domain.Approach, recommendation, userInput string) string {
	var b strings.Builder
	b.WriteString(`You are an approach selection interpreter for Gogitor.
The user was shown a list of implementation approaches and is now responding.
Determine what the user wants:
1. SELECT an existing approach (by number, name, description, or implicit reference like "the simpler one").
2. SELECT an approach WITH MODIFICATIONS (user accepts an approach but wants changes).
3. Start a completely NEW TASK unrelated to approach selection.

Return ONLY valid compact JSON. No markdown. No explanations outside JSON.
JSON schema:
{"action":"select|new_task","approach_id":1,"modification":"","reason":"short reason"}

RULES:
- action="select": user is choosing one of the presented approaches (with or without modifications).
- action="new_task": user's input is clearly a new, unrelated task/question.
- approach_id: 1-based ID of the chosen approach (only meaningful for action="select").
- modification: describe requested changes to the approach in user's language. Empty string if none.
- reason: one-sentence explanation of your interpretation in user's language.
- If user says "yes", "ok", "да", "хорошо", "согласен", "accept", they choose the RECOMMENDED approach.
- If user references an approach by characteristics (e.g. "без внешних зависимостей", "the simpler one"), match to the best-fitting approach.
- If user mentions a number (1, 2, 3) or ordinal ("первый", "second"), use it directly.

APPROACHES:
`)
	for _, a := range approaches {
		rec := ""
		if a.Recommended {
			rec = " [RECOMMENDED]"
		}
		fmt.Fprintf(&b, "ID %d%s: %s — %s\n", a.ID, rec, a.Name, a.Description)
		if a.Tradeoffs != "" {
			fmt.Fprintf(&b, "  Trade-offs: %s\n", a.Tradeoffs)
		}
	}
	if recommendation != "" {
		b.WriteString("\nRECOMMENDATION: " + recommendation + "\n")
	}
	b.WriteString("\nUSER RESPONSE:\n" + userInput + "\n")
	return b.String()
}

// Suggest просит LLM проанализировать проект и предложить конкретные улучшения.
func Suggest(projectContext, memory string) string {
	var b strings.Builder
	b.WriteString(`You are a senior Go architect performing a project health review.
Analyze the project files below and produce ACTIONABLE improvement suggestions.

Return your answer in GitHub Flavored Markdown.
Do NOT return --- File: blocks.
Do NOT modify files.

STRUCTURE (use exactly these headings):
## 🔴 Critical
Issues that cause bugs, security vulnerabilities, or data loss.

## 🟡 Tech Debt
Temporary solutions, duplicated code, overly complex logic.

## 🧪 Missing Tests
Exported functions or critical paths without test coverage.

## 🧹 Code Smells
Style issues, naming problems, unused code.

## 💡 Improvements
Performance, architecture, or API improvements.

RULES:
1. Each item MUST reference a specific file and function/line if visible.
2. Each item MUST be one sentence describing the problem + one sentence describing the fix.
3. Do NOT give generic advice like "add comments" or "improve error handling" without pointing to a specific location.
4. Maximum 5 items per section. Skip a section if nothing relevant.
5. If the project looks healthy, say so briefly.
6. Write in the same language as the file names and comments suggest (ru/en).
7. Do not repeat suggestions from PROJECT MEMORY.
`)
	if strings.TrimSpace(memory) != "" {
		b.WriteString("\nPROJECT MEMORY (do not repeat rejected ideas):\n")
		b.WriteString(memory)
		b.WriteString("\n")
	}
	b.WriteString("\nPROJECT FILES:\n")
	b.WriteString(projectContext)
	b.WriteString("\n")
	return b.String()
}

// PRDescription генерирует title и body для Pull Request на основе diff.
func PRDescription(diff, branchName string) string {
	var b strings.Builder
	b.WriteString(`You are a technical writer creating a Pull Request description.
Generate a concise PR title and body based strictly on the diff below.

Return ONLY valid compact JSON. No markdown fences. No explanations outside JSON.
JSON schema:
{"title": "type(scope): short description", "body": "markdown body"}

RULES:
1. Title follows Conventional Commits: type(scope): description (max 72 chars).
2. Type must be one of: feat, fix, refactor, docs, test, chore, perf, ci, build.
3. Body is GitHub Flavored Markdown, 3-10 lines, describing WHAT changed and WHY.
4. Do not invent changes not present in the diff.
5. Write in English.
6. If the diff is empty or trivial, use a minimal title and body.

BRANCH: `)
	b.WriteString(branchName)
	b.WriteString("\n\nDIFF:\n")
	if len(diff) > 8000 {
		b.WriteString(textutil.TruncateStringBytes(diff, 8000))
		b.WriteString("\n... (diff truncated)")
	} else {
		b.WriteString(diff)
	}
	b.WriteString("\n")
	return b.String()
}

// CodeFixPatch просит LLM исправить патч, который привёл к ошибке сборки/тестов.
// В отличие от CodeFix, здесь НЕ запрашивается полный файл — только исправленный патч.
func CodeFixPatch(task, projectContext, patchContent, errors string) string {
	var b strings.Builder
	b.WriteString(`You are a senior Go engineer. You previously generated a SEARCH/REPLACE patch, but it caused an error.
Fix the patch. Do NOT return the full file. Return ONLY the corrected patch.

ORIGINAL TASK:
`)
	b.WriteString(task)
	b.WriteString("\n")
	b.WriteString("EXISTING PROJECT FILES:\n")
	b.WriteString(projectContext)
	b.WriteString("\n")
	b.WriteString("YOUR PREVIOUS PATCH:\n")
	b.WriteString(patchContent)
	b.WriteString("\n")
	b.WriteString("ERROR THAT OCCURRED:\n")
	b.WriteString(errors)
	b.WriteString("\n")
	b.WriteString(`RULES:
1. Fix ONLY the error shown above.
2. Return the corrected patch in the SAME format:

--- Patch: path/to/file.go ---
--- Symbol: OptionalSymbol ---

<<<<<<< SEARCH
exact existing code that must be replaced
=======
new replacement code
>>>>>>> REPLACE

3. If the previous patch contained a Symbol line, preserve it unless the Symbol itself was incorrect.
4. The SEARCH block must match the original file content before the previous patch was applied.
5. The SEARCH block must match the ORIGINAL file content (before your previous patch was applied).
6. Do NOT rewrite the entire file.
7. Do NOT add unrelated changes.
8. The result must compile and pass tests.
9. Preserve existing behavior unless it is the cause of the error.
`)
	return b.String()
}