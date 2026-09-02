package security

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSafeJoin(t *testing.T) {
	root := t.TempDir()
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"valid", "main.go", false},
		{"nested", "internal/app/main.go", false},
		{"empty", "", true},
		{"absolute", "/etc/passwd", true},
		{"traversal", "../../etc/passwd", true},
		{"dotdot only", "..", true},
		{"null byte", "main.go\x00", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := SafeJoin(root, tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("SafeJoin(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

func TestSafeJoin_SymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "evil")
	if err := os.Symlink(outside, link); err != nil {
		t.Skip("cannot create symlink")
	}
	if _, err := SafeJoin(root, "evil/secret.txt"); err == nil {
		t.Error("expected error for symlink escape")
	}
}

func TestIsInside(t *testing.T) {
	tests := []struct {
		root, path string
		want       bool
	}{
		{"/project", "/project/main.go", true},
		{"/project", "/project/sub/main.go", true},
		{"/project", "/other/main.go", false},
		{"/project", "/project/../other/main.go", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := isInside(tt.root, tt.path); got != tt.want {
				t.Errorf("isInside(%q, %q) = %v, want %v", tt.root, tt.path, got, tt.want)
			}
		})
	}
}