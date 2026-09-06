package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gogitor/internal/domain"
)

// AgentModelCapability содержит настройки конкретной модели
// для Agent Harness.
type AgentModelCapability struct {
	Profile        string `json:"profile,omitempty"`
	PreferredDepth string `json:"preferred_depth,omitempty"`
	ContextTokens  int    `json:"context_tokens,omitempty"`
	PatchPolicy    string `json:"patch_policy,omitempty"`
	MaxSubtasks    int    `json:"max_subtasks,omitempty"`
}

// AgentTimeoutConfig содержит временные лимиты Agent по ролям.
// Все значения задаются в секундах.
type AgentTimeoutConfig struct {
	SessionSec             int `json:"session_sec"`
	SessionLargeContextSec int `json:"session_large_context_sec"`
	SessionHugeContextSec  int `json:"session_huge_context_sec"`

	RouterSec   int `json:"router_sec"`
	PlannerSec  int `json:"planner_sec"`
	CoderSec    int `json:"coder_sec"`
	ReviewerSec int `json:"reviewer_sec"`
	TesterSec   int `json:"tester_sec"`
	VerifierSec int `json:"verifier_sec"`
	SecuritySec int `json:"security_sec"`
	SearcherSec int `json:"searcher_sec"`
	DocsSec     int `json:"docs_sec"`
}

type Config struct {
	Provider                     string                          `json:"provider"`
	Model                        string                          `json:"model"`
	APIKey                       string                          `json:"api_key"`
	OllamaURL                    string                          `json:"ollama_url"`
	LogLevel                     string                          `json:"log_level"`
	Debug                        bool                            `json:"debug_mode"`
	DryRun                       bool                            `json:"dry_run"`
	LLMTimeout                   int                             `json:"llm_timeout"`
	MaxIterations                int                             `json:"max_iterations"`
    LLMMaxSessionRequests        int 							 `json:"llm_max_session_requests"`
    LLMCoderRequestMultiplier 	 int 							 `json:"llm_coder_request_multiplier"`
    LLMCoderMinRequests 		 int 							 `json:"llm_coder_min_requests"`
	AgentTimeouts                AgentTimeoutConfig              `json:"agent_timeouts"`
	RunnerTimeout                int                             `json:"runner_timeout"`
	AutoGitCommit                bool                            `json:"auto_git_commit"`
	GitAutoInit                  bool                            `json:"git_auto_init"`
	MultiAgent                   bool                            `json:"multi_agent_enabled"`
	Raw                          bool                            `json:"raw_output"`
	GitHubURL                    string                          `json:"github_url"`
	GitHubToken                  string                          `json:"github_token"`
	WorkDir                      string                          `json:"-"`
	MaxContextTokens             int                             `json:"max_context_tokens"`
	CompareApproaches            bool                            `json:"compare_approaches"`
	AutoSearch                   bool                            `json:"auto_search"`
	OutputFile                   string                          `json:"output_file"`
	AgentModelProfile            string                          `json:"agent_model_profile"`
	AgentDeepComplexityThreshold int                             `json:"agent_deep_complexity_threshold"`
	AgentModelCapabilities       map[string]AgentModelCapability `json:"agent_model_capabilities,omitempty"`
	DepsMode                     string                          `json:"deps_mode"`
	ConfirmApply                 bool                            `json:"confirm_apply"`
	FuzzyMinConfidence           float64                         `json:"fuzzy_min_confidence"`
	PatchProtocolMode            string                          `json:"patch_protocol_mode"`
	PatchAuditorMode             string                          `json:"patch_auditor_mode"`
	DiffTrace                    bool                            `json:"diff_trace"`
	DiffMatching                 domain.DiffMatchingConfig       `json:"diff_matching"`
	ComputerEnabled              bool                            `json:"computer_enabled"`
	ComputerAllowSudo            bool                            `json:"computer_allow_sudo"`
	ComputerConfirmHigh          bool                            `json:"computer_confirm_high"`
	ComputerCommandTimeout       int                             `json:"computer_command_timeout"`
	ComputerMaxOutput            int                             `json:"computer_max_output"`
	// Reasoning (thinking mode)
	ReasoningEnabled bool   `json:"reasoning_enabled"`
	ReasoningEffort  string `json:"reasoning_effort"` // "low"|"medium"|"high"
	ReasoningBudget  int    `json:"reasoning_budget"` // макс. токенов на thinking
	ReasoningShow    bool   `json:"reasoning_show"`   // показывать thinking в выводе
	ReasoningRouter  bool   `json:"reasoning_router"`
	// Autonomy (Autonomous Engineer mode)
	AutonomyEnabled       bool              `json:"autonomy_enabled"`
	AutonomyMode          string            `json:"autonomy_mode"` // "suggest" | "auto"
	AutonomyIntervalSec   int               `json:"autonomy_interval_sec"`
	AutonomyMutationLimit int               `json:"autonomy_mutation_limit"`
	PatchPolicies         map[string]string `json:"patch_policies,omitempty"`
}

