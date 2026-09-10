# Gogitor 2.2

Terminal AI coding assistant for Go projects. Gogitor is now a **TUI-only application**: there is no separate CLI mode or CLI package.

[Русская версия](README_RU.md)

## What changed in 2.2

- one TUI entry point, without the old CLI branch;
- one catalog of TUI commands reused by completion;
- fixed `:task-diff` command dispatch;
- `:agent enhanced <task>` is the preferred name for the stronger agent profile; `deep` remains accepted for compatibility;
- `:quit`, `:exit`, and `:q` behave identically;
- `:cls` is an alias for `:clear`;
- startup TODO/FIXME/HACK/BUG diagnostics are delivered through the Bubble Tea message loop instead of writing to the model from a background goroutine;
- `gpt-oss:20b` remains handled by the adaptive patch policy;
- a language registry was added. Go is the only registered language today, but language detection is no longer coupled to TUI command routing.

The working feature set remains: code generation/modification, DIFF/PATCH safety, AST indexing, agent pipeline, validation, Git/GitHub, web search, reasoning, computer mode, autonomy, mutation testing, test generation, TODO scanning, and logging.

## Startup parameters

Gogitor remains a TUI application, but all major runtime parameters can be supplied at launch. Precedence is **defaults → `~/.gogitor/config.json` → `.gogitor.json` → environment variables → command-line flags**.

Examples:

```bash
./gogitor --provider ollama --model gpt-oss:20b --repo ~/Code/myapp
./gogitor --provider 'openai-compatible+http://localhost:8000/v1' --model my-model --key '...'
```

Main options include `--provider`, `--model`, `--key`/`--api-key`, `--ollama-url`, `--repo`/`--workdir`, `--github`, `--key-github`, `--max-context`, `--llm-timeout`, `--runner-timeout`, `--max-iterations`, `--reasoning*`, `--auto-search`, `--computer*`, `--autonomy*`, `--patch-*`, `--agent-*`, `--auto-commit`, `--git-auto-init`, `--compare`, `--debug`, and `--log-level`. Full list: `./gogitor --help`.

Avoid putting secrets on the command line when possible; prefer `GOGITOR_API_KEY`, `OPENAI_API_KEY`, and `GOGITOR_GITHUB_TOKEN`.

## Zen TUI

Gogitor uses one fixed interface: **Zen**. There is no `--tui`, `--ui`, profile selector, startup menu, or runtime TUI switching.

The Zen layout deliberately keeps the terminal quiet: the output area receives most of the screen, the input editor is directly below it, and a compact status line remains visible. All application capabilities use the same service/event pipeline regardless of presentation.

## Architecture

```text
cmd/gogitor
      │
      ▼
  ui/tui                 presentation layer
      │
      ▼
  app                    application/service layer
      │
      ├── domain         events, results, contracts
      ├── language       language registry
      ├── workspace      DIFF/PATCH and workspace state
      ├── agent          multi-agent dispatcher
      ├── runner         Go build/test/run/lint/vet
      ├── index          AST and file relevance
      ├── git/github     Git and GitHub API
      ├── search         safe web search
      ├── computer       controlled command execution
      └── config/llm/... infrastructure
```

### Design rule

The TUI does not know **how** a task is executed. It submits a command to the application service layer. New functionality can therefore be added without rewriting keyboard handling and screen rendering.

Language support lives under `internal/language`. Go is currently registered as:

```text
Go → .go
```

A future language should be introduced by registering its definition and toolchain instead of spreading language checks through the TUI and command router.

## TUI capabilities

### Code and analysis

```text
:code <task>             automatic strategy selection
:fast <task>             single-pass generation
:agent <task>            adaptive multi-agent execution
:agent enhanced <task>  stronger agent profile
:agent interview <task> clarification helper
:agent reflect           reflection helper
:agent report            latest session report
:agent resume            resume an unfinished session
:agent undo              safely undo the latest agent commit
:fix <error>             repair an error/stack trace
:ask <question>          general chat
:analyze <task>          analysis without changing files
:search <query>          web search
:load <file>             load a .txt/.md task
:article <topic>         article generation
```

### Project, testing, diagnostics

```text
:run [file]
:test
:test lint
:vet
:todo
:suggest
:decisions
:task-diff
:diff-trace [on|off|status]
:reasoning [on|off|router]
```

### Git/GitHub

```text
:git status
:git diff
:git diff-task
:git commit
:git init
:git log
:git checkout ...
:git branch ...
:git merge ...
:git revert ...
:git reset ...
:git push ...
:git pull ...
:git fetch
:git clone ...
:git remote ...
:git create ...
:git pr
:git issue
:git changelog
:git pr-comment ...
```

### Additional modes

```text
:autonomy [on|off|status|run|clear]
:mutate [limit]
:autogen-tests [n]
:computer <task>
```

Computer mode is disabled by default and requires explicit configuration.

## DIFF/PATCH safety

For existing code, Gogitor uses structured changes instead of unconditional full-file regeneration. The existing safety layer remains in place:

- exact SEARCH/REPLACE;
- `REPLACE_ONLY`;
- symbol anchors;
- strict/balanced/advanced matching policies;
- fuzzy matching with threshold and margin;
- patch trace;
- scope diagnostics;
- patch audit for risky cases;
- no-op patch rejection;
- rollback and workspace integrity checks.

This subsystem was deliberately not rewritten without a concrete correctness reason because it is the critical path for modifying existing code safely.

## Agent

The full pipeline is:

```text
Planner → Coder → Reviewer → Verifier
```

The stronger profile enables stricter patch handling and validation gates. Agent sessions are stored under `.gogitor/agent/<timestamp>/`.

## Installation

Requires Go **1.25+**.

```bash
git clone https://github.com/SkalaSkalolaz/gogitor.git
cd gogitor
go mod tidy
go build -o gogitor ./cmd/gogitor/
```

Run:

```bash
./gogitor
```

Checks:

```bash
make fmt
make test
make vet
make build
```

## LLM providers

Local Ollama and OpenAI-compatible APIs are supported.

Main configuration lives in `~/.gogitor/config.json` and can be overridden with environment variables.

Ollama example:

```bash
export GOGITOR_PROVIDER=ollama
export GOGITOR_MODEL=gpt-oss:20b
export GOGITOR_OLLAMA_URL=http://localhost:11434
```

OpenAI-compatible example:

```bash
export GOGITOR_PROVIDER='openai-compatible+http://localhost:8000/v1'
export GOGITOR_MODEL='my-model'
export GOGITOR_API_KEY='...'
```

GitHub tokens can be supplied with `GOGITOR_GITHUB_TOKEN` or `GITHUB_TOKEN`.

## Keyboard shortcuts

| Key | Action |
|---|---|
| `Enter` | submit input |
| `Alt+Enter` | newline in input |
| `Up/Down` | move between input lines |
| `Tab` | switch input/output focus |
| `PgUp/PgDn` | command history |
| `F2` | mouse selection mode |
| `Ctrl+A` | copy all output |
| `Ctrl+C` | cancel task / quit |
| `Ctrl+D` | quit when idle |
| `Esc` | return focus to input |

## Security

Never commit API or GitHub tokens. With remote LLMs and automatic search enabled, project content may leave the local machine.

## License

BSD 3-Clause. See `LICENSE.txt`.
