package app

import (
	"testing"

	"gogitor/internal/domain"
)

func TestAgentMemory_Decisions(t *testing.T) {
	m := &agentMemory{}
	m.addDecision("test")
	if len(m.Decisions) != 1 {
		t.Fatalf("decisions = %d", len(m.Decisions))
	}
	m.addDecisionEntry(domain.DecisionEntry{Decision: "entry"})
	if len(m.DecisionLog) != 1 || m.DecisionLog[0].ID != 1 {
		t.Errorf("log = %+v", m.DecisionLog)
	}
}

func TestAgentMemory_Temporary(t *testing.T) {
	m := &agentMemory{}
	m.addTemporaryDecision("temp", "constraint", "test")
	if len(m.DecisionLog) != 1 || !m.DecisionLog[0].Temporary {
		t.Error("expected temporary entry")
	}
}

func TestAgentMemory_Summary(t *testing.T) {
	m := &agentMemory{Conventions: []string{"use tabs"}, Decisions: []string{"chose A"}}
	if m.summary(10) == "" {
		t.Error("empty summary")
	}
	var nilM *agentMemory
	if nilM.summary(10) != "" {
		t.Error("nil should return empty")
	}
}

func TestAgentMemory_Journal(t *testing.T) {
	m := &agentMemory{
		DecisionLog:      []domain.DecisionEntry{{ID: 1, Decision: "test"}},
		FailedApproaches: []string{"approach X"},
	}
	j := m.journal()
	if len(j.Entries) != 1 || len(j.FailedApproaches) != 1 {
		t.Error("journal mismatch")
	}
}

func TestAppendLimited(t *testing.T) {
	list := appendLimited(nil, "a", 3)
	list = appendLimited(list, "b", 3)
	list = appendLimited(list, "c", 3)
	list = appendLimited(list, "d", 3)
	if len(list) != 3 || list[0] != "b" {
		t.Errorf("list = %v", list)
	}
}

func TestLastN(t *testing.T) {
	last := lastN([]string{"a", "b", "c", "d", "e"}, 3)
	if len(last) != 3 || last[0] != "c" {
		t.Errorf("last = %v", last)
	}
}
