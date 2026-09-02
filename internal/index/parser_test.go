package index

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseFile(t *testing.T) {
	dir := t.TempDir()
	src := `package main

import (
	"fmt"
	"os"
)

// Hello prints a greeting.
func Hello(name string) {
	fmt.Println("Hello,", name)
}

type Server struct {
	Port int
}

func (s *Server) Start() error {
	return nil
}

const MaxRetries = 3

var DefaultPort = 8080
`
	path := filepath.Join(dir, "main.go")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	fi, err := parseFile(path, "main.go")
	if err != nil {
		t.Fatal(err)
	}
	if fi.Package != "main" {
		t.Errorf("package = %q", fi.Package)
	}
	if len(fi.Imports) != 2 {
		t.Errorf("imports = %d", len(fi.Imports))
	}
	if fi.IsTest {
		t.Error("main.go is not test")
	}
	symbols := map[string]Symbol{}
	for _, s := range fi.Symbols {
		symbols[s.Name] = s
	}
	if s, ok := symbols["Hello"]; !ok || s.Kind != KindFunc || s.Doc == "" {
		t.Error("Hello not found or wrong")
	}
	if s, ok := symbols["Start"]; !ok || s.Kind != KindMethod || s.Receiver != "Server" {
		t.Error("Start not found or wrong")
	}
	if s, ok := symbols["Server"]; !ok || s.Kind != KindStruct {
		t.Error("Server not found or wrong")
	}
	if s, ok := symbols["MaxRetries"]; !ok || s.Kind != KindConst {
		t.Error("MaxRetries not found or wrong")
	}
	if s, ok := symbols["DefaultPort"]; !ok || s.Kind != KindVar {
		t.Error("DefaultPort not found or wrong")
	}
	if len(fi.Calls) == 0 {
		t.Error("expected calls")
	}
}

func TestParseFile_TestFile(t *testing.T) {
	dir := t.TempDir()
	src := "package main\n\nimport \"testing\"\n\nfunc TestHello(t *testing.T) {}\n"
	path := filepath.Join(dir, "main_test.go")
	os.WriteFile(path, []byte(src), 0o644)
	fi, err := parseFile(path, "main_test.go")
	if err != nil {
		t.Fatal(err)
	}
	if !fi.IsTest {
		t.Error("should be test file")
	}
}