func Default() *Config {
	return &Config{
		Provider:   "ollama",
		Model:      "gemma3:4b",
		OllamaURL:  "http://localhost:11434",
		LogLevel:   "info",
		Debug:      false,
		DryRun:     false,
		LLMTimeout: 3600,
		AgentTimeouts: AgentTimeoutConfig{
			SessionSec:             3600,  // 60 минут
			SessionLargeContextSec: 10800, // 180 минут
			SessionHugeContextSec:  14400, // 240 минут

			RouterSec:   600,   // 10 минут
			PlannerSec:  3600,  // 60 минут
			CoderSec:    10800, // до 180 минут
			ReviewerSec: 1800,  // 30 минут
			TesterSec:   1800,  // 30 минут
			VerifierSec: 3600,  // 60 минут
			SecuritySec: 900,   // 15 минут
			SearcherSec: 600,   // 10 минут
			DocsSec:     900,   // 15 минут
		},
		RunnerTimeout:                600, // 10 минут
		MaxIterations:                5,
        LLMMaxSessionRequests:        960,
        LLMCoderRequestMultiplier:    64,
        LLMCoderMinRequests:          192,
		AutoGitCommit:                true,
		GitAutoInit:                  true,
		MultiAgent:                   true,
		Raw:                          false,
		MaxContextTokens:             0,
		CompareApproaches:            true,
		AutoSearch:                   false,
		AgentModelProfile:            "auto",
		AgentDeepComplexityThreshold: 6,
		AgentModelCapabilities:       defaultAgentModelCapabilities(),
		DepsMode:                     "auto",
		ConfirmApply:                 false,
		FuzzyMinConfidence:           0,
		PatchProtocolMode:            "auto",
		PatchAuditorMode:             "auto",
		DiffTrace:                    false,
		DiffMatching:                 domain.DefaultDiffMatchingConfig(),
		ComputerEnabled:              false,
		ComputerAllowSudo:            false,
		ComputerConfirmHigh:          true,
		ComputerCommandTimeout:       120,
		ComputerMaxOutput:            100000,
		ReasoningEnabled:             false,
		ReasoningEffort:              "medium",
		ReasoningBudget:              0, // 0 = сервер решает сам
		ReasoningShow:                false,
		ReasoningRouter:              false,
		AutonomyEnabled:              false,
		AutonomyMode:                 "suggest",
		AutonomyIntervalSec:          60,
		AutonomyMutationLimit:        20,
		PatchPolicies: map[string]string{
			"gemma3:4b":        "strict",
			"gemma4:12b":       "strict",
			"ornith-1.5:9b":    "strict",
			"qwen3.8:27b":      "balanced",
			"gemma4:26b":       "balanced",
			"gpt-oss:20b":      "balanced",
			"gemma4:31b-cloud": "advanced",
			"openai-compatible+http://localhost:8000/v1": "advanced",
			"llama3": "balanced",
		},
	}
}

// EffectiveContextTokens возвращает реальный лимит контекста.
func (c *Config) EffectiveContextTokens() int {
	if c.MaxContextTokens > 0 {
		return c.MaxContextTokens
	}
	return 131072
}

