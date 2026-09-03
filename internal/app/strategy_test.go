package app

import (
	"testing"

	"gogitor/internal/config"
	"gogitor/internal/workspace"
)

func TestDeepAgentUsesStrictPatchPolicy(
	t *testing.T,
) {
	cfg := config.Default()

	cfg.Provider = "ollama"
	cfg.Model = "qwen3.8:27b"

	svc := &Service{
		Cfg: cfg,
	}

	got := svc.patchPolicyForOptions(
		Options{
			AgentDepth: AgentDepthDeep,
		},
	)

	if got != workspace.PatchPolicyStrict {
		t.Fatalf(
			"deep policy = %v, want strict",
			got,
		)
	}
}

func TestNormalizeAgentDepth(
	t *testing.T,
) {
	tests := []struct {
		in   string
		want AgentDepth
	}{
		{"normal", AgentDepthNormal},
		{"standard", AgentDepthNormal},
		{"deep", AgentDepthDeep},
		{"strict", AgentDepthDeep},
		{"enhanced", AgentDepthDeep},
		{"auto", AgentDepthAuto},
		{"", AgentDepthAuto},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got :=
				normalizeAgentDepth(tt.in)

			if got != tt.want {
				t.Errorf(
					"normalizeAgentDepth(%q) = %v, want %v",
					tt.in,
					got,
					tt.want,
				)
			}

		})
	}
}

func TestNormalizeExecutionMode(
	t *testing.T,
) {
	tests := []struct {
		in   string
		want ExecutionMode
	}{
		{"simple", ExecutionModeSimple},
		{"fast", ExecutionModeSimple},
		{"быстро", ExecutionModeSimple},
		{"quick", ExecutionModeSimple},

		{"agent", ExecutionModeAgent},
		{"агент", ExecutionModeAgent},
		{"multi-agent", ExecutionModeAgent},
		{"multiagent", ExecutionModeAgent},

		{"auto", ExecutionModeAuto},
		{"", ExecutionModeAuto},
		{"default", ExecutionModeAuto},
		{"unknown", ExecutionModeAuto},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got :=
				normalizeExecutionMode(tt.in)

			if got != tt.want {
				t.Errorf(
					"normalizeExecutionMode(%q) = %v, want %v",
					tt.in,
					got,
					tt.want,
				)
			}
		})
	}
}

func TestUrlHostIsLocal(t *testing.T) {
	for _, tc := range []struct {
		url  string
		want bool
	}{
		{"http://localhost:11434", true},
		{"http://127.0.0.1:11434", true},
		{"http://192.168.1.10:11434", true},
		{"http://10.0.0.1:11434", true},
		{"https://api.example.com/v1", false},
		{"", true},
	} {
		if got := urlHostIsLocal(tc.url); got != tc.want {
			t.Errorf("urlHostIsLocal(%q) = %v, want %v", tc.url, got, tc.want)
		}
	}
}

func TestCloudModelIsRemoteOnLocalOllama(
	t *testing.T,
) {
	cfg := config.Default()

	cfg.Provider = "ollama"
	cfg.OllamaURL =
		"http://127.0.0.1:11434"
	cfg.Model = "gemma4:31b-cloud"

	svc := &Service{
		Cfg: cfg,
	}

	if svc.isLocalModelEndpoint() {
		t.Fatal(
			"cloud model must not be considered local",
		)
	}

	if !svc.isRemoteLLM() {
		t.Fatal(
			"cloud model must be considered remote",
		)
	}
}
