package app

import (
	"context"
	"fmt"
	"strings"

	"gogitor/internal/agent"
	"gogitor/internal/domain"
	"gogitor/internal/prompts"
	"gogitor/internal/workspace"
)

type patchAuditResult struct {
	Approved         bool     `json:"approved"`
	ScopeOK          bool     `json:"scope_ok"`
	SymbolOK         bool     `json:"symbol_ok"`
	UnrelatedChanges bool     `json:"unrelated_changes"`
	CriticalIssues   []string `json:"critical_issues,omitempty"`
}

func shouldAuditPatch(mode string, protocol workspace.PatchProtocol, deep bool) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "always":
		return true
	case "off":
		return false
	default:
		return deep || protocol == workspace.PatchProtocolReplaceOnly
	}
}

func (s *Service) auditPatch(
	ctx context.Context,
	task string,
	projectContext string,
	changes []domain.FileChange,
) (patchAuditResult, error) {
	var b strings.Builder

	for _, ch := range changes {
		if len(ch.Patches) == 0 {
			continue
		}

		b.WriteString("--- Patch: ")
		b.WriteString(ch.Path)
		b.WriteString(" ---\n")

		for _, p := range ch.Patches {
			if p.Symbol != "" {
				b.WriteString("--- Symbol: ")
				b.WriteString(p.Symbol)
				b.WriteString(" ---\n")
			}

			if p.ReplaceOnly {
				b.WriteString("<<<<<<< REPLACE_ONLY\n")
				b.WriteString(p.Replace)
				b.WriteString("\n>>>>>>> REPLACE_ONLY\n")
				continue
			}

			b.WriteString("<<<<<<< SEARCH\n")
			b.WriteString(p.Search)
			b.WriteString("\n=======\n")
			b.WriteString(p.Replace)
			b.WriteString("\n>>>>>>> REPLACE\n")
		}
	}

	if strings.TrimSpace(b.String()) == "" {
		return patchAuditResult{
			Approved: true,
			ScopeOK:  true,
			SymbolOK: true,
		}, nil
	}

	ctx = agent.WithRole(ctx, agent.RoleReviewer)
	ctx = agent.WithPriority(ctx, agent.PriorityHigh)
	ctx = agent.WithPurpose(ctx, "audit generated patch")

	prompt := prompts.PatchAudit(
		task,
		projectContext,
		b.String(),
	)

	var review patchAuditResult
	if err := s.sendAgentJSON(
		ctx,
		agent.RoleReviewer,
		agent.PriorityHigh,
		"audit generated patch",
		prompt,
		&review,
	); err != nil {
		return patchAuditResult{}, fmt.Errorf("patch auditor: %w", err)
	}

	if !review.ScopeOK ||
		!review.SymbolOK ||
		review.UnrelatedChanges ||
		len(review.CriticalIssues) > 0 {
		review.Approved = false
	}

	return review, nil
}