// ContextBudget возвращает количество токенов, доступных для промпта
func (c *Config) ContextBudget() int {
	total := c.EffectiveContextTokens()
	reserve := 8192 // запас на ответ модели
	if total <= reserve {
		return total / 2
	}
	return total - reserve
}

// ContextBytes возвращает максимальный объём контекста в байтах.
func (c *Config) ContextBytes() int {
	return c.ContextBudget() * 4
}

func Dir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".gogitor"
	}
	return filepath.Join(home, ".gogitor")
}

func Path() string {
	return filepath.Join(Dir(), "config.json")
}

// defaultPatchPolicies возвращает ваши политики по умолчанию
func defaultPatchPolicies() map[string]string {
	return map[string]string{
		"gemma3:4b":        "strict",
		"gemma4:12b":       "strict",
		"ornith-1.5:9b":    "strict",
		"qwen3.8:27b":      "balanced",
		"gemma4:26b":       "balanced",
		"gpt-oss:20b":      "strict",
		"gemma4:31b-cloud": "advanced",
		"openai-compatible+http://localhost:8000/v1": "advanced",
		"llama3": "balanced",
	}
}

func defaultAgentModelCapabilities() map[string]AgentModelCapability {
	return map[string]AgentModelCapability{
		"gemma3:4b": {
			Profile:        "small",
			PreferredDepth: "deep",
			PatchPolicy:    "strict",
			MaxSubtasks:    4,
		},
		"gemma4:12b": {
			Profile:        "medium",
			PreferredDepth: "normal",
			PatchPolicy:    "strict",
			MaxSubtasks:    5,
		},
		"ornith-1.5:9b": {
			Profile:        "small",
			PreferredDepth: "deep",
			PatchPolicy:    "strict",
			MaxSubtasks:    4,
		},
		"qwen3.8:27b": {
			Profile:        "medium",
			PreferredDepth: "deep",
			PatchPolicy:    "balanced",
			MaxSubtasks:    6,
		},
		"gemma4:26b": {
			Profile:        "medium",
			PreferredDepth: "normal",
			PatchPolicy:    "balanced",
			MaxSubtasks:    6,
		},
		"gpt-oss:20b": {
			Profile:        "medium",
			PreferredDepth: "normal",
			PatchPolicy:    "strict",
			MaxSubtasks:    5,
		},
		"gemma4:31b-cloud": {
			Profile:        "medium",
			PreferredDepth: "deep",
			PatchPolicy:    "advanced",
			MaxSubtasks:    6,
		},
		"llama3": {
			Profile:        "medium",
			PreferredDepth: "normal",
			PatchPolicy:    "balanced",
			MaxSubtasks:    5,
		},
	}
}

func Load() (*Config, error) {
	cfg := Default()

	data, err := os.ReadFile(Path())

	if err == nil {
		if jerr := json.Unmarshal(data, cfg); jerr != nil {
			cfg.loadEnv()
			cfg.loadLocal()

			return cfg, fmt.Errorf(
				"invalid config %s: %w",
				Path(),
				jerr,
			)
		}

		if merr := mergeMissingDefaultsIntoConfigFile(
			Path(),
			data,
		); merr != nil {
			// Нефатально: значения defaults уже есть
			// в cfg, поэтому работа программы возможна.
		}
	} else if os.IsNotExist(err) {
		if serr := cfg.Save(); serr != nil {
			cfg.loadEnv()
			cfg.loadLocal()

			return cfg, fmt.Errorf(
				"created default config in memory, but cannot write %s: %w",
				Path(),
				serr,
			)
		}
	} else {
		cfg.loadEnv()
		cfg.loadLocal()

		return cfg, err
	}

	cfg.loadEnv()
	cfg.loadLocal()

	cfg.normalizeTimeouts()

	cfg.DiffMatching =
		cfg.DiffMatching.Normalized()

	return cfg, nil
}

