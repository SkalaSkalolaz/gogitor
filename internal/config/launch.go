package config

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// LaunchOptions contains options which affect only the current process start.
// Persistent application settings remain in Config.
type LaunchOptions struct {
	SaveConfig bool
	Version    bool
}

// ParseLaunchArgs parses TUI startup parameters and overlays explicitly supplied
// flags on top of the already loaded Config. Environment variables and config
// files therefore keep their precedence until a command-line flag is supplied.
func ParseLaunchArgs(cfg *Config, args []string, out, errOut io.Writer) (LaunchOptions, error) {
	if cfg == nil {
		return LaunchOptions{}, fmt.Errorf("config is nil")
	}

	fs := flag.NewFlagSet("gogitor", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.Usage = func() {
		PrintLaunchUsage(out)
	}

	provider := fs.String("provider", cfg.Provider, "LLM provider: ollama, openai+URL, openai-compatible+URL, or HTTP(S) URL")
	fs.StringVar(provider, "p", cfg.Provider, "alias for --provider")
	model := fs.String("model", cfg.Model, "LLM model name")
	fs.StringVar(model, "m", cfg.Model, "alias for --model")
	apiKey := fs.String("key", cfg.APIKey, "LLM API key")
	fs.StringVar(apiKey, "k", cfg.APIKey, "alias for --key")
	apiKeyAlias := fs.String("api-key", cfg.APIKey, "alias for --key")
	ollamaURL := fs.String("ollama-url", cfg.OllamaURL, "Ollama base URL")
	workDir := fs.String("repo", cfg.WorkDir, "project/workspace directory")
	fs.StringVar(workDir, "r", cfg.WorkDir, "alias for --repo")
	workDirAlias := fs.String("workdir", cfg.WorkDir, "alias for --repo")
	githubURL := fs.String("github", cfg.GitHubURL, "GitHub repository URL")
	githubToken := fs.String("key-github", cfg.GitHubToken, "GitHub token")

	maxContext := fs.Int("max-context", cfg.MaxContextTokens, "maximum model context tokens; 0=auto")
	llmTimeout := fs.Int("llm-timeout", cfg.LLMTimeout, "LLM request timeout in seconds")
	runnerTimeout := fs.Int("runner-timeout", cfg.RunnerTimeout, "build/test command timeout in seconds")
	maxIterations := fs.Int("max-iterations", cfg.MaxIterations, "maximum correction iterations")

	reasoning := fs.Bool("reasoning", cfg.ReasoningEnabled, "enable model reasoning/thinking")
	reasoningEffort := fs.String("reasoning-effort", cfg.ReasoningEffort, "reasoning effort: low, medium, high")
	reasoningBudget := fs.Int("reasoning-budget", cfg.ReasoningBudget, "reasoning token budget; 0=server default")
	reasoningShow := fs.Bool("reasoning-show", cfg.ReasoningShow, "show model thinking in output")
	reasoningRouter := fs.Bool("reasoning-router", cfg.ReasoningRouter, "enable reasoning for the intent router")

	autoSearch := fs.Bool("auto-search", cfg.AutoSearch, "enable automatic web search for suitable tasks")
	computer := fs.Bool("computer", cfg.ComputerEnabled, "enable computer-control execution")
	allowSudo := fs.Bool("allow-sudo", cfg.ComputerAllowSudo, "allow sudo commands in computer mode")
	computerConfirm := fs.Bool("computer-confirm", cfg.ComputerConfirmHigh, "require confirmation policy for high-risk computer actions")
	computerTimeout := fs.Int("computer-timeout", cfg.ComputerCommandTimeout, "computer command timeout in seconds")
	computerMaxOutput := fs.Int("computer-max-output", cfg.ComputerMaxOutput, "maximum computer command output bytes")

	dryRun := fs.Bool("dry-run", cfg.DryRun, "show intended actions without applying them where supported")
	debug := fs.Bool("debug", cfg.Debug, "enable debug logging")
	raw := fs.Bool("raw", cfg.Raw, "disable presentation formatting where supported")
	compare := fs.Bool("compare", cfg.CompareApproaches, "compare implementation approaches for suitable tasks")
	autoCommit := fs.Bool("auto-commit", cfg.AutoGitCommit, "create a Git commit after successful agent implementation")
	gitAutoInit := fs.Bool("git-auto-init", cfg.GitAutoInit, "initialize Git automatically when needed")

	depsMode := fs.String("deps-mode", cfg.DepsMode, "dependency mode: auto, ask, never")
	confirmApply := fs.Bool("confirm-apply", cfg.ConfirmApply, "ask for confirmation before applying generated patches")
	fuzzy := fs.Float64("fuzzy-min-confidence", cfg.FuzzyMinConfidence, "minimum fuzzy patch confidence override")
	patchProtocol := fs.String("patch-protocol", cfg.PatchProtocolMode, "patch protocol: auto, search_replace, replace_only")
	patchAuditor := fs.String("patch-auditor", cfg.PatchAuditorMode, "patch auditor: auto, off, always")
	diffTrace := fs.Bool("diff-trace", cfg.DiffTrace, "enable detailed DIFF diagnostics")

	agentProfile := fs.String("agent-profile", cfg.AgentModelProfile, "agent model profile: auto, small, medium, large, enhanced")
	agentThreshold := fs.Int("agent-deep-threshold", cfg.AgentDeepComplexityThreshold, "complexity threshold for deeper agent execution")

	autonomy := fs.Bool("autonomy", cfg.AutonomyEnabled, "enable autonomous task monitoring")
	autonomyMode := fs.String("autonomy-mode", cfg.AutonomyMode, "autonomy mode: suggest or auto")
	autonomyInterval := fs.Int("autonomy-interval", cfg.AutonomyIntervalSec, "autonomy monitor interval in seconds")
	autonomyLimit := fs.Int("autonomy-limit", cfg.AutonomyMutationLimit, "maximum autonomous mutations")

	outputFile := fs.String("output", cfg.OutputFile, "automatically save the last result to a file")
	logLevel := fs.String("log-level", cfg.LogLevel, "log level: debug, info, warn, error")
	llamaBin := fs.String("llama-bin", cfg.LlamaBinPath,
		"path to llama-server binary (for --provider llama)")
	llamaHost := fs.String("llama-host", cfg.LlamaHost,
		"llama-server bind host")
	llamaThinkingKey := fs.String("llama-thinking-key", cfg.LlamaThinkingKey,
		"chat template key that controls reasoning in llama.cpp; "+
			"default \"enable_thinking\" for Qwen; ignored for non-llama providers")
	llamaTemp := fs.Float64(
		"llama-temp", cfg.LlamaTemperature,
		"sampling temperature for llama.cpp (default 0.2; 0 = greedy)",
	)
	llamaTopP := fs.Float64(
		"llama-top-p", cfg.LlamaTopP,
		"top-p sampling for llama.cpp (default 0.9)",
	)
	llamaTopK := fs.Int(
		"llama-top-k", cfg.LlamaTopK,
		"top-k sampling for llama.cpp (default 20)",
	)
	llamaRepeatPenalty := fs.Float64(
		"llama-repeat-penalty", cfg.LlamaRepeatPenalty,
		"repeat penalty for llama.cpp (default 1.05)",
	)
	llamaMinP := fs.Float64(
		"llama-min-p", cfg.LlamaMinP,
		"min-p sampling for llama.cpp (0 = disabled)",
	)
	llamaPort := fs.Int("llama-port", cfg.LlamaPort,
		"llama-server bind port")

	var llamaArgs stringSliceFlag
	llamaArgs = append(llamaArgs, cfg.LlamaArgs...)
	fs.Var(&llamaArgs, "llama-arg",
		"additional argument for llama-server (repeatable); for example: --llama-arg=-t --llama-arg=8")
	saveConfig := fs.Bool("save-config", false, "save the resulting configuration to ~/.gogitor/config.json")
	version := fs.Bool("version", false, "print Gogitor version and exit")
	fs.BoolVar(version, "v", false, "alias for --version")

	args = normalizeBooleanFlags(args)
	if err := fs.Parse(args); err != nil {
		return LaunchOptions{}, err
	}

	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "llama-thinking-key":
			cfg.LlamaThinkingKey = strings.TrimSpace(*llamaThinkingKey)
		case "llama-temp":
			cfg.LlamaTemperature = *llamaTemp
		case "llama-top-p":
			cfg.LlamaTopP = *llamaTopP
		case "llama-top-k":
			cfg.LlamaTopK = *llamaTopK
		case "llama-repeat-penalty":
			cfg.LlamaRepeatPenalty = *llamaRepeatPenalty
		case "llama-min-p":
			cfg.LlamaMinP = *llamaMinP
		case "llama-bin":
			cfg.LlamaBinPath = strings.TrimSpace(*llamaBin)
		case "llama-host":
			cfg.LlamaHost = strings.TrimSpace(*llamaHost)
		case "llama-port":
			cfg.LlamaPort = *llamaPort
		case "llama-arg":
			cfg.LlamaArgs = []string(llamaArgs)
		case "provider", "p":
			cfg.Provider = strings.TrimSpace(*provider)
		case "model", "m":
			cfg.Model = strings.TrimSpace(*model)
		case "key", "k":
			cfg.APIKey = strings.TrimSpace(*apiKey)
		case "api-key":
			cfg.APIKey = strings.TrimSpace(*apiKeyAlias)
		case "ollama-url":
			cfg.OllamaURL = strings.TrimSpace(*ollamaURL)
		case "repo", "r":
			cfg.WorkDir = strings.TrimSpace(*workDir)
		case "workdir":
			cfg.WorkDir = strings.TrimSpace(*workDirAlias)
		case "github":
			cfg.GitHubURL = strings.TrimSpace(*githubURL)
		case "key-github":
			cfg.GitHubToken = strings.TrimSpace(*githubToken)
		case "max-context":
			cfg.MaxContextTokens = *maxContext
		case "llm-timeout":
			cfg.LLMTimeout = *llmTimeout
		case "runner-timeout":
			cfg.RunnerTimeout = *runnerTimeout
		case "max-iterations":
			cfg.MaxIterations = *maxIterations
		case "reasoning":
			cfg.ReasoningEnabled = *reasoning
		case "reasoning-effort":
			cfg.ReasoningEffort = strings.TrimSpace(*reasoningEffort)
		case "reasoning-budget":
			cfg.ReasoningBudget = *reasoningBudget
		case "reasoning-show":
			cfg.ReasoningShow = *reasoningShow
		case "reasoning-router":
			cfg.ReasoningRouter = *reasoningRouter
		case "auto-search":
			cfg.AutoSearch = *autoSearch
		case "computer":
			cfg.ComputerEnabled = *computer
		case "allow-sudo":
			cfg.ComputerAllowSudo = *allowSudo
		case "computer-confirm":
			cfg.ComputerConfirmHigh = *computerConfirm
		case "computer-timeout":
			cfg.ComputerCommandTimeout = *computerTimeout
		case "computer-max-output":
			cfg.ComputerMaxOutput = *computerMaxOutput
		case "dry-run":
			cfg.DryRun = *dryRun
		case "debug":
			cfg.Debug = *debug
		case "raw":
			cfg.Raw = *raw
		case "compare":
			cfg.CompareApproaches = *compare
		case "auto-commit":
			cfg.AutoGitCommit = *autoCommit
		case "git-auto-init":
			cfg.GitAutoInit = *gitAutoInit
		case "deps-mode":
			cfg.DepsMode = strings.TrimSpace(*depsMode)
		case "confirm-apply":
			cfg.ConfirmApply = *confirmApply
		case "fuzzy-min-confidence":
			cfg.FuzzyMinConfidence = *fuzzy
		case "patch-protocol":
			cfg.PatchProtocolMode = strings.TrimSpace(*patchProtocol)
		case "patch-auditor":
			cfg.PatchAuditorMode = strings.TrimSpace(*patchAuditor)
		case "diff-trace":
			cfg.DiffTrace = *diffTrace
		case "agent-profile":
			cfg.AgentModelProfile = strings.TrimSpace(*agentProfile)
		case "agent-deep-threshold":
			cfg.AgentDeepComplexityThreshold = *agentThreshold
		case "autonomy":
			cfg.AutonomyEnabled = *autonomy
		case "autonomy-mode":
			cfg.AutonomyMode = strings.TrimSpace(*autonomyMode)
		case "autonomy-interval":
			cfg.AutonomyIntervalSec = *autonomyInterval
		case "autonomy-limit":
			cfg.AutonomyMutationLimit = *autonomyLimit
		case "output":
			cfg.OutputFile = strings.TrimSpace(*outputFile)
		case "log-level":
			cfg.LogLevel = strings.TrimSpace(*logLevel)
		case "save-config", "version":
		}
	})

	if len(fs.Args()) > 0 {
		return LaunchOptions{}, fmt.Errorf("unexpected argument %q; Gogitor is TUI-only; use flags before starting it", fs.Args()[0])
	}

	if strings.TrimSpace(cfg.WorkDir) == "" {
		wd, err := os.Getwd()
		if err != nil {
			return LaunchOptions{}, fmt.Errorf("cannot determine working directory: %w", err)
		}
		cfg.WorkDir = wd
	}

	return LaunchOptions{
		SaveConfig: *saveConfig,
		Version:    *version,
	}, nil
}

