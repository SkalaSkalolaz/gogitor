package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gogitor/internal/domain"
)

func TestSaveResultToFile_Text(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.md")
	res := domain.Result{Success: true, Response: "# Test\n\nHello"}
	if err := SaveResultToFile(res, path); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "Hello") {
		t.Error("missing content")
	}
}

func TestSaveResultToFile_JSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.json")
	res := domain.Result{Success: true, Mode: "test"}
	if err := SaveResultToFile(res, path); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `"mode": "test"`) {
		t.Error("missing JSON content")
	}
}

func TestSaveResultToFile_Go(t *testing.T) {
	path := filepath.Join(t.TempDir(), "main.go")
	res := domain.Result{Success: true, OutputFiles: []domain.OutputFile{{Path: "main.go", Content: "package main"}}}
	if err := SaveResultToFile(res, path); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "package main") {
		t.Error("missing Go content")
	}
}

func TestSaveResultToFile_CreatesDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "out.txt")
	if err := SaveResultToFile(domain.Result{Success: true, Response: "x"}, path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("file not created")
	}
}

func TestModelParameterCountB(t *testing.T) {
	tests := []struct {
		model string
		want  float64
	}{
		{"qwen3-coder:8b", 8},
		{"model:10b", 10},
		{"model:15b", 15},
		{"model:20b", 20},
		{"model:29b", 29},
		{"model:30b", 30},
		{"model", 0},
		{"", 0},
	}

	for _, tt := range tests {
		got := modelParameterCountB(tt.model)
		if got != tt.want {
			t.Errorf(
				"modelParameterCountB(%q) = %v, want %v",
				tt.model,
				got,
				tt.want,
			)
		}
	}
}
