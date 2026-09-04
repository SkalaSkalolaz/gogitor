package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gogitor/internal/domain"
)

func TestDiffTraceDoesNotChangePatchResult(
	t *testing.T,
) {
	content := `package main

func main() {
	println("hello")
}

`

	patch := domain.Patch{
		Search:  `println("hello")`,
		Replace: `println("world")`,
	}

	// Trace OFF.
	withoutTrace, err :=
		applyOnePatchWithPolicy(
			content,
			patch,
			PatchPolicyBalanced,
			0,
		)

	if err != nil {
		t.Fatalf(
			"trace-off patch failed: %v",
			err,
		)
	}

	// Trace ON.
	var trace []string

	ws := New(t.TempDir())

	ws.SetDiffTraceSink(
		func(message string) {
			trace = append(
				trace,
				message,
			)
		},
	)

	withTrace, err :=
		applyOnePatchWithPolicyCore(
			content,
			patch,
			PatchPolicyBalanced,
			0,
			newPatchTrace(
				ws.getDiffTraceSink(),
				"TEST",
				"main.go",
				1,
				1,
				PatchPolicyBalanced,
				patch,
			),
		)

	if err != nil {
		t.Fatalf(
			"trace-on patch failed: %v",
			err,
		)
	}

	if withoutTrace != withTrace {
		t.Fatalf(
			"DIFF result changed when tracing was enabled:\nwithout=%q\nwith=%q",
			withoutTrace,
			withTrace,
		)
	}

	if len(trace) == 0 {
		t.Fatal(
			"expected trace events when tracing is enabled",
		)
	}
}

func TestDiffTraceExactMatch(t *testing.T) {
	root := t.TempDir()
	sandbox := t.TempDir()

	rootPath := filepath.Join(root, "main.go")
	sandboxPath := filepath.Join(sandbox, "main.go")

	content := `package main

func main() {
	println("hello")
}
`

	if err := os.WriteFile(
		rootPath,
		[]byte(content),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		sandboxPath,
		[]byte(content),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	var trace []string

	ws := New(root)

	ws.SetDiffTraceSink(
		func(message string) {
			trace = append(trace, message)
		},
	)

	changes := []domain.FileChange{
		{
			Path: "main.go",
			Patches: []domain.Patch{
				{
					Search:  `println("hello")`,
					Replace: `println("world")`,
				},
			},
		},
	}

	// ------------------------------------------------------------
	// 1. Применяем patch в sandbox.
	// ------------------------------------------------------------
	if err := ws.ApplyChangesSmartWithPolicy(
		sandbox,
		changes,
		PatchPolicyBalanced,
		0,
	); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(sandboxPath)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(
		string(data),
		`println("world")`,
	) {
		t.Fatal(
			"patch was not applied to sandbox",
		)
	}

	// ------------------------------------------------------------
	// 2. Переносим уже подготовленный результат в root.
	// ------------------------------------------------------------
	if err := ws.CopyToRootSafe(
		sandbox,
		changes,
	); err != nil {
		t.Fatal(err)
	}

	data, err = os.ReadFile(rootPath)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(
		string(data),
		`println("world")`,
	) {
		t.Fatal(
			"patch was not copied to root",
		)
	}

	joined := strings.Join(
		trace,
		"\n",
	)

	if !strings.Contains(
		joined,
		"stage=EXACT",
	) {
		t.Fatalf(
			"EXACT trace not found:\n%s",
			joined,
		)
	}

	if !strings.Contains(
		joined,
		"stage=APPLY decision=OK",
	) {
		t.Fatalf(
			"APPLY success trace not found:\n%s",
			joined,
		)
	}

	if !strings.Contains(
		joined,
		"phase=SANDBOX_APPLY",
	) {
		t.Fatalf(
			"SANDBOX_APPLY trace not found:\n%s",
			joined,
		)
	}

	if !strings.Contains(
		joined,
		"phase=ROOT_APPLY",
	) {
		t.Fatalf(
			"ROOT_APPLY trace not found:\n%s",
			joined,
		)
	}
}

func TestDiffTraceDisabled(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")

	content := `package main

func main() {
	println("hello")
}
`

	if err := os.WriteFile(
		path,
		[]byte(content),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	ws := New(root)

	// Трасса выключена.
	ws.SetDiffTraceSink(nil)

	changes := []domain.FileChange{
		{
			Path: "main.go",
			Patches: []domain.Patch{
				{
					Symbol:  "main",
					Search:  `println("hello")`,
					Replace: `println("world")`,
				},
			},
		},
	}

	if err := ws.ApplyChangesSmartWithPolicy(
		root,
		changes,
		PatchPolicyBalanced,
		0,
	); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(
		string(data),
		`println("world")`,
	) {
		t.Fatal("patch was not applied")
	}
}
