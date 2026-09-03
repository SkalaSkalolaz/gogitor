package runner

import (
	"context"
	"fmt"
	"go/parser"
	"go/token"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"gogitor/internal/domain"
	"gogitor/internal/textutil"
)

var (
	testRunRE      = regexp.MustCompile(`^=== RUN\s+(\S+)`)
	testFailRE     = regexp.MustCompile(`^--- FAIL:\s+(\S+)`)
	testFileLineRE = regexp.MustCompile(`^\s*([^\s:]+\.go):(\d+):\s*(.*)$`)
	coverageRE     = regexp.MustCompile(`coverage:\s+([0-9]+(?:\.[0-9]+)?)%`)
	testPassLineRE = regexp.MustCompile(`^--- PASS:\s+\S+\s+\(.+\)\s*$`)
	testFailLineRE = regexp.MustCompile(`^--- FAIL:\s+\S+\s+\(.+\)\s*$`)
)

var lintIssueRE = regexp.MustCompile(
	`^[^\s]+\.go:\d+(?::\d+)?:\s+.+(\[[\w.-]+\]|\([\w.-]+\))\s*$`,
)

type Runner struct {
	Timeout  time.Duration
	Log      *slog.Logger
	DepsLog  func(string)
	DepsMode string
}

func New(timeout time.Duration, log *slog.Logger) *Runner {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}

	return &Runner{
		Timeout:  timeout,
		Log:      log,
		DepsMode: "auto",
	}
}

func (r *Runner) Build(ctx context.Context, dir string) error {
	if !hasGoFiles(dir) {
		return nil
	}

	if err := r.PrepareGoModule(ctx, dir); err != nil {
		return err
	}
	r.ResolveDeps(ctx, dir)

	if fmtErr := r.Format(ctx, dir); fmtErr != nil {
		if r.Log != nil {
			r.Log.Warn("gofmt warning (non-fatal)", "err", fmtErr)
		}
	}
	out, err := r.run(ctx, dir, "go", "build", "-o", "/dev/null", "./...")
	if err != nil {
		parsed := parseGoBuildErrors(out)

		if parsed != "" {
			return fmt.Errorf("build failed:\n%s", parsed)
		}

		return fmt.Errorf("build failed:\n%s", trim(out, 4000))
	}

	return nil

}

func (r *Runner) Test(ctx context.Context, dir string) (domain.TestsStatus, error) {
	status := domain.TestsStatus{}

	if !hasGoFiles(dir) {
		status.Skipped = true
		return status, nil
	}

	if err := r.PrepareGoModule(ctx, dir); err != nil {
		return status, err
	}

	r.ResolveDeps(ctx, dir)
	out, err := r.run(ctx, dir, "go", "test", "-v", "-cover", "./...")
	out = trim(out, 30000)
	status.Output = trim(out, 20000)
	status.Run = true

	if ctx.Err() != nil {
		return status, ctx.Err()
	}

	passed, failed := parseGoTestOutput(out)
	status.Passed = passed
	status.Failed = failed

	status.Coverage, status.CoverageOutput = parseGoCoverage(out)
	status.Failures = parseGoTestFailures(out)

	if passed == 0 && failed == 0 && strings.Contains(out, "[no test files]") {
		status.Skipped = true
		status.Run = false
		return status, nil
	}

	if err != nil && failed == 0 {
		status.Failed = 1

		if len(status.Failures) == 0 {
			status.Failures = append(status.Failures, domain.TestFailure{
				Message: trim(out, 4000),
			})
		}
	}

	return status, nil
}

func (r *Runner) Lint(ctx context.Context, dir string) (string, error) {
	if !hasGoFiles(dir) {
		return "", nil
	}
	if err := r.PrepareGoModule(ctx, dir); err != nil {
		return "", err
	}
	_ = r.EnsureLintConfig(ctx, dir)
	r.ResolveDeps(ctx, dir)

	if _, err := exec.LookPath("golangci-lint"); err != nil {
		return "", fmt.Errorf(
			"golangci-lint is not installed; install it with: " +
				"go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest",
		)
	}

	out, err := r.run(ctx, dir, "golangci-lint", "run", "./...")
	return trim(out, 30000), err
}

func CountLintIssues(output string) int {
	count := 0
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "level=") ||
			strings.HasPrefix(trimmed, "WARN") ||
			strings.HasPrefix(trimmed, "INFO") ||
			strings.HasPrefix(trimmed, "Running") ||
			strings.HasPrefix(trimmed, "WARN [") {
			continue
		}
		if lintIssueRE.MatchString(trimmed) {
			count++
		}
	}
	return count
}

