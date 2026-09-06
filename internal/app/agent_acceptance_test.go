package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInferAgentAcceptanceProfileArchitecture(t *testing.T) {
	profile := inferAgentAcceptanceProfile(
		"Проведи рефакторинг: HTTP handlers должны быть только HTTP-слоем, business logic должна находиться в service layer, repository отвечает только за storage. Сохрани все endpoint и тесты.",
		nil,
	)

	if !profile.ArchitectureTask {
		t.Fatal("expected architecture task")
	}

	if !profile.RequireServiceLayer {
		t.Fatal("expected service-layer requirement")
	}

	if !profile.PreserveTests {
		t.Fatal("expected preserve-tests requirement")
	}

	if !profile.PreserveRoutes {
		t.Fatal("expected preserve-routes requirement")
	}

	if !profile.RequireVerifierChecks {
		t.Fatal("expected structured verifier checks")
	}
}

func TestInferAgentAcceptanceProfileSimpleTask(t *testing.T) {
	profile := inferAgentAcceptanceProfile(
		"Добавь функцию ParseConfig()",
		nil,
	)

	if profile.ArchitectureTask {
		t.Fatal("simple function task should not be architecture task")
	}

	if profile.RequireVerifierChecks {
		t.Fatal("simple task should not require structured architecture verification")
	}
}

func TestCompareAgentTestInventoryDetectsRemovedTest(
	t *testing.T,
) {
	root := t.TempDir()

	testFile := filepath.Join(
		root,
		"main_test.go",
	)

	err := os.WriteFile(
		testFile,
		[]byte(`package main

import "testing"

func TestCreate(t *testing.T) {}
func TestGet(t *testing.T) {}
`),
		0o644,
	)

	if err != nil {
		t.Fatal(err)
	}

	before, err :=
		collectAgentTestInventory(root)

	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(
		testFile,
		[]byte(`package main

import "testing"

func TestCreate(t *testing.T) {}
`),
		0o644,
	)

	if err != nil {
		t.Fatal(err)
	}

	missing, err :=
		compareAgentTestInventory(
			before,
			root,
		)

	if err != nil {
		t.Fatal(err)
	}

	if len(missing) != 1 {
		t.Fatalf(
			"missing = %v, want one removed test",
			missing,
		)
	}
}
