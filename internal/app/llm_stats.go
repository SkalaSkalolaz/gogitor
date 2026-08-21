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
	tokens := (len(prompt) + 3) / 4

	switch {
	case tokens < 1024:
		return "s"
	case tokens < 4096:
		return "m"
	case tokens < 16384:
		return "l"
	default:
		return "xl"
	}
}

func purposeCategory(purpose string) string {
	lower := strings.ToLower(purpose)

	switch {
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

	case strings.Contains(lower, "analyze"):
		return "analyze"

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