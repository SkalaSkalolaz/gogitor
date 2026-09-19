package llama

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Alias — имя, под которым llama-server регистрирует загруженную модель.
// Gogitor использует его в запросах к /v1/chat/completions.
const Alias = "gogitor-model"

// DefaultContextSize — размер контекста по умолчанию, если не задан в ExtraArgs.
const DefaultContextSize = 16384

// Config — параметры запуска llama-server.
type Config struct {
	BinPath   string   // путь к llama-server (по умолчанию ищется в PATH)
	ModelPath string   // путь к .gguf
	Host      string   // 127.0.0.1
	Port      int      // 55555
	ExtraArgs []string // дополнительные флаги
	LogDir    string   // куда писать лог (обычно cfg.WorkDir)
}

// Manager управляет жизненным циклом llama-server.
type Manager struct {
	cfg    Config
	cmd    *exec.Cmd
	logFile *os.File

	mu     sync.Mutex
	client *http.Client
}

// New создаёт менеджер без запуска процесса.
func New(cfg Config) *Manager {
	if cfg.Host == "" {
		cfg.Host = "127.0.0.1"
	}
	if cfg.Port == 0 {
		cfg.Port = 55555
	}
	return &Manager{
		cfg:    cfg,
		client: &http.Client{Timeout: 2 * time.Second},
	}
}

// Start запускает llama-server и ждёт готовности.
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cmd != nil {
		return fmt.Errorf("llama-server already running")
	}

	// Нормализуем путь к модели: разворачиваем ~ и делаем абсолютным.
	modelPath, err := normalizeModelPath(m.cfg.ModelPath)
	if err != nil {
		return err
	}
	if _, err := os.Stat(modelPath); err != nil {
		return fmt.Errorf("model file is not accessible: %w", err)
	}

	binPath := m.cfg.BinPath
	if binPath == "" {
		binPath = "llama-server"
	}

	// Порт занят — значит либо наш прошлый инстанс, либо кто-то другой.
	if m.portInUse() {
		return fmt.Errorf(
			"port %d is already in use; stop the other process or set --llama-port to a free port",
			m.cfg.Port,
		)
	}

	// Собираем аргументы.
	args := []string{
		"-m", modelPath,
		"-a", Alias,
		"--host", m.cfg.Host,
		"--port", strconv.Itoa(m.cfg.Port),
	}
	args = append(args, defaultArgs()...)
	args = append(args, m.cfg.ExtraArgs...)

	cmd := exec.Command(binPath, args...)
	cmd.SysProcAttr = procAttr()

	// Лог.
	logPath := filepath.Join(m.cfg.LogDir, ".gogitor", "llama-server.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err == nil {
		if lf, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
			cmd.Stdout = lf
			cmd.Stderr = lf
			m.logFile = lf
		}
	}
	if cmd.Stdout == nil {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	if err := cmd.Start(); err != nil {
		if m.logFile != nil {
			m.logFile.Close()
			m.logFile = nil
		}
		return fmt.Errorf("cannot start llama-server (%s): %w", binPath, err)
	}

	m.cmd = cmd

	// Ожидание готовности (модель 176B грузится 1-3 минуты).
	if err := m.waitReady(ctx, 5*time.Minute); err != nil {
		_ = m.stopLocked()
		return err
	}
	return nil
}

// waitReady периодически опрашивает /v1/models.
func (m *Manager) waitReady(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if m.serverResponds(ctx) {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("llama-server did not become ready within %s (see .gogitor/llama-server.log)", timeout)
}

func (m *Manager) serverResponds(ctx context.Context) bool {
	url := fmt.Sprintf("http://%s:%d/v1/models", m.cfg.Host, m.cfg.Port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (m *Manager) portInUse() bool {
	conn, err := net.DialTimeout(
		"tcp",
		fmt.Sprintf("%s:%d", m.cfg.Host, m.cfg.Port),
		500*time.Millisecond,
	)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// Stop мягко останавливает llama-server.
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	_ = m.stopLocked()
}

func (m *Manager) stopLocked() error {
	if m.cmd == nil || m.cmd.Process == nil {
		if m.logFile != nil {
			m.logFile.Close()
			m.logFile = nil
		}
		return nil
	}

	// SIGTERM всей группе процессов.
	_ = syscall.Kill(-m.cmd.Process.Pid, syscall.SIGTERM)

	done := make(chan error, 1)
	go func() { done <- m.cmd.Wait() }()

	select {
	case <-done:
	case <-time.After(15 * time.Second):
		_ = syscall.Kill(-m.cmd.Process.Pid, syscall.SIGKILL)
		<-done
	}

	if m.logFile != nil {
		m.logFile.Close()
		m.logFile = nil
	}
	m.cmd = nil
	return nil
}

// BaseURL возвращает OpenAI-совместимый base URL.
func (m *Manager) BaseURL() string {
	return fmt.Sprintf("http://%s:%d/v1", m.cfg.Host, m.cfg.Port)
}

// ContextSize пытается извлечь -c из ExtraArgs или возвращает DefaultContextSize.
func (m *Manager) ContextSize() int {
	return extractContextSize(m.cfg.ExtraArgs, DefaultContextSize)
}

// ─── helpers ────────────────────────────────────────────────────────

func defaultArgs() []string {
	// Значения, оптимальные для MoE-моделей на ограниченной VRAM.
	// -ngl НЕ передаём: llama.cpp сам распределяет слои через --fit,
	// а ручное указание ломает авто-подгонку под VRAM.
	return []string{
		"-c", strconv.Itoa(DefaultContextSize),
		"-b", "2048",
		"-ub", "1024",
		"-np", "1",
		"--cache-type-k", "q8_0",
		"--cache-type-v", "q8_0",
	}
}

func extractContextSize(args []string, def int) int {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-c" || args[i] == "--ctx-size" {
			if n, err := strconv.Atoi(args[i+1]); err == nil && n > 0 {
				return n
			}
		}
	}
	return def
}

func normalizeModelPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", fmt.Errorf("empty model path")
	}
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			p = filepath.Join(home, p[2:])
		}
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	return abs, nil
}