func (r *Runner) RunDir(ctx context.Context, dir string) (string, error) {
	if !hasGoFiles(dir) {
		return "", fmt.Errorf("no Go files found")
	}

	if err := r.PrepareGoModule(ctx, dir); err != nil {
		return "", err
	}

	r.ResolveDeps(ctx, dir)

	if fmtErr := r.Format(ctx, dir); fmtErr != nil {
		if r.Log != nil {
			r.Log.Warn("gofmt warning (non-fatal)", "err", fmtErr)
		}
	}

	if !hasMainPackage(dir) {
		return "", fmt.Errorf("no main package found: create a file with 'package main' and func main()")
	}

	return r.run(ctx, dir, "go", "run", ".")
}

func (r *Runner) PrepareGoModule(ctx context.Context, dir string) error {
	if hasGoMod(dir) {
		return nil
	}

	if !hasGoFiles(dir) {
		return nil
	}

	module := sanitizeModuleName(filepath.Base(dir))
	_, err := r.run(ctx, dir, "go", "mod", "init", module)
	if err != nil {
		// If go.mod appeared concurrently or init failed mildly, continue.
		if hasGoMod(dir) {
			return nil
		}
		return fmt.Errorf("go mod init failed: %w", err)
	}

	return nil
}

func (r *Runner) ResolveDeps(ctx context.Context, dir string) {
	if !hasGoMod(dir) {
		return
	}
	if !hasExternalImports(dir) {
		return
	}

	mode := r.DepsMode
	if mode == "" {
		mode = "auto"
	}
	switch mode {
	case "never":
		r.depsLog("⚠ External imports detected but deps_mode=never; skipping go mod tidy")
		return
	case "ask":
		r.depsLog("⚠ External imports detected; deps_mode=ask — run 'go mod tidy' manually if needed")
		if r.Log != nil {
			r.Log.Warn("external imports detected, go mod tidy skipped (deps_mode=ask)", "dir", dir)
		}
		return
	}

	r.depsLog("Resolving external Go dependencies (go mod tidy)...")

	if r.Log != nil {
		r.Log.Debug("resolving external dependencies", "dir", dir)
	}

	tidyCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	out, err := r.run(tidyCtx, dir, "go", "mod", "tidy")
	if err != nil {
		if shouldRetryDependencyViaProxy(out) &&
			strings.TrimSpace(os.Getenv("GOPRIVATE")) == "" {
			r.depsLog(
				"⚠ Dependency fetch used Git/SSH; retrying through Go module proxy...",
			)

			retryCtx, retryCancel :=
				context.WithTimeout(ctx, 60*time.Second)

			retryOut, retryErr := r.runWithEnv(
				retryCtx,
				dir,
				map[string]string{
					"GOPROXY": "https://proxy.golang.org",
				},
				"go",
				"mod",
				"tidy",
			)

			retryCancel()

			if retryErr == nil {
				if installed := parseTidyOutput(retryOut); installed != "" {
					r.depsLog(
						"✓ Dependencies resolved through Go module proxy:\n" +
							installed,
					)
				} else {
					r.depsLog(
						"✓ Dependencies resolved through Go module proxy.",
					)
				}
				return
			}

			// Для дальнейшей диагностики используем результат
			// именно последней попытки.
			out = retryOut
			err = retryErr

			r.depsLog(
				"⚠ Go module proxy retry failed: " +
					trim(retryErr.Error(), 500),
			)
		}

		r.depsLog(
			"⚠ go mod tidy failed: " +
				trim(err.Error(), 500),
		)

		if r.Log != nil {
			r.Log.Warn(
				"go mod tidy failed (non-fatal)",
				"err",
				err,
			)
		}

		return
	}

	// Выводим краткую информацию об установленных пакетах.
	if installed := parseTidyOutput(out); installed != "" {
		r.depsLog("✓ Dependencies resolved:\n" + installed)
	} else {
		r.depsLog("✓ Dependencies resolved.")
	}
}

func shouldRetryDependencyViaProxy(output string) bool {
	lower := strings.ToLower(output)

	markers := []string{
		"git ls-remote",
		"permission denied (publickey)",
		"could not read from remote repository",
		"ssh: connect to host",
		"repository not found",
	}

	for _, marker := range markers {
		if strings.Contains(lower, marker) {
			return true
		}
	}

	return false
}

// depsLog вызывает колбэк, если он установлен.
func (r *Runner) depsLog(msg string) {
	if r.DepsLog != nil {
		r.DepsLog(msg)
	}
}

