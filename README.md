# Gogitor 2.1.13

Terminal AI coding assistant for Go projects.

Gogitor is a **TUI-only application** built around a fixed **Zen** terminal interface. Code creation and modification use the **Agent** pipeline, while analysis, testing, Git/GitHub, web research, computer control, autonomy, and diagnostics share the same application/service and event pipeline.

[Русская версия](README_RU.md)

## Overview

Gogitor is designed for working directly inside an existing Go project. It can:

* create and modify Go code;
* analyze a project without changing files;
* apply targeted DIFF/PATCH changes to existing code;
* run build, tests, coverage, `go vet`, and `golangci-lint`;
* automatically recover external Go dependencies;
* perform focused web research when current external information is useful;
* plan, implement, review, and verify larger changes through Agent;
* integrate with Git and GitHub;
* generate tests and perform mutation testing;
* monitor a project autonomously;
* optionally execute controlled system-administration commands;
* keep Agent sessions, decisions, diagnostics, and quality-gate results under `.gogitor/`.

Go is currently the only registered language/toolchain.

## Architecture

```text
cmd/gogitor
      │
      ▼
   ui/tui
      │
      ▼
    app
      │
      ├── domain       events, results, contracts
      ├── language     language registry
      ├── workspace    DIFF/PATCH and workspace state
      ├── agent        Agent dispatcher
      ├── runner       Go build/test/run/lint/vet
      ├── index        AST and file relevance
      ├── git/github   Git and GitHub integration
      ├── search       web search
      ├── computer     controlled command execution
      └── config/llm   configuration and LLM infrastructure
```

The TUI is intentionally separated from execution logic. The interface submits commands to the application service layer and renders events; it does not contain the implementation logic for the operations themselves.

Language support is isolated under `internal/language`. The current registry contains:

```text
Go → .go
```

## Agent-first code execution

For code creation and modification, Gogitor uses Agent as the main execution pipeline.

```text
Planner → Coder → Reviewer → Verifier
```

`:code` is an Agent entry point rather than a separate fast execution path.

### Agent depth

The default is adaptive:

```text
auto → normal or deep
```

The selected depth depends on the task and the configured model profile.

Use:

```text
:code <task>
:agent <task>
```

to let Gogitor choose the depth automatically.

Use:

```text
:agent deep <task>
```

to force the stronger Agent profile.

`enhanced` remains accepted as a compatibility alias:

```text
:agent enhanced <task>
```

### Agent stages

| Stage | Role     | Purpose                                                                    |
| ----- | -------- | -------------------------------------------------------------------------- |
| 1     | Planner  | Breaks the task into subtasks with acceptance criteria                     |
| 2     | Coder    | Implements the required changes                                            |
| 3     | Reviewer | Checks compilation, regressions, and important correctness/security issues |
| 4     | Verifier | Checks whether the original task was actually completed                    |

For complex tasks, the Agent can work through multiple subtasks while keeping a persistent session state.

### Agent session artifacts

Agent sessions are stored in:

```text
.gogitor/agent/<timestamp>/
```

A session may contain:

```text
inbox.md
research.md
plan.md
plan.json
process.md
result.json
state.json
gate-final.json
gate-task-XX.json
```

This makes the execution trace and final verification data available after the run.

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

`interview` generates clarification questions before execution.

`reflect` analyzes the latest Agent session and extracts lessons.

`report` shows the latest Agent result.

`resume` continues the latest resumable Agent session.

`undo` safely reverts the latest completed Agent commit.

## DIFF/PATCH safety

When an existing project is modified, Gogitor prefers structured changes over unconditional full-file regeneration.

The patch subsystem supports:

* exact SEARCH/REPLACE;
* `REPLACE_ONLY`;
* symbol anchors;
* multiple matching policies;
* fuzzy matching with confidence thresholds;
* patch repair and recovery;
* DIFF tracing;
* scope diagnostics;
* patch auditing;
* no-op patch detection;
* sandbox validation;
* rollback and workspace integrity checks.

