package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultLLMBudgets(t *testing.T) {
	cfg := Default()

	if cfg.LLMMaxSessionRequests != 960 {
		t.Fatalf(
			"LLMMaxSessionRequests = %d, want 960",
			cfg.LLMMaxSessionRequests,
		)
	}

	if cfg.LLMCoderRequestMultiplier != 64 {
		t.Fatalf(
			"LLMCoderRequestMultiplier = %d, want 64",
			cfg.LLMCoderRequestMultiplier,
		)
	}

	if cfg.LLMCoderMinRequests != 192 {
		t.Fatalf(
			"LLMCoderMinRequests = %d, want 192",
			cfg.LLMCoderMinRequests,
		)
	}
}

func TestOpenAIBaseFromProvider(t *testing.T) {
	tests := []struct {
		provider string
		wantBase string
		wantOK   bool
	}{
		{"openai+https://api.example.com/v1", "https://api.example.com/v1", true},
		{"openai-compatible+http://localhost:8000/v1", "http://localhost:8000/v1", true},
		{"OPENAI+https://api.example.com/v1", "https://api.example.com/v1", true},
		{"ollama", "", false},
		{"http://localhost:11434", "", false},
		{"openai+", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			base, ok := OpenAIBaseFromProvider(tt.provider)
			if ok != tt.wantOK || base != tt.wantBase {
				t.Errorf("got (%q, %v), want (%q, %v)", base, ok, tt.wantBase, tt.wantOK)
			}
		})
	}
}

