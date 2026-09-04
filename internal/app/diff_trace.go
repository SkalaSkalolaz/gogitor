package app

import (
	"fmt"
	"strings"

	"gogitor/internal/domain"
)

// installDiffTrace подключает диагностический sink Workspace
// к существующему event pipeline Gogitor.
func (s *Service) installDiffTrace(
	emit func(domain.Event),
) func() {
	if s == nil ||
		s.Cfg == nil ||
		s.WS == nil ||
		emit == nil ||
		!s.Cfg.DiffTrace {
		return func() {}
	}

	s.WS.SetDiffTraceSink(
		func(message string) {
			sendEvent(
				emit,
				domain.EventLog,
				message,
			)
		},
	)

	return func() {
		if s.WS != nil {
			s.WS.SetDiffTraceSink(nil)
		}
	}
}

// emitParsedDiffTrace показывает структуру patch,
// полученную непосредственно от парсера ответа LLM.
func emitParsedDiffTrace(
	emit func(domain.Event),
	changes []domain.FileChange,
	iteration int,
) {
	if emit == nil || len(changes) == 0 {
		return
	}

	for fileIndex, ch := range changes {
		if len(ch.Patches) == 0 {
			continue
		}

		for patchIndex, p := range ch.Patches {
			protocol := "SEARCH_REPLACE"
			if p.ReplaceOnly {
				protocol = "REPLACE_ONLY"
			}

			symbol := strings.TrimSpace(p.Symbol)

			message := fmt.Sprintf(
				"[DIFF] phase=PARSE file=%s patch=%d/%d stage=PARSE decision=OK iteration=%d protocol=%s search_lines=%d replace_lines=%d",
				ch.Path,
				patchIndex+1,
				len(ch.Patches),
				iteration,
				protocol,
				diffLineCount(p.Search),
				diffLineCount(p.Replace),
			)

			if symbol != "" {
				message += " symbol=" + symbol
			}

			_ = fileIndex

			sendEvent(
				emit,
				domain.EventLog,
				message,
			)
		}
	}
}

// ✅ ПРАВИЛЬНО:
func emitDiffMatchingConfigTrace(
	emit func(domain.Event),
	cfg domain.DiffMatchingConfig,
) {
	if emit == nil {
		return
	}
	cfg = cfg.Normalized()
	sendEvent(
		emit,
		domain.EventLog,
		fmt.Sprintf(
			"[DIFF] matching-config ast_weight=%.2f line_weight=%.2f ast_min_structure=%.2f base=%.2f balanced=%.2f/%.2f advanced=%.2f/%.2f",
			cfg.ASTWeight,
			cfg.LineWeight,
			cfg.ASTMinStructure,
			cfg.FuzzyBaseThreshold,
			cfg.BalancedThreshold,
			cfg.BalancedMargin,
			cfg.AdvancedThreshold,
			cfg.AdvancedMargin,
		),
	)
}

func diffLineCount(s string) int {
	s = strings.TrimSpace(
		strings.ReplaceAll(s, "\r\n", "\n"),
	)

	if s == "" {
		return 0
	}

	return strings.Count(s, "\n") + 1
}
