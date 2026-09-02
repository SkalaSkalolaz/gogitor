package git

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type Git struct {
	Dir string
	Log *slog.Logger

	authMu sync.Mutex
	authEnv atomic.Value // []string
}

func New(dir string, log *slog.Logger) *Git {
	g := &Git{
		Dir: dir,
		Log: log,
	}
	g.authEnv.Store([]string(nil))
	return g
}

func (g *Git) IsRepo(ctx context.Context) bool {
	out, err := g.run(ctx, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) == "true"
}

func (g *Git) Init(ctx context.Context) error {
	_, err := g.run(ctx, "init")
	if err != nil {
		return err
	}
	// Создаём/дополняем .gitignore, чтобы .gogitor/ не попадал в коммиты
	g.ensureGitignore(ctx)
	return nil
}

func (g *Git) ensureGitignore(ctx context.Context) {
    gitignorePath := filepath.Join(g.Dir, ".gitignore")
    entries := []string{".gogitor/", ".gogitor.json"}
    
    data, err := os.ReadFile(gitignorePath)
    if err != nil {
        _ = os.WriteFile(gitignorePath, []byte(strings.Join(entries, "\n")+"\n"), 0o644)
        return
    }
    
    content := string(data)
    var missing []string
    for _, e := range entries {
        if !strings.Contains(content, e) {
            missing = append(missing, e)
        }
    }
    if len(missing) == 0 {
        return
    }
    f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_WRONLY, 0o644)
    if err != nil {
        return
    }
    defer f.Close()
    _, _ = f.WriteString("\n" + strings.Join(missing, "\n") + "\n")
}

func (g *Git) EnsureRepo(ctx context.Context, autoInit bool) error {
	if g.IsRepo(ctx) {
		return nil
	}

	if !autoInit {
		return fmt.Errorf("not a git repository")
	}

	return g.Init(ctx)
}

func (g *Git) Status(ctx context.Context) (string, error) {
	return g.run(ctx, "status", "--short")
}

func (g *Git) Diff(ctx context.Context) (string, error) {
	return g.run(ctx, "diff")
}

func (g *Git) LogHistory(ctx context.Context, maxCount int) (string, error) {
	if maxCount <= 0 {
		maxCount = 20
	}
	return g.run(ctx, "log",
		"--oneline",
		"--decorate",
		"--date=short",
		"--pretty=format:%h %ad %s",
		fmt.Sprintf("-n%d", maxCount),
	)
}

func (g *Git) Checkout(ctx context.Context, hash string) (string, error) {
	return g.run(ctx, "checkout", hash)
}

// BranchList возвращает список локальных веток.
func (g *Git) BranchList(ctx context.Context) (string, error) {
	return g.run(ctx, "branch", "--list", "--no-color")
}

// BranchCreate создаёт новую ветку.
func (g *Git) BranchCreate(ctx context.Context, name string) (string, error) {
	return g.run(ctx, "branch", name)
}

// BranchDelete удаляет ветку. Если force=true, удаляет даже без мёржа.
func (g *Git) BranchDelete(ctx context.Context, name string, force bool) (string, error) {
	flag := "-d"
	if force {
		flag = "-D"
	}
	return g.run(ctx, "branch", flag, name)
}

// CheckoutNew создаёт ветку и сразу переключается на неё.
func (g *Git) CheckoutNew(ctx context.Context, name string) (string, error) {
	return g.run(ctx, "checkout", "-b", name)
}

// Merge сливает указанную ветку в текущую.
func (g *Git) Merge(ctx context.Context, branch string) (string, error) {
	return g.run(ctx, "merge", branch)
}

