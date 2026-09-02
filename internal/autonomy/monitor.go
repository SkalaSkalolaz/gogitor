package autonomy

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"log/slog"

    "gogitor/internal/i18n"
	"gogitor/internal/config"
	"gogitor/internal/workspace"
)

// Monitor — фоновый мониторинг проекта.
// Работает полностью детерминированно, без LLM.
// Запускает go build и go vet, сканирует TODO-маркеры.
// Результаты складывает в TaskQueue.
type Monitor struct {
	ws    *workspace.Workspace
	queue *TaskQueue
	cfg   *config.Config
	log   *slog.Logger

	mu           sync.Mutex
	running      bool
	stopCh       chan struct{}
	lastRun      time.Time
	lastBuildErr string
	lastVetOut   string
}

func NewMonitor(ws *workspace.Workspace, q *TaskQueue, cfg *config.Config, log *slog.Logger) *Monitor {
	return &Monitor{
		ws:    ws,
		queue: q,
		cfg:   cfg,
		log:   log,
	}
}

// Start запускает фоновый цикл мониторинга.
// Вызывается по команде :autonomy on.
func (m *Monitor) Start() {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	m.running = true
	m.stopCh = make(chan struct{})
	m.mu.Unlock()

	go m.loop()
}

// Stop останавливает фоновый цикл.
// Вызывается по команде :autonomy off и при закрытии Service.
func (m *Monitor) Stop() {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return
	}
	m.running = false
	close(m.stopCh)
	m.mu.Unlock()
}

func (m *Monitor) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

func (m *Monitor) loop() {
	interval := time.Duration(m.cfg.AutonomyIntervalSec) * time.Second
	if interval <= 0 {
		interval = 60 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Первый запуск сразу после включения
	m.check()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.check()
		}
	}
}

// check выполняет один цикл проверки.
// Использует прямые вызовы go build / go vet / сканер TODO,
// не затрагивая существующий Runner и не создавая песочницу.
func (m *Monitor) check() {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	m.mu.Lock()
	m.lastRun = time.Now()
	m.mu.Unlock()

	// 1. go build — только компиляция, без создания файлов
	if err := m.checkBuild(ctx); err != nil {
		m.queue.Add("build_error", "Build failed", err.Error(), "", 0, PriorityCritical)
		m.mu.Lock()
		m.lastBuildErr = err.Error()
		m.mu.Unlock()
	} else {
		m.mu.Lock()
		m.lastBuildErr = ""
		m.mu.Unlock()
	}

	// 2. go vet — статический анализ
	if out, err := m.checkVet(ctx); err != nil {
		m.queue.Add("vet_warning", "go vet found issues", out, "", 0, PriorityHigh)
		m.mu.Lock()
		m.lastVetOut = out
		m.mu.Unlock()
	} else {
		m.mu.Lock()
		m.lastVetOut = ""
		m.mu.Unlock()
	}

	// 3. TODO-сканер (уже существует в workspace.go, без LLM)
	todoItems := m.ws.ScanTODOs(10)
	for _, item := range todoItems {
		m.queue.Add(
			"todo",
			item.Kind+": "+item.Text,
			item.Text,
			item.File,
			item.Line,
			PriorityLow,
		)
	}
}

// checkBuild запускает go build без побочных эффектов.
// Использует -o /dev/null, чтобы не создавать бинарный файл.
func (m *Monitor) checkBuild(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "go", "build", "-o", os.DevNull, "./...")
	cmd.Dir = m.ws.Root
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("build failed:\n%s", strings.TrimSpace(string(out)))
	}
	return nil
}

// checkVet запускает go vet и возвращает вывод.
func (m *Monitor) checkVet(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "go", "vet", "./...")
	cmd.Dir = m.ws.Root
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))
	if err != nil && output != "" {
		return output, err
	}
	return output, nil
}

// Status возвращает текущее состояние монитора.

func (m *Monitor) Status() string {
    m.mu.Lock()
    defer m.mu.Unlock()
    if !m.running {
        return i18n.T("Autonomy monitor: stopped")
    }
    info := i18n.T("Autonomy monitor: running (last check: %s)", m.lastRun.Format("15:04:05"))
    if m.lastBuildErr != "" {
        info += "\n" + i18n.T("⚠ Build: FAILING")
    } else {
        info += "\n" + i18n.T("✓ Build: passing")
    }
    if m.lastVetOut != "" {
        info += "\n" + i18n.T("⚠ go vet: issues found")
    } else {
        info += "\n" + i18n.T("✓ go vet: clean")
    }
    return info
}