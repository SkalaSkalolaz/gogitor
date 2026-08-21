package computer

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
	"regexp"

	"gogitor/internal/security"
	"gogitor/internal/textutil"
)

// CommandResult — результат выполнения команды.
type CommandResult struct {
	Command    string             `json:"command"`
	ExitCode   int                `json:"exit_code"`
	Output     string             `json:"output"`
	Duration   time.Duration      `json:"duration"`
	TimedOut   bool               `json:"timed_out"`
	Risk       security.RiskLevel `json:"risk"`
	Reason     string             `json:"reason"`
}

// ExecutorConfig — конфигурация исполнителя.
type ExecutorConfig struct {
	CommandTimeout  time.Duration
	MaxOutputBytes  int
	WorkDir         string
	AllowSudo       bool
	ConfirmHighRisk bool
	ConfirmFunc     func(command string, risk security.RiskLevel, reason string) bool
}

func DefaultExecutorConfig(workDir string) ExecutorConfig {
	return ExecutorConfig{
		CommandTimeout:  120 * time.Second,
		MaxOutputBytes:  100_000,
		WorkDir:         workDir,
		AllowSudo:       false,
		ConfirmHighRisk: true,
	}
}

// Executor — безопасный исполнитель команд.
type Executor struct {
	config ExecutorConfig
}

func NewExecutor(config ExecutorConfig) *Executor {
	return &Executor{config: config}
}

// Execute выполняет команду с полной проверкой безопасности.
func (e *Executor) Execute(ctx context.Context, command string) (*CommandResult, error) {
	// 1. Оцениваем цепочку целиком
	maxRisk, reason, err := security.AssessChain(command, e.config.WorkDir)
	if err != nil {
		return nil, err // FORBIDDEN в цепочке
	}

	// 2. FORBIDDEN
	if maxRisk == security.RiskForbidden {
		return nil, fmt.Errorf("FORBIDDEN: %s (%s)", command, reason)
	}

	// 3. HIGH — подтверждение
	if maxRisk == security.RiskHigh && e.config.ConfirmHighRisk {
		if e.config.ConfirmFunc != nil {
			if !e.config.ConfirmFunc(command, maxRisk, reason) {
				return nil, fmt.Errorf("command rejected by user: %s", command)
			}
		} else {
			return nil, fmt.Errorf("HIGH risk requires confirmation: %s", command)
		}
	}

	// 4. Проверка привилегированных команд
	if !e.config.AllowSudo {
		privCmds := []string{"sudo", "doas", "pkexec", "runuser"}
		for _, pc := range privCmds {
			if strings.Contains(command, pc) {
				return nil, fmt.Errorf("%s not allowed: %s", pc, command)
			}
		}
		// su -c тоже проверяем
		if strings.Contains(command, "su ") && strings.Contains(command, "-c") {
			return nil, fmt.Errorf("su -c not allowed: %s", command)
		}
	}

	// 5. Проверка допустимости записи (если команда содержит перенаправление)
	if strings.Contains(command, ">") {
		redirectPaths := extractRedirectPaths(command)
		for _, rp := range redirectPaths {
			if !security.IsWriteAllowed(rp, e.config.WorkDir) {
				return nil, fmt.Errorf(
					"write to %s is not allowed (outside permitted directories)",
					rp,
				)
			}
		}
	}

	// 6. Выполняем
	return e.run(ctx, command, maxRisk, reason)
}

// extractRedirectPaths извлекает пути из перенаправлений > и >>
func extractRedirectPaths(command string) []string {
	var paths []string
	inSingle, inDouble := false, false
	for i := 0; i < len(command); i++ {
		ch := command[i]
		switch ch {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '>':
			if inSingle || inDouble {
				continue
			}
			// Пропускаем >>
			j := i + 1
			if j < len(command) && command[j] == '>' {
				j++
			}
			// Пропускаем пробелы
			for j < len(command) && (command[j] == ' ' || command[j] == '\t') {
				j++
			}
			// Извлекаем путь
			start := j
			for j < len(command) && command[j] != ' ' &&
				command[j] != '\t' && command[j] != ';' &&
				command[j] != '|' && command[j] != '&' {
				j++
			}
			if j > start {
				path := command[start:j]
				path = strings.Trim(path, "'\"")
				if path != "" && path != "/dev/null" {
					paths = append(paths, path)
				}
			}
			i = j - 1
		}
	}
	return paths
}

