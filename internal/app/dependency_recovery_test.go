package app

import "testing"

func TestIsDependencyFetchError(t *testing.T) {
	tests := []struct {
		name string
		text string
		want bool
	}{
		{
			name: "ssh publickey",
			text: "git@github.com: Permission denied (publickey).",
			want: true,
		},
		{
			name: "git ls remote",
			text: "git ls-remote -q https://github.com/example/repo",
			want: true,
		},
		{
			name: "missing module",
			text: "no required module provides package github.com/foo/bar",
			want: true,
		},
		{
			name: "module does not contain package",
			text: "module example.com/foo found, but does not contain package example.com/foo/bar",
			want: true,
		},
		{
			name: "compile error",
			text: "main.go:10:2: undefined: foo",
			want: false,
		},
		{
			name: "syntax error",
			text: "main.go:5:1: syntax error",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isDependencyFetchError(tt.text)
			if got != tt.want {
				t.Fatalf(
					"isDependencyFetchError() = %v, want %v",
					got,
					tt.want,
				)
			}
		})
	}
}

func TestExtractDependencyImportPath(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "imports line",
			text: "go: example.com/app imports\n\tgithub.com/foo/bar: github.com/foo/bar@v1.0.0: reading github.com/foo/bar/go.mod",
			want: "github.com/foo/bar",
		},
		{
			name: "missing module",
			text: "no required module provides package github.com/foo/bar",
			want: "github.com/foo/bar",
		},
		{
			name: "cannot find module",
			text: "cannot find module providing package charm.land/bubbletea/v2",
			want: "charm.land/bubbletea/v2",
		},
		{
			name: "reading go.mod",
			text: "reading github.com/foo/bar/go.mod at revision v1.2.3",
			want: "github.com/foo/bar",
		},
		{
			name: "empty",
			text: "main.go:5:2: undefined: foo",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractDependencyImportPath(tt.text)
			if got != tt.want {
				t.Fatalf(
					"extractDependencyImportPath() = %q, want %q",
					got,
					tt.want,
				)
			}
		})
	}
}