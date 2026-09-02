package app

import (
	"strings"
	"testing"

	"gogitor/internal/domain"
)

func TestParseIntentResponse(t *testing.T) {
	tests := []struct {
		name     string
		response string
		wantMode string
		wantErr  bool
	}{
		{"valid", `{"mode":"code","task":"create file"}`, "code", false},
		{"markdown", "```json\n{\"mode\":\"chat\",\"task\":\"hello\"}\n```", "chat", false},
		{"no json", "just text", "", true},
		{"json in text", `Result: {"mode":"analyze","task":"review"} done`, "analyze", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intent, err := parseIntentResponse(tt.response)
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && intent.Mode != tt.wantMode {
				t.Errorf("mode = %q, want %q", intent.Mode, tt.wantMode)
			}
		})
	}
}

func TestNormalizeIntentMode(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"code", "code"}, {"coding", "code"}, {"edit", "code"}, {"modify", "code"},
		{"create", "code"}, {"refactor", "code"}, {"fix", "fix"},
		{"analyze", "analyze"}, {"analysis", "analyze"}, {"review", "analyze"},
		{"search", "search"}, {"web", "search"}, {"run", "run"}, {"execute", "run"},
		{"test", "test"}, {"tests", "test"}, {"git", "git"},
		{"computer", "computer"}, {"shell", "computer"},
		{"chat", "chat"}, {"unknown", "chat"}, {"", "chat"}, {"article", "article"},
	} {
		if got := normalizeIntentMode(tc.in); got != tc.want {
			t.Errorf("normalizeIntentMode(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestExtractTargetFiles(t *testing.T) {
	for _, tc := range []struct {
		query string
		want  int
	}{
		{"create main.go", 1},
		{"modify internal/app/app.go and util.go", 2},
		{"no files here", 0},
		{"create index.html and style.css", 2},
	} {
		if got := extractTargetFiles(tc.query); len(got) != tc.want {
			t.Errorf("extractTargetFiles(%q) = %v, want %d items", tc.query, got, tc.want)
		}
	}
}

func TestHasKnownFileExtension(t *testing.T) {
	for _, tc := range []struct {
		word string
		want bool
	}{
		{"main.go", true}, {"file.txt", true}, {"script.sh", true},
		{"style.css", true}, {"index.html", true}, {"data.json", true},
		{"noext", false}, {"file.unknown", false}, {"", false},
	} {
		if got := hasKnownFileExtension(tc.word); got != tc.want {
			t.Errorf("hasKnownFileExtension(%q) = %v, want %v", tc.word, got, tc.want)
		}
	}
}

func TestDefaultPath(t *testing.T) {
	if got := defaultPath("create", []string{"main.go"}); got != "main.go" {
		t.Errorf("got %q", got)
	}
	if got := defaultPath("create html page", nil); got != "index.html" {
		t.Errorf("got %q", got)
	}
	if got := defaultPath("create program", nil); got != "main.go" {
		t.Errorf("got %q", got)
	}
}

func TestContainsAny(t *testing.T) {
	if !containsAny("hello world", []string{"world", "foo"}) {
		t.Error("expected true")
	}
	if containsAny("hello world", []string{"foo", "bar"}) {
		t.Error("expected false")
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("short = %q", got)
	}
	long := strings.Repeat("a", 100)
	if got := truncate(long, 10); len([]rune(got)) > 13 {
		t.Errorf("long too long: %d", len([]rune(got)))
	}
}

func TestCleanCommitMessage(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"valid", "feat(auth): add login", "feat(auth): add login"},
		{"quotes", `"feat: add login"`, "feat: add login"},
		{"fence", "```\nfeat: add login\n```", "feat: add login"},
		{"prefix", "Commit message: feat: add login", "feat: add login"},
		{"invalid type", "updated the code", "chore: updated the code"},
		{"empty", "", ""},
		{"too short", "ab", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := cleanCommitMessage(tc.in); got != tc.want {
				t.Errorf("cleanCommitMessage(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestIsAnalysisOnlyTask(t *testing.T) {
	for _, tc := range []struct {
		q    string
		want bool
	}{
		{"analyze the code", true},
		{"проанализируй код", true},
		{"explain this function", true},
		{"create a new file", false},
		{"analyze and create a file", false},
		{"find bugs and fix them", false},
	} {
		if got := isAnalysisOnlyTask(tc.q); got != tc.want {
			t.Errorf("isAnalysisOnlyTask(%q) = %v, want %v", tc.q, got, tc.want)
		}
	}
}

func TestIsSplitOrRefactor(t *testing.T) {
	for _, tc := range []struct {
		q    string
		want bool
	}{
		{"split the file", true}, {"раздели код", true},
		{"refactor the function", true}, {"extract the helper", true},
		{"create a new file", false}, {"fix the bug", false},
	} {
		if got := isSplitOrRefactor(tc.q); got != tc.want {
			t.Errorf("isSplitOrRefactor(%q) = %v, want %v", tc.q, got, tc.want)
		}
	}
}

func TestNormalizeGitSubcommand(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"status", "status"}, {"diff", "diff"}, {"commit", "commit"},
		{"push", "push"}, {"pull", "pull"},
		{"запушь изменения", "push"}, {"стяни изменения", "pull"},
		{"слить ветку", "merge"}, {"создай репозиторий", "create"}, {"", ""},
	} {
		if got := normalizeGitSubcommand(tc.in); got != tc.want {
			t.Errorf("normalizeGitSubcommand(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseCommitSplitArgs(t *testing.T) {
	for _, tc := range []struct {
		name      string
		args      []string
		wantFiles int
		wantSplit bool
	}{
		{"split files", []string{"--split", "main.go,util.go"}, 2, true},
		{"split no files", []string{"--split"}, 0, true},
		{"per-file", []string{"--per-file", "main.go"}, 1, true},
		{"equals", []string{"--split=main.go,util.go"}, 2, true},
		{"no split", []string{"-m", "msg"}, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files, has := ParseCommitSplitArgs(tc.args)
			if has != tc.wantSplit || len(files) != tc.wantFiles {
				t.Errorf("files=%v has=%v", files, has)
			}
		})
	}
}

func TestAllocateContextBudget(t *testing.T) {
	a := AllocateContextBudget(100000)
	sum := a.TaskBytes + a.HistoryBytes + a.ProjectSummaryBytes +
		a.PrimaryFilesBytes + a.RelatedFilesBytes + a.TestFilesBytes +
		a.GitDiffBytes + a.ReserveBytes
	if sum > a.TotalBytes {
		t.Errorf("sum %d > total %d", sum, a.TotalBytes)
	}
	if a.TaskBytes <= 0 || a.PrimaryFilesBytes <= 0 {
		t.Error("key allocations should be positive")
	}
	if a2 := AllocateContextBudget(0); a2.TotalBytes != 120000 {
		t.Errorf("default = %d", a2.TotalBytes)
	}
}

func TestSanitizeVerification(t *testing.T) {
	t.Run("runtime removed", func(t *testing.T) {
		v := &agentVerification{Completed: false, Missing: []string{"execute the script"}}
		sanitizeVerification(v)
		if !v.Completed {
			t.Error("should be completed")
		}
	})
	t.Run("file preserved", func(t *testing.T) {
		v := &agentVerification{Completed: false, Missing: []string{"file was not created: main.go"}}
		sanitizeVerification(v)
		if v.Completed {
			t.Error("should not be completed")
		}
	})
	t.Run("nil safe", func(t *testing.T) { sanitizeVerification(nil) })
}

func TestIsRuntimeOnlyVerificationItem(t *testing.T) {
	for _, tc := range []struct {
		item string
		want bool
	}{
		{"execute the script", true}, {"run the program", true},
		{"chmod +x script.sh", true},
		{"file was not created", false}, {"missing file main.go", false},
	} {
		if got := isRuntimeOnlyVerificationItem(tc.item); got != tc.want {
			t.Errorf("isRuntimeOnlyVerificationItem(%q) = %v, want %v", tc.item, got, tc.want)
		}
	}
}

func TestIsRuntimeOnlySubtask(t *testing.T) {
	for _, tc := range []struct {
		task string
		want bool
	}{
		{"chmod +x script.sh", true}, {"запусти программу", true},
		{"run the server", true},
		{"создай файл main.go", false}, {"create file script.sh", false},
	} {
		if got := isRuntimeOnlySubtask(tc.task); got != tc.want {
			t.Errorf("isRuntimeOnlySubtask(%q) = %v, want %v", tc.task, got, tc.want)
		}
	}
}

func TestExecutableScriptNames(t *testing.T) {
	names := executableScriptNames([]string{"main.go", "script.sh"}, []string{"run.bash", "data.json"})
	if len(names) != 2 {
		t.Fatalf("expected 2, got %d: %v", len(names), names)
	}
}

func TestParseAgentLessons(t *testing.T) {
	lessons := parseAgentLessons(
		"text\nLESSON: Rule one\nLESSON: Rule two\nnot a lesson",
	)

	if len(lessons) != 2 {
		t.Fatalf(
			"expected 2, got %d",
			len(lessons),
		)
	}

	if len(parseAgentLessons("no lessons")) != 0 {
		t.Error("expected 0")
	}
}

func TestMergeOutputFiles(t *testing.T) {
	base := []domain.OutputFile{{Path: "a.go", Content: "old"}}
	add := []domain.OutputFile{{Path: "a.go", Content: "new"}, {Path: "b.go", Content: "b"}}
	result := mergeOutputFiles(base, add)
	if len(result) != 2 {
		t.Fatalf("expected 2, got %d", len(result))
	}
	if result[0].Content != "new" {
		t.Errorf("a.go = %q", result[0].Content)
	}
}

func TestAppendUniqueStrings(t *testing.T) {
	result := appendUniqueStrings([]string{"a", "b"}, "b", "c", "a", "d")
	if len(result) != 4 {
		t.Fatalf("expected 4, got %d", len(result))
	}
}

func TestStringSet(t *testing.T) {
	set := stringSet([]string{"a", "b", "a"})
	if len(set) != 2 || !set["a"] || !set["b"] {
		t.Errorf("set = %v", set)
	}
}

func TestSortedKeys(t *testing.T) {
	keys := sortedKeys(map[string]bool{"c": true, "a": true, "b": true})
	if len(keys) != 3 || keys[0] != "a" || keys[1] != "b" || keys[2] != "c" {
		t.Errorf("keys = %v", keys)
	}
}
