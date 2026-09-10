package config

import (
	"bytes"
	"testing"
)

func TestParseLaunchArgsOverridesConfig(t *testing.T) {
	cfg := Default()
	cfg.Provider = "ollama"
	cfg.Model = "old-model"

	_, err := ParseLaunchArgs(cfg, []string{
		"--provider", "openai-compatible+http://localhost:8000/v1",
		"--model", "new-model",
		"--max-context", "262144",
		"--reasoning", "true",
	}, bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Provider != "openai-compatible+http://localhost:8000/v1" || cfg.Model != "new-model" {
		t.Fatalf("launch override failed: provider=%q model=%q", cfg.Provider, cfg.Model)
	}
	if cfg.MaxContextTokens != 262144 || !cfg.ReasoningEnabled {
		t.Fatalf("launch options not applied: context=%d reasoning=%v", cfg.MaxContextTokens, cfg.ReasoningEnabled)
	}
}

func TestParseLaunchArgsRejectsPositionalArguments(t *testing.T) {
	cfg := Default()
	if _, err := ParseLaunchArgs(cfg, []string{"some-task"}, bytes.NewBuffer(nil), bytes.NewBuffer(nil)); err == nil {
		t.Fatal("expected positional argument error")
	}
}

func TestParseLaunchArgsHasNoTUISelectionFlag(t *testing.T) {
	cfg := Default()
	_, err := ParseLaunchArgs(cfg, []string{"--tui", "5"}, bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	if err == nil {
		t.Fatal("expected --tui to be rejected")
	}
}
