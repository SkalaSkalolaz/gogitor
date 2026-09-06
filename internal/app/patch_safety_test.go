package app

import (
	"testing"
	"os"
	"path/filepath"

	"gogitor/internal/config"
	"gogitor/internal/workspace"
)

func TestAssessAgentSubtaskAlreadySatisfiedField(t *testing.T) {
	root := t.TempDir()

	source := `package repository

type Paste struct {
	ID        string
	ExpiresAt time.Time
}
`

	path := filepath.Join(root, "paste.go")
	if err := os.WriteFile(
		path,
		[]byte(source),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.WorkDir = root

	svc := &Service{
		Cfg: cfg,
		WS:  workspace.New(root),
	}

	got := svc.assessAgentSubtask(
		fullPlanSubtask{
			Task: "Добавить поле ExpiresAt в Paste",
		},
	)

	if got.State != agentSubtaskAlreadySatisfied {
		t.Fatalf(
			"state = %q, want already_satisfied; reason=%s",
			got.State,
			got.Reason,
		)
	}
}

func TestAssessAgentSubtaskAlreadySatisfiedMethod(
	t *testing.T,
) {
	root := t.TempDir()

	source := `package repository

type Repository struct{}

func (r *Repository) DeleteExpired(now time.Time) int {
	return 0
}
`

	path := filepath.Join(root, "repository.go")
	if err := os.WriteFile(
		path,
		[]byte(source),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.WorkDir = root

	svc := &Service{
		Cfg: cfg,
		WS:  workspace.New(root),
	}

	got := svc.assessAgentSubtask(
		fullPlanSubtask{
			Task: "Добавить метод DeleteExpired",
		},
	)

	if got.State != agentSubtaskAlreadySatisfied {
		t.Fatalf(
			"state = %q, want already_satisfied",
			got.State,
		)
	}
}

func TestAssessAgentSubtaskAlreadySatisfiedTicker(
	t *testing.T,
) {
	root := t.TempDir()

	source := `package main

import (
	"time"
)

func main() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
}
`

	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(
		path,
		[]byte(source),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.WorkDir = root

	svc := &Service{
		Cfg: cfg,
		WS:  workspace.New(root),
	}

	got := svc.assessAgentSubtask(
		fullPlanSubtask{
			Task: "Добавить фоновую горутину с time.NewTicker",
		},
	)

	if got.State != agentSubtaskAlreadySatisfied {
		t.Fatalf(
			"state = %q, want already_satisfied",
			got.State,
		)
	}
}

func TestRecoverableAgentSubtaskFailure(t *testing.T) {
	tests := []struct {
		name string
		errs []string
		want bool
	}{
		{
			name: "invalid patch",
			errs: []string{
				"LLM did not return a valid SEARCH/REPLACE patch",
			},
			want: true,
		},
		{
			name: "no op",
			errs: []string{
				"patch_error_code=no_op_patch",
			},
			want: true,
		},
		{
			name: "compile failure",
			errs: []string{
				"undefined: someFunction",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRecoverableAgentSubtaskFailure(
				tt.errs,
			)

			if got != tt.want {
				t.Fatalf(
					"isRecoverableAgentSubtaskFailure() = %v, want %v",
					got,
					tt.want,
				)
			}
		})
	}
}



func TestShouldAuditPatch(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		protocol workspace.PatchProtocol
		deep     bool
		want     bool
	}{
		{
			name:     "always",
			mode:     "always",
			protocol: workspace.PatchProtocolSearchReplace,
			want:     true,
		},
		{
			name:     "off",
			mode:     "off",
			protocol: workspace.PatchProtocolReplaceOnly,
			want:     false,
		},
		{
			name:     "auto deep",
			mode:     "auto",
			protocol: workspace.PatchProtocolSearchReplace,
			deep:     true,
			want:     true,
		},
		{
			name:     "auto replace only",
			mode:     "auto",
			protocol: workspace.PatchProtocolReplaceOnly,
			deep:     false,
			want:     true,
		},
		{
			name:     "auto ordinary shallow",
			mode:     "auto",
			protocol: workspace.PatchProtocolSearchReplace,
			deep:     false,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldAuditPatch(
				tt.mode,
				tt.protocol,
				tt.deep,
			)

			if got != tt.want {
				t.Fatalf(
					"shouldAuditPatch() = %v, want %v",
					got,
					tt.want,
				)
			}
		})
	}
}