func parseTidyOutput(output string) string {
	if strings.TrimSpace(output) == "" {
		return ""
	}
	var lines []string
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "go: downloading") ||
			strings.HasPrefix(trimmed, "go: added") ||
			strings.HasPrefix(trimmed, "go: finding") {
			lines = append(lines, "  "+trimmed)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	if len(lines) > 15 {
		lines = lines[:15]
		lines = append(lines, "  ... and more")
	}
	return strings.Join(lines, "\n")
}

func hasExternalImports(dir string) bool {
	found := false
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || found {
			return filepath.SkipAll
		}
		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == "node_modules" || name == ".gogitor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if detectExternalImport(string(data)) {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

func detectExternalImport(content string) bool {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Пропускаем комментарии
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") {
			continue
		}

		// Извлекаем строку в кавычках
		start := strings.Index(trimmed, "\"")
		if start == -1 {
			continue
		}
		end := strings.Index(trimmed[start+1:], "\"")
		if end == -1 {
			continue
		}
		importPath := trimmed[start+1 : start+1+end]

		// Пропускаем пустые и слишком короткие пути
		if len(importPath) < 4 {
			continue
		}

		// Первый сегмент пути (до первого "/")
		firstSlash := strings.Index(importPath, "/")
		var firstSegment string
		if firstSlash == -1 {
			firstSegment = importPath
		} else {
			firstSegment = importPath[:firstSlash]
		}

		if strings.Contains(firstSegment, ".") &&
			!strings.HasPrefix(importPath, "internal/") &&
			!strings.HasPrefix(importPath, "vendor/") {
			return true
		}
	}
	return false
}

func (r *Runner) Format(ctx context.Context, dir string) error {
	if _, err := exec.LookPath("gofmt"); err != nil {
		return nil
	}
	out, err := r.run(ctx, dir, "gofmt", "-w", ".")
	if err != nil {
		if r.Log != nil {
			r.Log.Warn("gofmt failed", "dir", dir, "err", err)
		}
		return fmt.Errorf("gofmt: %w", err)
	}
	_ = out
	return nil
}

func (r *Runner) runWithEnv(
	ctx context.Context,
	dir string,
	overrides map[string]string,
	name string,
	args ...string,
) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()

	cmd := r.command(ctx, dir, name, args...)

	env := os.Environ()

	for key, value := range overrides {
		prefix := key + "="
		replaced := false

		next := make([]string, 0, len(env)+1)

		for _, item := range env {
			if strings.HasPrefix(item, prefix) {
				if !replaced {
					next = append(next, prefix+value)
					replaced = true
				}
				continue
			}

			next = append(next, item)
		}

		if !replaced {
			next = append(next, prefix+value)
		}

		env = next
	}

	cmd.Env = env

	out, err := cmd.CombinedOutput()
	output := string(out)

	if ctx.Err() != nil {
		return output, ctx.Err()
	}

	if err != nil {
		return output, fmt.Errorf(
			"%s %s: %v\n%s",
			name,
			strings.Join(args, " "),
			err,
			trim(output, 5000),
		)
	}

	return output, nil
}

func (r *Runner) run(ctx context.Context, dir, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()

	cmd := r.command(ctx, dir, name, args...)

	out, err := cmd.CombinedOutput()
	output := string(out)

	if ctx.Err() != nil {
		return output, ctx.Err()
	}

	if err != nil {
		return output, fmt.Errorf("%s %s: %v\n%s", name, strings.Join(args, " "), err, trim(output, 5000))
	}

	return output, nil
}

func (r *Runner) command(ctx context.Context, dir, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()

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

	return cmd
}

func hasGoMod(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "go.mod"))
	return err == nil
}

func hasGoFiles(dir string) bool {
	found := false

	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == "node_modules" || name == ".gogitor" {
				return filepath.SkipDir
			}
			return nil
		}

		if strings.HasSuffix(info.Name(), ".go") {
			found = true
			return filepath.SkipAll
		}

		return nil
	})

	return found
}

func hasMainPackage(dir string) bool {
	found := false
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == "node_modules" || name == ".gogitor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".go") {
			return nil
		}

		fset := token.NewFileSet()
		f, parseErr := parser.ParseFile(fset, path, nil, parser.PackageClauseOnly)
		if parseErr != nil {
			return nil
		}
		if f.Name.Name == "main" {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

func parseGoTestOutput(output string) (passed, failed int) {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if testPassLineRE.MatchString(line) {
			passed++
		}
		if testFailLineRE.MatchString(line) {
			failed++
		}
	}
	return passed, failed
}