// PrintLaunchUsage prints the full startup contract. It intentionally focuses on
// TUI startup rather than preserving obsolete CLI subcommands.
// PrintLaunchUsage prints the full startup contract. It intentionally focuses on
// TUI startup rather than preserving obsolete CLI subcommands.
func PrintLaunchUsage(w io.Writer) {
	if w == nil {
		w = io.Discard
	}
	_, _ = fmt.Fprint(w, `Gogitor — TUI AI coding assistant

Usage:
  gogitor [flags]

Core startup parameters:
  -p, --provider <name>   ollama, llama, openai+URL, openai-compatible+URL, or HTTP(S) URL
  -m, --model <name>      model name (or path to .gguf when using --provider llama)
  -k, --key <token>       LLM/API key (prefer environment for secrets)
      --api-key <token>   alias for --key
      --ollama-url <URL>  Ollama base URL
  -r, --repo <path>       project/workspace directory
      --workdir <path>    alias for --repo
  --github <URL>          GitHub repository URL
  --key-github <token>    GitHub token

llama.cpp provider (--provider llama):
  --llama-bin <path>      llama-server binary (default: search in PATH)
  --llama-host <host>     bind host (default 127.0.0.1)
  --llama-port <port>     bind port (default 55555)
  --llama-arg <arg>       additional llama-server flag (repeatable)
  --llama-thinking-key <key>
                          chat template key for reasoning control
                          (default: enable_thinking; use "thinking" for
                          DeepSeek R1, "reasoning" for some vLLM builds)

  Gogitor launches llama-server with these defaults:
    -c 16384 -b 2048 -ub 1024 -np 1
    --cache-type-k q8_0 --cache-type-v q8_0

  If you pass --llama-arg=-c N, Gogitor automatically uses N as
  its own context limit, so you do not need to set --max-context.

Interface:
  The interface is fixed to the Zen TUI. There is no TUI-selection flag.

Execution:
  --max-context <n>       maximum context tokens, 0=auto
  --llm-timeout <sec>     LLM request timeout
  --runner-timeout <sec>  build/test timeout
  --max-iterations <n>    correction iterations
  --agent-profile <name>  agent model profile
  --agent-deep-threshold  complexity threshold for deeper execution
  --auto-commit <bool>    commit successful agent changes
  --git-auto-init <bool>  initialize Git automatically
  --compare <bool>        compare implementation approaches
  --auto-search <bool>    automatic web search

Reasoning:
  --reasoning <bool>      enable thinking/reasoning
  --reasoning-effort      low, medium, high
  --reasoning-budget      reasoning token budget; 0=server default
  --reasoning-show <bool> show thinking content
  --reasoning-router      reasoning for intent router

Patch safety:
  --deps-mode <mode>      auto, ask, never
  --confirm-apply <bool>  confirm generated patches
  --fuzzy-min-confidence <f>  fuzzy confidence override
  --patch-protocol <mode> auto, search_replace, replace_only
  --patch-auditor <mode>  auto, off, always
  --diff-trace <bool>     detailed patch diagnostics

Computer/autonomy:
  --computer <bool>       enable computer control
  --allow-sudo <bool>     allow sudo commands
  --computer-confirm      high-risk confirmation policy
  --computer-timeout      command timeout in seconds
  --computer-max-output   maximum command output bytes
  --autonomy <bool>       enable autonomous monitoring
  --autonomy-mode         suggest or auto
  --autonomy-interval     monitoring interval in seconds
  --autonomy-limit        mutation limit

Misc:
  --output <file>         save the last result automatically
  --raw <bool>            raw output mode
  --dry-run <bool>        dry-run behavior where supported
  --debug <bool>          debug logging
  --log-level <level>     debug, info, warn, error
  --save-config           persist the effective startup configuration
  --version               print version
  --help                  show this help

── llama-server arguments ────────────────────────────────────────────

  --llama-arg passes a single flag to llama-server. Repeat it for each
  flag, including separate values:

      --llama-arg=-t --llama-arg=8
      --llama-arg=-c --llama-arg=65536

  Do NOT use space-separated form (--llama-arg "-t 8"). Each flag
  and each value must be its own --llama-arg.

  Useful flags by purpose:

    Speed / performance
      -t N                     CPU threads (default: auto-detected)
      -fa, --flash-attn        Flash Attention (may save VRAM)
      --no-reasoning-preserve  disable Qwen thinking cache (faster)
      --speculative mtp        Multi-Token Prediction (Qwen3.8-27B+)

    Memory / context
      -c N                     context size in tokens (default 16384)
      -np N                    parallel slots (default 1)
      --cache-type-k <type>    KV K cache: q8_0, q4_0, f16
      --cache-type-v <type>    KV V cache: q8_0, q4_0, f16

    MoE models (Qwen3.8-Flash-Next 176B, gpt-oss 120B, etc.)
      --cpu-moe                keep ALL expert layers on CPU (safe)
      --n-cpu-moe N            keep first N expert layers on CPU

  Examples:

    Small dense model that fits in VRAM (7B–32B):
      --provider llama --model ~/GGUF/qwen27b.gguf
      --llama-arg=-ngl --llama-arg=99

    Large MoE model that does NOT fit in VRAM (100B+):
      --provider llama --model ~/GGUF/qwen176b.gguf
      --llama-arg=--cpu-moe
      --llama-arg=-c --llama-arg=65536
      --llama-arg=--no-reasoning-preserve

    Qwen3.8 for fast chat:
      --llama-arg=--no-reasoning-preserve
      --llama-arg=-fa

  WARNINGS:

    - Do NOT pass -ngl for MoE models on a GPU with limited VRAM.
      Manual -ngl overrides llama.cpp auto-fit and causes CUDA OOM
      on the first expert tensor. Let llama.cpp decide.

    - For MoE, use --cpu-moe (all experts on CPU) or --n-cpu-moe N
      (partial offload). The --n-cpu-moe value is the number of
      expert layers kept on CPU; increase it if you get OOM.

    - Flags are passed literally to llama-server. For the complete,
      version-specific list run:
          llama-server --help
    - When Gogitor owns llama-server, :reasoning on/off is translated into
      chat_template_kwargs on each request, so no restart is needed.

── Examples ──────────────────────────────────────────────────────────

  Local Ollama:
    gogitor --provider ollama --model gpt-oss:20b --repo ~/Code/myapp

  Remote OpenAI-compatible endpoint:
    gogitor --provider openai+https://api.openai.com/v1 \
            --model <model> --key <token>

  Local GGUF via llama.cpp (dense model):
    gogitor --provider llama \
            --model ~/GGUF/Qwen3.8-27B-UD-Q4_K_M.gguf \
            --repo ~/Code/myapp \
            --llama-arg=-ngl --llama-arg=99

  Local GGUF via llama.cpp (176B MoE on a small GPU):
    gogitor --provider llama \
            --model ~/GGUF/Qwen3.8-Flash-Next-UD-Q4_K_XL-merged.gguf \
            --repo ~/Code/myapp \
            --llama-arg=--cpu-moe \
            --llama-arg=-c --llama-arg=65536 \
            --llama-arg=--no-reasoning-preserve

Environment variables and ~/.gogitor/config.json remain supported.
Command-line flags have the highest precedence for the current launch.
`)
}

