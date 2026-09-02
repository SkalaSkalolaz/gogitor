package git

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

func TestInjectToken(t *testing.T) {
	for _, tc := range []struct{ name, url, token, contains string }{
		{"https", "https://github.com/user/repo", "ghp_tok", "x-access-token:ghp_tok@github.com"},
		{"ssh", "git@github.com:user/repo", "ghp_tok", "x-access-token:ghp_tok@github.com"},
		{"empty token", "https://github.com/user/repo", "", "https://github.com/user/repo"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := InjectToken(tc.url, tc.token)
			if !strings.Contains(got, tc.contains) {
				t.Errorf("InjectToken(%q) = %q, should contain %q", tc.url, got, tc.contains)
			}
		})
	}
}

func TestWithAuthenticatedRemoteBuildsURLRewrite(t *testing.T) {
	ctx := context.Background()

	dir := t.TempDir()
	g := New(dir, nil)

	// Создаём минимальный git-репозиторий.
	if _, err := exec.Command("git", "-C", dir, "init").CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v", err)
	}

	cmds := [][]string{
		{"remote", "add", "origin", "git@github.com:user/repo.git"},
		{"config", "remote.origin.pushurl", "https://github.com/user/repo.git"},
	}

	for _, args := range cmds {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	_, err := g.WithAuthenticatedRemote(
		ctx,
		"origin",
		"ghp_test_token",
		func() (string, error) {
			// Проверяем временное переопределение fetch URL через Git config.
			fetchURL, err := g.run(
				ctx,
				"config",
				"--get",
				"remote.origin.url",
			)
			if err != nil {
				return "", err
			}

			wantFetch := "https://x-access-token:ghp_test_token@github.com/user/repo.git"
			if strings.TrimSpace(fetchURL) != wantFetch {
				return "", fmt.Errorf(
					"unexpected authenticated fetch URL: %q, want %q",
					strings.TrimSpace(fetchURL),
					wantFetch,
				)
			}

			// Проверяем временное переопределение push URL через Git config.
			pushURL, err := g.run(
				ctx,
				"config",
				"--get",
				"remote.origin.pushurl",
			)
			if err != nil {
				return "", err
			}

			wantPush := "https://x-access-token:ghp_test_token@github.com/user/repo.git"
			if strings.TrimSpace(pushURL) != wantPush {
				return "", fmt.Errorf(
					"unexpected authenticated push URL: %q, want %q",
					strings.TrimSpace(pushURL),
					wantPush,
				)
			}

			return "ok", nil
		},
	)

	if err != nil {
		t.Fatal(err)
	}
}

func TestGitCommandEnv(t *testing.T) {
	env := gitCommandEnv("EXTRA=value")
	for _, kv := range env {
		if strings.HasPrefix(kv, "GIT_TERMINAL_PROMPT=") && kv != "GIT_TERMINAL_PROMPT=0" {
			t.Error("GIT_TERMINAL_PROMPT should be 0")
		}
	}
	found := false
	for _, kv := range env {
		if kv == "EXTRA=value" {
			found = true
		}
	}
	if !found {
		t.Error("extra var not found")
	}
}

func TestGitCommandEnv_RemovesConfig(t *testing.T) {
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "test")
	env := gitCommandEnv()
	for _, kv := range env {
		if strings.HasPrefix(kv, "GIT_CONFIG_COUNT=") || strings.HasPrefix(kv, "GIT_CONFIG_KEY_") {
			t.Errorf("should be removed: %q", kv)
		}
	}
}