By default, existing-file modifications use **PATCH** semantics.

An explicit request to completely rewrite a file can switch the task to **FULL** mode.

### Patch protocols

Supported startup modes include:

```text
auto
search_replace
replace_only
```

`REPLACE_ONLY` is the strictest protocol because full-file fallback is not allowed when patch repair is exhausted.

### Patch auditing

Patch auditing can be configured as:

```text
auto
off
always
```

In automatic mode, auditing is used for deeper or otherwise higher-risk patch scenarios.

### DIFF trace

Enable detailed patch diagnostics with:

```text
:diff-trace on
```

Check the state:

```text
:diff-trace status
```

Disable it:

```text
:diff-trace off
```

A trace can show stages such as:

```text
PARSE
SOURCE
SYMBOL
EXACT
RELAXED
NORMALIZED
REBASE
FUZZY
APPLY
PREFLIGHT
SANDBOX_APPLY
BUILD
TARGETED_TESTS
FULL_TESTS
ROOT_APPLY
```

Possible stage states include:

```text
OK
MISS
REJECT
SKIP
RUN
```

## Validation and quality gates

Gogitor integrates the standard Go toolchain into its execution pipeline.

### Build

The project is built with:

```bash
go build ./...
```

### Tests

The standard test runner uses:

```bash
go test -v -cover ./...
```

Gogitor parses:

* passed tests;
* failed tests;
* coverage;
* test locations;
* failure messages.

Targeted package testing is also used by the Agent pipeline when appropriate.

### Vet

Run:

```text
:vet
```

which executes:

```bash
go vet ./...
```

### Lint

Run:

```text
:test lint
```

The runner uses:

```bash
golangci-lint run ./...
```

When a project does not yet have a `.golangci.yml`, Gogitor can create its default lint configuration before running the linter.

### Deep Agent quality gates

The stronger Agent profile uses deterministic quality gates including:

```text
gofmt
go build
go test
go vet
golangci-lint
```

Quality gates are evaluated in a sandbox. Failed checks can cause the current subtask to be rolled back.

The final Agent commit is intentionally deferred until the complete Agent run has passed final verification.

## Go dependency handling

When external imports are detected, Gogitor can automatically attempt:

```bash
go mod tidy
```

The dependency policy is configurable:

```text
auto
ask
never
```

In `auto` mode, dependency resolution is attempted when external imports are present.

If dependency retrieval appears to have fallen back to Git/SSH and fails, Gogitor can retry through:

```text
https://proxy.golang.org
```

Dependency output is summarized before being passed into the relevant diagnostic or recovery workflow.

## Automatic web research

With:

```text
--auto-search
```

Gogitor can perform focused web research for suitable technical tasks.

Research classification includes areas such as:

```text
dependency
library
api
migration
version
toolchain
security
lint
architecture
performance
documentation
os
```

Research queries are generated specifically for the current technical task rather than as unrestricted general searches.

The research layer is also used for dependency-recovery scenarios and can feed technical evidence into Agent or error-repair workflows.

### Remote LLM warning

When automatic search is enabled together with a remote LLM provider, Gogitor warns that project code and search queries may be sent to external services.

Use a local Ollama instance for sensitive projects when keeping project data on the local machine is important.

## TUI

Gogitor has one fixed interface:

```text
Zen
```

There is no TUI selector and no runtime UI switching.

The application is launched as:

```bash
gogitor [flags]
```

Positional arguments after the flags are rejected. Command execution happens inside the TUI.

### Keyboard shortcuts

| Key         | Action                                   |
| ----------- | ---------------------------------------- |
| `Enter`     | Submit input                             |
| `Alt+Enter` | Insert a newline                         |
| `Up/Down`   | Move between input lines                 |
| `Tab`       | Switch input/output focus                |
| `PgUp/PgDn` | Command history                          |
| `F2`        | Toggle mouse selection mode              |
| `Ctrl+A`    | Copy all output                          |
| `Ctrl+C`    | Cancel the current task / quit when idle |
| `Ctrl+D`    | Quit when idle                           |
| `Esc`       | Return focus to input                    |

