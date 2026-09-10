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
	multiAgent := fs.Bool("multi-agent", cfg.MultiAgent, "enable adaptive multi-agent execution")

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
	saveConfig := fs.Bool("save-config", false, "save the resulting configuration to ~/.gogitor/config.json")
	version := fs.Bool("version", false, "print Gogitor version and exit")
	fs.BoolVar(version, "v", false, "alias for --version")

	args = normalizeBooleanFlags(args)
	if err := fs.Parse(args); err != nil {
		return LaunchOptions{}, err
	}

	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
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
		case "multi-agent":
			cfg.MultiAgent = *multiAgent
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
func PrintLaunchUsage(w io.Writer) {
	if w == nil {
		w = io.Discard
	}
	_, _ = fmt.Fprint(w, `Gogitor — TUI AI coding assistant

Usage:
  gogitor [flags]

Core startup parameters:
  -p, --provider <name>   ollama, openai+URL, openai-compatible+URL, or HTTP(S) URL
  -m, --model <name>      model name
  -k, --key <token>       LLM/API key (prefer environment for secrets)
      --api-key <token>   alias for --key
      --ollama-url <URL>  Ollama base URL
  -r, --repo <path>       project/workspace directory
      --workdir <path>    alias for --repo
  --github <URL>          GitHub repository URL
  --key-github <token>    GitHub token

Interface:
  The interface is fixed to the Zen TUI. There is no TUI-selection flag.

Execution:
  --max-context <n>       maximum context tokens, 0=auto
  --llm-timeout <sec>     LLM request timeout
  --runner-timeout <sec>  build/test timeout
  --max-iterations <n>    correction iterations
  --multi-agent <bool>    adaptive agent execution
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

Examples:
  gogitor --provider ollama --model gpt-oss:20b --repo ~/Code/myapp
  gogitor --provider openai+https://api.openai.com/v1 --model <model> --key <token>

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
	"multi-agent":      {},
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