var booleanLaunchFlags = map[string]struct{}{
	"reasoning":        {},
	"reasoning-show":   {},
	"reasoning-router": {},
	"auto-search":      {},
	"computer":         {},
	"allow-sudo":       {},
	"computer-confirm": {},
	"dry-run":          {},
	"debug":            {},
	"raw":              {},
	"compare":          {},
	"auto-commit":      {},
	"git-auto-init":    {},
	"confirm-apply":    {},
	"diff-trace":       {},
	"autonomy":         {},
}

// normalizeBooleanFlags lets users use both `--flag` and `--flag=true` forms.
// The standard flag package only treats the latter as an explicit boolean value.
func normalizeBooleanFlags(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--") && !strings.Contains(arg, "=") {
			name := strings.TrimPrefix(arg, "--")
			if _, ok := booleanLaunchFlags[name]; ok {
				if i+1 < len(args) {
					next := strings.ToLower(strings.TrimSpace(args[i+1]))
					if next == "true" || next == "false" {
						out = append(out, arg+"="+next)
						i++
						continue
					}
				}
				out = append(out, arg+"=true")
				continue
			}
		}
		out = append(out, arg)
	}
	return out
}

// ParseBoolArgument is kept small for code which needs to consume a boolean
// configuration value outside flag parsing.
func ParseBoolArgument(v string) (bool, error) {
	b, err := strconv.ParseBool(strings.TrimSpace(v))
	if err != nil {
		return false, fmt.Errorf("invalid boolean %q", v)
	}
	return b, nil
}

// stringSliceFlag реализует flag.Value для повторяющихся аргументов.
type stringSliceFlag []string

func (s *stringSliceFlag) String() string {
	return strings.Join(*s, " ")
}

func (s *stringSliceFlag) Set(v string) error {
	*s = append(*s, v)
	return nil
}