package security

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// RiskLevel — уровень риска команды.
type RiskLevel int

const (
	RiskLow       RiskLevel = iota // ls, cat, pwd — автовыполнение
	RiskMedium                     // apt install, git clone — лог + опциональное подтверждение
	RiskHigh                       // rm, chmod, sudo — обязательное подтверждение
	RiskForbidden                  // rm -rf /, mkfs — немедленный отказ
)

func (r RiskLevel) String() string {
	switch r {
	case RiskLow:
		return "low"
	case RiskMedium:
		return "medium"
	case RiskHigh:
		return "high"
	case RiskForbidden:
		return "forbidden"
	default:
		return "unknown"
	}
}

// CommandAssessment — результат оценки команды.
type CommandAssessment struct {
	Command      string
	Risk         RiskLevel
	Reason       string
	RequiresSudo bool
	WritePaths   []string
}

// ─── FORBIDDEN: детерминированный blacklist ───────────────────────
// Блокирует команду мгновенно, без участия LLM.
var forbiddenPatterns = []*regexp.Regexp{
	// Удаление корня / home
	regexp.MustCompile(`(?i)rm\s+(-[a-zA-Z]*\s+)*(-[a-zA-Z]*r[a-zA-Z]*\s+|--recursive\s+)*(-[a-zA-Z]*f[a-zA-Z]*\s+|--force\s+)*(/|/\*|~|\$HOME)(\s|$|;|&)`),
	regexp.MustCompile(`(?i)rm\s+-[a-zA-Z]*r[a-zA-Z]*f[a-zA-Z]*\s+(/|~|\$HOME)`),
	regexp.MustCompile(`(?i)rm\s+-[a-zA-Z]*f[a-zA-Z]*r[a-zA-Z]*\s+(/|~|\$HOME)`),

	// Форматирование / запись на устройства
	regexp.MustCompile(`(?i)\bmkfs\.`),
	regexp.MustCompile(`(?i)\bdd\s+.*\bof=/dev/[sh]d`),
	regexp.MustCompile(`(?i)\bdd\s+.*\bof=/dev/nvme`),
	regexp.MustCompile(`>\s*/dev/[sh]d`),
	regexp.MustCompile(`>\s*/dev/nvme`),

	// Опасные chmod на корень
	regexp.MustCompile(`(?i)chmod\s+(-R\s+)?777\s+/(\s|$)`),
	regexp.MustCompile(`(?i)chmod\s+(-R\s+)?000\s+/(\s|$)`),

	// Fork bomb
	regexp.MustCompile(`:\(\)\s*\{\s*:\s*\|\s*:\s*&\s*\}\s*;\s*:`),

	// Выключение / перезагрузка
	regexp.MustCompile(`(?i)\b(shutdown|reboot|halt|poweroff)\b`),
	regexp.MustCompile(`(?i)\binit\s+[06]\b`),

	// Убийство PID 1 / init
	regexp.MustCompile(`(?i)\bkill\s+(-\d+\s+)?1\b`),
	regexp.MustCompile(`(?i)\bpkill\s+(-\d+\s+)?(systemd|init)\b`),

	// Очистка файрвола
	regexp.MustCompile(`(?i)iptables\s+-F\b`),
	regexp.MustCompile(`(?i)nft\s+flush\s+ruleset`),

	// Удаление crontab
	regexp.MustCompile(`(?i)crontab\s+-r\b`),

	// Остановка критичных сервисов
	regexp.MustCompile(`(?i)systemctl\s+(stop|disable|mask)\s+(systemd|dbus|network|NetworkManager|ssh|sshd)\b`),

	// Удаление системных директорий
	regexp.MustCompile(`(?i)rm\s+.*(/etc|/usr|/bin|/sbin|/boot|/lib|/lib64|/var|/proc|/sys)(\s|$|/)`),

	// Загрузка и выполнение скриптов из интернета
	regexp.MustCompile(`(?i)curl\s+.*\|\s*(ba|z|da)?sh`),
	regexp.MustCompile(`(?i)wget\s+.*\|\s*(ba|z|da)?sh`),
	// ─── Интерпретаторы с произвольным выполнением кода ─────
	regexp.MustCompile(`(?i)\bpython3?\s+-c\s`),
	regexp.MustCompile(`(?i)\bnode\s+-e\s`),
	regexp.MustCompile(`(?i)\bperl\s+-e\s`),
	regexp.MustCompile(`(?i)\bruby\s+-e\s`),
	regexp.MustCompile(`(?i)\bphp\s+-r\s`),
	regexp.MustCompile(`(?i)\blua\s+-e\s`),

	// ─── Обход через кодирование ────────────────────────────
	regexp.MustCompile(`(?i)\bbase64\s+(-d|-D|--decode)\b.*\|\s*(ba|z|da)?sh`),
	regexp.MustCompile(`(?i)\bxxd\s+-r\b.*\|\s*(ba|z|da)?sh`),

	// ─── find с деструктивными действиями ───────────────────
	regexp.MustCompile(`(?i)\bfind\b.*-delete\b`),
	regexp.MustCompile(`(?i)\bfind\b.*-exec\s+rm\b`),
	regexp.MustCompile(`(?i)\bfind\b.*-execdir\s+rm\b`),

	// ─── xargs с деструктивными командами ───────────────────
	regexp.MustCompile(`(?i)\bxargs\b.*\brm\b`),
	regexp.MustCompile(`(?i)\bxargs\b.*\bshred\b`),

	// ─── Альтернативы sudo ──────────────────────────────────
	regexp.MustCompile(`(?i)\bpkexec\b`),
	regexp.MustCompile(`(?i)\brunuser\b`),
	regexp.MustCompile(`(?i)\bsu\s+-c\b`),
}