func sanitizeModuleName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" || name == "." || name == "/" {
		return "app"
	}
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ReplaceAll(name, "/", "-")
	return name
}

func trim(s string, max int) string {
	return textutil.LimitRunes(s, max, "...")
}

func parseGoCoverage(output string) (float64, string) {
	var values []float64
	var lines []string

	for _, line := range strings.Split(output, "\n") {
		if !strings.Contains(line, "coverage:") {
			continue
		}

		m := coverageRE.FindStringSubmatch(line)
		if len(m) != 2 {
			continue
		}

		v, err := strconv.ParseFloat(m[1], 64)
		if err != nil {
			continue
		}

		values = append(values, v)
		lines = append(lines, strings.TrimSpace(line))
	}

	if len(values) == 0 {
		return 0, ""
	}

	var sum float64
	for _, v := range values {
		sum += v
	}

	coverageOutput := strings.Join(lines, " \n ")
	coverageOutput = textutil.LimitRunes(coverageOutput, 500, "...")

	return sum / float64(len(values)), coverageOutput
}

func parseGoTestFailures(output string) []domain.TestFailure {
	var failures []domain.TestFailure

	currentTest := ""
	var details []string

	flush := func(name string) {
		f := domain.TestFailure{
			Test:     name,
			Function: functionFromTestName(name),
			Message:  strings.TrimSpace(strings.Join(details, "\n")),
		}

		for _, d := range details {
			m := testFileLineRE.FindStringSubmatch(d)
			if len(m) != 4 {
				continue
			}

			f.File = m[1]

			if n, err := strconv.Atoi(m[2]); err == nil {
				f.Line = n
			}

			if strings.TrimSpace(m[3]) != "" && strings.TrimSpace(f.Message) == "" {
				f.Message = strings.TrimSpace(m[3])
			}

			break
		}

		f.Message = trim(f.Message, 2000)
		if f.Message == "" {
			f.Message = "test failed"
		}

		failures = append(failures, f)
	}

	for _, line := range strings.Split(output, "\n") {
		if m := testRunRE.FindStringSubmatch(line); len(m) == 2 {
			currentTest = m[1]
			details = nil
			continue
		}

		if m := testFailRE.FindStringSubmatch(line); len(m) == 2 {
			flush(m[1])
			currentTest = ""
			details = nil
			continue
		}

		if currentTest == "" {
			continue
		}

		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if strings.HasPrefix(line, "    ") || testFileLineRE.MatchString(line) {
			details = append(details, trimmed)
		}
	}

	return failures
}

func functionFromTestName(name string) string {
	if idx := strings.Index(name, "/"); idx != -1 {
		name = name[:idx]
	}

	prefixes := []string{
		"Test",
		"Example",
		"Benchmark",
		"Fuzz",
	}

	for _, prefix := range prefixes {
		if strings.HasPrefix(name, prefix) && len(name) > len(prefix) {
			return name[len(prefix):]
		}
	}

	return name
}

func FormatFeedback(status domain.TestsStatus) string {
	if len(status.Failures) == 0 {
		return "All tests passed.\n" + trim(status.Output, 2000)
	}

	var b strings.Builder

	if status.CoverageOutput != "" {
		fmt.Fprintf(&b, "%s\n", status.CoverageOutput)
	}

	b.WriteString("TEST FAILURES (fix ONLY listed issues):\n")

	maxFailures := 10
	for i, f := range status.Failures {
		if i >= maxFailures {
			fmt.Fprintf(&b, "... and %d more failed tests\n", len(status.Failures)-maxFailures)
			break
		}

		fmt.Fprintf(&b, "- test: %s\n", f.Test)
		if f.Function != "" {
			fmt.Fprintf(&b, "  function: %s\n", f.Function)
		}
		if f.File != "" {
			fmt.Fprintf(&b, "  location: %s:%d\n", f.File, f.Line)
		}

		b.WriteByte('\n')

		if f.Message != "" {
			msg := trim(f.Message, 1200)
			msg = strings.ReplaceAll(msg, "\n", "\n  ")
			fmt.Fprintf(&b, "  %s\n", msg)
		}
	}

	if status.Output != "" {
		fmt.Fprintf(&b, "\nRaw output (truncated):\n%s", trim(status.Output, 1500))
	}

	return b.String()
}

func parseGoBuildErrors(output string) string {
	var lines []string

	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, ".go:") {
			lines = append(lines, strings.TrimSpace(line))
		}
	}

	if len(lines) == 0 {
		return ""
	}

	if len(lines) > 20 {
		lines = lines[:20]
		lines = append(lines, "... truncated")
	}

	return strings.Join(lines, "\n")
}

