package config

import (
	"testing"
)

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
		_ = cfg.Validate()
		if cfg.LLMTimeout != 300 {
			t.Errorf("LLMTimeout = %d, want 300", cfg.LLMTimeout)
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
