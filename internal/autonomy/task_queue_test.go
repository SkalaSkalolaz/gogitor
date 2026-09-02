package autonomy

import (
	"testing"
	"time"
)

func TestTaskQueue_AddAndPending(t *testing.T) {
	q := NewTaskQueue()
	q.Add("build_error", "Build failed", "", "", 0, PriorityCritical)
	q.Add("vet_warning", "Vet issues", "", "", 0, PriorityHigh)
	q.Add("todo", "TODO item", "", "main.go", 10, PriorityLow)

	pending := q.Pending()
	if len(pending) != 3 {
		t.Fatalf("pending = %d", len(pending))
	}
	if pending[0].Priority != PriorityCritical {
		t.Errorf("first priority = %v", pending[0].Priority)
	}
	if pending[2].Priority != PriorityLow {
		t.Errorf("last priority = %v", pending[2].Priority)
	}
}

func TestTaskQueue_Deduplication(t *testing.T) {
	q := NewTaskQueue()
	q.Add("build_error", "Build failed", "d1", "main.go", 10, PriorityCritical)
	q.Add("build_error", "Build failed again", "d2", "main.go", 10, PriorityCritical)
	if len(q.Pending()) != 1 {
		t.Error("deduplication failed")
	}
}

func TestTaskQueue_SetStatus(t *testing.T) {
	q := NewTaskQueue()
	task := q.Add("build_error", "Build failed", "", "", 0, PriorityCritical)
	q.SetStatus(task.ID, "applied")
	if len(q.Pending()) != 0 {
		t.Error("status change failed")
	}
}

func TestTaskQueue_FormatPending(t *testing.T) {
	q := NewTaskQueue()
	q.Add("build_error", "Build failed", "", "", 0, PriorityCritical)
	if q.FormatPending() == "" {
		t.Error("empty format")
	}
	if NewTaskQueue().FormatPending() == "" {
		t.Error("empty queue should still format")
	}
}

func TestTaskPriorityOrdering(t *testing.T) {
	q := NewTaskQueue()
	q.Add("todo", "Low", "", "", 0, PriorityLow)
	time.Sleep(time.Millisecond)
	q.Add("vet", "High", "", "", 0, PriorityHigh)
	time.Sleep(time.Millisecond)
	q.Add("build", "Critical", "", "", 0, PriorityCritical)
	pending := q.Pending()
	if pending[0].Source != "build" || pending[1].Source != "vet" || pending[2].Source != "todo" {
		t.Errorf("order: %v, %v, %v", pending[0].Source, pending[1].Source, pending[2].Source)
	}
}