func (r *Runner) EnsureLintConfig(ctx context.Context, dir string) error {
	configPath := filepath.Join(dir, ".golangci.yml")
	if _, err := os.Stat(configPath); err == nil {
		return nil
	}
	const defaultConfig = `version: "2"

run:
timeout: 5m
tests: true
linters:
enable:
- errcheck
- govet
- ineffassign
- staticcheck
- unused
- revive
- misspell
- testifylint
- thelper
- whitespace
formatters:
enable:
- gofmt
- goimports
linters-settings:
revive:
rules:
- name: package-comments
disabled: true
testifylint:
enable-all: true
`

	if err := os.WriteFile(
		configPath,
		[]byte(defaultConfig),
		0o644,
	); err != nil {
		return err
	}

	if runtime.GOOS != "windows" {
		if err := os.Chmod(
			configPath,
			0o644,
		); err != nil {
			return fmt.Errorf(
				"cannot secure config file: %w",
				err,
			)
		}
	}
	return nil
}

// Vet выполняет go vet ./... и возвращает вывод.
func (r *Runner) Vet(ctx context.Context, dir string) (string, error) {
	if !hasGoFiles(dir) {
		return "", nil
	}
	if err := r.PrepareGoModule(ctx, dir); err != nil {
		return "", err
	}
	r.ResolveDeps(ctx, dir)
	out, err := r.run(ctx, dir, "go", "vet", "./...")
	return trim(out, 30000), err
}

// ParseLintIssues извлекает структурированные проблемы из вывода линтера.
type LintIssue struct {
	File    string
	Line    int
	Col     int
	Message string
	Linter  string
}

// ParseLintOutput парсит вывод линтера в структурированные проблемы.
func ParseLintOutput(output string) []LintIssue {
	var issues []LintIssue
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "level=") ||
			strings.HasPrefix(trimmed, "WARN") ||
			strings.HasPrefix(trimmed, "INFO") ||
			strings.HasPrefix(trimmed, "Running") {
			continue
		}
		if !lintIssueRE.MatchString(trimmed) {
			continue
		}
		// Парсим: файл:строка:столбец: сообщение (линтёры)
		re := regexp.MustCompile(`^([^\s]+\.go):(\d+)(?::(\d+))?:\s+(.+?)(?:\s+\[[\w.-]+\]|\s+\([\w.-]+\))\s*$`)
		m := re.FindStringSubmatch(trimmed)
		if m == nil {
			continue
		}
		issue := LintIssue{File: m[1], Message: m[4]}
		if n, err := strconv.Atoi(m[2]); err == nil {
			issue.Line = n
		}
		if len(m) > 3 && m[3] != "" {
			if n, err := strconv.Atoi(m[3]); err == nil {
				issue.Col = n
			}
		}
		issues = append(issues, issue)
	}
	return issues
}

// FilterNewIssues сравнивает два набора проблем и возвращает только новые.
// Проблема считается существующей, если совпадают файл, строка и сообщение.
func FilterNewIssues(before, after []LintIssue) []LintIssue {
	type issueKey struct {
		File    string
		Line    int
		Message string
	}
	beforeSet := make(map[issueKey]bool, len(before))
	for _, b := range before {
		beforeSet[issueKey{b.File, b.Line, b.Message}] = true
	}
	var newIssues []LintIssue
	for _, a := range after {
		key := issueKey{a.File, a.Line, a.Message}
		if !beforeSet[key] {
			newIssues = append(newIssues, a)
		}
	}
	return newIssues
}

// LintWithBaseline выполняет линт и возвращает только НОВЫЕ проблемы
// относительно базовой линии (состояния до изменений).
func (r *Runner) LintWithBaseline(ctx context.Context, dir string, baselineIssues []LintIssue) ([]LintIssue, string, error) {
	rawOutput, err := r.Lint(ctx, dir)
	allIssues := ParseLintOutput(rawOutput)
	if err != nil && len(allIssues) == 0 {
		// Линт вернул ошибку, но проблем нет — возможно, проблема конфигурации.
		return nil, rawOutput, err
	}
	newIssues := FilterNewIssues(baselineIssues, allIssues)
	if len(newIssues) > 0 {
		return newIssues, rawOutput, fmt.Errorf("lint found %d new issue(s)", len(newIssues))
	}
	return nil, rawOutput, nil
}

// hasGoFilesInList проверяет, содержит ли список путей хотя бы один .go файл.
func hasGoFilesInList(files []string) bool {
	for _, f := range files {
		if strings.HasSuffix(f, ".go") {
			return true
		}
	}
	return false
}
