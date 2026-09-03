package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTestPackagesRunsAffectedPackage(t *testing.T) {
	root := t.TempDir()

	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/testpackages\n\ngo 1.25\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	pkgDir := filepath.Join(
		root,
		"internal",
		"foo",
	)

	if err := os.MkdirAll(
		pkgDir,
		0o755,
	); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		filepath.Join(pkgDir, "foo.go"),
		[]byte(`package foo

func Add(a, b int) int {
	return a + b
}
`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		filepath.Join(pkgDir, "foo_test.go"),
		[]byte(`package foo

import "testing"

func TestAdd(t *testing.T) {
	if got := Add(2, 3); got != 5 {
		t.Fatalf("Add() = %d, want 5", got)
	}
}
`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	r := New(
		15*time.Second,
		nil,
	)

	ctx, cancel :=
		context.WithTimeout(
			context.Background(),
			15*time.Second,
		)
	defer cancel()

	status, err := r.TestPackages(
		ctx,
		root,
		[]string{"internal/foo"},
	)

	if err != nil {
		t.Fatalf(
			"TestPackages returned error: %v",
			err,
		)
	}

	if !status.Run {
		t.Fatal("expected targeted tests to run")
	}

	if status.Failed != 0 {
		t.Fatalf(
			"targeted tests failed: %d\n%s",
			status.Failed,
			status.Output,
		)
	}

	if status.Passed == 0 {
		t.Fatalf(
			"expected at least one passed test\n%s",
			status.Output,
		)
	}

	if !strings.Contains(
		status.Output,
		"PACKAGE ./internal/foo",
	) {
		t.Fatalf(
			"unexpected TestPackages output: %s",
			status.Output,
		)
	}
}