// ─── HIGH: требует обязательного подтверждения ─────────────────────
var highRiskPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\brm\b`),
	regexp.MustCompile(`(?i)\bmv\b.*/(etc|usr|bin|sbin|boot|sys|proc)\b`),
	regexp.MustCompile(`(?i)\bsudo\b`),
	regexp.MustCompile(`(?i)\bdoas\b`),
	regexp.MustCompile(`(?i)\bchmod\b`),
	regexp.MustCompile(`(?i)\bchown\b`),
	regexp.MustCompile(`(?i)\bsystemctl\s+(start|stop|restart|enable|disable)\b`),
	regexp.MustCompile(`(?i)\bservice\s+\w+\s+(start|stop|restart)`),
	regexp.MustCompile(`(?i)\b(apt|apt-get)\s+(remove|purge)\b`),
	regexp.MustCompile(`(?i)\bdnf\s+(remove|erase)\b`),
	regexp.MustCompile(`(?i)\bpacman\s+-R`),
	regexp.MustCompile(`(?i)\bbrew\s+uninstall\b`),
	regexp.MustCompile(`(?i)\btruncate\b`),
	regexp.MustCompile(`(?i)\bshred\b`),
	// ─── Сетевые команды (эксфильтрация / загрузка payload) ─
	regexp.MustCompile(`(?i)\bcurl\b`),
	regexp.MustCompile(`(?i)\bwget\b`),
	regexp.MustCompile(`(?i)\bnc\b`),
	regexp.MustCompile(`(?i)\bncat\b`),
	regexp.MustCompile(`(?i)\bssh\b`),
	regexp.MustCompile(`(?i)\bscp\b`),
	regexp.MustCompile(`(?i)\bsftp\b`),
	regexp.MustCompile(`(?i)\bftp\b`),
	regexp.MustCompile(`(?i)\btelnet\b`),
	regexp.MustCompile(`(?i)\bopenssl\s+s_client\b`),

	// ─── Интерпретаторы (выполнение произвольного кода) ─────
	regexp.MustCompile(`(?i)\bpython3?\b`),
	regexp.MustCompile(`(?i)\bnode\b`),
	regexp.MustCompile(`(?i)\bperl\b`),
	regexp.MustCompile(`(?i)\bruby\b`),
	regexp.MustCompile(`(?i)\bphp\b`),
	regexp.MustCompile(`(?i)\blua\b`),

	// ─── Инструменты с произвольным выполнением ─────────────
	regexp.MustCompile(`(?i)\bfind\b`),
	regexp.MustCompile(`(?i)\bgit\b`),
	regexp.MustCompile(`(?i)\bgo\s+(run|tool)\b`),
	regexp.MustCompile(`(?i)\bxargs\b`),
	regexp.MustCompile(`(?i)\bmake\b`),
	regexp.MustCompile(`(?i)\bdocker\s+exec\b`),
}

// ─── MEDIUM: логируется, не блокируется ────────────────────────────
var mediumRiskPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(apt|apt-get|dnf|yum|pacman|brew|winget|apk|zypper)\s+(install|add)\b`),
	regexp.MustCompile(`(?i)\bgit\s+clone\b`),
	regexp.MustCompile(`(?i)\bwget\b`),
	regexp.MustCompile(`(?i)\bcurl\s+-[oO]`),
	regexp.MustCompile(`(?i)\bmkdir\b`),
	regexp.MustCompile(`(?i)\bcp\b`),
	regexp.MustCompile(`(?i)\bmv\b`),
	regexp.MustCompile(`(?i)\btar\b.*-x`),
	regexp.MustCompile(`(?i)\bunzip\b`),
	regexp.MustCompile(`(?i)\bpip3?\s+install\b`),
	regexp.MustCompile(`(?i)\bnpm\s+install\b`),
	regexp.MustCompile(`(?i)\bgo\s+install\b`),
	regexp.MustCompile(`(?i)\bcargo\s+install\b`),
	regexp.MustCompile(`(?i)\bdocker\s+(run|pull|build)\b`),
	regexp.MustCompile(`(?i)\bgit\s+(status|log|diff|branch|show)\b`),
	regexp.MustCompile(`(?i)\bgo\s+(build|test|vet|fmt|mod)\b`),
}