func (c *Config) loadEnv() {
	if v := os.Getenv("GOGITOR_REASONING"); v != "" {
		c.ReasoningEnabled = parseBool(v)
	}
	if v := os.Getenv("GOGITOR_REASONING_EFFORT"); v != "" {
		c.ReasoningEffort = v
	}
	if v := os.Getenv("GOGITOR_REASONING_BUDGET"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.ReasoningBudget = n
		}
	}
	if v := os.Getenv("GOGITOR_REASONING_ROUTER"); v != "" {
		c.ReasoningRouter = parseBool(v)
	}
	if v := os.Getenv("GOGITOR_RAW"); v != "" {
		c.Raw = parseBool(v)
	}
	if v := os.Getenv("GOGITOR_PROVIDER"); v != "" {
		c.Provider = v
	}

	if v := os.Getenv("GOGITOR_GITHUB_URL"); v != "" {
		c.GitHubURL = v
	}
	if v := os.Getenv("GOGITOR_COMPARE_APPROACHES"); v != "" {
		c.CompareApproaches = parseBool(v)
	}
	if v := os.Getenv("GOGITOR_GITHUB_TOKEN"); v != "" {
		c.GitHubToken = v
	}
	if c.GitHubToken == "" {
		if v := os.Getenv("GITHUB_TOKEN"); v != "" {
			c.GitHubToken = v
		}
	}
	if v := os.Getenv("GOGITOR_AUTO_SEARCH"); v != "" {
		c.AutoSearch = parseBool(v)
	}
	if v := os.Getenv("GOGITOR_MAX_CONTEXT_TOKENS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.MaxContextTokens = n
		}
	}
	if v := os.Getenv("GOGITOR_MODEL"); v != "" {
		c.Model = v
	}
	if v := os.Getenv("GOGITOR_API_KEY"); v != "" {
		c.APIKey = v
	}

	if c.APIKey == "" {
		lowerProvider := strings.ToLower(c.Provider)
		if strings.HasPrefix(lowerProvider, "openai+") ||
			strings.HasPrefix(lowerProvider, "openai-compatible+") {
			c.APIKey = os.Getenv("OPENAI_API_KEY")
		}
	}

	if v := os.Getenv("GOGITOR_OLLAMA_URL"); v != "" {
		c.OllamaURL = v
	}
	if v := os.Getenv("GOGITOR_LOG_LEVEL"); v != "" {
		c.LogLevel = v
	}
	if v := os.Getenv("GOGITOR_DEBUG"); v != "" {
		c.Debug = parseBool(v)
	}
	if v := os.Getenv("GOGITOR_DRY_RUN"); v != "" {
		c.DryRun = parseBool(v)
	}
	if v := os.Getenv("GOGITOR_LLM_TIMEOUT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.LLMTimeout = n
		}
	}
	if v := os.Getenv("GOGITOR_MAX_ITERATIONS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.MaxIterations = n
		}
	}
	if v := os.Getenv("GOGITOR_AUTO_GIT_COMMIT"); v != "" {
		c.AutoGitCommit = parseBool(v)
	}
	if v := os.Getenv("GOGITOR_GIT_AUTO_INIT"); v != "" {
		c.GitAutoInit = parseBool(v)
	}
	if v := os.Getenv("GOGITOR_MULTI_AGENT"); v != "" {
		c.MultiAgent = parseBool(v)
	}
	if v := os.Getenv("GOGITOR_DEPS_MODE"); v != "" {
		c.DepsMode = v
	}
	if v := os.Getenv("GOGITOR_CONFIRM_APPLY"); v != "" {
		c.ConfirmApply = parseBool(v)
	}
	if v := os.Getenv("GOGITOR_PATCH_PROTOCOL"); v != "" {
		c.PatchProtocolMode = strings.TrimSpace(v)
	}

	if v := os.Getenv("GOGITOR_PATCH_AUDITOR"); v != "" {
		c.PatchAuditorMode = strings.TrimSpace(v)
	}
	if v := os.Getenv("GOGITOR_COMPUTER_ENABLED"); v != "" {
		c.ComputerEnabled = parseBool(v)
	}
	if v := os.Getenv("GOGITOR_COMPUTER_ALLOW_SUDO"); v != "" {
		c.ComputerAllowSudo = parseBool(v)
	}
	if v := os.Getenv("GOGITOR_AUTONOMY"); v != "" {
		c.AutonomyEnabled = parseBool(v)
	}
	if v := os.Getenv("GOGITOR_AUTONOMY_MODE"); v != "" {
		c.AutonomyMode = v
	}
	if v := os.Getenv("GOGITOR_AUTONOMY_INTERVAL"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.AutonomyIntervalSec = n
		}
	}
	if v := os.Getenv("GOGITOR_AUTONOMY_MUTATION_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.AutonomyMutationLimit = n
		}
	}
}

