# Gogitor — AI Coding Assistant for Go

[![License](https://img.shields.io/badge/License-BSD--3--Clause-blue.svg)](LICENSE.txt)
[![Go](https://img.shields.io/badge/Go-1.25.1-00ADD8.svg)](https://go.dev/)
[![Version](https://img.shields.io/badge/version-1.1.3-blue.svg)](https://github.com/SkalaSkalolaz/gogitor)
[![Platform](https://img.shields.io/badge/platform-Linux%20%7C%20macOS%20%7C%20WSL-lightgrey.svg)](#requirements)

[Русская версия](README_RU.md)

**Gogitor** is an AI-assisted terminal engineering tool for Go development.

It combines:

* a command-line interface (CLI);
* an interactive terminal UI (TUI);
* project-aware code generation and modification;
* AST-based project indexing;
* minimal SEARCH/REPLACE patching;
* sandboxed build and test validation;
* automatic execution-strategy selection;
* a multi-agent engineering pipeline;
* Git and GitHub integration;
* web search;
* article and documentation generation;
* project health analysis;
* reasoning support;
* image analysis;
* computer/system-control mode;
* autonomous engineering assistance;
* mutation testing;
* automatic test generation;
* TODO/FIXME scanning;
* `go vet`;
* decision and task history.

Gogitor is primarily designed for Go projects and supports local LLMs through **Ollama** as well as remote models through **OpenAI-compatible APIs**.

> **Current source version:** `1.1.4`
>
> Gogitor is primarily focused on Go development. The application UI is available in English and Russian.

---

## Contents

* [What Gogitor Does](#what-gogitor-does)
* [Architecture and Execution Flow](#architecture-and-execution-flow)
* [Features](#features)

  * [Interactive TUI](#interactive-tui)
  * [Code Generation](#code-generation)
  * [Patch Engine](#patch-engine)
  * [Validation and Error Fixing](#validation-and-error-fixing)
  * [Automatic Execution Strategy](#automatic-execution-strategy)
  * [Multi-Agent Engineering](#multi-agent-engineering)
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
  * [Engineering Decisions](#engineering-decisions)
  * [Task History](#task-history)
* [Requirements](#requirements)
* [Installation](#installation)
* [Quick Start](#quick-start)
* [LLM Providers](#llm-providers)
* [CLI Commands](#cli-commands)
* [CLI Flags](#cli-flags)
* [TUI Commands](#tui-commands)
* [Execution Modes](#execution-modes)
* [Code Generation Pipeline](#code-generation-pipeline)
* [Project Indexing](#project-indexing)
* [Configuration](#configuration)
* [Environment Variables](#environment-variables)
* [Examples](#examples)
* [Diagnostics](#diagnostics)
* [Project Structure](#project-structure)
* [Security](#security)
* [Troubleshooting](#troubleshooting)
* [Development](#development)
* [License](#license)
* [Contributing](#contributing)

---

# What Gogitor Does

Gogitor is intended to act as an engineering layer between the developer and an LLM.

Instead of simply sending a prompt to a model and copying the result back into the project, Gogitor builds a controlled execution pipeline:

```text
User Request
     │
     ▼
Intent Detection
     │
     ├── Chat
     ├── Analysis
     ├── Web Search
     ├── Code Generation
     ├── Fix
     ├── Run
     ├── Test
     ├── Git
     ├── Article
     └── Computer
            │
            ▼
      Project Context
            │
            ▼
        AST Index
            │
            ▼
           LLM
            │
            ▼
    Files / SEARCH-REPLACE
            │
            ▼
    Temporary Workspace
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
       Validated Result
            │
            ├── Apply Changes
            └── Git Commit
```

For more complex tasks Gogitor can switch from a single-pass strategy to the full multi-agent pipeline.

---

# Architecture and Execution Flow

The codebase separates responsibilities into dedicated internal packages.

The main execution flow is roughly:

```text
CLI / TUI
   │
   ▼
Application Service
   │
   ├── Intent Router
   ├── Strategy Selection
   ├── Agent Orchestration
   ├── Project Context
   ├── Workspace
   ├── Runner
   └── Git / GitHub
          │
          ▼
        LLM
```

The important architectural principle is that generated code is treated as a candidate change and is validated before being copied back to the real project.

---

# Features

## Interactive TUI

The TUI is built with Bubble Tea.

It provides:

* Markdown-oriented output;
* conversation history;
* command completion;
* streaming LLM output;
* input/output focus switching;
* mouse selection mode;
* progress information;
* multi-agent plan display;
* agent-stage status;
* diff visualization;
* current-agent information;
* project and LLM diagnostics;
* English and Russian interface;
* image support for vision-capable models.

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

Gogitor can create and modify Go projects.

Supported operations include:

* creating new files;
* modifying existing files;
* refactoring;
* splitting code between files;
* extracting functions and components;
* implementing new features;
* fixing build errors;
* fixing failed tests;
* executing task files;
* applying minimal patches;
* falling back to full-file generation when a safe patch cannot be constructed.

Example:

```bash
./gogitor code "create a REST API with /health and /version endpoints"
```

For existing projects Gogitor prefers minimal changes over arbitrary file rewrites.

---

## Patch Engine

For existing files Gogitor can use minimal `SEARCH/REPLACE` patches:

```text
--- Patch: internal/server/server.go ---
<<<<<<< SEARCH
exact existing source
=======
replacement source
>>>>>>> REPLACE
```

The `SEARCH` block should come from the existing source.

The Patch Engine supports:

* exact matching;
* normalized matching;
* fuzzy matching where policy allows it;
* Symbol anchors;
* strict, balanced and advanced patch policies;
* confidence thresholds;
* AST-based symbol detection;
* fallback to complete-file replacement when a patch is unsafe or cannot be applied.

### Patch policies

```text
strict
balanced
advanced
```

The policies are intended for different model capabilities.

### Strict

Strict mode is the safest policy.

It:

* does not automatically use fuzzy matching;
* limits large SEARCH blocks;
* requires Symbol anchors for larger function/method changes;
* validates the target symbol through the Go AST.

A strict patch with a SEARCH block larger than 10 lines is rejected.

### Balanced

Balanced mode is intended for medium and larger coding models.

The default fuzzy threshold is approximately:

```text
confidence >= 0.82
margin >= 0.08
```

### Advanced

Advanced mode allows more permissive matching.

The default thresholds are approximately:

```text
confidence >= 0.85
margin >= 0.05
```

### Symbol anchors

A patch may specify:

```text
--- Symbol: ParseConfig ---
```

or:

```text
--- Symbol: Handler.ServeHTTP ---
```

The symbol is resolved through the Go AST.

This is especially useful when similar source fragments appear multiple times in the same file.

### Model-aware policy

Gogitor can select a patch policy according to provider/model configuration.

Typical built-in defaults include:

| Model / endpoint                             | Default policy |
| -------------------------------------------- | -------------- |
| `gemma3:4b`                                  | strict         |
| `gemma4:12b`                                 | strict         |
| `ornith-1.5:9b`                              | strict         |
| `gpt-oss:20b`                                | strict         |
| `qwen3.8:27b`                                | balanced       |
| `gemma4:26b`                                 | balanced       |
| `llama3`                                     | balanced       |
| `gemma4:31b-cloud`                           | advanced       |
| `openai-compatible+http://localhost:8000/v1` | advanced       |

These defaults can be overridden through configuration.

---

## Validation and Error Fixing

Generated changes are first tested in a temporary workspace.

Depending on the operation, Gogitor can perform:

```text
go mod init
go mod tidy
gofmt
go build ./...
go test -v -cover ./...
go vet ./...
golangci-lint run ./...
```

The test parser extracts information such as:

* passed tests;
* failed tests;
* test names;
* associated functions;
* files;
* line numbers;
* error messages;
* coverage information.

That information can be fed back into the LLM for targeted corrections.

### Fix mode

The `fix` command is intended for:

* compiler errors;
* panics;
* stack traces;
* runtime errors;
* failed tests.

Example:

```bash
./gogitor fix "panic: runtime error: index out of range"
```

The intent router can also recognize common Go error patterns such as:

```text
panic:
runtime error
goroutine
.go:123
--- FAIL
```

---

## Automatic Execution Strategy

Code tasks support three active execution modes:

```text
auto
fast
agent
```

The old standalone `workflow` execution mode is not part of the current execution engine.

### `auto`

This is the default.

Gogitor selects the strategy using factors such as:

* task complexity;
* local vs remote provider;
* model profile;
* configured model capabilities;
* task size;
* risk;
* explicit user preferences.

A typical decision looks like:

```text
Simple task
    ↓
fast / simple

Moderate task
    ↓
agent

Complex task
    ↓
agent deep
```

For local models, complex tasks can be routed directly to deeper agent execution.

For suitable external providers Gogitor can also use an LLM-assisted strategy selection step.

### `fast`

Fast mode uses a single-pass generation path without the full agent pipeline.

Example:

```bash
./gogitor code "rename this function" --mode fast
```

TUI:

```text
:fast rename this function
```

### `agent`

Agent mode uses the multi-agent pipeline.

Example:

```bash
./gogitor code "refactor the authentication module" --mode agent
```

Equivalent TUI command:

```text
:agent refactor the authentication module
```

### `agent deep`

Deep execution is available through the agent subsystem:

```text
:agent deep <task>
```

It enables stricter patch handling and stronger validation gates.

The CLI can also force deeper agent execution with:

```bash
./gogitor code "redesign the storage layer" --agent --deep
```

### Removed workflow mode

The following is intentionally unsupported:

```bash
./gogitor code "task" --mode workflow
```

or:

```bash
./gogitor workflow "task"
```

Use:

```bash
./gogitor code "task" --mode agent
```

or:

```text
:agent deep task
```

instead.

---

## Multi-Agent Engineering

The full agent pipeline consists of four roles:

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

The planner:

* breaks the task into concrete subtasks;
* defines acceptance criteria;
* keeps each subtask independently verifiable.

### Coder

The coder:

* implements subtasks;
* works with the project workspace;
* generates and applies code changes;
* validates changes as work progresses.

### Reviewer

The reviewer checks:

* build errors;
* incorrect implementation;
* possible nil dereferences;
* security issues;
* regressions;
* deviations from the original task.

### Verifier

The verifier determines whether the original user goal was actually achieved.

The agent subsystem also supports:

* LLM request queues;
* priorities;
* role budgets;
* retries;
* exponential backoff;
* usage statistics;
* execution timing statistics;
* progress and ETA;
* persistent agent memory;
* checkpoints;
* rollback.

### Agent commands

```text
:agent <task>
:agent deep <task>
:agent interview <task>
:agent reflect
:agent report
:agent resume
:agent undo
```

Examples:

```text
:agent refactor authentication into a separate package
```

```text
:agent deep create a REST API with middleware and tests
```

```text
:agent interview add caching to the API layer
```

The latest agent session is stored under:

```text
.gogitor/agent/<timestamp>/
```

Deep sessions can contain planning, state, result and checkpoint artifacts.

`:agent undo` is designed to revert the latest agent commit safely. If that commit is no longer the current `HEAD`, Gogitor refuses the unsafe rollback and recommends a normal Git revert.

---

## Project Intelligence

Gogitor does not blindly send all project files to the LLM.

The project index uses Go AST information and relevance ranking.

The index considers:

* Go files;
* packages;
* imports;
* functions;
* methods;
* call relationships;
* project structure;
* file importance;
* textual relevance.

The ranking layer uses:

* import graph;
* call graph;
* PageRank-like importance;
* BM25-style text relevance;
* Russian/English synonym expansion.

The workspace context builder prioritizes explicitly referenced files and then adds the most relevant indexed files.

The index is cached and refreshed when source files change.

---

## Approach Comparison

For sufficiently complex tasks Gogitor can compare multiple implementation approaches before coding.

The comparison can consider:

* complexity;
* performance;
* readability;
* dependencies;
* testability;
* trade-offs.

The user can select an approach by number or provide a natural-language choice.

Disable this behavior with:

```bash
./gogitor code "create an HTTP server" --no-compare
```

or:

```bash
export GOGITOR_COMPARE_APPROACHES=false
```

---

## Git and GitHub

Gogitor supports:

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

The commit subsystem can generate commit messages from the actual diff using Conventional Commits.

Examples:

```text
feat(auth): add JWT token validation
fix(runner): handle empty test output
refactor(workspace): extract patch application
test(index): add BM25 ranking coverage
```

### GitHub API

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

Push using a token:

```bash
./gogitor git push \
  --github https://github.com/user/repository \
  --key-github ghp_xxx
```

Recognized GitHub token prefixes include:

```text
ghp_...
github_pat_...
```

Do not commit tokens into the repository.

---

## Web Search

Gogitor can perform web searches and pass retrieved content to an LLM.

Example:

```bash
./gogitor search "latest Go release"
```

The search subsystem includes:

* rate limiting;
* domain controls;
* SSRF protection;
* secret detection in search queries;
* content sanitization;
* prompt-injection protection;
* explicit treatment of retrieved content as untrusted.

Automatic search can be enabled for coding tasks:

```bash
./gogitor code \
  "research current approaches to Go HTTP routing and implement the best one" \
  --auto-search
```

### Privacy

When `--auto-search` is used with a remote provider, project code and search-related information can be sent to external services.

For sensitive projects, prefer a local Ollama endpoint.

---

## Articles and Documentation

Gogitor can generate:

* technical articles;
* tutorials;
* how-to guides;
* reviews;
* stories;
* news-oriented text;
* code descriptions;
* other long-form content.

Simple mode:

```bash
./gogitor article "how garbage collection works in Go"
```

Full mode:

```bash
./gogitor article "middleware pattern deep dive" --full
```

Supported genre categories include:

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

* project context;
* web search;
* an outline;
* sequential section generation;
* context from previous sections.

---

## Project Health Analysis

Use:

```bash
./gogitor suggest
```

The analysis is organized around:

* critical issues;
* technical debt;
* missing tests;
* code smells;
* improvements.

Suggestions are intended to reference concrete files and functions rather than remaining purely generic.

---

## Reasoning Mode

Gogitor supports reasoning/thinking features for models and providers that expose them.

CLI flags:

```text
--reasoning
--reasoning-effort <low|medium|high>
--reasoning-budget <n>
--reasoning-show
--reasoning-router
```

Example:

```bash
./gogitor code \
  "design a concurrent worker pool" \
  --reasoning \
  --reasoning-effort high \
  --reasoning-budget 8192
```

TUI commands:

```text
:reasoning
:reasoning on
:reasoning off
:reasoning router on
:reasoning router off
```

Provider behavior depends on the API.

For example:

* Ollama can use its `think` mechanism;
* OpenAI-compatible providers can expose `reasoning_effort`.

If the selected model does not support reasoning, the option may have no effect or the request may be retried without it.

---

## Image Analysis

The `ask` and `analyze` commands support images.

Example:

```bash
./gogitor ask \
  "what is shown in this screenshot?" \
  --image screenshot.png
```

```bash
./gogitor analyze \
  "find the error shown in this screenshot" \
  --image error.png
```

Supported image extensions include:

```text
.png
.jpg
.jpeg
.gif
.webp
.bmp
```

Image input is intended for vision-capable models.

Typical use cases:

* screenshots;
* error dialogs;
* terminal output;
* UI inspection;
* architecture diagrams;
* screenshots containing code;
* technical images.

---

## Computer Mode

Computer mode allows Gogitor to plan and execute real operating-system commands.

It is **disabled by default**.

Example:

```bash
./gogitor computer \
  "show disk usage" \
  --computer \
  --dry-run
```

Enable it with:

```bash
export GOGITOR_COMPUTER_ENABLED=true
```

or:

```json
{
  "computer_enabled": true
}
```

Additional flags:

```text
--computer
--dry-run
--allow-sudo
```

Safety mechanisms include:

* forbidden-command blocking;
* risk classification;
* confirmation for high-risk commands;
* command-substitution restrictions;
* command auditing;
* optional sudo permission;
* dry-run;
* post-execution verification.

Audit history is stored in:

```text
.gogitor/computer_audit.json
```

`--allow-sudo` should only be enabled when necessary.

---

## Autonomy

Autonomy is an engineering-monitoring mechanism that detects fixable problems and places specific corrective tasks into a queue.

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

The intended flow is:

```text
Problem detected
      ↓
Task added to queue
      ↓
User inspects queue
      ↓
autonomy run
      ↓
Specific corrective task
      ↓
Validation
```

Autonomy is conservative by default and does not simply send the model a generic "improve the project" request.

---

## Mutation Testing

Mutation testing is deterministic and does not require an LLM.

Run:

```bash
./gogitor mutate
```

or limit the number of mutations:

```bash
./gogitor mutate 10
```

Supported mutation operators include substitutions such as:

```text
>= → >
<= → <
&& → ||
|| → &&
== → !=
!= → ==
```

The report includes:

* generated mutations;
* killed mutations;
* surviving mutations;
* errors;
* mutation score.

A killed mutation means the test suite detected the mutation.

A surviving mutation means the test suite did not detect that change.

---

## Automatic Test Generation

Gogitor can inspect the Go AST for exported functions that do not have corresponding tests.

Run:

```bash
./gogitor autogen-tests
```

Limit the number of generated tests:

```bash
./gogitor autogen-tests 3
```

The pipeline is:

```text
AST Scan
   ↓
Untested Exported Functions
   ↓
Generate Test
   ↓
Create Test File
   ↓
Run Tests
   ↓
Keep Only Passing Generated Files
```

The generated test file is retained only when validation succeeds.

---

## TODO/FIXME Scanner

Scan the project without an LLM:

```bash
./gogitor todo
```

The scanner recognizes:

```text
TODO
FIXME
HACK
XXX
BUG
```

The TUI also performs a lightweight TODO scan at startup and points the user to `:todo` when markers are detected.

---

## Go Vet

Run:

```bash
./gogitor vet
```

Equivalent Go operation:

```bash
go vet ./...
```

`vet` does not require an LLM.

It is executed in the temporary validation workspace.

---

## Engineering Decisions

Gogitor can maintain a journal of engineering decisions made during agent-assisted work.

The journal can record:

* selected approaches;
* rejected alternatives;
* constraints;
* reasons for decisions;
* failed approaches.

View the journal with:

```bash
./gogitor decisions
```

The TUI aliases are:

```text
:decisions
:journal
```

The system can also identify "decision debt": temporary decisions whose original constraints may no longer apply.

---

## Task History

Task execution history is stored in:

```text
.gogitor/task_history.json
```

The history stores recent task metadata such as:

* status;
* task ID;
* timestamp;
* query;
* execution mode;
* affected-file count;
* added/removed lines;
* commit hash.

Up to 100 entries are retained.

Use:

```text
:history
```

The TUI shows up to the most recent 20 entries.

For the cumulative diff of the latest completed task:

```text
:task-diff
```

`task-diff` can also be used as:

```text
task-diff
```

---

# Requirements

Gogitor is primarily intended for Unix-like development environments.

## Required

* **Go 1.25.1** or a compatible Go toolchain;
* **Ollama** or an **OpenAI-compatible API endpoint**;
* network access when downloading dependencies or using remote services.

## Recommended

* Git;
* a capable coding model for complex tasks;
* a model with sufficient context for the project size.

## Supported environments

* Linux;
* macOS;
* Windows through WSL.

## Optional

For linting:

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

Check:

```bash
./gogitor --help
```

Version:

```bash
./gogitor version
```

Expected current version:

```text
gogitor 1.1.3
```

Install globally if desired:

```bash
sudo mv gogitor /usr/local/bin/
```

---

# Quick Start

## 1. Start Ollama

```bash
ollama serve
```

## 2. Start Gogitor

```bash
./gogitor
```

or:

```bash
./gogitor tui
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
./gogitor code "create a CLI calculator in Go"
```

## 5. Analyze the project

```bash
./gogitor analyze "find potential bugs and suggest improvements"
```

## 6. Run tests

```bash
./gogitor test
```

## 7. Inspect the environment

```bash
./gogitor doctor
```

---

# LLM Providers

Gogitor supports Ollama-compatible endpoints and OpenAI-compatible APIs.

## Ollama

Local Ollama:

```bash
./gogitor tui \
  --provider ollama \
  --model gemma3:4b
```

Ollama-compatible HTTP endpoint:

```bash
./gogitor tui \
  --provider http://192.168.1.10:11434 \
  --model gemma3:4b
```

Supported provider forms include:

```text
ollama
http://host:11434
https://host:11434
```

## OpenAI-compatible APIs

OpenAI-style endpoint:

```bash
./gogitor ask \
  "explain Go generics" \
  --provider openai+https://api.example.com/v1 \
  --model gpt-4o-mini \
  --key sk-...
```

Generic OpenAI-compatible endpoint:

```bash
./gogitor code \
  "create main.go" \
  --provider openai-compatible+http://localhost:8000/v1 \
  --model local-model
```

Provider forms:

| Provider                           | Meaning                       |
| ---------------------------------- | ----------------------------- |
| `ollama`                           | Local Ollama                  |
| `http://host:11434`                | Ollama-compatible HTTP        |
| `https://host:11434`               | Ollama-compatible HTTPS       |
| `openai+https://host/v1`           | OpenAI-style API              |
| `openai-compatible+http://host/v1` | Generic OpenAI-compatible API |

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

gogitor article <topic> [--full] [flags]

gogitor computer <task> [flags]
gogitor autonomy [on|off|status|run|clear] [flags]
gogitor mutate [limit] [flags]
gogitor autogen-tests [count] [flags]

gogitor decisions [flags]
gogitor journal [flags]

gogitor git <subcommand> [flags]

gogitor doctor [flags]
gogitor help
```

There is no active `gogitor workflow` command.

---

# CLI Flags

## Common flags

| Flag                                     | Short | Description                            |
| ---------------------------------------- | :---: | -------------------------------------- |
| `--provider <name>`                      |  `-p` | LLM provider                           |
| `--model <model>`                        |  `-m` | Model name                             |
| `--key <key>`                            |  `-k` | LLM API key                            |
| `--repo <path>`                          |  `-r` | Project root                           |
| `--github <url>`                         |       | GitHub repository URL                  |
| `--key-github <token>`                   |       | GitHub token                           |
| `--image <path>`                         |       | Image for `ask` / `analyze`            |
| `--max-context <n>`                      |       | Maximum context size                   |
| `--output <file>`                        |  `-o` | Save result                            |
| `--debug`                                |       | Detailed logging                       |
| `--raw`                                  |       | Raw result content                     |
| `--pretty`                               |       | Human-readable formatting              |
| `--auto-search`                          |       | Enable automatic web search            |
| `--reasoning`                            |       | Enable reasoning                       |
| `--reasoning-effort <low\|medium\|high>` |       | Reasoning effort                       |
| `--reasoning-budget <n>`                 |       | Reasoning token budget                 |
| `--reasoning-show`                       |       | Show reasoning output when supported   |
| `--reasoning-router`                     |       | Enable reasoning for the intent router |
| `--computer`                             |       | Enable computer mode                   |
| `--help`                                 |  `-h` | Show help                              |

## Code flags

```text
--mode <auto|fast|agent>
--agent
--deep
--dry-run
--no-commit
--no-tests
--no-compare
--json
```

`--agent` forces agent execution.

`--deep` requests deep agent execution.

## Task-file flags

```text
--code
--json
```

`--code` forces code mode instead of automatic intent detection.

Task files must be:

```text
.txt
.md
```

They must contain non-empty text and are size-limited.

## Computer flags

```text
--computer
--dry-run
--allow-sudo
```

---

# TUI Commands

| Command                   | Description                                 |
| ------------------------- | ------------------------------------------- |
| `:help`                   | Show help                                   |
| `:clear`                  | Clear in-memory conversation context        |
| `:cls`                    | Clear visual screen                         |
| `:code <task>`            | Create or modify code                       |
| `:fast <task>`            | Force single-pass mode                      |
| `:agent <task>`           | Run multi-agent pipeline                    |
| `:agent deep <task>`      | Run deep agent pipeline                     |
| `:agent interview <task>` | Ask questions before execution              |
| `:agent reflect`          | Reflect on the latest agent session         |
| `:agent report`           | Show the latest agent report                |
| `:agent resume`           | Resume a failed agent session               |
| `:agent undo`             | Undo the latest agent commit                |
| `:fix <error>`            | Fix an error                                |
| `:ask <question>`         | General chat                                |
| `:analyze <task>`         | Project-aware analysis without file changes |
| `:search <query>`         | Web search                                  |
| `:load <file>`            | Load a `.txt` or `.md` task file            |
| `:run [file]`             | Run the project or Go file                  |
| `:test`                   | Run tests                                   |
| `:test lint`              | Run lint and process fixes                  |
| `:vet`                    | Run `go vet`                                |
| `:todo`                   | Scan TODO/FIXME/HACK/XXX/BUG                |
| `:suggest`                | Analyze project health                      |
| `:article <topic>`        | Generate an article                         |
| `:git <subcommand>`       | Git operation                               |
| `:decisions`              | Show engineering decisions                  |
| `:journal`                | Alias for decisions                         |
| `:history`                | Show task execution history                 |
| `:task-diff`              | Show cumulative diff of the latest task     |
| `:reasoning`              | Show reasoning state                        |
| `:reasoning on`           | Enable reasoning                            |
| `:reasoning off`          | Disable reasoning                           |
| `:reasoning router on`    | Enable reasoning for intent routing         |
| `:reasoning router off`   | Disable reasoning for intent routing        |
| `:computer <task>`        | Execute a system task                       |
| `:autonomy`               | Show autonomy status                        |
| `:autonomy on`            | Enable autonomy                             |
| `:autonomy off`           | Disable autonomy                            |
| `:autonomy status`        | Show autonomy status                        |
| `:autonomy run`           | Execute queued tasks                        |
| `:autonomy clear`         | Clear queued tasks                          |
| `:mutate [limit]`         | Mutation testing                            |
| `:autogen-tests [n]`      | Generate missing tests                      |

### Keyboard shortcuts

| Key         | Action                                      |
| ----------- | ------------------------------------------- |
| `Enter`     | Submit input                                |
| `Alt+Enter` | Insert a new line                           |
| `Up/Down`   | Move between input lines / navigate history |
| `Tab`       | Switch between input and output             |
| `F2`        | Mouse text-selection mode                   |
| `Ctrl+A`    | Copy all output to clipboard                |
| `PgUp/PgDn` | Browse command history                      |
| `Ctrl+C`    | Cancel a running operation or exit          |

---

# Execution Modes

## Auto

Default strategy.

```bash
./gogitor code "implement a feature"
```

Gogitor decides between simple and agent-based execution according to task complexity and model/provider capabilities.

## Fast

Single-pass execution:

```bash
./gogitor code "rename this function" --mode fast
```

## Agent

Full planner/coder/reviewer/verifier pipeline:

```bash
./gogitor code \
  "refactor authentication into separate packages" \
  --mode agent
```

## Deep Agent

Use:

```text
:agent deep <task>
```

or:

```bash
./gogitor code "redesign the storage layer" --agent --deep
```

Deep agent execution uses stricter patching and stronger quality gates.

## Not supported

Do not use:

```text
workflow
```

The previous standalone workflow mode has been removed.

---

# Code Generation Pipeline

A typical code modification follows these stages:

## 1. Intent

The user request is classified as code, fix, analysis, search, run, test, Git, article, computer, or chat.

## 2. Project context

Gogitor selects relevant files using the project index.

## 3. Strategy

Gogitor selects:

```text
simple
```

or:

```text
agent
```

with the possibility of deep agent execution for complex tasks.

## 4. Generation

For existing files the model is asked to produce minimal patches where appropriate.

For new files it can produce complete file contents.

## 5. Patch application

The Patch Engine:

* verifies SEARCH blocks;
* validates Symbol anchors when used;
* evaluates confidence for allowed fuzzy matching;
* rejects unsafe patches;
* optionally falls back to complete-file replacement.

## 6. Temporary validation

The generated changes are copied into a temporary workspace and validated.

## 7. Build/test/lint loop

Depending on the execution path:

```text
gofmt
go build
go test
go vet
golangci-lint
```

may be executed.

## 8. Repair

When validation fails, the error information can be sent back to the model for a targeted fix.

The code-generation path can perform several correction iterations.

## 9. Apply

Only after validation succeeds are the changes copied back into the real project.

## 10. Git

When automatic commits are enabled, Gogitor can create a Git commit from the validated changes.

---

# Project Indexing

The project index is designed to reduce irrelevant LLM context.

It extracts structural information through the Go AST.

Important relationships include:

```text
File
 ├── Package
 ├── Imports
 ├── Functions
 ├── Methods
 └── Call relationships
```

The relevance layer combines structural and textual ranking.

The context selection process prioritizes:

1. explicitly referenced files;
2. highly relevant indexed files;
3. additional Go files needed to fill the context budget.

The index is refreshed when the workspace changes.

---

# Configuration

Configuration is resolved in this order:

1. defaults;
2. global configuration;
3. environment variables;
4. project `.gogitor.json`;
5. CLI flags.

## Global configuration

```text
~/.gogitor/config.json
```

## Project configuration

```text
.gogitor.json
```

in the project root.

## Logs

```text
~/.gogitor/logs/
```

## Example

```json
{
  "provider": "ollama",
  "model": "gemma3:4b",
  "auto_git_commit": false,
  "git_auto_init": true,
  "dry_run": false,
  "compare_approaches": true,
  "auto_search": false,
  "raw_output": false,
  "multi_agent": true,
  "max_context_tokens": 0,
  "agent_model_profile": "auto",
  "agent_deep_complexity_threshold": 6,
  "deps_mode": "auto",
  "confirm_apply": false,
  "fuzzy_min_confidence": 0,
  "reasoning_enabled": false,
  "reasoning_effort": "medium",
  "reasoning_budget": 0,
  "reasoning_show": false,
  "reasoning_router": false,
  "computer_enabled": false,
  "computer_allow_sudo": false,
  "computer_confirm_high": true,
  "computer_command_timeout": 120,
  "computer_max_output": 100000,
  "autonomy_enabled": false,
  "autonomy_mode": "suggest",
  "autonomy_interval_sec": 60,
  "autonomy_mutation_limit": 20
}
```

Only the parameters that need to be overridden must be specified.

### Important defaults

The default configuration includes:

```text
provider                 = ollama
model                    = gemma3:4b
ollama_url               = http://localhost:11434
log_level                = info
llm_timeout              = 3000
max_iterations           = 5
auto_git_commit          = true
git_auto_init            = true
multi_agent              = true
max_context_tokens       = 0
compare_approaches       = true
auto_search              = false
agent_model_profile      = auto
agent_deep_complexity_threshold = 6
deps_mode                = auto
confirm_apply            = false
fuzzy_min_confidence     = 0
computer_enabled         = false
computer_allow_sudo      = false
computer_confirm_high    = true
computer_command_timeout = 120
computer_max_output      = 100000
reasoning_enabled        = false
reasoning_effort         = medium
reasoning_budget         = 0
reasoning_show            = false
reasoning_router         = false
autonomy_enabled         = false
autonomy_mode            = suggest
autonomy_interval_sec   = 60
autonomy_mutation_limit  = 20
```

### Context size

Set the context explicitly:

```bash
./gogitor code \
  "refactor the entire project" \
  --max-context 262144
```

or:

```json
{
  "max_context_tokens": 262144
}
```

When the value is `0`, Gogitor uses automatic context sizing.

The practical context limit still depends on the selected provider and model.

---

# Environment Variables

| Variable                          | Purpose                                          |
| --------------------------------- | ------------------------------------------------ |
| `GOGITOR_PROVIDER`                | LLM provider                                     |
| `GOGITOR_MODEL`                   | Model                                            |
| `GOGITOR_API_KEY`                 | LLM API key                                      |
| `OPENAI_API_KEY`                  | Fallback API key for OpenAI-compatible providers |
| `GOGITOR_OLLAMA_URL`              | Ollama URL                                       |
| `GOGITOR_LOG_LEVEL`               | Log level                                        |
| `GOGITOR_DEBUG`                   | Debug logging                                    |
| `GOGITOR_DRY_RUN`                 | Dry-run mode                                     |
| `GOGITOR_RAW`                     | Raw output                                       |
| `GOGITOR_LLM_TIMEOUT`             | LLM timeout                                      |
| `GOGITOR_MAX_ITERATIONS`          | Maximum correction iterations                    |
| `GOGITOR_AUTO_GIT_COMMIT`         | Automatic Git commits                            |
| `GOGITOR_GIT_AUTO_INIT`           | Automatic Git initialization                     |
| `GOGITOR_MULTI_AGENT`             | Enable multi-agent behavior                      |
| `GOGITOR_COMPARE_APPROACHES`      | Enable approach comparison                       |
| `GOGITOR_MAX_CONTEXT_TOKENS`      | Maximum context                                  |
| `GOGITOR_GITHUB_URL`              | GitHub repository URL                            |
| `GOGITOR_GITHUB_TOKEN`            | GitHub token                                     |
| `GITHUB_TOKEN`                    | Fallback GitHub token                            |
| `GOGITOR_AUTO_SEARCH`             | Automatic web search                             |
| `GOGITOR_DEPS_MODE`               | Dependency-resolution mode                       |
| `GOGITOR_CONFIRM_APPLY`           | Apply confirmation setting                       |
| `GOGITOR_COMPUTER_ENABLED`        | Enable computer mode                             |
| `GOGITOR_COMPUTER_ALLOW_SUDO`     | Allow sudo commands                              |
| `GOGITOR_REASONING`               | Enable reasoning                                 |
| `GOGITOR_REASONING_EFFORT`        | `low`, `medium`, `high`                          |
| `GOGITOR_REASONING_BUDGET`        | Reasoning token budget                           |
| `GOGITOR_REASONING_ROUTER`        | Reasoning for intent routing                     |
| `GOGITOR_AUTONOMY`                | Enable autonomy                                  |
| `GOGITOR_AUTONOMY_MODE`           | Autonomy mode                                    |
| `GOGITOR_AUTONOMY_INTERVAL`       | Autonomy polling interval                        |
| `GOGITOR_AUTONOMY_MUTATION_LIMIT` | Mutation limit                                   |

---

# Examples

## Ollama

```bash
./gogitor tui \
  --provider ollama \
  --model gemma3:4b
```

## Remote Ollama-compatible endpoint

```bash
./gogitor tui \
  --provider http://192.168.1.10:11434 \
  --model gemma3:4b
```

## OpenAI-compatible API

```bash
./gogitor ask \
  "explain context.Context" \
  --provider openai+https://api.example.com/v1 \
  --model gpt-4o-mini \
  --key sk-...
```

## Generate code

```bash
./gogitor code \
  "create a REST API with /health and /version endpoints"
```

## Analyze a project

```bash
./gogitor analyze \
  "find potential bugs and architectural improvements"
```

## Analyze an image

```bash
./gogitor analyze \
  "find the error in this screenshot" \
  --image screenshot.png
```

## Fast mode

```bash
./gogitor code \
  "rename ParseConfig to LoadConfig" \
  --mode fast
```

## Agent mode

```bash
./gogitor code \
  "refactor the authentication layer" \
  --mode agent
```

## Deep agent mode

```bash
./gogitor code \
  "redesign the storage architecture and add tests" \
  --agent \
  --deep
```

## Dry run

```bash
./gogitor code \
  "refactor main.go" \
  --dry-run
```

## Disable automatic commit

```bash
./gogitor code \
  "split the code into packages" \
  --no-commit
```

## Skip tests

```bash
./gogitor code \
  "add logging" \
  --no-tests
```

Skipping validation should be considered a temporary development option.

## Disable approach comparison

```bash
./gogitor code \
  "create an HTTP server" \
  --no-compare
```

## Task file

```bash
./gogitor task ./tasks/feature.txt
```

## Force code mode for a task file

```bash
./gogitor file ./tasks/refactor.md --code
```

## JSON

```bash
./gogitor test --json
```

## Raw output

```bash
./gogitor ask \
  "explain context.Context" \
  --raw
```

## Save output

```bash
./gogitor ask \
  "explain context.Context" \
  --output answer.md
```

```bash
./gogitor code \
  "create hello world" \
  --output main.go
```

## Large context

```bash
./gogitor code \
  "refactor the entire project" \
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

## Automatic tests

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

## Autonomy

```bash
./gogitor autonomy status
```

## Git

```bash
./gogitor git status
```

```bash
./gogitor git diff
```

## Diagnostics

```bash
./gogitor doctor
```

---

# Diagnostics

Use:

```bash
./gogitor doctor
```

Diagnostics can report:

* active provider;
* active model;
* effective context size;
* Ollama URL;
* working directory;
* configuration path;
* log path;
* timeout;
* maximum iterations;
* automatic Git settings;
* multi-agent settings;
* auto-search;
* dry-run;
* reasoning configuration;
* computer/autonomy configuration.

For detailed logs:

```bash
./gogitor --debug
```

Logs are stored under:

```text
~/.gogitor/logs/
```

---

# Project Structure

The main responsibilities are separated into packages:

```text
.
├── main.go
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
│   │   Shared application types
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
│   │   AST indexing and relevance ranking
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

# Security

Gogitor can:

* generate code;
* modify files;
* execute generated programs;
* build and test code;
* interact with Git repositories;
* search the web;
* execute real operating-system commands in computer mode.

LLM-generated code must therefore be treated as **untrusted until validated and reviewed**.

## Recommended practices

* keep projects under Git;
* use `--dry-run` for unfamiliar operations;
* review generated changes before committing;
* use trusted LLM endpoints;
* avoid sending secrets to external models;
* review task files before execution;
* do not assume that successful compilation proves correctness.

## Sandbox

Normal code generation and validation use a temporary workspace before changes are applied to the real project.

This reduces the risk of immediately placing broken generated code into the working tree.

However, the sandbox is **not a complete security container, VM, or isolation boundary**.

Generated programs can still perform operations permitted by the operating system and the current user account.

## Path protection

Generated file paths are validated before changes are applied in order to prevent path traversal outside the project root.

## Computer mode

Computer mode has significantly greater power because it can execute actual system commands.

Use it only when necessary and review the generated plan before allowing execution.

---

# Troubleshooting

## `unsupported provider`

Use one of the supported provider formats:

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

Test it with:

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

For an error introduced by generated code:

```bash
./gogitor fix "paste the build error here"
```

## Tests fail

Run:

```bash
./gogitor test --json
```

For temporary development work you can skip tests:

```bash
./gogitor code "task" --no-tests
```

Skipping tests is not a replacement for validation.

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
./gogitor code \
  "refactor the entire project" \
  --max-context 262144
```

or:

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

For changes affecting generated-code quality, a full validation cycle is recommended:

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

Issues, bug reports, feature proposals and Pull Requests are welcome.

When reporting a problem, include where possible:

* the Gogitor version;
* provider and model;
* the command or TUI operation;
* relevant error output;
* `doctor` information;
* reproduction steps;
* the affected project area.

Do not include API keys, GitHub tokens, passwords or other secrets in issues or Pull Requests.
