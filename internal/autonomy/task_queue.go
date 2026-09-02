package autonomy

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// TaskPriority — приоритет автономной задачи.
// Детерминированная приоритизация: критические тесты → линт → покрытие → долг.
type TaskPriority int

const (
	PriorityCritical TaskPriority = 100 // падающая сборка, падающие тесты
	PriorityHigh     TaskPriority = 80  // go vet, линт
	PriorityNormal   TaskPriority = 50  // отсутствие тестов, покрытие
	PriorityLow      TaskPriority = 20  // TODO, технический долг
)

// QueuedTask — одна задача в очереди автономии.
type QueuedTask struct {
	ID       int
	Priority TaskPriority
	Source   string // build_error, vet_warning, test_fail, lint, todo, coverage
	Title    string
	Detail   string
	FilePath string
	Line     int
	Created  time.Time
	Status   string // pending, applied, failed, dismissed
}

// TaskQueue — потокобезопасная очередь задач.
// Задачи формируются детерминированно (монитором, мутатором, сканером),
// а не генерируются моделью.
type TaskQueue struct {
	mu     sync.Mutex
	tasks  []QueuedTask
	nextID int
}

func NewTaskQueue() *TaskQueue {
	return &TaskQueue{nextID: 1}
}

// Add добавляет задачу в очередь. Возвращает созданную задачу.
// Дубликаты по Source+FilePath+Line не создаются.
func (q *TaskQueue) Add(source, title, detail, filePath string, line int, priority TaskPriority) QueuedTask {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Дедупликация: не добавлять одну и ту же проблему дважды
	for _, t := range q.tasks {
		if t.Source == source && t.FilePath == filePath && t.Line == line && t.Status == "pending" {
			return t
		}
	}

	t := QueuedTask{
		ID:       q.nextID,
		Priority: priority,
		Source:   source,
		Title:    title,
		Detail:   detail,
		FilePath: filePath,
		Line:     line,
		Created:  time.Now(),
		Status:   "pending",
	}
	q.nextID++
	q.tasks = append(q.tasks, t)
	return t
}

// Pending возвращает задачи в статусе "pending", отсортированные по приоритету.
func (q *TaskQueue) Pending() []QueuedTask {
	q.mu.Lock()
	defer q.mu.Unlock()
	var out []QueuedTask
	for _, t := range q.tasks {
		if t.Status == "pending" {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		return out[i].Created.Before(out[j].Created)
	})
	return out
}

// SetStatus обновляет статус задачи.
func (q *TaskQueue) SetStatus(id int, status string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i := range q.tasks {
		if q.tasks[i].ID == id {
			q.tasks[i].Status = status
			return
		}
	}
}

// FormatPending формирует человекочитаемое представление очереди.
func (q *TaskQueue) FormatPending() string {
	tasks := q.Pending()
	if len(tasks) == 0 {
		return "Autonomy queue is empty. No pending tasks."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Autonomy Queue: %d pending task(s)\n\n", len(tasks))
	for _, t := range tasks {
		icon := "○"
		switch {
		case t.Priority >= PriorityCritical:
			icon = "●"
		case t.Priority >= PriorityHigh:
			icon = "◐"
		case t.Priority >= PriorityNormal:
			icon = "◌"
		}
		fmt.Fprintf(&b, "%s #%d [%s] %s", icon, t.ID, t.Source, t.Title)
		if t.FilePath != "" {
			fmt.Fprintf(&b, " (%s:%d)", t.FilePath, t.Line)
		}
		b.WriteByte('\n')
		if t.Detail != "" {
			detail := t.Detail
			if len(detail) > 200 {
				detail = detail[:200] + "..."
			}
			fmt.Fprintf(&b, "  %s\n", detail)
		}
	}
	b.WriteString("\nUse `:autonomy run` to execute fixable tasks.\n")
	return b.String()
}