## TUI commands

### General

```text
:help
:help <topic>
:clear
:cls
:save <file>
:reasoning
:reasoning on
:reasoning off
:diff-trace
:diff-trace on
:diff-trace off
:quit
:exit
:q
```

`:clear` / `:cls` clears the in-memory conversation context.

`:save <file>` saves the latest result. Supported result formats include `.md`, `.txt`, `.go`, and `.json`.

### Code and analysis

```text
:code <task>
:fix <error>
:ask <question>
:analyze <task>
:search <query>
:suggest
:load <file>
```

Examples:

```text
:code add a health endpoint
:fix panic: runtime error: index out of range
:analyze review the authentication package
:search Go 1.25 context cancellation changes
:load ./task.md
```

`:analyze` is analysis-only and does not modify project files.

### Articles

```text
:article <topic>
:article --full <topic>
```

The simple mode generates a shorter article.

The `--full` form generates a more structured multi-section article.

### Execution and testing

```text
:run [file]
:test
:test lint
:test unusual
:vet
:todo
:check
:task-diff
```

`:test unusual` performs additional checks against unexpected input for suitable code.

`:todo` scans project files for markers such as:

```text
TODO
FIXME
HACK
BUG
```

`:task-diff` shows the cumulative Git diff produced by the most recent completed task.

## Git and GitHub

Gogitor integrates with local Git and, when configured, GitHub.

### Git commands

```text
:git status
:git diff
:git diff-task
:git commit
:git commit --split <file1,file2>
:git init
:git log
:git checkout <ref>
:git checkout -b <name>
:git branch
:git branch <name>
:git branch -d <name>
:git merge <branch>
:git revert [hash]
:git reset [--hard] <hash>
:git push [branch]
:git pull [branch]
:git fetch
:git clone <url>
:git remote
:git remote add <name> <url>
:git remote remove <name>
:git create <name>
:git pr
:git issue
:git changelog
:git pr-comment <number> [text]
```

Examples:

```text
:git status
:git diff
:git commit
:git commit --split main.go,internal/app/app.go
:git push
:git pr
```

`:git reset --hard` is destructive because it rewrites the working tree/history state.

`:git pr` requires a configured GitHub token.

`:git issue` can create an issue from failing test information.

`:git changelog` generates `CHANGELOG.md` from Conventional Commit history.

### Automatic commit

Use:

```text
--auto-commit
```

to create a Git commit after a successful Agent implementation.

The Agent pipeline intentionally keeps the final commit until final verification succeeds.

### Automatic Git initialization

Use:

```text
--git-auto-init
```

to allow Gogitor to initialize Git automatically when required by the configured workflow.

## Autonomy

Autonomy provides background project monitoring and an optional task queue.

Enable it with:

```text
--autonomy
```

or:

```text
:autonomy on
```

Commands:

```text
:autonomy on
:autonomy off
:autonomy status
:autonomy run
:autonomy clear
```

The monitor can check:

* `go build`;
* `go vet`;
* TODO/FIXME/HACK markers.

The relevant configuration includes:

```text
autonomy_enabled
autonomy_interval_sec
autonomy_mutation_limit
```

The default autonomy state is disabled.

## Mutation testing

Run:

```text
:mutate
```

or:

```text
:mutate 50
```

Mutation testing is deterministic and does **not** use the LLM.

It generates source mutations using operator substitutions and checks whether existing tests detect them.

Typical results are:

```text
Killed
Survived
Error
```

The mutation score is based on the proportion of killed mutations.

## Automatic test generation

Use:

```text
:autogen-tests [n]
```

to request automatic unit-test generation for suitable untested functions.

The generated tests then participate in the normal Go testing workflow.

## Computer mode

Computer mode is disabled by default.

Enable it explicitly with:

```text
--computer
```

or:

