package app

import (
	// "context"
	// "crypto/sha256"
	// "fmt"
	// "io"
	// "os"
	// "path/filepath"
	// "sort"
	// "strings"
	// "time"
// 
	// "gogitor/internal/security"
	"testing"
)

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

	checkpointDir := t.TempDir()

	if err := copyDir(
		context.Background(),
		root,
		checkpointDir,
	); err != nil {
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

	svc := &Service{
		Cfg: &config.Config{
			WorkDir: root,
		},
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