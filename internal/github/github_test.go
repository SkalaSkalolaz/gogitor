package github

import (
	"strings"
	"testing"
)

func TestParseRepoURL(t *testing.T) {
	tests := []struct {
		url       string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{"https://github.com/user/repo", "user", "repo", false},
		{"https://github.com/user/repo.git", "user", "repo", false},
		{"https://github.com/user/repo/", "user", "repo", false},
		{"git@github.com:user/repo.git", "user", "repo", false},
		{"git@github.com:user/repo", "user", "repo", false},
		{"http://github.com/user/repo", "user", "repo", false},
		{"https://www.github.com/user/repo", "user", "repo", false},
		{"", "", "", true},
		{"not-a-url", "", "", true},
		{"https://gitlab.com/user/repo", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			owner, repo, err := ParseRepoURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if owner != tt.wantOwner || repo != tt.wantRepo {
				t.Errorf("got (%q, %q), want (%q, %q)", owner, repo, tt.wantOwner, tt.wantRepo)
			}
		})
	}
}

func TestTokenType(t *testing.T) {
	tests := []struct{ token, want string }{
		{"ghp_abc123", "classic PAT"},
		{"github_pat_abc123", "fine-grained PAT"},
		{"gho_abc123", "OAuth"},
		{"ghs_abc123", "GitHub App (server-to-server)"},
		{"ghu_abc123", "GitHub App (user)"},
		{"", "none"},
		{"random_token", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.token, func(t *testing.T) {
			if got := TokenType(tt.token); got != tt.want {
				t.Errorf("TokenType(%q) = %q, want %q", tt.token, got, tt.want)
			}
		})
	}
}

func TestSnippet(t *testing.T) {
	if got := snippet([]byte("hello")); got != "hello" {
		t.Errorf("short = %q", got)
	}
	long := strings.Repeat("a", 1000)
	if got := snippet([]byte(long)); len([]rune(got)) > 503 {
		t.Errorf("long snippet too long: %d runes", len([]rune(got)))
	}
}