// CurrentBranch возвращает имя текущей ветки.
func (g *Git) CurrentBranch(ctx context.Context) (string, error) {
	out, err := g.run(ctx, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (g *Git) DiffLast(ctx context.Context) (string, error) {
	out, err := g.run(ctx, "diff", "HEAD~1")
	if err != nil {
		return g.run(ctx, "show", "--stat", "HEAD")
	}
	return out, nil
}

func (g *Git) AddAll(ctx context.Context) error {
	_, err := g.run(ctx, "add", "-A")
	return err
}

func (g *Git) AutoCommit(ctx context.Context, message string) (string, error) {
	status, err := g.Status(ctx)
	if err != nil {
		return "", err
	}

	if strings.TrimSpace(status) == "" {
		return "", nil
	}

	if err := g.ensureConfig(ctx); err != nil {
		return "", err
	}

	if err := g.AddAll(ctx); err != nil {
		return "", err
	}

	if _, err := g.run(ctx, "commit", "-m", message); err != nil {
		return "", err
	}

	hash, err := g.run(ctx, "rev-parse", "--short", "HEAD")
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(hash), nil
}

func (g *Git) ensureConfig(ctx context.Context) error {
	name, _ := g.run(ctx, "config", "--local", "user.name")
	if strings.TrimSpace(name) == "" {
		if _, err := g.run(ctx, "config", "--local", "user.name", "Gogitor"); err != nil {
			return err
		}
	}

	email, _ := g.run(ctx, "config", "--local", "user.email")
	if strings.TrimSpace(email) == "" {
		if _, err := g.run(ctx, "config", "--local", "user.email", "gogitor@local"); err != nil {
			return err
		}
	}

	return nil
}

func (g *Git) run(ctx context.Context, args ...string) (string, error) {
	return g.runWithTimeout(ctx, 60*time.Second, args...)
}

// runWithTimeout позволяет указать отдельный таймаут для длительных операций.
func (g *Git) runWithTimeout(ctx context.Context, timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = g.Dir

	cmd.Env = gitCommandEnv(g.currentAuthEnv()...)

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	cmd.Cancel = func() error {
		if cmd.Process != nil {
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}

	cmd.WaitDelay = 3 * time.Second

	out, err := cmd.CombinedOutput()
	output := string(out)

	if ctx.Err() != nil {
		return output, ctx.Err()
	}

	if err != nil {
		return output, fmt.Errorf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}

	return output, nil
}

func InjectToken(repoURL, token string) string {
	if token == "" {
		return repoURL
	}
    if strings.HasPrefix(repoURL, "git@") {
    	rest := strings.TrimPrefix(repoURL, "git@")
    	parts := strings.SplitN(rest, ":", 2)
    	if len(parts) == 2 {
    		credentials := url.UserPassword("x-access-token", token).String()
    
    		return fmt.Sprintf(
    			"https://%s@%s/%s",
    			credentials,
    			parts[0],
    			parts[1],
    		)
    	}
    }

	u, err := url.Parse(repoURL)
	if err != nil {
		return repoURL
	}
	if u.Scheme == "https" || u.Scheme == "http" {
		u.User = url.UserPassword("x-access-token", token)
		return u.String()
	}
	return repoURL
}



func (g *Git) WithCloneAuth(ctx context.Context, repoURL, token string, fn func() (string, error)) (string, error) {
	if token == "" {
		return fn()
	}
	g.authMu.Lock()
	defer g.authMu.Unlock()

	u, err := url.Parse(repoURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return fn()
	}

	authBase := fmt.Sprintf("%s://x-access-token:%s@%s", u.Scheme, token, u.Host)
	cleanBase := fmt.Sprintf("%s://%s", u.Scheme, u.Host)

	env := []string{
		"GIT_CONFIG_COUNT=1",
		fmt.Sprintf("GIT_CONFIG_KEY_0=url.%s/.insteadOf", authBase),
		fmt.Sprintf("GIT_CONFIG_VALUE_0=%s/", cleanBase),
	}
	
	old, _ := g.authEnv.Load().([]string)
	g.authEnv.Store(env)
	defer g.authEnv.Store(old)

	return fn()
}

func (g *Git) WithAuthenticatedRemote(
	ctx context.Context,
	remote, token string,
	fn func() (string, error),
) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return fn()
	}

	g.authMu.Lock()
	defer g.authMu.Unlock()

	// Получаем обычный URL remote.
	origURL, err := g.run(ctx, "remote", "get-url", remote)
	if err != nil {
		return fn()
	}

	origURL = strings.TrimSpace(origURL)
	if origURL == "" {
		return fn()
	}

	// Превращаем SSH/HTTPS URL в аутентифицированный HTTPS URL.
	authURL := InjectToken(origURL, token)
    safeAuthURL := authURL
    
    if u, err := url.Parse(authURL); err == nil {
    	u.User = nil
    	safeAuthURL = u.String()
    }
    
    if g.Log != nil {
    	g.Log.Debug(
    		"github authentication configured",
    		"remote", remote,
    		"original_url", origURL,
    		"authenticated_url", safeAuthURL,
    	)
    }
	if authURL == origURL {
		return fn()
	}

	env := []string{
		"GIT_CONFIG_COUNT=2",

		fmt.Sprintf(
			"GIT_CONFIG_KEY_0=remote.%s.url",
			remote,
		),
		fmt.Sprintf(
			"GIT_CONFIG_VALUE_0=%s",
			authURL,
		),

		fmt.Sprintf(
			"GIT_CONFIG_KEY_1=remote.%s.pushurl",
			remote,
		),
		fmt.Sprintf(
			"GIT_CONFIG_VALUE_1=%s",
			authURL,
		),
	}

	old, _ := g.authEnv.Load().([]string)
	g.authEnv.Store(env)

	defer g.authEnv.Store(old)

	return fn()
}

// EnsureRemote создаёт remote, если его нет, или обновляет URL.
func (g *Git) EnsureRemote(ctx context.Context, remote, repoURL string) error {
	out, err := g.run(ctx, "remote", "get-url", remote)
	if err != nil {
		// remote не существует — создаём
		_, err = g.run(ctx, "remote", "add", remote, repoURL)
		return err
	}
	if strings.TrimSpace(out) != repoURL {
		_, err = g.run(ctx, "remote", "set-url", remote, repoURL)
		return err
	}
	return nil
}

func (g *Git) Push(ctx context.Context, remote, branch string, force bool) (string, error) {
	args := []string{"-c", "credential.helper=", "push"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, remote)
	if branch != "" {
		args = append(args, branch)
	} else {
		cur, err := g.CurrentBranch(ctx)
		if err == nil && cur != "" {
			args = append(args, cur)
		}
	}
	return g.runNet(ctx, 2*time.Minute, args...)
}

func (g *Git) Pull(ctx context.Context, remote, branch string) (string, error) {
	args := []string{"-c", "credential.helper=", "pull", remote}
	if branch != "" {
		args = append(args, branch)
	}
	return g.runNet(ctx, 2*time.Minute, args...)
}

func (g *Git) Fetch(ctx context.Context, remote string) (string, error) {
	if remote == "" {
		remote = "--all"
	}
	return g.runNet(ctx, 2*time.Minute, "-c", "credential.helper=", "fetch", remote)
}

func (g *Git) Clone(ctx context.Context, repoURL, dir string) (string, error) {
	args := []string{"-c", "credential.helper=", "clone", repoURL}
	if dir != "" {
		args = append(args, dir)
	}
	return g.runNet(ctx, 5*time.Minute, args...)
}

func (g *Git) RemoteList(ctx context.Context) (string, error) {
	return g.run(ctx, "remote", "-v")
}

func (g *Git) RemoteAdd(ctx context.Context, name, repoURL string) (string, error) {
	return g.run(ctx, "remote", "add", name, repoURL)
}

func (g *Git) RemoteRemove(ctx context.Context, name string) (string, error) {
	return g.run(ctx, "remote", "remove", name)
}

func (g *Git) RemoteSetURL(ctx context.Context, name, url string) (string, error) {
	return g.run(ctx, "remote", "set-url", name, url)
}

func (g *Git) Revert(ctx context.Context, hash string) (string, error) {
	return g.run(ctx, "revert", "--no-edit", hash)
}

func (g *Git) Reset(ctx context.Context, hash string, hard bool) (string, error) {
	if hard {
		return g.run(ctx, "reset", "--hard", hash)
	}
	return g.run(ctx, "reset", hash)
}

func (g *Git) runNet(ctx context.Context, timeout time.Duration, args ...string) (string, error) {
	return g.runWithTimeout(ctx, timeout, args...)
}


// HeadHash возвращает хеш текущего HEAD.
func (g *Git) HeadHash(ctx context.Context) (string, error) {
	out, err := g.run(ctx, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// DiffRange возвращает diff между двумя коммитами (from..to).
func (g *Git) DiffRange(ctx context.Context, from, to string) (string, error) {
	return g.run(ctx, "diff", from+".."+to)
}

// AddIntentToAll помечает новые файлы как intent-to-add,
// чтобы они появились в git diff.
func (g *Git) AddIntentToAll(ctx context.Context) error {
	_, err := g.run(ctx, "add", "-N", ".")
	return err
}

// ResetAll отменяет индексацию (git reset .).
func (g *Git) ResetAll(ctx context.Context) error {
	_, err := g.run(ctx, "reset", ".")
	return err
}

// currentAuthEnv возвращает временные аутентификационные переменные окружения.
func (g *Git) currentAuthEnv() []string {
	if v, ok := g.authEnv.Load().([]string); ok {
		return v
	}
	return nil
}

// Мы удаляем чужие GIT_CONFIG_COUNT/KEY/VALUE, чтобы гарантировать,
// что наши временные настройки не смешиваются с унаследованными.
func gitCommandEnv(extra ...string) []string {
	env := os.Environ()

	out := make([]string, 0, len(env)+len(extra)+3)

	for _, kv := range env {
		if strings.HasPrefix(kv, "GIT_CONFIG_COUNT=") ||
			strings.HasPrefix(kv, "GIT_CONFIG_KEY_") ||
			strings.HasPrefix(kv, "GIT_CONFIG_VALUE_") ||
			strings.HasPrefix(kv, "GIT_TERMINAL_PROMPT=") ||
			strings.HasPrefix(kv, "GIT_ASKPASS=") ||
			strings.HasPrefix(kv, "SSH_ASKPASS=") {
			continue
		}

		out = append(out, kv)
	}

	out = append(out,
		"GIT_TERMINAL_PROMPT=0",
		"GIT_ASKPASS=",
		"SSH_ASKPASS=",
	)

	out = append(out, extra...)

	return out
}

// RemoteURL возвращает URL указанного remote.
func (g *Git) RemoteURL(ctx context.Context, name string) (string, error) {
	out, err := g.run(ctx, "remote", "get-url", name)
	if err != nil {
		return "", fmt.Errorf("remote %q not found: %w", name, err)
	}
	return strings.TrimSpace(out), nil
}

// LogSubjects возвращает subject'ы коммитов (для changelog).
func (g *Git) LogSubjects(ctx context.Context, maxCount int) ([]string, error) {
	if maxCount <= 0 {
		maxCount = 200
	}
	out, err := g.run(ctx, "log",
		"--pretty=format:%s",
		fmt.Sprintf("-n%d", maxCount),
	)
	if err != nil {
		return nil, err
	}
	var subjects []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			subjects = append(subjects, line)
		}
	}
	return subjects, nil
}

// DiffFile возвращает diff для конкретного файла (до git add).
func (g *Git) DiffFile(ctx context.Context, file string) (string, error) {
	return g.run(ctx, "diff", "--", file)
}

// AddFile добавляет конкретный файл в индекс.
func (g *Git) AddFile(ctx context.Context, file string) error {
	_, err := g.run(ctx, "add", "--", file)
	return err
}

// CommitMessage создаёт коммит с указанным сообщением.
// В отличие от AutoCommit, НЕ выполняет git add.
func (g *Git) CommitMessage(ctx context.Context, message string) error {
	if err := g.ensureConfig(ctx); err != nil {
		return err
	}
	_, err := g.run(ctx, "commit", "-m", message)
	return err
}

// StatusFiles возвращает список изменённых файлов (имена без статусов).
func (g *Git) StatusFiles(ctx context.Context) ([]string, error) {
	out, err := g.run(ctx, "status", "--porcelain")
	if err != nil {
		return nil, err
	}
	var files []string
	for _, line := range strings.Split(out, "\n") {
		if len(line) < 4 {
			continue
		}
		// Формат porcelain: XY filename
		// или XY orig -> filename (для rename)
		name := strings.TrimSpace(line[3:])
		// Для rename: "R  old -> new" — берём new
		if idx := strings.Index(name, " -> "); idx != -1 {
			name = name[idx+4:]
		}
		if name != "" {
			files = append(files, name)
		}
	}
	return files, nil
}