// ─── LOW: безопасные команды ──────────────────────────────────────
var safeCommands = map[string]bool{
	"ls": true, "cat": true, "pwd": true, "echo": true,
	"which": true, "whereis": true, "type": true,
	"env": true, "printenv": true, "id": true, "whoami": true,
	"uname": true, "hostname": true, "date": true,
	"df": true, "du": true, "free": true, "uptime": true,
	"ps": true, "pgrep": true,
	"grep": true, "egrep": true, "fgrep": true,
	"locate": true, "wc": true,
	"head": true, "tail": true, "sort": true, "uniq": true,
	"file": true, "stat": true, "readlink": true,
	"basename": true, "dirname": true, "realpath": true,
	"md5sum": true, "sha256sum": true, "sha1sum": true,
	"less": true, "more": true, "diff": true, "cmp": true,
	"true": true, "false": true, "test": true, "[": true,
}

// ─── Запись разрешена только в эти директории ──────────────────────
func defaultWritableDirs(workDir string) []string {
	dirs := []string{"/tmp"}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, home)
	}
	for _, env := range []string{"XDG_DATA_HOME", "XDG_CONFIG_HOME", "XDG_CACHE_HOME"} {
		if v := os.Getenv(env); v != "" {
			dirs = append(dirs, v)
		}
	}
	if workDir != "" {
		dirs = append(dirs, workDir)
	}
	return dirs
}

// ─── Главная функция оценки ────────────────────────────────────────
// AssessCommand оценивает риск команды детерминированно (без LLM).
// Вызывается ДО любого обращения к LLM.
func AssessCommand(command string, workDir string) CommandAssessment {
	a := CommandAssessment{Command: command, Risk: RiskLow}

	expanded := expandForAnalysis(command)

	// 1. FORBIDDEN
	for _, p := range forbiddenPatterns {
		if p.MatchString(expanded) {
			a.Risk = RiskForbidden
			a.Reason = "matches forbidden pattern: " + p.String()
			return a
		}
	}

	// 1.5. Подстановка команд — автоматически HIGH
	if ContainsCommandSubstitution(expanded) {
		a.Risk = RiskHigh
		a.Reason = "contains command substitution ($(...), `...`, >(...), <(...))"
		return a
	}

	// 2. HIGH
	for _, p := range highRiskPatterns {
		if p.MatchString(expanded) {
			a.Risk = RiskHigh
			a.Reason = "matches high-risk pattern: " + p.String()
			if strings.Contains(expanded, "sudo") || strings.Contains(expanded, "doas") {
				a.RequiresSudo = true
			}
			return a
		}
	}

	// 3. MEDIUM
	for _, p := range mediumRiskPatterns {
		if p.MatchString(expanded) {
			a.Risk = RiskMedium
			a.Reason = "matches medium-risk pattern: " + p.String()
			return a
		}
	}

	// 4. Известная безопасная команда
	first := firstCommand(expanded)
	if safeCommands[first] {
		a.Risk = RiskLow
		a.Reason = "known safe command: " + first
		return a
	}

	// 5. Неизвестная команда → MEDIUM
	a.Risk = RiskMedium
	a.Reason = "unknown command, defaulting to medium risk"
	return a
}

// AssessChain разбивает цепочку и оценивает каждую часть.
// Возвращает максимальный риск и первую запрещённую команду (если есть).
func AssessChain(command string, workDir string) (RiskLevel, string, error) {
	if ContainsCommandSubstitution(command) {
		return RiskHigh,
			"contains command substitution ($(...), `...`, >(...), <(...))",
			nil
	}

	if !ContainsPipeOrChain(command) {
		a := AssessCommand(command, workDir)
		return a.Risk, a.Reason, nil
	}
	parts := SplitCommandChain(command)
	maxRisk := RiskLow
	for _, part := range parts {
		a := AssessCommand(part, workDir)
		if a.Risk == RiskForbidden {
			return RiskForbidden, a.Reason, fmt.Errorf(
				"FORBIDDEN command in chain: %s (%s)", part, a.Reason,
			)
		}
		if a.Risk > maxRisk {
			maxRisk = a.Risk
		}
	}
	return maxRisk, "chain assessed", nil
}

