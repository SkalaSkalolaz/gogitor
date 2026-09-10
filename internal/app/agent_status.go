package app

import (
	"time"

	"gogitor/internal/agent"
	"gogitor/internal/domain"
	"gogitor/internal/i18n"
)

func (s *Service) agentStatusEmitter(emit func(domain.Event)) agent.StatusFunc {
	return func(se agent.StatusEvent) {
		if emit == nil {
			return
		}
		typ := domain.EventAgent
		if se.Kind == agent.StatusDone && se.Err != nil {
			typ = domain.EventWarn
		}
		emit(domain.Event{
			Type:    typ,
			Message: formatAgentStatus(se),
			Agent: &domain.AgentStatus{
				Role:     agentRoleName(se.Role),
				Purpose:  se.Purpose,
				Requests: se.Session.Requests,
				Tokens:   se.Session.EstimatedTokens,
				Duration: se.Session.Duration.Round(time.Millisecond).String(),
				Queue:    se.Queue,
				Kind:     string(se.Kind),
			},
		})
	}
}

func formatAgentStatus(e agent.StatusEvent) string {
	role := agentRoleName(e.Role)
	purpose := ""
	if e.Purpose != "" {
		purpose = " (" + e.Purpose + ")"
	}

	switch e.Kind {
	case agent.StatusQueued:
		return i18n.T(
			"LLM queue: agent %s%s waiting; queue=%d; budget: %s",
			role, purpose, e.Queue, formatAgentUsage(e.Session),
		)
	case agent.StatusStart:
		return i18n.T(
			"LLM request: agent %s%s is using LLM; queue=%d; budget: %s",
			role, purpose, e.Queue, formatAgentUsage(e.Session),
		)
	case agent.StatusRetry:
		return i18n.T(
			"LLM retry: agent %s%s — %v",
			role, purpose, e.Err,
		)
	case agent.StatusDone:
		if e.Err != nil {
			return i18n.T(
				"LLM request: agent %s%s finished with error; budget: %s",
				role, purpose, formatAgentUsage(e.Session),
			)
		}
		return i18n.T(
			"LLM request: agent %s%s finished; budget: %s",
			role, purpose, formatAgentUsage(e.Session),
		)
	default:
		return i18n.T(
			"LLM status: agent %s%s; queue=%d; budget: %s",
			role, purpose, e.Queue, formatAgentUsage(e.Session),
		)
	}
}

func agentRoleName(r agent.Role) string {
	if r == "" {
		return "default"
	}

	switch r {
	case agent.RoleRouter:
		return "router"
	case agent.RolePlanner:
		return "planner"
	case agent.RoleCoder:
		return "coder"
	case agent.RoleReviewer:
		return "reviewer"
	case agent.RoleTester:
		return "tester"
	case agent.RoleVerifier:
		return "verifier"
	case agent.RoleSecurity:
		return "security"
	case agent.RoleDocs:
		return "docs"
	default:
		return string(r)
	}
}

func formatAgentUsage(u agent.Usage) string {
	return i18n.T(
		"requests=%d, tokens≈%d, duration=%s",
		u.Requests,
		u.EstimatedTokens,
		u.Duration.Round(time.Millisecond),
	)
}

func (s *Service) emitAgentBudget(emit func(domain.Event), stage string) {
	if s.Agents == nil || emit == nil {
		return
	}

	session, _ := s.Agents.Snapshot()
	queue := s.Agents.QueueLen()

	msg := i18n.T(
		"budget after stage %s: requests=%d, tokens≈%d, duration=%s, queue=%d",
		stage,
		session.Requests,
		session.EstimatedTokens,
		session.Duration.Round(time.Millisecond),
		queue,
	)

	emit(domain.Event{
		Type:    domain.EventAgent,
		Message: msg,
	})
}
