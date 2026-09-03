package app

import "testing"

func TestClassifyAutoResearch(t *testing.T) {
	tests := []struct {
		name string
		task string
		err  string
		want AutoResearchKind
	}{
		{
			name: "dependency",
			err: "git@github.com: Permission denied (publickey).",
			want: AutoResearchDependency,
		},
		{
			name: "security",
			task: "check CVE-2026-12345 in this dependency",
			want: AutoResearchSecurity,
		},
		{
			name: "migration",
			task: "migrate this library from v1 to v2",
			want: AutoResearchMigration,
		},
		{
			name: "toolchain",
			task: "module requires Go 1.27",
			want: AutoResearchToolchain,
		},
		{
			name: "version",
			task: "which current version of this library is compatible?",
			want: AutoResearchVersion,
		},
		{
			name: "lint",
			task: "fix golangci-lint SA1019",
			want: AutoResearchLint,
		},
		{
			name: "performance",
			task: "optimize allocations and benchmark throughput",
			want: AutoResearchPerformance,
		},
		{
			name: "documentation",
			task: "update README with current API documentation",
			want: AutoResearchDocumentation,
		},
		{
			name: "api",
			task: "use github.com/foo/sdk API client endpoint",
			want: AutoResearchAPI,
		},
		{
			name: "library",
			task: "use external library github.com/foo/bar",
			want: AutoResearchLibrary,
		},
		{
			name: "local code",
			task: "rename this local function",
			want: AutoResearchGeneral,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyAutoResearch(
				tt.task,
				tt.err,
			)

			if got != tt.want {
				t.Fatalf(
					"classifyAutoResearch() = %q, want %q",
					got,
					tt.want,
				)
			}
		})
	}
}

func TestShouldAutoResearchCodeTask(t *testing.T) {
	tests := []struct {
		task string
		want bool
	}{
		{
			task: "rename local function ParseConfig to LoadConfig",
			want: false,
		},
		{
			task: "create a TUI using github.com/charmbracelet/bubbletea",
			want: true,
		},
		{
			task: "migrate library from v1 to v2",
			want: true,
		},
		{
			task: "fix CVE-2026-12345",
			want: true,
		},
		{
			task: "update README with current API docs",
			want: true,
		},
		{
			task: "=== UNTRUSTED AUTO-RESEARCH ===\npackage docs",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.task, func(t *testing.T) {
			got := shouldAutoResearchCodeTask(tt.task)

			if got != tt.want {
				t.Fatalf(
					"shouldAutoResearchCodeTask() = %v, want %v",
					got,
					tt.want,
				)
			}
		})
	}
}

func TestShouldAutoResearchArticle(t *testing.T) {
	if !shouldAutoResearchArticle(
		"current Go HTTP API documentation",
		"technical",
	) {
		t.Fatal("expected technical article to require research")
	}

	if shouldAutoResearchArticle(
		"fictional story about a programmer",
		"story",
	) {
		t.Fatal("story should not require research")
	}

	if !shouldAutoResearchArticle(
		"latest Go release",
		"news",
	) {
		t.Fatal("news should require research")
	}
}