func (c *Config) loadLocal() {

	data, err := os.ReadFile(".gogitor.json")
	if err != nil {
		return
	}

	var local map[string]any
	if err := json.Unmarshal(data, &local); err != nil {
		return
	}

	if v, ok := local["reasoning_router"].(bool); ok {
		c.ReasoningRouter = v
	}
	if v, ok := local["reasoning_enabled"].(bool); ok {
		c.ReasoningEnabled = v
	}
	if v, ok := local["reasoning_effort"].(string); ok && v != "" {
		c.ReasoningEffort = v
	}
	if v, ok := local["reasoning_budget"].(float64); ok && v > 0 {
		c.ReasoningBudget = int(v)
	}
	if v, ok := local["reasoning_show"].(bool); ok {
		c.ReasoningShow = v
	}

	if v, ok := local["raw_output"].(bool); ok {
		c.Raw = v
	}
	if v, ok := local["auto_search"].(bool); ok {
		c.AutoSearch = v
	}

	if v, ok := local["compare_approaches"].(bool); ok {
		c.CompareApproaches = v
	}

	if v, ok := local["provider"].(string); ok && v != "" {
		c.Provider = v
	}
	if v, ok := local["model"].(string); ok && v != "" {
		c.Model = v
	}
	if v, ok := local["api_key"].(string); ok && v != "" {
		c.APIKey = v
	}
	if v, ok := local["ollama_url"].(string); ok && v != "" {
		c.OllamaURL = v
	}
	if v, ok := local["log_level"].(string); ok && v != "" {
		c.LogLevel = v
	}
	if v, ok := local["max_context_tokens"].(float64); ok && v > 0 {
		c.MaxContextTokens = int(v)
	}
	if v, ok := local["debug_mode"].(bool); ok {
		c.Debug = v
	}
	if v, ok := local["dry_run"].(bool); ok {
		c.DryRun = v
	}
	if v, ok := local["llm_timeout"].(float64); ok {
		c.LLMTimeout = int(v)
	}
	if v, ok := local["max_iterations"].(float64); ok {
		c.MaxIterations = int(v)
	}
	if v, ok := local["auto_git_commit"].(bool); ok {
		c.AutoGitCommit = v
	}
	if v, ok := local["git_auto_init"].(bool); ok {
		c.GitAutoInit = v
	}
	if v, ok := local["multi_agent_enabled"].(bool); ok {
		c.MultiAgent = v
	}
	if v, ok := local["github_url"].(string); ok && v != "" {
		c.GitHubURL = v
	}
	if v, ok := local["github_token"].(string); ok && v != "" {
		c.GitHubToken = v
	}

	if v, ok := local["agent_model_profile"].(string); ok && strings.TrimSpace(v) != "" {
		c.AgentModelProfile = strings.TrimSpace(v)
	}

	if v, ok := local["agent_deep_complexity_threshold"].(float64); ok && v > 0 {
		c.AgentDeepComplexityThreshold =
			int(v)
	}

	if raw, ok := local["agent_model_capabilities"]; ok {
		data, err := json.Marshal(raw)
		if err == nil {
			var capabilities map[string]AgentModelCapability

			if err := json.Unmarshal(
				data,
				&capabilities,
			); err == nil {
				c.AgentModelCapabilities =
					capabilities
			}
		}
	}
	if v, ok := local["deps_mode"].(string); ok && strings.TrimSpace(v) != "" {
		c.DepsMode = strings.TrimSpace(v)
	}
	if v, ok := local["confirm_apply"].(bool); ok {
		c.ConfirmApply = v
	}
	if v, ok := local["fuzzy_min_confidence"].(float64); ok && v > 0 {
		c.FuzzyMinConfidence = v
	}
	if v, ok := local["patch_protocol_mode"].(string); ok &&
		strings.TrimSpace(v) != "" {
		c.PatchProtocolMode = strings.TrimSpace(v)
	}

	if v, ok := local["patch_auditor_mode"].(string); ok &&
		strings.TrimSpace(v) != "" {
		c.PatchAuditorMode = strings.TrimSpace(v)
	}
	if v, ok := local["computer_enabled"].(bool); ok {
		c.ComputerEnabled = v
	}
	if v, ok := local["computer_allow_sudo"].(bool); ok {
		c.ComputerAllowSudo = v
	}
	if v, ok := local["computer_confirm_high"].(bool); ok {
		c.ComputerConfirmHigh = v
	}
	if v, ok := local["computer_command_timeout"].(float64); ok && v > 0 {
		c.ComputerCommandTimeout = int(v)
	}
	if v, ok := local["autonomy_enabled"].(bool); ok {
		c.AutonomyEnabled = v
	}
	if v, ok := local["autonomy_mode"].(string); ok && strings.TrimSpace(v) != "" {
		c.AutonomyMode = strings.TrimSpace(v)
	}
	if v, ok := local["autonomy_interval_sec"].(float64); ok && v > 0 {
		c.AutonomyIntervalSec = int(v)
	}
	if v, ok := local["autonomy_mutation_limit"].(float64); ok && v > 0 {
		c.AutonomyMutationLimit = int(v)
	}
}