func (e *Executor) run(
	ctx context.Context,
	command string,
	risk security.RiskLevel,
	reason string,
) (*CommandResult, error) {
	ctx, cancel := context.WithTimeout(ctx, e.config.CommandTimeout)
	defer cancel()

	start := time.Now()
	shell, args := detectShell()

	cmd := exec.CommandContext(ctx, shell, append(args, command)...)
	cmd.Dir = e.config.WorkDir
	cmd.Env = sanitizedEnv()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}
	cmd.WaitDelay = 3 * time.Second

	out, err := cmd.CombinedOutput()
	duration := time.Since(start)

	output := string(out)
	if len(output) > e.config.MaxOutputBytes {
		output = textutil.TruncateStringBytes(output, e.config.MaxOutputBytes)
		output += "\n... [output truncated for safety]"
	}
	output = sanitizeCommandOutput(output)

	result := &CommandResult{
		Command:  command,
		Output:   output,
		Duration: duration,
		Risk:     risk,
		Reason:   reason,
	}

	if ctx.Err() != nil {
		result.TimedOut = true
		result.ExitCode = -1
		return result, ctx.Err()
	}
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
		}
	}
	return result, nil
}

func detectShell() (string, []string) {
	if _, err := exec.LookPath("bash"); err == nil {
		return "bash", []string{"-c"}
	}
	return "sh", []string{"-c"}
}

func sanitizedEnv() []string {
	dangerous := []string{
		"LD_PRELOAD", "LD_LIBRARY_PATH",
		"DYLD_INSERT_LIBRARIES", "DYLD_LIBRARY_PATH",
		"PYTHONSTARTUP", "PYTHONPATH", "NODE_OPTIONS",
		"BASH_ENV", "ENV",
	}
	var out []string
	for _, kv := range os.Environ() {
		skip := false
		for _, d := range dangerous {
			if strings.HasPrefix(kv, d+"=") {
				skip = true
				break
			}
		}
		if !skip {
			out = append(out, kv)
		}
	}
	out = append(out,
		"GIT_TERMINAL_PROMPT=0",
		"DEBIAN_FRONTEND=noninteractive",
	)
	return out
}

// sanitizeCommandOutput удаляет prompt-injection из вывода команды.
var outputInjectionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(ignore|forget|disregard)\s+(all\s+)?(previous|above|prior|earlier)\s+(instructions?|prompts?|rules?|context)`),
	regexp.MustCompile(`(?i)you\s+are\s+now\s+(a|an|in)\s+`),
	regexp.MustCompile(`(?i)new\s+(system\s+)?instructions?:`),
	regexp.MustCompile(`(?i)system\s*prompt\s*:`),
	regexp.MustCompile(`(?i)<\s*/?\s*system\s*>`),
	regexp.MustCompile(`(?i)\[\s*INST\s*\]|\[\s*/\s*INST\s*\]`),
	regexp.MustCompile(`(?i)<<\s*SYS\s*>>|<<\s*/\s*SYS\s*>>`),
	regexp.MustCompile(`(?i)do\s+not\s+follow\s+(the\s+)?(above|previous|prior)`),
	regexp.MustCompile(`(?i)override\s+(all\s+)?(safety|security|rules?|restrictions?)`),
	regexp.MustCompile(`(?i)jailbreak`),
	regexp.MustCompile(`(?i)DAN\s+mode`),
	regexp.MustCompile(`(?i)developer\s+mode\s+(enabled|activated|on)`),
	regexp.MustCompile(`(?i)(execute|run|eval)\s+(the\s+)?(following|this)\s+(command|code|script)`),
	regexp.MustCompile(`(?i)rm\s+-rf\s+/`),
	regexp.MustCompile(`(?i)chmod\s+777`),
	regexp.MustCompile(`(?i)curl\s+.*\|\s*(ba)?sh`),
	regexp.MustCompile(`(?i)wget\s+.*\|\s*(ba)?sh`),
}

func sanitizeCommandOutput(output string) string {
	// Удаляем управляющие символы и escape-последовательности
	output = strings.Map(func(r rune) rune {
		if r < 32 && r != '\n' && r != '\t' && r != '\r' {
			return -1
		}
		// Удаляем Unicode-конфьюзаблы, которые могут обходить фильтрацию
		if r == '\u200B' || r == '\u200C' || r == '\u200D' ||
			r == '\uFEFF' || r == '\u00A0' {
			return -1
		}
		return r
	}, output)

	// Применяем regex-паттерны
	for _, pattern := range outputInjectionPatterns {
		output = pattern.ReplaceAllString(output, "[FILTERED]")
	}
	return output
}