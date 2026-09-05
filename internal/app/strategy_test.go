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

func TestTaskRequiresAgent(t *testing.T) {
	tests := []struct {
		name  string
		task  string
		want  bool
	}{
		{
			name: "health endpoint is simple",
			task: "add GET /health endpoint",
			want: false,
		},
		{
			name: "api endpoint is simple",
			task: "add GET /api/cars endpoint returning JSON",
			want: false,
		},
		{
			name: "server terminology is not enough",
			task: "add HTTP health check to the server",
			want: false,
		},
		{
			name: "refactor requires agent",
			task: "refactor authentication into a separate package",
			want: true,
		},
		{
			name: "package split requires agent",
			task: "move business logic into a new service package",
			want: true,
		},
		{
			name: "architecture requires agent",
			task: "redesign the application architecture",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := taskRequiresAgent(tt.task)

			if got != tt.want {
				t.Fatalf(
					"taskRequiresAgent(%q) = %v, want %v",
					tt.task,
					got,
					tt.want,
				)
			}
		})
	}
}

func TestValidateExecutionRecommendationPrefersFastForLocalTask(
	t *testing.T,
) {
	signals := executionSignals{
		Score:         2,
		TargetFiles:   1,
		RequiresAgent: false,
		BroadTask:     false,
	}

	recommendation := ExecutionStrategy{
		Mode:       ExecutionModeAgent,
		AgentDepth: AgentDepthDeep,
		Confidence: 95,
		Complexity: "high",
		Risk:       "high",
		Reason:     "LLM thinks agent is safer",
		Source:     "llm",
	}

	got := validateExecutionRecommendation(
		recommendation,
		signals,
	)

	if got.Mode != ExecutionModeSimple {
		t.Fatalf(
			"mode = %q, want simple; reason=%s",
			got.Mode,
			got.Reason,
		)
	}

	if got.AgentDepth != AgentDepthNormal {
		t.Fatalf(
			"depth = %q, want normal",
			got.AgentDepth,
		)
	}
}

func TestValidateExecutionRecommendationPromotesArchitecturalTask(
	t *testing.T,
) {
	signals := executionSignals{
		Score:         5,
		TargetFiles:   2,
		RequiresAgent: true,
		BroadTask:     false,
	}

	recommendation := ExecutionStrategy{
		Mode:       ExecutionModeSimple,
		AgentDepth: AgentDepthNormal,
		Confidence: 95,
		Complexity: "medium",
		Risk:       "medium",
		Reason:     "single coding pass",
		Source:     "llm",
	}

	got := validateExecutionRecommendation(
		recommendation,
		signals,
	)

	if got.Mode != ExecutionModeAgent {
		t.Fatalf(
			"mode = %q, want agent; reason=%s",
			got.Mode,
			got.Reason,
		)
	}
}

func TestValidateExecutionRecommendationDowngradesDeep(
	t *testing.T,
) {
	signals := executionSignals{
		Score:         5,
		TargetFiles:   2,
		RequiresAgent: true,
		BroadTask:     false,
	}

	recommendation := ExecutionStrategy{
		Mode:       ExecutionModeAgent,
		AgentDepth: AgentDepthDeep,
		Confidence: 95,
		Complexity: "medium",
		Risk:       "medium",
		Reason:     "deep preferred",
		Source:     "llm",
	}

	got := validateExecutionRecommendation(
		recommendation,
		signals,
	)

	if got.Mode != ExecutionModeAgent {
		t.Fatalf(
			"mode = %q, want agent",
			got.Mode,
		)
	}

	if got.AgentDepth != AgentDepthNormal {
		t.Fatalf(
			"depth = %q, want normal; reason=%s",
			got.AgentDepth,
			got.Reason,
		)
	}
}

func TestTaskComplexityDoesNotOverrateCommonDevelopmentTerms(
	t *testing.T,
) {
	root := t.TempDir()

	ws := workspace.New(root)
	defer ws.Close()

	svc := &Service{
		WS: ws,
	}

	tests := []struct {
		name string
		task string
		max  int
	}{
		{
			name: "health endpoint",
			task: "add HTTP GET /health endpoint returning OK",
			max: 2,
		},
		{
			name: "API endpoint",
			task: "add GET /api/cars endpoint returning JSON",
			max: 2,
		},
		{
			name: "server check",
			task: "add a server health check with timeout",
			max: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score, _ := svc.taskComplexityScore(tt.task)

			if score > tt.max {
				t.Fatalf(
					"taskComplexityScore(%q) = %d, want <= %d",
					tt.task,
					score,
					tt.max,
				)
			}
		})
	}
}