func (c *Config) normalizeTimeouts() {
	defaults := Default().AgentTimeouts

	if c.AgentTimeouts.SessionSec <= 0 {
		c.AgentTimeouts.SessionSec =
			defaults.SessionSec
	}

	if c.AgentTimeouts.SessionLargeContextSec <= 0 {
		c.AgentTimeouts.SessionLargeContextSec =
			defaults.SessionLargeContextSec
	}

	if c.AgentTimeouts.SessionHugeContextSec <= 0 {
		c.AgentTimeouts.SessionHugeContextSec =
			defaults.SessionHugeContextSec
	}

	if c.AgentTimeouts.RouterSec <= 0 {
		c.AgentTimeouts.RouterSec =
			defaults.RouterSec
	}

	if c.AgentTimeouts.PlannerSec <= 0 {
		c.AgentTimeouts.PlannerSec =
			defaults.PlannerSec
	}

	if c.AgentTimeouts.CoderSec <= 0 {
		c.AgentTimeouts.CoderSec =
			defaults.CoderSec
	}

	if c.AgentTimeouts.ReviewerSec <= 0 {
		c.AgentTimeouts.ReviewerSec =
			defaults.ReviewerSec
	}

	if c.AgentTimeouts.TesterSec <= 0 {
		c.AgentTimeouts.TesterSec =
			defaults.TesterSec
	}

	if c.AgentTimeouts.VerifierSec <= 0 {
		c.AgentTimeouts.VerifierSec =
			defaults.VerifierSec
	}

	if c.AgentTimeouts.SecuritySec <= 0 {
		c.AgentTimeouts.SecuritySec =
			defaults.SecuritySec
	}

	if c.AgentTimeouts.SearcherSec <= 0 {
		c.AgentTimeouts.SearcherSec =
			defaults.SearcherSec
	}

	if c.AgentTimeouts.DocsSec <= 0 {
		c.AgentTimeouts.DocsSec =
			defaults.DocsSec
	}

	if c.RunnerTimeout <= 0 {
		c.RunnerTimeout = 600
	}

	if c.LLMTimeout <= 0 {
		c.LLMTimeout = 3600
	}

	// Не допускаем уменьшения лимита при переходе
	// на больший context window.
	if c.AgentTimeouts.SessionLargeContextSec <
		c.AgentTimeouts.SessionSec {

		c.AgentTimeouts.SessionLargeContextSec =
			c.AgentTimeouts.SessionSec
	}

	if c.AgentTimeouts.SessionHugeContextSec <
		c.AgentTimeouts.SessionLargeContextSec {

		c.AgentTimeouts.SessionHugeContextSec =
			c.AgentTimeouts.SessionLargeContextSec
	}
}