// IsWriteAllowed проверяет, разрешена ли запись в путь.
func IsWriteAllowed(path string, workDir string) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	for _, dir := range defaultWritableDirs(workDir) {
		absDir, err := filepath.Abs(dir)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(absDir, abs)
		if err != nil {
			continue
		}
		if !strings.HasPrefix(rel, "..") {
			return true
		}
	}
	return false
}

// ─── Вспомогательные функции ───────────────────────────────────────

func expandForAnalysis(command string) string {
	home, _ := os.UserHomeDir()
	pwd, _ := os.Getwd()
	command = strings.ReplaceAll(command, "~/", home+"/")
	command = strings.ReplaceAll(command, "~", home)
	command = strings.ReplaceAll(command, "${HOME}", home)
	command = strings.ReplaceAll(command, "$HOME", home)
	command = strings.ReplaceAll(command, "${PWD}", pwd)
	command = strings.ReplaceAll(command, "$PWD", pwd)

	// Раскрываем переменные окружения (один уровень)
	// для обнаружения обходов через $VAR
	for _, kv := range os.Environ() {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) != 2 {
			continue
		}
		name, value := parts[0], parts[1]
		// Пропускаем длинные значения и потенциально опасные
		if len(value) > 512 || strings.ContainsAny(value, "\n\r\x00") {
			continue
		}
		command = strings.ReplaceAll(command, "${"+name+"}", value)
		// Заменяем $VAR только если за ним не следует буква/цифра/подчёркивание
		command = replaceEnvVar(command, name, value)
	}
	return command
}

// replaceEnvVar заменяет $NAME на value, но только если после NAME
// не следует допустимый символ имени переменной (буква, цифра, _).
func replaceEnvVar(command, name, value string) string {
	prefix := "$" + name
	idx := 0
	var b strings.Builder
	for {
		i := strings.Index(command[idx:], prefix)
		if i == -1 {
			b.WriteString(command[idx:])
			break
		}
		b.WriteString(command[idx : idx+i])
		end := idx + i + len(prefix)
		if end < len(command) {
			next := command[end]
			if (next >= 'a' && next <= 'z') ||
				(next >= 'A' && next <= 'Z') ||
				(next >= '0' && next <= '9') ||
				next == '_' {
				// Это часть более длинного имени переменной
				b.WriteString(prefix)
				idx = end
				continue
			}
		}
		b.WriteString(value)
		idx = end
	}
	return b.String()
}

func firstCommand(command string) string {
	command = strings.TrimSpace(command)
	for _, prefix := range []string{"sudo ", "doas ", "env "} {
		command = strings.TrimPrefix(command, prefix)
		command = strings.TrimSpace(command)
	}
	for strings.Contains(command, "=") {
		fields := strings.Fields(command)
		if len(fields) == 0 {
			break
		}
		if strings.Contains(fields[0], "=") {
			command = strings.Join(fields[1:], " ")
		} else {
			break
		}
	}
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return ""
	}
	return filepath.Base(fields[0])
}

func ContainsPipeOrChain(command string) bool {
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
		case ';':
			if !inSingle && !inDouble {
				return true
			}
		case '|':
			if !inSingle && !inDouble {
				return true
			}
		case '&':
			if !inSingle && !inDouble {
				return true
			}
		}
	}
	return false
}

func SplitCommandChain(command string) []string {
	var parts []string
	var cur strings.Builder
	inSingle, inDouble := false, false

	for i := 0; i < len(command); i++ {
		ch := command[i]
		switch ch {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
			cur.WriteByte(ch)
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
			cur.WriteByte(ch)
		case ';', '|', '&':
			if !inSingle && !inDouble {
				if s := strings.TrimSpace(cur.String()); s != "" {
					parts = append(parts, s)
				}
				cur.Reset()
				if (ch == '|' || ch == '&') && i+1 < len(command) && command[i+1] == ch {
					i++
				}
			} else {
				cur.WriteByte(ch)
			}
		default:
			cur.WriteByte(ch)
		}
	}
	if s := strings.TrimSpace(cur.String()); s != "" {
		parts = append(parts, s)
	}
	return parts
}

// ContainsCommandSubstitution проверяет наличие подстановки команд,
// которая позволяет обойти regex-оценку: $(...), `...`, >(...), <(...)
func ContainsCommandSubstitution(command string) bool {
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
		case '$':
			if !inSingle && i+1 < len(command) && command[i+1] == '(' {
				return true
			}
		case '`':
			if !inSingle && !inDouble {
				return true
			}
		case '>':
			if !inSingle && !inDouble && i+1 < len(command) && command[i+1] == '(' {
				return true
			}
		case '<':
			if !inSingle && !inDouble && i+1 < len(command) && command[i+1] == '(' {
				return true
			}
		}
	}
	return false
}