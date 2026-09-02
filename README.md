# Gogitor — AI Coding Assistant for Go

[![License](https://img.shields.io/badge/License-BSD--3--Clause-blue.svg)](LICENSE.txt)
[![Go](https://img.shields.io/badge/Go-1.25.1-00ADD8.svg)](https://go.dev/)
[![Version](https://img.shields.io/badge/version-1.0-blue.svg)](https://github.com/SkalaSkalolaz/gogitor)
[![Platform](https://img.shields.io/badge/platform-Linux%20%7C%20macOS%20%7C%20WSL-lightgrey.svg)](#requirements)

[Русская версия](README_RU.md)

**Gogitor** is an AI-powered terminal assistant for Go developers. It combines a classic CLI, an interactive terminal UI, project-aware code generation, automated validation, multi-agent engineering, workflow orchestration, Git/GitHub integration, web search, project indexing, reasoning mode, image analysis, autonomous engineering tools, and computer automation.

Gogitor is designed primarily for Go projects and works with both local and remote LLMs through **Ollama** and **OpenAI-compatible APIs**.

The application can classify a natural-language request, inspect the existing project, select relevant source files, generate either complete files or minimal patches, validate changes in a temporary workspace, run tests and quality checks, and apply validated changes to the real project.

> **Current source version:** `1.0`
>
> Gogitor is primarily intended for Go development. The interface is available in English and Russian.

---

## Table of Contents

* [What Gogitor Does](#what-gogitor-does)
* [Architecture and Execution Flow](#architecture-and-execution-flow)
* [Features](#features)

  * [Interactive TUI](#interactive-tui)
  * [Code Generation](#code-generation)
  * [Patch Engine](#patch-engine)
  * [Validation and Error Fixing](#validation-and-error-fixing)
  * [Automatic Execution Strategy](#automatic-execution-strategy)
  * [Multi-Agent Engineering](#multi-agent-engineering)
  * [Workflow Mode](#workflow-mode)
  * [Project Intelligence](#project-intelligence)
  * [Approach Comparison](#approach-comparison)
  * [Git and GitHub](#git-and-github)
  * [Web Search](#web-search)
  * [Articles and Documentation](#articles-and-documentation)
  * [Project Health Analysis](#project-health-analysis)
  * [Reasoning Mode](#reasoning-mode)
  * [Image Analysis](#image-analysis)
  * [Computer Mode](#computer-mode)
  * [Autonomy](#autonomy)
  * [Mutation Testing](#mutation-testing)
  * [Automatic Test Generation](#automatic-test-generation)
  * [TODO/FIXME Scanner](#todofixme-scanner)
  * [Go Vet](#go-vet)
  * [Decision Journal](#decision-journal)
* [Requirements](#requirements)
* [Installation](#installation)
* [Quick Start](#quick-start)
* [LLM Providers](#llm-providers)
* [CLI Commands](#cli-commands)
* [CLI Flags](#cli-flags)
* [TUI Commands](#tui-commands)
* [Execution Modes](#execution-modes)
* [Code Generation Workflow](#code-generation-workflow)
* [Workflow Mode in Detail](#workflow-mode-in-detail)
* [Project Indexing](#project-indexing)
* [Git and GitHub Integration](#git-and-github-integration)
* [Configuration](#configuration)
* [Environment Variables](#environment-variables)
* [Examples](#examples)
* [Diagnostics](#diagnostics)
* [Project Structure](#project-structure)
* [Safety](#safety)
* [Troubleshooting](#troubleshooting)
* [Development](#development)
* [License](#license)
* [Contributing](#contributing)

---

## What Gogitor Does

Gogitor is an engineering layer between a developer and an LLM.

Instead of simply sending a prompt to a model and copying the generated code into a project, Gogitor organizes the operation into a controlled development process:

```text
User request
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
     ├── Article
     ├── Computer
     └── Image analysis
            │
            ▼
     Project context
            │
            ▼
       AST index
            │
            ▼
           LLM
            │
            ▼
    Files / SEARCH-REPLACE patches
            │
            ▼
     Temporary workspace
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

For complex tasks this process can be expanded into multiple specialized agents or a traceable workflow with persistent artifacts.

---

# Architecture and Execution Flow

Gogitor separates the development process into several major layers:

```text
CLI / TUI
   │
   ▼
Application Service
   │
   ├── Intent Router
   ├── Execution Strategy
   ├── Agent Orchestration
   ├── Workflow Engine
   ├── Autonomy Controller
   └── Computer Controller
   │
   ▼
Project Context / AST Index
   │
   ▼
LLM Dispatcher
   │
   ├── Ollama
   └── OpenAI-compatible APIs
   │
   ▼
Code Generation / Patch Engine
   │
   ▼
Workspace / Sandbox
   │
   ▼
Runner
   ├── build
   ├── test
   ├── vet
   ├── lint
   └── run
   │
   ▼
Git / GitHub
```

The implementation is intentionally biased toward validation rather than blindly trusting generated code.

---

# Features

## Interactive TUI

Gogitor provides an interactive terminal UI based on Bubble Tea.

The TUI includes:

* Markdown rendering
* Conversation history
* Command autocomplete
* Streaming LLM responses
* Input/output focus switching
* Mouse text selection
* Live task progress
* Execution plan display
* Task status indicators
* Diff visualization
* Agent-stage information
* Project and LLM diagnostics
* English and Russian localization
* Image-aware requests for supported vision models

Start the TUI with:

```bash
./gogitor
```

or:

```bash
./gogitor tui
```

---

## Code Generation

Gogitor can create new Go code and modify existing projects using the current project as the source of truth.

Supported operations include:

* Creating new files
* Modifying existing files
* Refactoring
* Splitting code between files
* Extracting functions and components
* Implementing new features
* Fixing compilation errors
* Fixing failed tests
* Executing task files
* Applying minimal patches
* Generating complete files when a patch is unsuitable

Example:

```bash
./gogitor code "create a REST API with health and version endpoints"
```

For existing projects, Gogitor can use the project index to select relevant context instead of blindly sending the complete repository to the model.

---

## Patch Engine

For existing files Gogitor can ask the LLM for a minimal `SEARCH/REPLACE` patch:

```text
--- Patch: internal/server/server.go ---
<<<<<<< SEARCH
exact existing code
=======
new replacement code
>>>>>>> REPLACE
```

The `SEARCH` section should come verbatim from the supplied project source.

The patch engine supports:

* Exact matching
* Symbol anchors
* Model-aware patch policies
* Strict, balanced, and advanced matching strategies
* Confidence-based fuzzy matching where allowed
* Automatic fallback to complete-file generation when a patch cannot be safely applied

### Patch Policies

```text
strict
balanced
advanced
```

The policy is selected according to the provider/model profile when possible.

As a general rule:

* Smaller local models use stricter matching.
* Medium-size models use balanced matching.
* Strong cloud or larger models may use advanced matching.

Strict mode does not automatically use fuzzy matching.

---

## Validation and Error Fixing

Generated changes are first applied to a temporary project copy.

Depending on the operation Gogitor can run:

```text
go mod init
go mod tidy
gofmt
go build
go test -v -cover
go vet
golangci-lint
```

The test runner extracts information such as:

* Passed tests
* Failed tests
* Test names
* Related functions
* Source files
* Line numbers
* Failure messages
* Coverage information

That information can be sent back to the LLM for targeted correction.

### Fix Mode

The `fix` command is designed for compiler errors, panics, stack traces, runtime failures, and failed tests:

```bash
./gogitor fix "panic: runtime error: index out of range"
```

The intent router also recognizes typical Go error signatures such as:

```text
panic:
runtime error
goroutine
.go:123
--- FAIL
```

---

## Automatic Execution Strategy

For code-generation tasks Gogitor can automatically select an execution mode.

Available modes:

```text
auto
fast
agent
workflow
```

The strategy takes into account:

* Task complexity
* Local vs remote model
* Model size/profile
* Project context
* Risk
* Explicit user selection

Typical behavior:

```text
Simple task
    ↓
fast

Medium task
    ↓
agent

Large or high-complexity task
    ↓
workflow
```

For local smaller models, complex tasks can be routed directly into workflow mode to reduce context loss.

For remote providers, sufficiently complex tasks may use an LLM-assisted execution-strategy decision.

You can explicitly select a mode:

```bash
./gogitor code "fix the authentication module" --mode fast
```

```bash
./gogitor code "refactor the storage layer" --mode agent
```

```bash
./gogitor code "redesign the project architecture" --mode workflow
```

In the TUI, the equivalent commands are:

```text
:fast <task>
:agent <task>
:workflow <task>
```

---

## Multi-Agent Engineering

Complex tasks can be executed through specialized agents:

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

Builds an implementation plan and decomposes the task into concrete subtasks.

### Coder

Implements the subtasks and works with the project workspace.

### Reviewer

Checks for important problems such as:

* Compilation errors
* Incorrect implementation
* Nil dereferences
* Security problems
* Regressions
* Violations of the original task

### Verifier

Checks whether the original goal was actually achieved and can generate an additional correction task when needed.

The agent subsystem also provides:

* Priority-based request queues
* Per-role budgets
* Retry handling
* Exponential backoff
* Usage statistics
* Execution timing
* Progress and ETA information
* Persistent agent memory
* Checkpoints
* Rollback support

---

## Workflow Mode

Workflow mode is intended for larger engineering tasks where traceability and repeatability matter.

Start a workflow with:

```bash
./gogitor workflow "create a REST API with authentication and tests"
```

A workflow session is stored under:

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

A workflow can:

1. Refine the original goal.
2. Decompose it into concrete subtasks.
3. Execute the subtasks through the coding agent.
4. Run quality gates after each task.
5. Record execution information.
6. Create per-task commits when automatic commits are enabled.
7. Stop when a required quality gate fails.

---

## Project Intelligence

Gogitor maintains an AST-based project index used to select relevant context for LLM requests.

The index can represent:

* Files
* Packages
* Imports
* Functions
* Methods
* Call relationships
* Project structure
* File importance
* Text relevance

The relevance system combines:

* Import graph
* Call graph
* PageRank
* BM25
* English/Russian synonym expansion

The index is cached and refreshed as project files change.

This reduces the need to send an entire repository to the model for every request.

---

## Approach Comparison

For sufficiently complex coding tasks Gogitor can generate several fundamentally different implementation approaches before coding starts.

Typical comparison criteria include:

* Complexity
* Performance
* Readability
* Dependencies
* Testability
* Trade-offs

Example:

```text
## Comparative Analysis of Approaches

| # | Approach        | Complexity | Performance | Readability | Dependencies |
|---|-----------------|------------|-------------|-------------|--------------|
| 1 | stdlib HTTP mux | low        | good        | excellent   | stdlib only  |
| 2 | chi router      | medium     | excellent   | good        | 1 external   |
| 3 | gRPC            | high       | excellent   | adequate    | 3 external   |
```

The user can then:

* Select an approach by number
* Accept the recommendation
* Modify the recommended approach
* Start a different implementation direction

Disable it with:

```bash
./gogitor code "create an HTTP server" --no-compare
```

or:

```bash
export GOGITOR_COMPARE_APPROACHES=false
```

---

## Git and GitHub

Gogitor includes Git integration for common repository operations:

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
revert
reset
push
pull
fetch
clone
remote
create
pr
issue
changelog
pr-comment
```

Examples:

```bash
./gogitor git status
```

```bash
./gogitor git diff
```

```bash
./gogitor git commit
```

Gogitor can generate commit messages from the actual Git diff using Conventional Commits.

Examples:

```text
feat(auth): add JWT token validation
fix(runner): handle empty test output
refactor(workspace): extract patch application
test(index): add BM25 ranking coverage
```

### GitHub API

Gogitor can work with GitHub using a token.

Create a repository:

```bash
./gogitor git create myproject --private --desc "My project"
```

Clone a repository:

```bash
./gogitor git clone https://github.com/user/repository \
  --key-github ghp_xxx
```

Push using a one-off URL and token:

```bash
./gogitor git push \
  --github https://github.com/user/repository \
  --key-github ghp_xxx
```

Supported token formats include:

```text
ghp_...
github_pat_...
```

---

## Web Search

Gogitor can perform web searches and pass retrieved information to an LLM for summarization.

Example:

```bash
./gogitor search "latest Go release"
```

The search subsystem contains additional protections such as:

* Rate limiting
* Domain controls
* SSRF protection
* Secret detection in search queries
* Content sanitization
* Prompt-injection protection
* Explicit treatment of retrieved pages as untrusted content

Automatic search can also be enabled for complex multi-agent coding tasks:

```bash
./gogitor code \
  "research current approaches to Go HTTP routing and implement the best option" \
  --auto-search
```

### Important privacy note

When `--auto-search` is used with a remote LLM provider, project code and search-related information may be sent to external services.

Use a local Ollama endpoint for sensitive projects.

---

## Articles and Documentation

Gogitor can generate:

* Technical articles
* Tutorials
* Reviews
* How-to guides
* Stories
* Code descriptions
* Other long-form text

Simple mode:

```bash
./gogitor article "how garbage collection works in Go"
```

Full multi-section mode:

```bash
./gogitor article "middleware pattern deep dive" --full
```

The article subsystem can automatically classify genres such as:

```text
technical
news
story
review
howto
code_desc
free
```

Depending on the task, article generation can use:

* Project context
* Web search
* An outline
* Multiple section-generation steps
* Context from previous sections

---

## Project Health Analysis

The `suggest` command performs a focused project review:

```bash
./gogitor suggest
```

The analysis is organized around:

* Critical issues
* Technical debt
* Missing tests
* Code smells
* Improvements

Recommendations are designed to reference concrete project locations rather than provide only generic advice.

---

## Reasoning Mode

Gogitor supports reasoning/thinking mode for models that expose such a capability.

Examples include reasoning-capable model families such as DeepSeek-R1, QwQ, Qwen3 and compatible OpenAI-style reasoning models.

CLI flags:

```text
--reasoning
--reasoning-effort <low|medium|high>
--reasoning-budget <tokens>
--reasoning-show
--reasoning-router
```

Example:

```bash
./gogitor code "design a concurrent job scheduler" \
  --reasoning \
  --reasoning-effort high \
  --reasoning-budget 8192
```

In the TUI:

```text
:reasoning
:reasoning on
:reasoning off
:reasoning router on
:reasoning router off
```

Provider behavior:

* Ollama uses the `think` option where supported.
* OpenAI-compatible providers use `reasoning_effort` where supported.
* Unsupported reasoning parameters may be ignored or the request may be retried without them, depending on the provider.

---

## Image Analysis

`ask` and `analyze` can accept an image file:

```bash
./gogitor ask "what is shown in this image?" --image screenshot.png
```

```bash
./gogitor analyze "find the error shown in this screenshot" \
  --image error.png
```

Supported image formats include:

```text
.png
.jpg
.jpeg
.gif
.webp
.bmp
```

The image is passed to a vision-capable LLM.

Image analysis is useful for:

* Screenshots
* UI problems
* Error messages
* Architecture diagrams
* Code screenshots
* Technical illustrations

A compatible vision-capable model is required.

---

## Computer Mode

Computer mode allows Gogitor to plan and execute real system administration commands.

Example:

```bash
./gogitor computer "show disk usage"
```

or from the TUI:

```text
:computer show disk usage
```

Computer mode is **disabled by default**.

Enable it with:

```bash
./gogitor computer "list the largest files" --computer
```

or:

```bash
export GOGITOR_COMPUTER_ENABLED=true
```

or in `.gogitor.json`:

```json
{
  "computer_enabled": true
}
```

Safety controls include:

* Forbidden-command blocking
* Confirmation for high-risk commands
* Command-substitution restrictions
* Command auditing
* Optional sudo permission
* Dry-run support
* Post-execution verification

Audit history is stored in:

```text
.gogitor/computer_audit.json
```

Additional flags:

```text
--computer
--dry-run
--allow-sudo
```

`--allow-sudo` should only be enabled when necessary.

---

## Autonomy

Autonomy is an engineering-assistance mechanism that can monitor the project and place fixable problems into a task queue.

TUI:

```text
:autonomy
:autonomy on
:autonomy off
:autonomy status
:autonomy run
:autonomy clear
```

CLI:

```bash
./gogitor autonomy status
```

```bash
./gogitor autonomy on
```

```bash
./gogitor autonomy run
```

```bash
./gogitor autonomy clear
```

The intended model is:

```text
Problem detected
      ↓
Task added to autonomy queue
      ↓
User inspects queue
      ↓
autonomy run
      ↓
Specific corrective task
      ↓
Validation
```

Autonomy is configured conservatively by default. The task runner works on specific queued problems rather than giving the model an unrestricted instruction to "improve the project".

---

## Mutation Testing

Gogitor can perform deterministic mutation testing:

```bash
./gogitor mutate
```

Optionally specify a mutation limit:

```bash
./gogitor mutate 10
```

The mutation subsystem reports:

* Generated mutations
* Killed mutations
* Surviving mutations
* Errors
* Mutation score

Mutation testing is intended to evaluate whether existing tests detect meaningful code changes.

---

## Automatic Test Generation

Gogitor can search the AST for exported functions that do not have tests and generate tests for them:

```bash
./gogitor autogen-tests
```

Limit the number of functions:

```bash
./gogitor autogen-tests 3
```

The process is:

```text
AST scan
   ↓
Untested exported functions
   ↓
Generate test
   ↓
Create test file
   ↓
Run tests
   ↓
Keep the file if tests pass
```

This feature is designed to verify generated tests before retaining them.

---

## TODO/FIXME Scanner

Scan the project without using an LLM:

```bash
./gogitor todo
```

The scanner searches for:

```text
TODO
FIXME
HACK
XXX
BUG
```

---

## Go Vet

Run standard Go static analysis:

```bash
./gogitor vet
```

Equivalent operation:

```bash
go vet ./...
```

No LLM is required.

---

# Requirements

Gogitor is intended primarily for Unix-like development environments.

## Required

* **Go 1.25.1** or a compatible Go toolchain.
* **Ollama** or an **OpenAI-compatible API endpoint**.
* Network access when downloading dependencies or using remote services.

## Recommended

* **Git** for source control and rollback.
* A capable coding model for complex engineering tasks.
* A model with sufficiently large context for large projects.

## Supported environments

* Linux
* macOS
* Windows through WSL

## Optional

`golangci-lint` is required for lint operations:

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

Build:

```bash
go build -o gogitor .
```

Verify:

```bash
./gogitor --help
```

```bash
./gogitor version
```

The current source reports:

```text
1.0
```

Optionally install the binary system-wide:

```bash
sudo mv gogitor /usr/local/bin/
```

---

# Quick Start

## 1. Start Ollama

Make sure Ollama is running:

```bash
ollama serve
```

## 2. Start Gogitor

```bash
./gogitor
```

## 3. Select a model

For example:

```bash
./gogitor tui \
  --provider ollama \
  --model gemma3:4b
```

## 4. Generate code

```bash
./gogitor code "create a command-line calculator in Go"
```

## 5. Analyze a project

```bash
./gogitor analyze "find potential bugs and suggest improvements"
```

## 6. Run tests

```bash
./gogitor test
```

## 7. Check the environment

```bash
./gogitor doctor
```

---

# LLM Providers

Gogitor supports local and remote LLM endpoints.

## Ollama

Local Ollama:

```bash
./gogitor tui \
  --provider ollama \
  --model gemma3:4b
```

Remote Ollama-compatible server:

```bash
./gogitor tui \
  --provider http://192.168.1.10:11434 \
  --model gemma3:4b
```

Supported formats:

```text
ollama
http://host:11434
https://host:11434
```

## OpenAI-Compatible APIs

HTTPS:

```bash
./gogitor ask "explain generics in Go" \
  --provider openai+https://api.example.com/v1 \
  --model gpt-4o-mini \
  --key sk-...
```

Generic OpenAI-compatible endpoint:

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

# CLI Commands

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

gogitor test [flags]
gogitor test lint [flags]
gogitor vet [flags]
gogitor todo [flags]
gogitor suggest [flags]
gogitor decisions [flags]

gogitor article <topic> [--full] [flags]

gogitor workflow <task> [flags]
gogitor workflow interview <task> [flags]
gogitor workflow reflect [flags]
gogitor workflow pr [flags]

gogitor computer <task> [flags]
gogitor autonomy [on|off|status|run|clear] [flags]
gogitor mutate [limit] [flags]
gogitor autogen-tests [count] [flags]

gogitor git <subcommand> [flags]

gogitor doctor [flags]
gogitor help
```

---

# CLI Flags

## Common flags

| Flag                   | Short | Description                                     |
| ---------------------- | :---: | ----------------------------------------------- |
| `--provider <name>`    |  `-p` | LLM provider                                    |
| `--model <model>`      |  `-m` | Model name                                      |
| `--key <key>`          |  `-k` | LLM API key                                     |
| `--repo <path>`        |  `-r` | Project root                                    |
| `--image <path>`       |       | Image for `ask` / `analyze`                     |
| `--github <url>`       |       | GitHub repository URL                           |
| `--key-github <token>` |       | GitHub token                                    |
| `--max-context <n>`    |       | Maximum LLM context in tokens                   |
| `--auto-search`        |       | Enable automatic web search in multi-agent mode |
| `--output <file>`      |  `-o` | Save result to a file                           |
| `--debug`              |       | Enable debug logging                            |
| `--raw`                |       | Output only result content                      |
| `--pretty`             |       | Force human-readable output                     |
| `--help`               |  `-h` | Show help                                       |

## Reasoning flags

| Flag                                     | Description                            |
| ---------------------------------------- | -------------------------------------- |
| `--reasoning`                            | Enable reasoning/thinking mode         |
| `--reasoning-effort <low\|medium\|high>` | Reasoning depth                        |
| `--reasoning-budget <n>`                 | Maximum reasoning tokens               |
| `--reasoning-show`                       | Show reasoning content when supported  |
| `--reasoning-router`                     | Enable reasoning for the intent router |

## Computer flags

| Flag           | Description                                          |
| -------------- | ---------------------------------------------------- |
| `--computer`   | Enable computer mode                                 |
| `--dry-run`    | Show/validate the computer plan without executing it |
| `--allow-sudo` | Allow sudo commands                                  |

## Code flags

| Flag                                   | Description                            |
| -------------------------------------- | -------------------------------------- |
| `--mode <auto\|fast\|agent\|workflow>` | Explicit execution mode                |
| `--dry-run`                            | Validate changes without applying them |
| `--no-commit`                          | Disable automatic Git commit           |
| `--no-tests`                           | Skip tests                             |
| `--no-compare`                         | Disable approach comparison            |
| `--json`                               | JSON output                            |

## Task-file flags

| Flag     | Description     |
| -------- | --------------- |
| `--code` | Force code mode |
| `--json` | JSON output     |

Flags can be placed before or after the task text.

If the task text itself starts with `-`, use `--`:

```bash
./gogitor code -- --fix something
```

---

# TUI Commands

| Command                      | Description                                   |
| ---------------------------- | --------------------------------------------- |
| `:help`                      | Show help                                     |
| `:clear`                     | Clear in-memory conversation context          |
| `:cls`                       | Clear the screen                              |
| `:code <task>`               | Create or modify code                         |
| `:fast <task>`               | Force fast execution mode                     |
| `:agent <task>`              | Force agent mode                              |
| `:fix <error>`               | Fix an error                                  |
| `:ask <question>`            | Chat                                          |
| `:analyze <task>`            | Analyze without modifying files               |
| `:search <query>`            | Web search                                    |
| `:run [file]`                | Run the project                               |
| `:test`                      | Run tests                                     |
| `:test lint`                 | Run linting and process fixes                 |
| `:vet`                       | Run `go vet`                                  |
| `:todo`                      | Scan TODO/FIXME/HACK markers                  |
| `:git <subcommand>`          | Git operation                                 |
| `:save <file>`               | Save the last result                          |
| `:article <topic>`           | Generate an article                           |
| `:suggest`                   | Review project health                         |
| `:decisions`                 | Show decision journal                         |
| `:reasoning`                 | Show reasoning state                          |
| `:reasoning on`              | Enable reasoning                              |
| `:reasoning off`             | Disable reasoning                             |
| `:reasoning router on`       | Enable router reasoning                       |
| `:reasoning router off`      | Disable router reasoning                      |
| `:computer <task>`           | Execute a system task                         |
| `:autonomy`                  | Show autonomy status                          |
| `:autonomy on`               | Start the autonomy monitor                    |
| `:autonomy off`              | Stop the monitor                              |
| `:autonomy run`              | Execute queued autonomy tasks                 |
| `:autonomy clear`            | Clear the autonomy queue                      |
| `:mutate [limit]`            | Run mutation testing                          |
| `:autogen-tests [n]`         | Generate tests                                |
| `:workflow <task>`           | Start a workflow                              |
| `:workflow interview <task>` | Interview before workflow                     |
| `:workflow reflect`          | Analyze the latest workflow                   |
| `:workflow pr`               | Create a Pull Request from workflow artifacts |
| `:quit` / `:q`               | Exit                                          |

## Keyboard shortcuts

| Key             | Action                             |
| --------------- | ---------------------------------- |
| `Enter`         | Send input                         |
| `Alt+Enter`     | Insert a new line                  |
| `Tab`           | Switch focus / accept autocomplete |
| `Esc`           | Return to input                    |
| `PgUp` / `PgDn` | Scroll history/output              |
| `F2`            | Toggle mouse text selection        |
| `Ctrl+C`        | Cancel current operation or quit   |

---

# Execution Modes

## `auto`

Default mode.

Gogitor chooses the execution strategy using task complexity, model/provider characteristics, and configuration.

## `fast`

Designed for simple, local, low-risk tasks.

```bash
./gogitor code "rename this function" --mode fast
```

## `agent`

Designed for multi-step changes that benefit from planning, review, and verification.

```bash
./gogitor code "refactor the authentication module" --mode agent
```

## `workflow`

Designed for larger or architectural tasks where traceability matters.

```bash
./gogitor code "redesign the storage layer" --mode workflow
```

---

# Code Generation Workflow

For normal code generation, Gogitor follows a validation-oriented process:

### 1. Understand the task

The request is classified into an appropriate operation.

### 2. Build project context

The project index is used to select relevant source files.

### 3. Generate changes

The LLM can return either full files:

```text
--- File: main.go ---
package main
...
```

or minimal patches:

```text
--- Patch: internal/server/server.go ---
<<<<<<< SEARCH
old code
=======
new code
>>>>>>> REPLACE
```

### 4. Create a temporary workspace

The current project is copied to a temporary working directory.

### 5. Apply changes

Generated files and patches are applied to the temporary workspace.

### 6. Validate

Depending on the task, Gogitor can execute:

```text
go mod init
go mod tidy
gofmt
go build
go test -v -cover
go vet
golangci-lint
```

### 7. Apply the validated result

After successful validation the changes can be copied back to the real project.

### 8. Git integration

When enabled, Gogitor can create a Git commit using a message generated from the actual diff.

---

# Workflow Mode in Detail

Workflow mode adds persistent artifacts and explicit quality gates.

Typical session:

```text
.gogitor/workflow/<timestamp>/
├── inbox.md
├── research.md
├── plan.md
├── prd.json
├── process.md
├── gate-report-task-01.json
├── gate-report-task-02.json
└── reflection.md
```

## Workflow interview

Use:

```bash
./gogitor workflow interview "add caching to the API layer"
```

The planner generates several task-specific questions.

Possible answers:

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

to accept proposed defaults.

## Workflow reflection

After a workflow:

```bash
./gogitor workflow reflect
```

The reflection can include:

* What went well
* What could be improved
* Metrics
* Problems
* Recommendations
* Final assessment

## Workflow Pull Request

Create a PR from the latest workflow:

```bash
./gogitor workflow pr
```

GitHub integration requires:

* A Git repository
* A configured `origin`
* A GitHub token

The workflow PR can include:

* Original goal
* Implementation plan
* Task statuses
* Commit information
* Quality-gate results
* Execution log
* Workflow reflection

---

# Project Indexing

The project index is designed to improve context selection.

It analyzes:

```text
Files
 │
 ├── Packages
 ├── Imports
 ├── Functions
 ├── Methods
 └── Calls
```

## Import Graph

Tracks relationships between packages and source files.

## Call Graph

Tracks resolvable relationships between functions and methods.

## PageRank

Helps identify structurally important files.

## BM25

Ranks files by text relevance to the current task.

## Multilingual retrieval

English/Russian synonym expansion improves retrieval for multilingual projects and queries.

The index is cached in the user's operating-system cache directory and refreshed when source files change.

---

# Git and GitHub Integration

Common Git operations:

```bash
./gogitor git status
./gogitor git diff
./gogitor git diff-task
./gogitor git commit
./gogitor git init
./gogitor git log
./gogitor git branch
./gogitor git checkout
./gogitor git merge
./gogitor git push
./gogitor git pull
./gogitor git fetch
./gogitor git clone <url>
./gogitor git remote
```

GitHub-specific operations include repository creation and Pull Request/issue related actions supported by the current CLI.

Never put credentials into source-controlled files.

---

# Configuration

Configuration is loaded in this order:

1. Built-in defaults
2. Global configuration
3. Environment variables
4. Project configuration
5. Command-line flags

## Global configuration

```text
~/.gogitor/config.json
```

## Project configuration

```text
.gogitor.json
```

This file is read from the project root.

## Logs

```text
~/.gogitor/logs/
```

## Example project configuration

```json
{
  "provider": "ollama",
  "model": "gemma3:4b",
  "auto_git_commit": false,
  "dry_run": false,
  "compare_approaches": true,
  "auto_search": false,
  "raw_output": false,
  "reasoning_enabled": false,
  "reasoning_effort": "medium",
  "reasoning_budget": 0,
  "reasoning_show": false,
  "reasoning_router": false,
  "workflow_mode": "auto",
  "workflow_model_profile": "auto",
  "workflow_local_complex_threshold": 6,
  "workflow_ask_user": true,
  "deps_mode": "auto",
  "confirm_apply": false,
  "fuzzy_min_confidence": 0,
  "computer_enabled": false,
  "computer_allow_sudo": false,
  "computer_confirm_high": true,
  "computer_command_timeout": 120,
  "autonomy_enabled": false,
  "autonomy_mode": "suggest",
  "autonomy_interval_sec": 60,
  "autonomy_mutation_limit": 20
}
```

Not every field needs to be specified. Unspecified values use built-in defaults.

## Context size

`--max-context` controls the maximum context Gogitor requests from the model.

Example:

```bash
./gogitor code "refactor the entire project" \
  --max-context 262144
```

A value of `0` means automatic/default behavior.

The effective default context in the current source is `131072` tokens, but the actual usable context is also constrained by the selected model/provider.

---

# Environment Variables

| Variable                          | Description                                  |
| --------------------------------- | -------------------------------------------- |
| `GOGITOR_PROVIDER`                | Default LLM provider                         |
| `GOGITOR_MODEL`                   | Default model                                |
| `GOGITOR_API_KEY`                 | LLM API key                                  |
| `OPENAI_API_KEY`                  | Fallback key for OpenAI-compatible providers |
| `GOGITOR_OLLAMA_URL`              | Ollama URL                                   |
| `GOGITOR_LOG_LEVEL`               | Log level                                    |
| `GOGITOR_DEBUG`                   | Enable debug mode                            |
| `GOGITOR_DRY_RUN`                 | Enable dry-run mode                          |
| `GOGITOR_RAW`                     | Enable raw output                            |
| `GOGITOR_LLM_TIMEOUT`             | LLM timeout                                  |
| `GOGITOR_MAX_ITERATIONS`          | Maximum correction iterations                |
| `GOGITOR_AUTO_GIT_COMMIT`         | Enable automatic commits                     |
| `GOGITOR_GIT_AUTO_INIT`           | Enable automatic Git initialization          |
| `GOGITOR_MULTI_AGENT`             | Enable multi-agent execution                 |
| `GOGITOR_COMPARE_APPROACHES`      | Enable approach comparison                   |
| `GOGITOR_MAX_CONTEXT_TOKENS`      | Maximum LLM context                          |
| `GOGITOR_GITHUB_URL`              | GitHub repository URL                        |
| `GOGITOR_GITHUB_TOKEN`            | GitHub token                                 |
| `GITHUB_TOKEN`                    | Fallback GitHub token                        |
| `GOGITOR_AUTO_SEARCH`             | Enable automatic web search                  |
| `GOGITOR_DEPS_MODE`               | Dependency resolution mode                   |
| `GOGITOR_CONFIRM_APPLY`           | Require confirmation before applying changes |
| `GOGITOR_COMPUTER_ENABLED`        | Enable computer mode                         |
| `GOGITOR_COMPUTER_ALLOW_SUDO`     | Allow sudo in computer mode                  |
| `GOGITOR_REASONING`               | Enable reasoning                             |
| `GOGITOR_REASONING_EFFORT`        | `low`, `medium`, or `high`                   |
| `GOGITOR_REASONING_BUDGET`        | Reasoning token budget                       |
| `GOGITOR_REASONING_ROUTER`        | Enable reasoning for intent routing          |
| `GOGITOR_AUTONOMY`                | Enable autonomy                              |
| `GOGITOR_AUTONOMY_MODE`           | Autonomy mode                                |
| `GOGITOR_AUTONOMY_INTERVAL`       | Autonomy interval in seconds                 |
| `GOGITOR_AUTONOMY_MUTATION_LIMIT` | Mutation limit used by autonomy              |

---

# Examples

## Start with Ollama

```bash
./gogitor tui \
  --provider ollama \
  --model gemma3:4b
```

## Remote Ollama

```bash
./gogitor tui \
  --provider http://192.168.1.10:11434 \
  --model gemma3:4b
```

## OpenAI-compatible API

```bash
./gogitor ask "explain generics in Go" \
  --provider openai+https://api.example.com/v1 \
  --model gpt-4o-mini \
  --key sk-...
```

## Generate code

```bash
./gogitor code "create a REST API with /health and /version endpoints"
```

## Force execution mode

```bash
./gogitor code "fix the parser" --mode fast
```

```bash
./gogitor code "refactor the parser package" --mode agent
```

```bash
./gogitor code "redesign the parser architecture" --mode workflow
```

## Analyze a project

```bash
./gogitor analyze "find potential bugs and suggest improvements"
```

## Analyze an image

```bash
./gogitor analyze \
  "identify the error in this screenshot" \
  --image screenshot.png
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

Useful for shell pipelines:

```bash
echo "write hello world in Go" | ./gogitor code --raw > main.go
```

```bash
./gogitor ask "explain context.Context" --raw
```

## Save output

```bash
./gogitor ask "explain context.Context" --output answer.md
```

```bash
./gogitor code "create hello world" --output main.go
```

```bash
./gogitor test --output report.json
```

## Large context

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

## Project health

```bash
./gogitor suggest
```

## TODO scan

```bash
./gogitor todo
```

## Decision journal

```bash
./gogitor decisions
```

## Mutation testing

```bash
./gogitor mutate 10
```

## Automatic test generation

```bash
./gogitor autogen-tests 3
```

## Reasoning

```bash
./gogitor code \
  "design a concurrent worker pool" \
  --reasoning \
  --reasoning-effort high
```

## Computer mode

```bash
./gogitor computer \
  "show disk usage" \
  --computer \
  --dry-run
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

# Diagnostics

When Gogitor behaves unexpectedly, run:

```bash
./gogitor doctor
```

Diagnostics can include:

* Active provider
* Active model
* Effective context size
* Working directory
* Configuration locations
* Log location
* Timeouts
* Enabled features
* Reasoning settings
* Computer/autonomy settings

Enable detailed logs with:

```bash
./gogitor --debug
```

Logs are stored under:

```text
~/.gogitor/logs/
```

---

# Project Structure

The current codebase is organized into separate responsibilities:

```text
.
├── main.go
│
├── internal/
│   ├── app/
│   │   Application orchestration
│   │
│   ├── agent/
│   │   LLM dispatcher, queues, budgets, retries and agent memory
│   │
│   ├── autonomy/
│   │   Autonomous engineering, mutation testing and test generation
│   │
│   ├── codegen/
│   │   Parsing and application of generated files and patches
│   │
│   ├── computer/
│   │   Computer-mode planning, execution and auditing
│   │
│   ├── config/
│   │   Configuration loading and validation
│   │
│   ├── domain/
│   │   Shared domain types
│   │
│   ├── git/
│   │   Git operations
│   │
│   ├── github/
│   │   GitHub API integration
│   │
│   ├── i18n/
│   │   Localization
│   │
│   ├── index/
│   │   AST project indexing and relevance ranking
│   │
│   ├── llm/
│   │   LLM clients and providers
│   │
│   ├── prompts/
│   │   Prompt builders and execution strategies
│   │
│   ├── runner/
│   │   Build, test, run, vet and lint
│   │
│   ├── search/
│   │   Web search and result processing
│   │
│   ├── security/
│   │   Security and path validation
│   │
│   ├── workspace/
│   │   Project files and temporary workspaces
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

Gogitor can:

* Generate code
* Modify files
* Execute generated programs
* Execute tests and build commands
* Access Git repositories
* Search the web
* In computer mode, execute real system commands

LLM-generated code should therefore be treated as **untrusted until validated and reviewed**.

## Recommended practices

* Use Git.
* Keep important projects under version control.
* Use `--dry-run` for unfamiliar operations.
* Review generated changes before committing.
* Use trusted LLM endpoints.
* Avoid sending secrets to external models.
* Review task files before executing them.
* Do not assume that successful compilation proves correctness.

## Sandbox

Normal code generation and validation use a temporary workspace before changes are applied to the real project.

This protects the working tree from immediately receiving broken generated code.

However, the sandbox is **not a complete security container, VM, or isolation boundary**.

Generated programs may still perform operations allowed by the operating system and current user account.

## Path protection

File paths are validated before generated changes are applied in order to prevent path traversal outside the project root.

## Computer mode

Computer mode is substantially more powerful because it can execute real operating-system commands.

Use it only when explicitly required and review the generated plan before execution.

---

# Troubleshooting

## `unsupported provider`

Use a supported provider format:

```bash
--provider ollama
```

or:

```bash
--provider http://localhost:11434
```

or:

```bash
--provider openai+https://api.example.com/v1
```

or:

```bash
--provider openai-compatible+http://localhost:8000/v1
```

## Ollama is not reachable

Start Ollama:

```bash
ollama serve
```

Then test:

```bash
./gogitor tui \
  --provider http://127.0.0.1:11434 \
  --model gemma3:4b
```

## Build fails

Verify the project independently:

```bash
go build ./...
```

Then run Gogitor again.

For an error introduced by generated code:

```bash
./gogitor fix "paste the build error here"
```

## Tests fail

Run:

```bash
./gogitor test --json
```

Temporarily skipping tests is possible:

```bash
./gogitor code "task" --no-tests
```

Skipping tests should be treated as a temporary development option, not as a replacement for validation.

## `golangci-lint` is not installed

Install it:

```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

Then:

```bash
./gogitor test lint
```

## Context is too small

Increase it:

```bash
./gogitor code "refactor the entire project" \
  --max-context 262144
```

or configure:

```json
{
  "max_context_tokens": 262144
}
```

The actual usable context depends on the selected model and provider.

## Inspect current configuration

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

Build:

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

Format source:

```bash
gofmt -w .
```

Run lint:

```bash
golangci-lint run ./...
```

For changes that affect generated-code quality, it is recommended to run the complete validation set:

```bash
go build ./...
go test ./...
go vet ./...
golangci-lint run ./...
```

---

# License

Gogitor is distributed under the **BSD 3-Clause License**.

See [LICENSE.txt](LICENSE.txt) for the complete license text.

---

# Contributing

Issues, bug reports, feature proposals, and Pull Requests are welcome.

When reporting a problem, include where possible:

* Gogitor version
* Go version
* Operating system
* LLM provider
* Model name
* Executed command
* Relevant error output
* Whether the problem occurred in TUI or CLI mode

Before submitting a Pull Request, verify:

```bash
go build ./...
go test ./...
go vet ./...
golangci-lint run ./...
```