func TestIsSupportedProvider(t *testing.T) {
	tests := []struct {
		provider string
		want     bool
	}{
		{"ollama", true},
		{"http://localhost:11434", true},
		{"openai+https://api.example.com/v1", true},
		{"openai-compatible+http://localhost:8000/v1", true},
		{"unsupported", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			cfg := Default()
			cfg.Provider = tt.provider
			if got := cfg.IsSupportedProvider(); got != tt.want {
				t.Errorf("IsSupportedProvider(%q) = %v, want %v", tt.provider, got, tt.want)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		if err := Default().Validate(); err != nil {
			t.Errorf("unexpected: %v", err)
		}
	})
	t.Run("empty model", func(t *testing.T) {
		cfg := Default()
		cfg.Model = ""
		if err := cfg.Validate(); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("invalid provider", func(t *testing.T) {
		cfg := Default()
		cfg.Provider = "invalid"
		if err := cfg.Validate(); err == nil {
			t.Error("expected error")
		}
	})

	t.Run("negative timeout fixed", func(t *testing.T) {
		cfg := Default()
		cfg.LLMTimeout = -1

		if err := cfg.Validate(); err != nil {
			t.Fatalf(
				"Validate() failed: %v",
				err,
			)
		}

		if cfg.LLMTimeout != 3600 {
			t.Errorf(
				"LLMTimeout = %d, want 3600",
				cfg.LLMTimeout,
			)
		}
	})
}

func TestEffectiveContextTokens(t *testing.T) {
	cfg := Default()
	if got := cfg.EffectiveContextTokens(); got != 131072 {
		t.Errorf("default = %d", got)
	}
	cfg.MaxContextTokens = 262144
	if got := cfg.EffectiveContextTokens(); got != 262144 {
		t.Errorf("custom = %d", got)
	}
}

func TestContextBudget(t *testing.T) {
	cfg := Default()
	total := cfg.EffectiveContextTokens()
	if budget := cfg.ContextBudget(); budget != total-8192 {
		t.Errorf("budget = %d, want %d", budget, total-8192)
	}
}

func TestContextBytes(t *testing.T) {
	cfg := Default()
	if got := cfg.ContextBytes(); got != cfg.ContextBudget()*4 {
		t.Errorf("bytes = %d", got)
	}
}

func TestParseBool(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"1", true}, {"true", true}, {"on", true}, {"yes", true}, {"TRUE", true},
		{"0", false}, {"false", false}, {"off", false}, {"no", false}, {"", false},
	} {
		if got := parseBool(tc.in); got != tc.want {
			t.Errorf("parseBool(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestDefaultAgentTimeouts(t *testing.T) {
	cfg := Default()

	if cfg.LLMTimeout != 3600 {
		t.Fatalf(
			"LLMTimeout = %d, want 3600",
			cfg.LLMTimeout,
		)
	}

	if cfg.AgentTimeouts.SessionSec != 3600 {
		t.Fatalf(
			"SessionSec = %d, want 3600",
			cfg.AgentTimeouts.SessionSec,
		)
	}

	if cfg.AgentTimeouts.SessionLargeContextSec != 10800 {
		t.Fatalf(
			"SessionLargeContextSec = %d, want 10800",
			cfg.AgentTimeouts.SessionLargeContextSec,
		)
	}

	if cfg.AgentTimeouts.SessionHugeContextSec != 14400 {
		t.Fatalf(
			"SessionHugeContextSec = %d, want 14400",
			cfg.AgentTimeouts.SessionHugeContextSec,
		)
	}

	if cfg.AgentTimeouts.RouterSec != 600 {
		t.Fatalf(
			"RouterSec = %d, want 600",
			cfg.AgentTimeouts.RouterSec,
		)
	}

	if cfg.AgentTimeouts.VerifierSec != 3600 {
		t.Fatalf(
			"VerifierSec = %d, want 3600",
			cfg.AgentTimeouts.VerifierSec,
		)
	}

	if cfg.RunnerTimeout != 600 {
		t.Fatalf(
			"RunnerTimeout = %d, want 600",
			cfg.RunnerTimeout,
		)
	}
}

func TestLoadCreatesDefaultConfigWithTimeouts(
	t *testing.T,
) {
	home := t.TempDir()

	t.Setenv("HOME", home)

	cfg, err := Load()
	if err != nil {
		t.Fatalf(
			"Load() failed: %v",
			err,
		)
	}

	if cfg.AgentTimeouts.SessionSec != 3600 {
		t.Fatalf(
			"loaded SessionSec = %d, want 3600",
			cfg.AgentTimeouts.SessionSec,
		)
	}

	path := filepath.Join(
		home,
		".gogitor",
		"config.json",
	)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf(
			"cannot read generated config: %v",
			err,
		)
	}

	var disk Config

	if err := json.Unmarshal(
		data,
		&disk,
	); err != nil {
		t.Fatalf(
			"generated config is invalid JSON: %v",
			err,
		)
	}

	if disk.AgentTimeouts.SessionHugeContextSec != 14400 {
		t.Fatalf(
			"disk SessionHugeContextSec = %d, want 14400",
			disk.AgentTimeouts.SessionHugeContextSec,
		)
	}

	if disk.RunnerTimeout != 600 {
		t.Fatalf(
			"disk RunnerTimeout = %d, want 600",
			disk.RunnerTimeout,
		)
	}
}

func TestLoadMigratesMissingTimeoutConfig(
	t *testing.T,
) {
	home := t.TempDir()

	t.Setenv("HOME", home)

	dir := filepath.Join(home, ".gogitor")

	if err := os.MkdirAll(
		dir,
		0o755,
	); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(
		dir,
		"config.json",
	)

	oldConfig := `{
  "provider": "ollama",
  "model": "qwen3.8:27b",
  "llm_timeout": 3000
}`

	if err := os.WriteFile(
		path,
		[]byte(oldConfig),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf(
			"Load() failed: %v",
			err,
		)
	}

	if cfg.Model != "qwen3.8:27b" {
		t.Fatalf(
			"model = %q, want qwen3.8:27b",
			cfg.Model,
		)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var disk Config

	if err := json.Unmarshal(
		data,
		&disk,
	); err != nil {
		t.Fatalf(
			"migrated config is invalid JSON: %v",
			err,
		)
	}

	if disk.LLMMaxSessionRequests != 960 {
		t.Fatalf(
			"disk LLMMaxSessionRequests = %d, want 960",
			disk.LLMMaxSessionRequests,
		)
	}

	if disk.LLMCoderRequestMultiplier != 64 {
		t.Fatalf(
			"disk LLMCoderRequestMultiplier = %d, want 64",
			disk.LLMCoderRequestMultiplier,
		)
	}

	if disk.LLMCoderMinRequests != 192 {
		t.Fatalf(
			"disk LLMCoderMinRequests = %d, want 192",
			disk.LLMCoderMinRequests,
		)
	}
	var raw map[string]json.RawMessage

	if err := json.Unmarshal(
		data,
		&raw,
	); err != nil {
		t.Fatalf(
			"migrated config is invalid JSON: %v",
			err,
		)
	}

	if _, ok := raw["agent_timeouts"]; !ok {
		t.Fatal(
			"agent_timeouts was not added",
		)
	}

	if _, ok := raw["runner_timeout"]; !ok {
		t.Fatal(
			"runner_timeout was not added",
		)
	}
}

func TestValidateNormalizesTimeouts(t *testing.T) {
	cfg := Default()

	cfg.LLMTimeout = -1
	cfg.RunnerTimeout = -1

	cfg.AgentTimeouts.SessionSec = -1
	cfg.AgentTimeouts.RouterSec = -1
	cfg.AgentTimeouts.VerifierSec = -1

	if err := cfg.Validate(); err != nil {
		t.Fatalf(
			"Validate() failed: %v",
			err,
		)
	}

	if cfg.LLMTimeout != 3600 {
		t.Errorf(
			"LLMTimeout = %d, want 3600",
			cfg.LLMTimeout,
		)
	}

	if cfg.RunnerTimeout != 600 {
		t.Errorf(
			"RunnerTimeout = %d, want 600",
			cfg.RunnerTimeout,
		)
	}

	if cfg.AgentTimeouts.SessionSec != 3600 {
		t.Errorf(
			"SessionSec = %d, want 3600",
			cfg.AgentTimeouts.SessionSec,
		)
	}

	if cfg.AgentTimeouts.RouterSec != 600 {
		t.Errorf(
			"RouterSec = %d, want 600",
			cfg.AgentTimeouts.RouterSec,
		)
	}

	if cfg.AgentTimeouts.VerifierSec != 3600 {
		t.Errorf(
			"VerifierSec = %d, want 3600",
			cfg.AgentTimeouts.VerifierSec,
		)
	}
}
