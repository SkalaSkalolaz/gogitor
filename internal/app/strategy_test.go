package app

import (
	"testing"

	"gogitor/internal/config"
	"gogitor/internal/workspace"
)

func TestNormalizeEditMode(t *testing.T) {
	tests := []struct {
		in   string
		want EditMode
	}{
		{"patch", EditModePatch},
		{"diff", EditModePatch},
		{"minimal", EditModePatch},

		{"full", EditModeFull},
		{"full-file", EditModeFull},
		{"full_file", EditModeFull},
		{"rewrite", EditModeFull},

		{"auto", EditModeAuto},
		{"", EditModeAuto},
		{"unknown", EditModeAuto},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := normalizeEditMode(tt.in)

			if got != tt.want {
				t.Errorf(
					"normalizeEditMode(%q) = %q, want %q",
					tt.in,
					got,
					tt.want,
				)
			}
		})
	}
}

func TestAgentEditModeDefaultsToPatch(t *testing.T) {
	got := agentEditModeForTask(
		"add GET /health endpoint",
		EditModeAuto,
	)

	if got != EditModePatch {
		t.Fatalf(
			"edit mode = %q, want patch",
			got,
		)
	}
}

func TestAgentEditModeHonorsExplicitFullRewrite(t *testing.T) {
	got := agentEditModeForTask(
		"Полностью перепиши файл main.go с нуля",
		EditModeAuto,
	)

	if got != EditModeFull {
		t.Fatalf(
			"edit mode = %q, want full",
			got,
		)
	}
}

func TestAgentEditModeHonorsExplicitFullMode(t *testing.T) {
	got := agentEditModeForTask(
		"modify main.go",
		EditModeFull,
	)

	if got != EditModeFull {
		t.Fatalf(
			"edit mode = %q, want full",
			got,
		)
	}
}

func TestDetectModelProfile31B(t *testing.T) {
	cfg := config.Default()
	cfg.Model = "qwen3:31b"

	svc := &Service{
		Cfg: cfg,
	}

	if got := svc.detectModelProfile(); got != modelProfileMedium {
		t.Fatalf(
			"profile = %q, want medium",
			got,
		)
	}
}

func TestAgentDepthForTaskUsesAdaptiveThreshold(t *testing.T) {
	cfg := config.Default()
	cfg.Model = "qwen3:31b"
	cfg.AgentDeepComplexityThreshold = 3

	svc := &Service{
		Cfg: cfg,
	}

	normal := svc.agentDepthForTask(
		"add GET /health endpoint",
	)

	if normal != AgentDepthNormal {
		t.Fatalf(
			"normal task depth = %q, want normal",
			normal,
		)
	}

	deep := svc.agentDepthForTask(
		"redesign the application architecture",
	)

	if deep != AgentDepthDeep {
		t.Fatalf(
			"architectural task depth = %q, want deep",
			deep,
		)
	}
}
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
			max:  2,
		},
		{
			name: "API endpoint",
			task: "add GET /api/cars endpoint returning JSON",
			max:  2,
		},
		{
			name: "server check",
			task: "add a server health check with timeout",
			max:  2,
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
