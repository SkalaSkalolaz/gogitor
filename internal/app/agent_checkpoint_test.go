package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gogitor/internal/config"
	"gogitor/internal/workspace"
)

func TestProjectInstructions(t *testing.T) {
	root := t.TempDir()

	cfg := config.Default()
	cfg.WorkDir = root

	svc := &Service{
		Cfg: cfg,
	}

	data := []byte(
		"# Rules\n\nUse Go.\n",
	)

	if err := os.WriteFile(
		filepath.Join(root, ".gogitor.md"),
		data,
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	got := svc.projectInstructions()

	if !strings.Contains(
		got,
		"Use Go.",
	) {
		t.Fatalf(
			"unexpected instructions: %q",
			got,
		)
	}
}

func TestAgentModelCapabilities(t *testing.T) {
	cfg := config.Default()

	cfg.Provider = "ollama"
	cfg.Model = "qwen3.8:27b"

	cfg.AgentModelCapabilities =
		map[string]config.AgentModelCapability{
			"qwen3.8:27b": {
				PreferredDepth: "deep",
				ContextTokens:  131072,
				PatchPolicy:    "strict",
				MaxSubtasks:    6,
			},
		}

	svc := &Service{
		Cfg: cfg,
	}

	caps := svc.agentModelCapabilities()

	if caps.PreferredDepth !=
		AgentDepthDeep {
		t.Fatal(
			"expected deep preference",
		)
	}

	if caps.ContextTokens !=
		131072 {
		t.Fatal(
			"unexpected context limit",
		)
	}

	if !caps.HasPatchPolicy {
		t.Fatal(
			"expected patch policy",
		)
	}

	if caps.MaxSubtasks != 6 {
		t.Fatal(
			"unexpected max subtasks",
		)
	}
}

func TestRollbackAgentCheckpointRestoresActualTree(
	t *testing.T,
) {
	root := t.TempDir()

	original := filepath.Join(
		root,
		"main.go",
	)

	if err := os.WriteFile(
		original,
		[]byte("package main\n"),
		0o755,
	); err != nil {
		t.Fatal(err)
	}

	svc := &Service{
		Cfg: &config.Config{
			WorkDir: root,
		},
		WS: workspace.New(root),
	}
	defer svc.WS.Close()

	checkpointDir, err := svc.WS.PrepareSandbox(
		context.Background(),
	)
	if err != nil {
		t.Fatal(err)
	}
	// Меняем существующий файл.
	if err := os.WriteFile(
		original,
		[]byte("package main\n\n// changed\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	// Создаём новый файл.
	newFile := filepath.Join(
		root,
		"new.go",
	)

	if err := os.WriteFile(
		newFile,
		[]byte("package main\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	// Удаляем существовавший файл.
	deleted := filepath.Join(
		root,
		"deleted.go",
	)

	if err := os.WriteFile(
		deleted,
		[]byte("package main\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(deleted); err != nil {
		t.Fatal(err)
	}

	cp := &agentCheckpoint{
		Dir: checkpointDir,
	}

	if err := svc.rollbackAgentCheckpoint(
		cp,
		nil,
		nil,
	); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(
		original,
	)
	if err != nil {
		t.Fatal(err)
	}

	if string(data) != "package main\n" {
		t.Fatalf(
			"original file was not restored: %q",
			string(data),
		)
	}

	if _, err := os.Stat(newFile); !os.IsNotExist(err) {
		t.Fatalf(
			"new file still exists",
		)
	}
}

func TestBuildAgentJSONRepairPrompt(
	t *testing.T,
) {
	prompt := buildAgentJSONRepairPrompt(
		"review changes",
		`return {"approved": true}`,
		"Here is JSON: {bad}",
		fmt.Errorf(
			"unexpected EOF",
		),
	)

	for _, want := range []string{
		"Return ONLY one valid JSON object",
		"review changes",
		"unexpected EOF",
	} {
		if !strings.Contains(
			prompt,
			want,
		) {
			t.Fatalf(
				"repair prompt missing %q",
				want,
			)
		}
	}
}
