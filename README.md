# Gogitor — AI Coding Assistant for Go

[![License](https://img.shields.io/badge/License-BSD--3--Clause-blue.svg)](LICENSE.txt)
[![Go](https://img.shields.io/badge/Go-1.25.1-00ADD8.svg)](https://go.dev/)
[![Version](https://img.shields.io/badge/version-1.8.1-blue.svg)](https://github.com/SkalaSkalolaz/gogitor)
[![Platform](https://img.shields.io/badge/platform-Linux%20%7C%20macOS%20%7C%20WSL-lightgrey.svg)](#requirements)

[Русская версия](README_RU.md)

Gogitor is an AI-powered terminal assistant for Go developers. It combines a classic CLI, an interactive terminal UI, project-aware code generation, automated validation, multi-agent orchestration, Git/GitHub integration, web search, project analysis, and persistent engineering context.

Gogitor can work with local or remote LLMs through **Ollama** or **OpenAI-compatible APIs**. It can understand a natural-language task, inspect the existing project, generate or modify code, validate changes in an isolated temporary workspace, run tests and quality checks, and apply the changes only after successful validation.

For complex tasks, Gogitor can use a multi-agent pipeline with planning, implementation, review, and verification.

> **Current version:** `0.9.0`

> Gogitor is designed primarily for Go projects. The application interface is available in English and Russian.

---

## Table of Contents

* [What Gogitor Does](#what-gogitor-does)
* [Features](#features)

  * [Interactive TUI](#interactive-tui)
  * [Code Generation](#code-generation)
  * [Validation and Error Fixing](#validation-and-error-fixing)
  * [Multi-Agent Engineering](#multi-agent-engineering)
  * [Workflow Mode](#workflow-mode)
  * [Project Intelligence](#project-intelligence)
  * [Git and GitHub](#git-and-github)
  * [Web Search](#web-search)
  * [Documentation and Project Analysis](#documentation-and-project-analysis)
* [Requirements](#requirements)
* [Installation](#installation)
* [Quick Start](#quick-start)
* [LLM Providers](#llm-providers)
* [Main Commands](#main-commands)
* [CLI Usage](#cli-usage)
* [TUI Mode](#tui-mode)
* [Code Generation Workflow](#code-generation-workflow)
* [Multi-Agent Mode](#multi-agent-mode)
* [Workflow Mode in Detail](#workflow-mode-in-detail)
* [Project Indexing](#project-indexing)
* [Configuration](#configuration)
* [Environment Variables](#environment-variables)
* [Git and GitHub Integration](#git-and-github-integration)
* [Examples](#examples)
* [Project Structure](#project-structure)
* [Safety](#safety)
* [Troubleshooting](#troubleshooting)
* [Development](#development)
* [License](#license)

---

## What Gogitor Does

Gogitor is intended to sit between a developer and an LLM.

Instead of simply sending a prompt to a model and copying the generated code back into a project, Gogitor provides an engineering workflow around the model:

```text
User task
   │
   ▼
Intent detection
   │
   ├── Chat
   ├── Analysis
   ├── Web search
   ├── Code generation
   ├── Fix
   ├── Run
   ├── Test
   ├── Git
   └── Article
        │
        ▼
Project context / AST index
        │
        ▼
LLM
        │
        ▼
Files or SEARCH/REPLACE patches
        │
        ▼
Temporary sandbox
        │
        ├── go mod init
        ├── go mod tidy
        ├── gofmt
        ├── go build
        ├── go test
        ├── go vet
        └── golangci-lint
        │
        ▼
Validated result
        │
        ├── Apply changes
        └── Optional Git commit
```

For complex work, the pipeline can be expanded into multiple agents and a traceable workflow with persistent artifacts.

---

# Features

## Interactive TUI

Gogitor provides a terminal UI based on Bubble Tea.

The TUI includes:

* Markdown rendering.
* Conversation history.
* Command autocomplete.
* Streaming LLM responses.
* Separate input and output focus.
* Mouse text-selection mode.
* Live execution progress.
* Plan board for multi-agent tasks.
* Task status indicators.
* Estimated completion time.
* Project and LLM diagnostics.
* English and Russian localization.

Start the TUI simply with:

```bash
./gogitor
```

Or explicitly:

```bash
./gogitor tui
```

---

## Code Generation

Gogitor can create new Go code and modify existing projects while taking the current project context into account.

Supported operations include:

* Creating new files.
* Modifying existing files.
* Refactoring.
* Splitting code between files.
* Extracting functions or components.
* Fixing compilation errors.
* Fixing failed tests.
* Implementing new features.
* Working with task files.
* Applying minimal patches.
* Generating complete files when patches are unsuitable.

### Patch Mode

For existing code, Gogitor can ask the LLM to produce a minimal `SEARCH/REPLACE` patch:

```text
--- Patch: path/to/file.go ---
<<<<<<< SEARCH
exact existing code
=======
new replacement code
>>>>>>> REPLACE
```

The search block is expected to match the original file exactly, including indentation and surrounding context.

If a patch cannot be safely applied or validation fails, Gogitor can fall back to full-file generation.

---

## Validation and Error Fixing

Generated changes are not immediately written into the working project.

Gogitor first works with a temporary copy of the project and performs validation.

Depending on the operation, it can run:

```text
go mod init
go mod tidy
gofmt
go build
go test -v -cover
go vet
golangci-lint
```

Test output is parsed to extract:

* Passed tests.
* Failed tests.
* Test names.
* Functions.
* Source files.
* Line numbers.
* Failure messages.
* Coverage information.

This information can then be supplied back to the LLM for targeted correction.

### Fix Mode

The `fix` command accepts compiler output, test failures, panic messages, or stack traces:

```bash
./gogitor fix "panic: runtime error: index out of range"
```

The intent router also recognizes typical Go errors such as:

```text
panic:
runtime error
goroutine
.go:123
--- FAIL
```

and can automatically select the fix workflow.

---

## Multi-Agent Engineering

For complex tasks Gogitor can use multiple specialized agents:

```text
┌──────────┐
│ Planner  │
└────┬─────┘
     │
     ▼
┌──────────┐
│  Coder   │
└────┬─────┘
     │
     ▼
┌──────────┐
│ Reviewer │
└────┬─────┘
     │
     ▼
┌──────────┐
│ Verifier │
└──────────┘
```

### Planner

Breaks a complex task into several concrete subtasks and defines acceptance criteria.

### Coder

Implements the subtasks and validates the resulting project.

### Reviewer

Looks for important problems such as:

* Compilation errors.
* Incorrect implementation.
* Nil dereferences.
* Security problems.
* Regressions.
* Violations of the original task.

### Verifier

Checks whether the original goal was actually achieved and can generate an additional correction task when necessary.

The agent system also includes:

* Priority-based LLM request queue.
* Per-role budgets.
* Retry with exponential backoff.
* Usage tracking.
* Execution timing statistics.
* Progress and ETA information.
* Persistent agent memory.
* Checkpoint and rollback support.

---

## Workflow Mode

Workflow mode is designed for larger engineering tasks where traceability matters.

Start a workflow with:

```bash
./gogitor workflow "create a REST API with authentication and tests"
```

A workflow creates a dedicated session directory:

```text
.gogitor/workflow/<timestamp>/
```

Typical artifacts include:

```text
inbox.md
research.md
plan.md
prd.json
process.md
gate-report-task-01.json
gate-report-task-02.json
...
reflection.md
```

The workflow:

1. Refines the original goal.
2. Decomposes the task into 2–5 atomic subtasks.
3. Executes each task through the coding agent.
4. Runs quality gates after each task.
5. Records execution information.
6. Creates separate commits when automatic commits are enabled.
7. Stops if a required quality gate fails.

Quality gates include:

```text
go build
go test
go vet
gofmt -l
golangci-lint
```

This makes a workflow session inspectable after execution instead of leaving only a final diff.

---

## Workflow Interview

For ambiguous tasks, use:

```bash
./gogitor workflow interview "add caching to the API layer"
```

The planner generates several targeted questions.

You can answer them using:

```text
1: use an in-memory cache
2: cache GET requests only
3: invalidate after writes
```

You can also use:

```text
skip
```

or:

```text
go
```

to accept the proposed defaults.

Gogitor then creates a refined task specification and starts the normal workflow.

---

## Workflow Reflection

After completing a workflow:

```bash
./gogitor workflow reflect
```

Gogitor reads the workflow artifacts and asks the LLM to perform a retrospective analysis.

The resulting `reflection.md` can contain:

* What went well.
* What could be improved.
* Execution metrics.
* Problems encountered.
* Recommendations.
* Final assessment.

For smaller local models Gogitor uses a simplified reflection prompt.

---

## Workflow Pull Requests

A completed workflow can be turned into a GitHub Pull Request:

```bash
./gogitor workflow pr
```

The generated PR description can include:

* Original goal.
* Implementation plan.
* Task table.
* Commit information.
* Quality-gate results.
* Execution log.
* Workflow reflection.

The workflow PR integration can create or reuse an appropriate feature branch, push it to `origin`, and create the Pull Request through the GitHub API.

A Git repository, configured `origin`, and GitHub token are required.

---

## Project Intelligence

Gogitor does not blindly send every project file to the model.

It maintains an AST-based index and uses it to identify relevant project context.

The index contains information such as:

* Go source files.
* Packages.
* Imports.
* Function and method relationships.
* Call relationships.
* Project structure.
* File importance.
* Text relevance.

The ranking system combines:

* AST information.
* Import graph.
* Call graph.
* PageRank centrality.
* BM25 relevance.
* Russian/English synonym expansion.

The index is cached under the user's cache directory and refreshed when project files change.

This allows Gogitor to work with larger projects without sending the entire repository to every LLM request.

---

# Approach Comparison

For sufficiently complex tasks Gogitor can compare several fundamentally different implementation approaches before coding begins.

For example:

```text
## Comparative Analysis of Approaches

| # | Approach        | Complexity | Performance | Readability | Dependencies |
|---|-----------------|------------|-------------|-------------|--------------|
| 1 | stdlib HTTP mux | low        | good        | excellent   | stdlib only  |
| 2 | chi router      | medium     | excellent   | good        | 1 external   |
| 3 | gRPC            | high       | excellent   | adequate    | 3 external   |
```

You can then:

* Select an approach by number.
* Accept the recommendation with `yes`.
* Describe a different approach.
* Modify the recommended approach.

Disable this feature with:

```bash
./gogitor code "create an HTTP server" --no-compare
```

or:

```bash
GOGITOR_COMPARE_APPROACHES=false
```

---

# Automatic Intent Detection

Gogitor can determine the appropriate operation from a natural-language request.

Examples:

```text
"write an HTTP server"
        → code

"explain context.Context"
        → chat

"find bugs in the project"
        → analyze

"latest Go version"
        → search

"run the tests"
        → test

"commit the changes"
        → git

"fix this panic: runtime error..."
        → fix

"write an article about Go"
        → article
```

The intent router distinguishes between requests that require code modification and requests that should only analyze or explain existing code.

---

# Main Commands

| Command                             | Purpose                                                      |
| ----------------------------------- | ------------------------------------------------------------ |
| `gogitor`                           | Start the interactive TUI                                    |
| `gogitor tui`                       | Start the TUI explicitly                                     |
| `gogitor code <task>`               | Create or modify Go code                                     |
| `gogitor fix <error>`               | Fix compiler errors, test failures, panics, and stack traces |
| `gogitor ask <question>`            | Ask a general development question                           |
| `gogitor analyze <question>`        | Analyze the project without modifying it                     |
| `gogitor search <query>`            | Search the web and summarize results                         |
| `gogitor run [file]`                | Run a Go project or program                                  |
| `gogitor test`                      | Run Go tests                                                 |
| `gogitor test lint`                 | Run `golangci-lint` and process issues                       |
| `gogitor vet`                       | Run `go vet`                                                 |
| `gogitor todo`                      | Scan for TODO/FIXME/HACK/XXX/BUG markers                     |
| `gogitor suggest`                   | Review project health                                        |
| `gogitor decisions`                 | Inspect decision history and decision debt                   |
| `gogitor task <file>`               | Execute a task from a `.txt` or `.md` file                   |
| `gogitor file <file>`               | Execute a task file with explicit mode selection             |
| `gogitor article <topic>`           | Generate an article or technical document                    |
| `gogitor workflow <task>`           | Run a traceable multi-step workflow                          |
| `gogitor workflow interview <task>` | Clarify an ambiguous workflow task                           |
| `gogitor workflow reflect`          | Analyze the latest workflow                                  |
| `gogitor workflow pr`               | Create a Pull Request from workflow artifacts                |
| `gogitor git <subcommand>`          | Perform Git/GitHub operations                                |
| `gogitor doctor`                    | Show diagnostics and active configuration                    |
| `gogitor version`                   | Show the Gogitor version                                     |

---

# CLI Usage

```text
gogitor
gogitor tui [flags]
gogitor code <task> [flags]
gogitor fix <error / stack trace> [flags]
gogitor task <path/to/file.txt|file.md> [flags]
gogitor file <path/to/file.txt|file.md> [flags]
gogitor ask <question> [flags]
gogitor analyze <question> [flags]
gogitor search <query> [flags]
gogitor run [file] [flags]
gogitor test [lint] [flags]
gogitor article <topic> [--full] [flags]
gogitor suggest [flags]
gogitor vet [flags]
gogitor todo [flags]
gogitor decisions [flags]
gogitor workflow <task> [flags]
gogitor workflow interview <task> [flags]
gogitor workflow reflect [flags]
gogitor workflow pr [flags]
gogitor git <subcommand> [flags]
gogitor doctor [flags]
gogitor help
gogitor version
```

## Common Flags

| Flag                   | Short | Description                                          |
| ---------------------- | :---: | ---------------------------------------------------- |
| `--provider <name>`    |  `-p` | LLM provider                                         |
| `--model <model>`      |  `-m` | Model name                                           |
| `--key <key>`          |  `-k` | LLM API key                                          |
| `--repo <path>`        |  `-r` | Project root directory                               |
| `--github <url>`       |       | GitHub repository URL                                |
| `--key-github <token>` |       | GitHub token                                         |
| `--max-context <n>`    |       | Maximum model context in tokens; `0` means automatic |
| `--auto-search`        |       | Enable automatic web search in multi-agent mode      |
| `--output <file>`      |  `-o` | Save the result to a file                            |
| `--debug`              |       | Enable debug logging                                 |
| `--raw`                |       | Output only result content                           |
| `--pretty`             |       | Force human-readable output                          |
| `--help`               |  `-h` | Show help                                            |

### Code Flags

| Flag           | Description                            |
| -------------- | -------------------------------------- |
| `--dry-run`    | Validate changes without applying them |
| `--no-commit`  | Disable automatic Git commit           |
| `--no-tests`   | Skip tests                             |
| `--no-compare` | Disable approach comparison            |
| `--json`       | Produce JSON output                    |

### Task File Flags

| Flag     | Description                                           |
| -------- | ----------------------------------------------------- |
| `--code` | Force code mode instead of automatic intent detection |
| `--json` | Produce JSON output                                   |

Flags may be placed before or after the task.

If the task itself starts with a dash, use `--`:

```bash
./gogitor code -- --fix something
```

This is also useful when a task contains text that looks like a command-line option.

---

# TUI Mode

Start:

```bash
./gogitor
```

or:

```bash
./gogitor tui
```

## Built-in Commands

| Command                      | Description                          |
| ---------------------------- | ------------------------------------ |
| `:help`                      | Show help                            |
| `:clear`                     | Clear in-memory conversation context |
| `:cls`                       | Clear the screen                     |
| `:code <task>`               | Code generation/modification         |
| `:ask <question>`            | Chat                                 |
| `:analyze <task>`            | Analyze project                      |
| `:search <query>`            | Web search                           |
| `:run [file]`                | Run the project                      |
| `:test`                      | Run tests                            |
| `:test lint`                 | Run linting and automatic fixing     |
| `:fix <error>`               | Fix an error                         |
| `:git <subcommand>`          | Git operation                        |
| `:save <file>`               | Save the last result                 |
| `:article <topic>`           | Generate an article                  |
| `:suggest`                   | Review project health                |
| `:vet`                       | Run `go vet`                         |
| `:todo`                      | Scan TODO/FIXME/HACK markers         |
| `:decisions`                 | Show decision journal                |
| `:workflow <task>`           | Start workflow                       |
| `:workflow interview <task>` | Start workflow interview             |
| `:workflow reflect`          | Reflect on the latest workflow       |
| `:workflow pr`               | Create a PR from workflow artifacts  |
| `:quit` / `:q`               | Exit                                 |

## Keyboard Shortcuts

| Key             | Action                             |
| --------------- | ---------------------------------- |
| `Enter`         | Send input                         |
| `Alt+Enter`     | Insert a new line                  |
| `Tab`           | Switch focus / accept autocomplete |
| `Esc`           | Return to input                    |
| `PgUp` / `PgDn` | Scroll history or output           |
| `F2`            | Toggle mouse selection             |
| `Ctrl+C`        | Cancel current operation or quit   |

---

# Reasoning Mode
Gogitor supports reasoning/thinking mode for models that support it
(DeepSeek-R1, QwQ, Qwen3, OpenAI o1/o3/o4-mini, etc.).

Command-line flags:
  --reasoning            Enable reasoning/thinking mode
  --reasoning-effort <v> Reasoning depth: low, medium, high
  --reasoning-budget <n> Max tokens for reasoning (0=server default)
  --reasoning-show       Display thinking content in output

TUI command:
  :reasoning             Show current reasoning mode status
  :reasoning on          Enable reasoning mode
  :reasoning off         Disable reasoning mode

Environment variables:
  GOGITOR_REASONING=true         Enable reasoning mode
  GOGITOR_REASONING_EFFORT=high  Reasoning depth
  GOGITOR_REASONING_BUDGET=8192  Max tokens for reasoning

Configuration (.gogitor.json):
  {
    "reasoning_enabled": true,
    "reasoning_effort": "high",
    "reasoning_budget": 8192,
    "reasoning_show": false
  }

Note:
  - For Ollama, the "think": true parameter is used
  - For OpenAI-compatible APIs, "reasoning_effort" is used
  - If the model does not support reasoning, the parameter is ignored
    or the request is retried without it (depends on provider)
---

# LLM Providers

Gogitor supports local, remote, and OpenAI-compatible LLM endpoints.

## Ollama

Local Ollama:

```bash
./gogitor tui \
  --provider ollama \
  --model gemma3:4b
```

A remote Ollama-compatible server can be specified directly:

```bash
./gogitor tui \
  --provider http://192.168.1.10:11434 \
  --model gemma3:4b
```

Supported provider formats include:

```text
ollama
http://host:11434
https://host:11434
```

## OpenAI-Compatible API

OpenAI-compatible HTTPS endpoint:

```bash
./gogitor ask "explain generics in Go" \
  --provider openai+https://api.example.com/v1 \
  --model gpt-4o-mini \
  --key sk-...
```

Generic OpenAI-compatible HTTP endpoint:

```bash
./gogitor code "create main.go" \
  --provider openai-compatible+http://localhost:8000/v1 \
  --model local-model
```

Provider formats:

| Provider                           | Meaning                        |
| ---------------------------------- | ------------------------------ |
| `ollama`                           | Local Ollama                   |
| `http://host:11434`                | Ollama-compatible HTTP server  |
| `https://host:11434`               | Ollama-compatible HTTPS server |
| `openai+https://host/v1`           | OpenAI-compatible API          |
| `openai-compatible+http://host/v1` | OpenAI-compatible API          |

---

# Code Generation Workflow

When a coding task is executed, Gogitor follows a validation-oriented process.

### 1. Understand the task

The request is classified and, for complex tasks, may be decomposed into subtasks.

### 2. Build project context

The project index is used to select relevant source files and relationships.

### 3. Generate changes

The LLM returns either complete file blocks or minimal patches.

Example:

```text
--- File: main.go ---
package main

...
```

or:

```text
--- Patch: internal/server/server.go ---
<<<<<<< SEARCH
old code
=======
new code
>>>>>>> REPLACE
```

### 4. Create a temporary workspace

The current project is copied to a temporary sandbox.

### 5. Apply the changes

Generated files or patches are applied to the sandbox.

### 6. Validate

Gogitor can perform:

```text
go mod init
go mod tidy
gofmt
go build
go test -v -cover
```

Additional quality checks are used by workflow mode.

### 7. Apply validated changes

Only after successful validation are changes copied back to the actual project.

### 8. Git integration

If enabled, Gogitor can automatically create a commit based on the actual diff.

If patch application is unsuccessful, Gogitor can fall back to a full-file generation strategy.

---

# Project Indexing

The project index is designed to improve context selection for LLM requests.

It analyzes Go source code and maintains relationships such as:

```text
Files
 │
 ├── Packages
 │
 ├── Imports
 │
 ├── Functions
 │
 └── Calls
```

The relevance system combines structural and textual signals.

### Import Graph

Tracks dependencies between project files and packages.

### Call Graph

Tracks function and method relationships where they can be resolved from the source.

### PageRank

Identifies files that are structurally important to the project.

### BM25

Ranks files according to their textual relevance to the current task.

Gogitor also applies English/Russian synonym expansion to improve retrieval for multilingual projects and requests.

The index is cached under the operating system's user cache directory and refreshed when source files change.

---

# Git and GitHub Integration

Gogitor integrates Git directly into the development workflow.

Supported operations include:

```text
status
diff
diff-task
commit
init
log
checkout
branch
merge
push
pull
fetch
clone
remote
```

Examples:

```bash
./gogitor git status
```

```bash
./gogitor git diff
```

```bash
./gogitor git diff-task
```

```bash
./gogitor git commit
```

## Automatic Commits

After successful code generation, Gogitor can generate a commit message from the actual Git diff.

Messages follow Conventional Commits:

```text
feat(auth): add JWT token validation
fix(runner): handle empty test output
refactor(workspace): extract patch application
test(index): add BM25 ranking coverage
```

The commit generator is instructed to base the message on the actual diff rather than assumptions about the task.

## GitHub API

Gogitor can interact with GitHub using a token.

Create a repository:

```bash
./gogitor git create myproject \
  --private \
  --desc "My project"
```

Clone a repository:

```bash
./gogitor git clone https://github.com/user/repository \
  --key-github ghp_xxx
```

Push using a one-off GitHub URL and token:

```bash
./gogitor git push \
  --github https://github.com/user/repository \
  --key-github ghp_xxx
```

Supported GitHub token formats include:

```text
ghp_...
github_pat_...
```

---

# Web Search

Gogitor can perform web searches and pass the retrieved information to an LLM for summarization.

```bash
./gogitor search "latest Go release"
```

The search pipeline is designed with additional protections:

* Rate limiting.
* Domain controls.
* SSRF protection.
* Secret detection in search queries.
* Sanitization of retrieved content.
* Prompt-injection protection.
* Explicit treatment of retrieved pages as untrusted content.

Search results can be summarized by the LLM while preserving the distinction between retrieved information and generated explanation.

Automatic search can also be enabled for multi-agent tasks:

```bash
./gogitor code "research current approaches to Go HTTP routing and implement the best option" \
  --auto-search
```

---

# Articles and Technical Documentation

Gogitor can generate technical articles, tutorials, stories, reviews, and other long-form content.

Simple mode:

```bash
./gogitor article "how garbage collection works in Go"
```

Complex multi-section mode:

```bash
./gogitor article "middleware pattern deep dive" --full
```

The article system can automatically distinguish between several genres, including:

```text
technical
news
story
review
howto
code_desc
```

Depending on the task, project context and web search can be incorporated into the generation process.

---

# Project Health Analysis

The `suggest` command asks the LLM to perform a focused project health review:

```bash
./gogitor suggest
```

The analysis is organized around:

* Critical issues.
* Technical debt.
* Missing tests.
* Code smells.
* Improvements.

Suggestions are expected to reference concrete project locations rather than providing generic advice.

---

# TODO / FIXME Scanner

Gogitor can scan Go source files without using an LLM:

```bash
./gogitor todo
```

It looks for markers such as:

```text
TODO
FIXME
HACK
XXX
BUG
```

This provides a quick way to identify unfinished or intentionally temporary areas of the project.

---

# Go Vet

Run static analysis using the Go toolchain:

```bash
./gogitor vet
```

`go vet ./...` is executed without requiring an LLM.

---

# Decision Journal

Multi-agent sessions can record important engineering decisions.

The decision journal stores information about:

* Decisions made.
* Alternatives considered.
* Constraints.
* Sources of decisions.
* Failed approaches.

Inspect the decision history with:

```bash
./gogitor decisions
```

Gogitor can also ask the LLM to identify **decision debt** — temporary decisions whose original constraints may no longer apply.

This helps identify technical compromises that should potentially be revisited later.

---

# Configuration

Configuration is loaded in the following order:

1. Built-in defaults.
2. Global configuration.
3. Environment variables.
4. Local project configuration.
5. Command-line flags.

Global configuration:

```text
~/.gogitor/config.json
```

Logs:

```text
~/.gogitor/logs/gogitor_YYYY-MM-DD.log
```

Project-specific configuration:

```text
.gogitor.json
```

in the project root.

## Example Global Configuration

```json
{
  "provider": "ollama",
  "model": "gemma3:4b",
  "api_key": "",
  "ollama_url": "http://localhost:11434",
  "log_level": "info",
  "debug_mode": false,
  "dry_run": false,
  "llm_timeout": 300,
  "max_iterations": 5,
  "auto_git_commit": true,
  "git_auto_init": true,
  "multi_agent_enabled": true,
  "raw_output": false,
  "max_context_tokens": 0,
  "compare_approaches": true,
  "auto_search": false,
  "output_file": ""
}
```

## Example Project Configuration

Create `.gogitor.json`:

```json
{
  "provider": "ollama",
  "model": "gemma3:4b",
  "auto_git_commit": false,
  "dry_run": false,
  "compare_approaches": true,
  "auto_search": false,
  "raw_output": false
}
```

---

# Environment Variables

| Variable                     | Description                                      |
| ---------------------------- | ------------------------------------------------ |
| `GOGITOR_PROVIDER`           | Default LLM provider                             |
| `GOGITOR_MODEL`              | Default model                                    |
| `GOGITOR_API_KEY`            | LLM API key                                      |
| `OPENAI_API_KEY`             | Fallback API key for OpenAI-compatible providers |
| `GOGITOR_OLLAMA_URL`         | Ollama URL                                       |
| `OLLAMA_HOST`                | Fallback Ollama host                             |
| `GOGITOR_LOG_LEVEL`          | Log level                                        |
| `GOGITOR_DEBUG`              | Enable debug mode                                |
| `GOGITOR_DRY_RUN`            | Enable dry-run mode                              |
| `GOGITOR_RAW`                | Enable raw output                                |
| `GOGITOR_LLM_TIMEOUT`        | LLM timeout in seconds                           |
| `GOGITOR_MAX_ITERATIONS`     | Maximum code-fix iterations                      |
| `GOGITOR_AUTO_GIT_COMMIT`    | Enable automatic Git commits                     |
| `GOGITOR_GIT_AUTO_INIT`      | Enable automatic Git initialization              |
| `GOGITOR_MULTI_AGENT`        | Enable multi-agent execution                     |
| `GOGITOR_COMPARE_APPROACHES` | Enable approach comparison                       |
| `GOGITOR_MAX_CONTEXT_TOKENS` | Maximum LLM context                              |
| `GOGITOR_GITHUB_URL`         | GitHub repository URL                            |
| `GOGITOR_GITHUB_TOKEN`       | GitHub token                                     |
| `GOGITOR_LANG`               | Interface language: `en` or `ru`                 |
| `GOGITOR_AUTO_SEARCH`        | Enable automatic web search                      |
| `GOGITOR_MARKDOWN_STYLE`     | TUI Markdown style                               |

---

# Examples

## Start with Ollama

```bash
./gogitor tui \
  --provider ollama \
  --model gemma3:4b
```

## Use a remote Ollama server

```bash
./gogitor tui \
  --provider http://192.168.1.10:11434 \
  --model gemma3:4b
```

## Use an OpenAI-compatible API

```bash
./gogitor ask "explain generics in Go" \
  --provider openai+https://api.example.com/v1 \
  --model gpt-4o-mini \
  --key sk-...
```

## Generate code

```bash
./gogitor code "create a REST API with health and version endpoints"
```

## Analyze a project

```bash
./gogitor analyze "find potential bugs and suggest improvements"
```

## Dry run

```bash
./gogitor code "refactor main.go" --dry-run
```

## Disable automatic commits

```bash
./gogitor code "split the code into packages" --no-commit
```

## Skip tests

```bash
./gogitor code "add logging" --no-tests
```

## Disable approach comparison

```bash
./gogitor code "create an HTTP server" --no-compare
```

## Execute a task file

```bash
./gogitor task ./tasks/feature.txt
```

## Force code mode for a task file

```bash
./gogitor file ./tasks/refactor.md --code
```

## JSON output

```bash
./gogitor test --json
```

## Raw output

Raw mode is useful for shell pipelines:

```bash
echo "write hello world in Go" | ./gogitor code --raw > main.go
```

```bash
./gogitor ask "explain context.Context" --raw
```

## Save output to a file

```bash
./gogitor ask "explain context.Context" --output answer.md
```

```bash
./gogitor code "create hello world" --output main.go
```

```bash
./gogitor test --output report.json
```

## Large-context model

```bash
./gogitor code "refactor the entire project" \
  --provider ollama \
  --model llama3.3:70b \
  --max-context 262144
```

## Fix a panic

```bash
./gogitor fix \
  "panic: runtime error: index out of range [3] with length 2"
```

## Project health review

```bash
./gogitor suggest
```

## Decision journal

```bash
./gogitor decisions
```

## Workflow

```bash
./gogitor workflow \
  "create a REST API with authentication and tests"
```

## Workflow interview

```bash
./gogitor workflow interview \
  "add caching to the API layer"
```

## Workflow reflection

```bash
./gogitor workflow reflect
```

## Workflow Pull Request

```bash
./gogitor workflow pr \
  --key-github ghp_xxx
```

## Diagnostics

```bash
./gogitor doctor
```

---

# Requirements

Gogitor is intended for Unix-like development environments.

### Required

* **Go 1.25.1** or a compatible Go installation used to build the project.
* **Ollama** or an **OpenAI-compatible API endpoint**.
* Network access when downloading dependencies or using remote services.

### Recommended

* **Git** for version control and safe rollback.
* A sufficiently capable coding model for complex tasks.

### Supported operating environments

* Linux.
* macOS.
* Windows through WSL.

### Optional

`golangci-lint` is required for linting:

```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

---

# Installation

Clone the repository:

```bash
git clone https://github.com/SkalaSkalolaz/gogitor.git
cd gogitor
```

Download dependencies:

```bash
go mod tidy
```

Build Gogitor:

```bash
go build -o gogitor .
```

Verify the installation:

```bash
./gogitor --help
```

```bash
./gogitor version
```

The current source defines version:

```text
1.8.1
```

Optionally install the binary system-wide:

```bash
sudo mv gogitor /usr/local/bin/
```

---

# Diagnostics

If Gogitor behaves unexpectedly, run:

```bash
./gogitor doctor
```

The diagnostic command reports information such as:

* Active provider.
* Active model.
* Effective context window.
* Working directory.
* Configuration locations.
* Log location.
* Timeouts.
* Enabled features.

Debug logging can be enabled with:

```bash
./gogitor --debug
```

Logs are stored under:

```text
~/.gogitor/logs/
```

---

# Project Structure

The main source tree is organized around distinct responsibilities:

```text
.
├── main.go
│
├── internal/
│   ├── app/
│   │   Application orchestration
│   │
│   ├── agent/
│   │   LLM dispatcher, queues, budgets and retries
│   │
│   ├── codegen/
│   │   Parsing and application of LLM-generated files and patches
│   │
│   ├── config/
│   │   Configuration loading and validation
│   │
│   ├── domain/
│   │   Shared application/domain types
│   │
│   ├── git/
│   │   Git operations
│   │
│   ├── github/
│   │   GitHub API integration
│   │
│   ├── i18n/
│   │   English/Russian localization
│   │
│   ├── index/
│   │   AST-based project indexing and relevance ranking
│   │
│   ├── llm/
│   │   LLM clients and provider integration
│   │
│   ├── prompts/
│   │   Prompt builders and execution strategies
│   │
│   ├── runner/
│   │   Go build/test/run/vet/lint execution
│   │
│   ├── search/
│   │   Web search and result processing
│   │
│   ├── security/
│   │   Path and security checks
│   │
│   ├── workspace/
│   │   Project files and temporary sandbox handling
│   │
│   └── ui/
│       ├── cli/
│       │   CLI interface
│       │
│       └── tui/
│           Bubble Tea terminal interface
│
├── LICENSE.txt
├── README.md
└── README_RU.md
```

---

# Safety

Gogitor is an engineering tool that can execute generated code and modify files on your computer.

LLM-generated code should therefore be treated as untrusted until validated and reviewed.

## Recommended Practices

* Use Git.
* Keep important projects under version control.
* Use `--dry-run` when evaluating an unfamiliar task.
* Review generated changes before committing.
* Use trusted LLM endpoints.
* Avoid sending secrets to external models.
* Review task files before executing them.
* Do not assume that a successful compilation means that the implementation is correct.

## Sandbox

Code generation and validation are performed in a temporary workspace before validated changes are applied to the real project.

The sandbox is intended to protect the working tree from immediately receiving broken generated code.

It is **not a complete security container or virtual machine**.

Generated programs may still perform operations permitted by the operating system and user account.

## Path Protection

Gogitor validates file paths before applying generated changes and prevents path traversal attempts from writing outside the project root.

---

# Troubleshooting

## `unsupported provider`

Use one of the supported formats:

```bash
--provider ollama
```

```bash
--provider http://localhost:11434
```

```bash
--provider openai+https://api.example.com/v1
```

```bash
--provider openai-compatible+http://localhost:8000/v1
```

---

## Ollama Is Not Reachable

Make sure Ollama is running:

```bash
ollama serve
```

Then try:

```bash
./gogitor tui \
  --provider http://127.0.0.1:11434 \
  --model gemma3:4b
```

---

## Build Fails

First verify the project independently:

```bash
go build ./...
```

Then run Gogitor again.

If the failure was introduced by generated code, use:

```bash
./gogitor fix "paste the build error here"
```

---

## Tests Fail

Run tests with JSON output for easier inspection:

```bash
./gogitor test --json
```

For code generation you can temporarily skip tests:

```bash
./gogitor code "task" --no-tests
```

Skipping tests should generally be treated as a temporary development option rather than a replacement for validation.

---

## `golangci-lint` Is Not Installed

Install it with:

```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

Then:

```bash
./gogitor test lint
```

---

## Context Is Too Small

For large projects or large refactoring tasks, increase the model context:

```bash
./gogitor code "refactor the entire project" \
  --max-context 262144
```

Or configure it permanently:

```json
{
  "max_context_tokens": 262144
}
```

The actual usable context depends on the selected model and provider.

---

## Inspect Configuration

Use:

```bash
./gogitor doctor
```

For detailed logs:

```bash
./gogitor --debug
```

---

# Development

Build the project:

```bash
go build ./...
```

Run tests:

```bash
go test ./...
```

Run static analysis:

```bash
go vet ./...
```

Format the source:

```bash
gofmt -w .
```

Run the configured linter:

```bash
golangci-lint run ./...
```

The same categories of checks are also used by Gogitor itself when validating generated code and executing workflow quality gates.

---

# License

Gogitor is distributed under the **BSD 3-Clause License**.

See [LICENSE.txt](LICENSE.txt) for the complete license text.

---

## Contributing

Issues, bug reports, feature proposals, and pull requests are welcome.

When reporting a problem, include where possible:

* Gogitor version.
* Go version.
* Operating system.
* LLM provider.
* Model name.
* Command that was executed.
* Relevant error output.
* Whether the problem occurs in TUI or CLI mode.

For code changes, please ensure that the project builds and tests pass before submitting a Pull Request.
