package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"gogitor/internal/agent"
	"gogitor/internal/codegen"
)

func (s *Service) sendAgentJSON(
	ctx context.Context,
	role agent.Role,
	priority agent.Priority,
	purpose string,
	prompt string,
	out any,
) error {
	estimatedTokens := (len(prompt) + 3) / 4
	warnThreshold := s.Cfg.EffectiveContextTokens() * 80 / 100
	if estimatedTokens > warnThreshold {
		s.Log.Warn("prompt may exceed model context window",
			"purpose", purpose,
			"estimated_tokens", estimatedTokens,
			"context_limit", s.Cfg.EffectiveContextTokens(),
			"prompt_len", len(prompt),
		)
	}
	ctx = agent.WithRole(ctx, role)
	ctx = agent.WithPriority(ctx, priority)
	ctx = agent.WithPurpose(ctx, purpose)
	response, err := s.LLM.Send(ctx, prompt)
	if err != nil {
		return err
	}
	if err := parseAgentJSON(response, out); err != nil {
		return fmt.Errorf("%s: %w", purpose, err)
	}
	return nil
}

func extractJSONCandidate(text string) ([]byte, error) {
	// Быстрый путь: если текст начинается с '{', пробуем сразу
	trimmed := strings.TrimSpace(text)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		var raw json.RawMessage
		if err := json.Unmarshal([]byte(trimmed), &raw); err == nil {
			return []byte(raw), nil
		}
	}

	// Сканируем все позиции '{' и пробуем декодировать
	for i := 0; i < len(text); i++ {
		if text[i] != '{' {
			continue
		}
		var raw json.RawMessage
		decoder := json.NewDecoder(strings.NewReader(text[i:]))
		if err := decoder.Decode(&raw); err != nil {
			continue
		}
		// Убеждаемся, что это именно объект, а не другой тип
		if len(raw) > 0 && raw[0] == '{' {
			return []byte(raw), nil
		}
	}
	return nil, fmt.Errorf("no valid JSON object found")
}

// extractAllJSONCandidates извлекает все валидные JSON-объекты из текста.
// Используется когда нужно перебрать кандидаты и выбрать наиболее подходящий.
func extractAllJSONCandidates(text string) [][]byte {
	var results [][]byte
	i := 0
	for i < len(text) {
		if text[i] != '{' {
			i++
			continue
		}
		var raw json.RawMessage
		decoder := json.NewDecoder(strings.NewReader(text[i:]))
		if err := decoder.Decode(&raw); err != nil {
			i++
			continue
		}
		if len(raw) > 0 && raw[0] == '{' {
			results = append(results, []byte(raw))
			// Пропускаем вперёд за найденный объект,
			// чтобы не находить вложенные объекты повторно
			i += len(raw)
		} else {
			i++
		}
	}
	return results
}

func parseAgentJSON(response string, out any) error {
	cleaned := codegen.CleanCode(response)

	jsonPart, err := extractJSONCandidate(cleaned)
	if err != nil {
		return fmt.Errorf("no JSON object found")
	}

	if err := json.Unmarshal(jsonPart, out); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}
