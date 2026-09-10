package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gogitor/internal/agent"
)

// llmStats хранит приближённую историческую статистику длительности LLM-запросов.
// Используется для оценки ETA в TUI.
type llmStats struct {
	mu      sync.Mutex
	path    string
	Entries map[string]statEntry `json:"entries"`
}

type statEntry struct {
	Count         int       `json:"count"`
	AvgDurationMs int64     `json:"avg_duration_ms"`
	AvgTokens     int       `json:"avg_tokens"`
	Updated       time.Time `json:"updated"`
}

func loadLLMStats(root string) *llmStats {
	s := &llmStats{
		path:    filepath.Join(root, ".gogitor", "llm_stats.json"),
		Entries: make(map[string]statEntry),
	}

	data, err := os.ReadFile(s.path)
	if err != nil {
		return s
	}

	_ = json.Unmarshal(data, s)

	if s.Entries == nil {
		s.Entries = make(map[string]statEntry)
	}

	return s
}

func (s *llmStats) record(
	role agent.Role,
	purpose, prompt string,
	d time.Duration,
	tokens int,
) {
	if s == nil || d <= 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := statsKey(role, purpose, prompt)
	e := s.Entries[key]

	newMs := d.Milliseconds()

	if e.Count == 0 {
		e.AvgDurationMs = newMs
	} else {
		// EWMA: новые наблюдения важнее старых, но история не сбрасывается резко.
		e.AvgDurationMs = int64(float64(e.AvgDurationMs)*0.7 + float64(newMs)*0.3)
	}

	e.AvgTokens = int(float64(e.AvgTokens)*0.7 + float64(tokens)*0.3)
	e.Count++
	e.Updated = time.Now()

	s.Entries[key] = e
	s.saveLocked()
}

func (s *llmStats) estimate(role agent.Role, purpose, prompt string) time.Duration {
	if s == nil {
		return heuristicDuration((len(prompt) + 3) / 4)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := statsKey(role, purpose, prompt)

	if e, ok := s.Entries[key]; ok && e.Count > 0 {
		return time.Duration(e.AvgDurationMs) * time.Millisecond
	}

	return heuristicDuration((len(prompt) + 3) / 4)
}

func (s *llmStats) saveLocked() {
	if s.path == "" {
		return
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}

	_ = os.Rename(tmp, s.path)
}

func statsKey(role agent.Role, purpose, prompt string) string {
	return string(role) + "/" + purposeCategory(purpose) + "/" + promptBucket(prompt)
}

func promptBucket(prompt string) string {
	size := len(prompt)
	switch {
	case size < 2000:
		return "s"
	case size < 10000:
		return "m"
	case size < 100000:
		return "l"
	default:
		return "xl"
	}
}

func purposeCategory(purpose string) string {
	lower := strings.ToLower(purpose)
	switch {
	case strings.Contains(lower, "analyze"):
		return "analyze"
	case strings.Contains(lower, "subtask"),
		strings.Contains(lower, "coder"),
		strings.Contains(lower, "code"):
		return "code"
	case strings.Contains(lower, "plan"):
		return "plan"

	case strings.Contains(lower, "review"):
		return "review"

	case strings.Contains(lower, "verify"):
		return "verify"

	case strings.Contains(lower, "intent"):
		return "intent"

	case strings.Contains(lower, "chat"):
		return "chat"

	case strings.Contains(lower, "search"):
		return "search"

	case strings.Contains(lower, "commit"):
		return "commit"

	case strings.Contains(lower, "compare"):
		return "compare"

	default:
		return "other"
	}
}

func heuristicDuration(tokens int) time.Duration {
	ms := 3000 + tokens*8

	if ms < 1500 {
		ms = 1500
	}

	if ms > 180000 {
		ms = 180000
	}

	return time.Duration(ms) * time.Millisecond
}

// ─── Оценка полных подзадач ──────────────────────────────────────

// recordSubtask записывает реальную длительность выполнения
// всей подзадачи (coder + reviewer + build + test + gates + commit).
func (s *llmStats) recordSubtask(prompt string, d time.Duration) {
	if s == nil || d <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := "subtask/full/" + promptBucket(prompt)
	e := s.Entries[key]
	newMs := d.Milliseconds()
	if e.Count == 0 {
		e.AvgDurationMs = newMs
	} else {
		e.AvgDurationMs = int64(
			float64(e.AvgDurationMs)*0.7 + float64(newMs)*0.3,
		)
	}
	e.Count++
	e.Updated = time.Now()
	s.Entries[key] = e
	s.saveLocked()
}

// estimateSubtask оценивает полную длительность подзадачи,
// включая все этапы: coder, reviewer, verifier, сборку,
// тесты, quality gates и commit.
func (s *llmStats) estimateSubtask(
	taskPrompt string,
	multiAgent bool,
) time.Duration {
	if s == nil {
		return heuristicSubtaskDuration(
			(len(taskPrompt)+3)/4, multiAgent,
		)
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. Если есть накопленная статистика полных подзадач
	//    (≥2 наблюдений) — используем её.
	subtaskKey := "subtask/full/" + promptBucket(taskPrompt)
	if e, ok := s.Entries[subtaskKey]; ok && e.Count >= 2 {
		return time.Duration(e.AvgDurationMs) * time.Millisecond
	}

	// 2. Иначе суммируем оценки отдельных этапов.
	total := time.Duration(0)

	// Coder-запрос
	coderKey := statsKey(agent.RoleCoder, "code", taskPrompt)
	if e, ok := s.Entries[coderKey]; ok && e.Count > 0 {
		total += time.Duration(e.AvgDurationMs) * time.Millisecond
	} else {
		total += heuristicDuration((len(taskPrompt) + 3) / 4)
	}

	// Сборка + зависимости
	total += 30 * time.Second

	// Тесты
	total += 20 * time.Second

	if multiAgent {
		// Reviewer-запрос
		reviewerKey := string(agent.RoleReviewer) + "/review/m"
		if e, ok := s.Entries[reviewerKey]; ok && e.Count > 0 {
			total += time.Duration(e.AvgDurationMs) * time.Millisecond
		} else {
			total += 60 * time.Second
		}
		// Verifier-запрос
		verifierKey := string(agent.RoleVerifier) + "/verify/m"
		if e, ok := s.Entries[verifierKey]; ok && e.Count > 0 {
			total += time.Duration(e.AvgDurationMs) * time.Millisecond
		} else {
			total += 60 * time.Second
		}
	}

	// Quality gates (vet, gofmt, lint, go mod tidy ×4)
	total += 40 * time.Second

	// Git commit
	total += 10 * time.Second

	// Коэффициент на возможные итерации исправления (×1.3)
	total = time.Duration(float64(total) * 1.3)

	return total
}

// heuristicSubtaskDuration — эвристика для подзадачи
// при отсутствии накопленной статистики.
func heuristicSubtaskDuration(tokens int, multiAgent bool) time.Duration {
	total := heuristicDuration(tokens) // coder-запрос
	total += 50 * time.Second          // build + test + tidy
	if multiAgent {
		total += 120 * time.Second // reviewer + verifier
	}
	total += 50 * time.Second                   // quality gates + commit
	total = time.Duration(float64(total) * 1.5) // запас на итерации
	return total
}
