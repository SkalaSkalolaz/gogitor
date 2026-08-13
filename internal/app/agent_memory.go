package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gogitor/internal/domain"
)

type agentMemory struct {
	Conventions      []string               `json:"conventions,omitempty"`
	Decisions        []string               `json:"decisions,omitempty"`
	FailedApproaches []string               `json:"failed_approaches,omitempty"`
	Lessons          []string               `json:"lessons,omitempty"`
	DecisionLog      []domain.DecisionEntry `json:"decision_log,omitempty"`
	UpdatedAt        time.Time              `json:"updated_at,omitempty"`
	nextDecisionID   int
}

func agentMemoryPath(root string) string {
	return filepath.Join(root, ".gogitor", "agent_memory.json")
}

func loadAgentMemory(root string) *agentMemory {
	m := &agentMemory{}
	data, err := os.ReadFile(agentMemoryPath(root))
	if err != nil {
		return m
	}
	_ = json.Unmarshal(data, m)
	// Вычисляем следующий ID.
	m.nextDecisionID = 1
	for _, e := range m.DecisionLog {
		if e.ID >= m.nextDecisionID {
			m.nextDecisionID = e.ID + 1
		}
	}
	return m
}

func (m *agentMemory) save(root string) error {
	if m == nil {
		return nil
	}
	m.UpdatedAt = time.Now()
	dir := filepath.Dir(agentMemoryPath(root))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp := agentMemoryPath(root) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, agentMemoryPath(root))
}

func (m *agentMemory) addConvention(s string) {
	m.Conventions = appendLimited(m.Conventions, s, 50)
}

func (m *agentMemory) addDecision(s string) {
	m.Decisions = appendLimited(m.Decisions, s, 80)
}

func (m *agentMemory) addFailed(s string) {
	m.FailedApproaches = appendLimited(m.FailedApproaches, s, 80)
}

func (m *agentMemory) addLesson(s string) {
	m.Lessons = appendLimited(m.Lessons, s, 30)
}

// addDecisionEntry добавляет структурированную запись в журнал решений.
func (m *agentMemory) addDecisionEntry(entry domain.DecisionEntry) {
	if entry.ID == 0 {
		entry.ID = m.nextDecisionID
		m.nextDecisionID++
	}
	if entry.Date == "" {
		entry.Date = time.Now().Format("2006-01-02 15:04")
	}
	m.DecisionLog = append(m.DecisionLog, entry)
	// Ограничиваем размер журнала.
	if len(m.DecisionLog) > 200 {
		m.DecisionLog = m.DecisionLog[len(m.DecisionLog)-200:]
	}
}

// addDecisionSimple — обёртка для быстрой записи решения без мета-данных.
func (m *agentMemory) addDecisionSimple(decision, source string) {
	m.addDecisionEntry(domain.DecisionEntry{
		Decision: decision,
		Source:   source,
	})
}

// addTemporaryDecision записывает решение, принятое как временное.
func (m *agentMemory) addTemporaryDecision(decision, constraint, source string) {
	m.addDecisionEntry(domain.DecisionEntry{
		Decision:  decision,
		Temporary: true,
		Constraint: constraint,
		Source:    source,
	})
}

// addDecisionWithAlternatives записывает решение с отклонёнными альтернативами.
func (m *agentMemory) addDecisionWithAlternatives(
	decision, context string,
	alternatives []domain.Alternative,
	source string,
) {
	m.addDecisionEntry(domain.DecisionEntry{
		Decision:     decision,
		Context:      context,
		Alternatives: alternatives,
		Source:       source,
	})
}

// journal возвращает доменный журнал решений.
func (m *agentMemory) journal() *domain.DecisionJournal {
	if m == nil {
		return &domain.DecisionJournal{}
	}
	return &domain.DecisionJournal{
		Entries:          m.DecisionLog,
		FailedApproaches: m.FailedApproaches,
	}
}

// summary возвращает краткую текстовую сводку памяти для передачи в промпты.
func (m *agentMemory) summary(maxItems int) string {
	if m == nil {
		return ""
	}
	if maxItems <= 0 {
		maxItems = 20
	}
	var b strings.Builder
	if len(m.Conventions) > 0 {
		b.WriteString("conventions:\n")
		for _, item := range lastN(m.Conventions, maxItems) {
			b.WriteString("- ")
			b.WriteString(item)
			b.WriteByte('\n')
		}
	}
	if len(m.Decisions) > 0 {
		b.WriteString("decisions:\n")
		for _, item := range lastN(m.Decisions, maxItems) {
			b.WriteString("- ")
			b.WriteString(item)
			b.WriteByte('\n')
		}
	}
	if len(m.FailedApproaches) > 0 {
		b.WriteString("failed approaches to avoid:\n")
		for _, item := range lastN(m.FailedApproaches, maxItems) {
			b.WriteString("- ")
			b.WriteString(item)
			b.WriteByte('\n')
		}
	}
	if len(m.Lessons) > 0 {
		b.WriteString("lessons from previous workflows:\n")
		for _, item := range lastN(m.Lessons, maxItems) {
			b.WriteString("- ")
			b.WriteString(item)
			b.WriteByte('\n')
		}
	}

	return strings.TrimSpace(b.String())
}

// journalForPrompt формирует текстовое представление журнала для LLM.
func (m *agentMemory) journalForPrompt(maxEntries int) string {
	if m == nil || len(m.DecisionLog) == 0 {
		return ""
	}
	if maxEntries <= 0 {
		maxEntries = 50
	}
	entries := m.DecisionLog
	if len(entries) > maxEntries {
		entries = entries[len(entries)-maxEntries:]
	}
	var b strings.Builder
	for _, e := range entries {
		fmt.Fprintf(&b, "[%s] #%d: %s", e.Date, e.ID, e.Decision)
		if e.Temporary {
			b.WriteString(" (ВРЕМЕННОЕ)")
		}
		if e.Constraint != "" {
			fmt.Fprintf(&b, " | ограничение: %s", e.Constraint)
		}
		if e.Context != "" {
			fmt.Fprintf(&b, " | контекст: %s", e.Context)
		}
		for _, alt := range e.Alternatives {
			fmt.Fprintf(&b, "\n    ✗ отклонено: %s (причина: %s)", alt.Description, alt.Reason)
		}
		b.WriteByte('\n')
	}
	if len(m.FailedApproaches) > 0 {
		b.WriteString("\nНеудачные подходы:\n")
		for _, f := range lastN(m.FailedApproaches, 20) {
			b.WriteString("  ✗ ")
			b.WriteString(f)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func appendLimited(list []string, s string, max int) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return list
	}
	list = append(list, s)
	if len(list) > max {
		list = list[len(list)-max:]
	}
	return list
}

func lastN(list []string, n int) []string {
	if n <= 0 || len(list) <= n {
		return list
	}
	return list[len(list)-n:]
}