```text
GOGITOR_COMPUTER_ENABLED=true
```

It can also be enabled through `.gogitor.json`.

Use:

```text
:computer <task>
```

Examples:

```text
:computer show disk usage
:computer list the largest files in the current directory
:computer install curl
```

Computer mode executes real operating-system commands.

Gogitor performs command assessment before execution and blocks forbidden command patterns. Commands using shell command substitution such as `$()` or backticks are rejected. `sudo`-style commands require explicit permission via:

```text
--allow-sudo
```

All computer activity is audited in:

```text
.gogitor/computer_audit.json
```

For sensitive environments, review the computer configuration before enabling this feature.

## Reasoning

Reasoning can be controlled at startup:

```text
--reasoning
--reasoning-effort low|medium|high
--reasoning-budget <n>
--reasoning-show
--reasoning-router
```

At runtime:

```text
:reasoning
:reasoning on
:reasoning off
:reasoning router
```

Reasoning controls affect model behavior; they do not create a separate code-execution pipeline.

## Startup configuration

Gogitor supports startup configuration from multiple sources.

Precedence is:

```text
defaults
    ↓
~/.gogitor/config.json
    ↓
.gogitor.json
    ↓
environment variables
    ↓
command-line flags
```

Command-line flags therefore have the highest precedence for the current launch.

### Basic startup examples

Local Ollama:

```bash
./gogitor \
  --provider ollama \
  --model gpt-oss:20b \
  --repo ~/Code/myapp
```

OpenAI-compatible endpoint:

```bash
./gogitor \
  --provider 'openai-compatible+http://localhost:8000/v1' \
  --model my-model
```

Remote OpenAI-compatible endpoint with key:

```bash
./gogitor \
  --provider 'openai+https://api.example.com/v1' \
  --model my-model \
  --key '...'
```

### Provider forms

The launcher accepts providers such as:

```text
ollama
openai+URL
openai-compatible+URL
HTTP(S) URL
```

For example:

```text
--provider ollama
--provider openai+https://api.openai.com/v1
--provider openai-compatible+http://localhost:8000/v1
```

### Environment variables

Commonly used variables include:

```text
GOGITOR_PROVIDER
GOGITOR_MODEL
GOGITOR_OLLAMA_URL
GOGITOR_API_KEY
OPENAI_API_KEY
GOGITOR_GITHUB_TOKEN
GITHUB_TOKEN
GOGITOR_COMPUTER_ENABLED
```

Prefer environment variables or configuration files for secrets rather than putting credentials directly into the shell command line.

## Startup flags

Run:

```bash
./gogitor --help
```

for the complete current launcher contract.

### Core

| Flag               | Purpose                     |
| ------------------ | --------------------------- |
| `-p`, `--provider` | LLM provider/endpoint       |
| `-m`, `--model`    | Model name                  |
| `-k`, `--key`      | LLM/API key                 |
| `--api-key`        | Alias for `--key`           |
| `--ollama-url`     | Ollama base URL             |
| `-r`, `--repo`     | Project/workspace directory |
| `--workdir`        | Alias for `--repo`          |
| `--github`         | GitHub repository URL       |
| `--key-github`     | GitHub token                |

### Execution

| Flag                     | Purpose                                        |
| ------------------------ | ---------------------------------------------- |
| `--max-context`          | Maximum model context; `0` means automatic     |
| `--llm-timeout`          | LLM request timeout                            |
| `--runner-timeout`       | Build/test command timeout                     |
| `--max-iterations`       | Maximum correction iterations                  |
| `--agent-profile`        | `auto`, `small`, `medium`, `large`, `enhanced` |
| `--agent-deep-threshold` | Complexity threshold for deeper execution      |
| `--auto-commit`          | Commit successful Agent changes                |
| `--git-auto-init`        | Initialize Git automatically                   |
| `--compare`              | Compare implementation approaches              |
| `--auto-search`          | Enable automatic web research                  |

### Reasoning