func mergeMissingDefaultsIntoConfigFile(
	path string,
	data []byte,
) error {
	var current map[string]json.RawMessage

	if err := json.Unmarshal(data, &current); err != nil {
		return err
	}

	defaultsData, err := json.Marshal(Default())
	if err != nil {
		return err
	}

	var defaults map[string]json.RawMessage

	if err := json.Unmarshal(
		defaultsData,
		&defaults,
	); err != nil {
		return err
	}

	changed := false

	for key, value := range defaults {
		if _, exists := current[key]; exists {
			continue
		}

		current[key] = value
		changed = true
	}

	if !changed {
		return nil
	}

	merged, err := json.MarshalIndent(
		current,
		"",
		"  ",
	)
	if err != nil {
		return err
	}

	return os.WriteFile(
		path,
		merged,
		0o600,
	)
}

func (c *Config) Validate() error {
	c.DiffMatching =
		c.DiffMatching.Normalized()

	c.normalizeTimeouts()

	if c.MaxIterations <= 0 {
		c.MaxIterations = 3
	}
	if strings.TrimSpace(c.Model) == "" {
		return fmt.Errorf("model must not be empty")
	}
	if !c.IsSupportedProvider() {
		return fmt.Errorf(
			"unsupported provider %q; supported: ollama, openai+http(s)://..., openai-compatible+http(s)://..., or http(s) URL for Ollama-compatible server",
			c.Provider,
		)
	}

	switch strings.ToLower(strings.TrimSpace(c.PatchProtocolMode)) {
	case "", "auto",
		"search_replace", "search-replace",
		"replace_only", "replace-only":
	default:
		return fmt.Errorf(
			"invalid patch_protocol_mode %q; use auto, search_replace, or replace_only",
			c.PatchProtocolMode,
		)
	}

	switch strings.ToLower(strings.TrimSpace(c.PatchAuditorMode)) {
	case "", "auto", "off", "always":
	default:
		return fmt.Errorf(
			"invalid patch_auditor_mode %q; use auto, off, or always",
			c.PatchAuditorMode,
		)
	}
	return nil
}

func (c *Config) IsSupportedProvider() bool {
	p := strings.ToLower(strings.TrimSpace(c.Provider))

	switch p {
	case "ollama":
		return true
	}

	if base, ok := OpenAIBaseFromProvider(c.Provider); ok {
		u, err := url.Parse(base)
		return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
	}

	u, err := url.Parse(c.Provider)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// Save записывает текущую конфигурацию в ~/.gogitor/config.json.
// Директория ~/.gogitor создаётся автоматически, если не существует.
func (c *Config) Save() error {
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		return fmt.Errorf("cannot create config dir %s: %w", Dir(), err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("cannot marshal config: %w", err)
	}
	return os.WriteFile(Path(), data, 0o600)
}

func parseBool(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "on", "yes":
		return true
	default:
		return false
	}
}

func OpenAIBaseFromProvider(provider string) (string, bool) {
	p := strings.TrimSpace(provider)
	if p == "" {
		return "", false
	}

	lower := strings.ToLower(p)

	prefixes := []string{
		"openai-compatible+",
		"openai+",
	}

	for _, prefix := range prefixes {
		if strings.HasPrefix(lower, prefix) {
			base := strings.TrimSpace(p[len(prefix):])
			if base == "" {
				return "", false
			}

			return strings.TrimRight(base, "/"), true
		}
	}

	return "", false
}
