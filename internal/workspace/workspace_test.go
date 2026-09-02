package workspace

import (
	"testing"
)

func TestShouldSkipDir(t *testing.T) {
	for _, tc := range []struct {
		name string
		want bool
	}{
		{".git", true}, {".gogitor", true}, {".idea", true},
		{".vscode", true}, {"node_modules", true},
		{"src", false}, {"internal", false},
	} {
		if got := shouldSkipDir(tc.name); got != tc.want {
			t.Errorf("shouldSkipDir(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestShouldSkipFile(t *testing.T) {
	for _, tc := range []struct {
		name string
		want bool
	}{
		{".DS_Store", true}, {"file.gogitor.bak", true}, {"file.gogitor.tmp", true},
		{"main.go", false}, {"README.md", false},
	} {
		if got := shouldSkipFile(tc.name); got != tc.want {
			t.Errorf("shouldSkipFile(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestIsExecutableScriptPath(t *testing.T) {
	for _, tc := range []struct {
		path string
		want bool
	}{
		{"script.sh", true}, {"script.bash", true}, {"script.zsh", true},
		{"script.fish", true}, {"script.command", true},
		{"main.go", false}, {"data.json", false},
		{"SCRIPT.SH", true},
	} {
		if got := isExecutableScriptPath(tc.path); got != tc.want {
			t.Errorf("isExecutableScriptPath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestShouldWatchSkipDir(t *testing.T) {
	for _, tc := range []struct {
		name string
		want bool
	}{
		{".git", true}, {"vendor", true}, {"src", false},
	} {
		if got := shouldWatchSkipDir(tc.name); got != tc.want {
			t.Errorf("shouldWatchSkipDir(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}