| Flag                 | Purpose                                |
| -------------------- | -------------------------------------- |
| `--reasoning`        | Enable model reasoning                 |
| `--reasoning-effort` | `low`, `medium`, `high`                |
| `--reasoning-budget` | Reasoning token budget                 |
| `--reasoning-show`   | Show model thinking output             |
| `--reasoning-router` | Enable reasoning for the intent router |

### Patch and dependency safety

| Flag                     | Purpose                                   |
| ------------------------ | ----------------------------------------- |
| `--deps-mode`            | `auto`, `ask`, `never`                    |
| `--confirm-apply`        | Ask before applying generated patches     |
| `--fuzzy-min-confidence` | Override fuzzy-patch confidence threshold |
| `--patch-protocol`       | `auto`, `search_replace`, `replace_only`  |
| `--patch-auditor`        | `auto`, `off`, `always`                   |
| `--diff-trace`           | Enable detailed patch diagnostics         |

### Computer and autonomy

| Flag                    | Purpose                                     |
| ----------------------- | ------------------------------------------- |
| `--computer`            | Enable computer control                     |
| `--allow-sudo`          | Allow sudo-like commands in computer mode   |
| `--computer-confirm`    | Configure high-risk computer command policy |
| `--computer-timeout`    | Computer command timeout                    |
| `--computer-max-output` | Maximum captured command output             |
| `--autonomy`            | Enable autonomous monitoring                |
| `--autonomy-mode`       | `suggest` or `auto`                         |
| `--autonomy-interval`   | Monitoring interval                         |
| `--autonomy-limit`      | Maximum autonomous mutations                |

### Miscellaneous

| Flag            | Purpose                                                              |
| --------------- | -------------------------------------------------------------------- |
| `--output`      | Automatically save the last result                                   |
| `--raw`         | Reduce presentation formatting where supported                       |
| `--dry-run`     | Validate/show intended actions without applying them where supported |
| `--debug`       | Enable debug logging                                                 |
| `--log-level`   | `debug`, `info`, `warn`, `error`                                     |
| `--save-config` | Save the effective configuration to `~/.gogitor/config.json`         |
| `--version`     | Print version and exit                                               |
| `-v`            | Alias for `--version`                                                |
| `--help`        | Show launcher help                                                   |

There is no `--tui` or `--ui` flag because Zen is the only supported interface.

## Development

Requirements:

* Go **1.25+**;
* a working Go toolchain;
* an LLM provider configured for AI-assisted commands.

Build:

```bash
go build -o gogitor ./cmd/gogitor/
```

Format:

```bash
gofmt -w \
  internal/app/*.go \
  internal/config/*.go \
  internal/prompts/*.go \
  internal/ui/tui/*.go
```

Test:

```bash
go test ./...
```

Vet:

```bash
go vet ./...
```

Run:

```bash
./gogitor
```

Check the current launcher contract:

```bash
./gogitor --help
./gogitor --version
```

## Project data and local state

Project-local Gogitor state is stored under:

```text
.gogitor/
```

Depending on enabled features, this may include:

```text
agent/
computer_audit.json
logs/
```

Agent session artifacts are stored below:

```text
.gogitor/agent/
```

Do not commit credentials, tokens, or other sensitive runtime data to Git.

## Security and privacy

Gogitor can work with local or remote LLM providers.

With a local Ollama deployment, model requests can stay on the local machine.

With remote LLM providers, project context sent to the model is processed by the configured external service.

When `--auto-search` is enabled, search queries and research-related project information may also leave the local machine.

Treat remote models and web research as external data-processing boundaries.

Store secrets outside source control. Prefer:

```text
GOGITOR_API_KEY
OPENAI_API_KEY
GOGITOR_GITHUB_TOKEN
GITHUB_TOKEN
```

over embedding credentials directly into project files or shell history.

Computer mode requires special care because it can execute real operating-system commands.

## License

BSD 3-Clause License.

See [LICENSE.txt](LICENSE.txt).
