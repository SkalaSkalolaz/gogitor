package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"gogitor/internal/agent"
	"gogitor/internal/codegen"
	"gogitor/internal/textutil"
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
    
    parseErr := parseAgentJSON(
    	response,
    	out,
    )
    
    if parseErr == nil {
    	return nil
    }
    
    s.Log.Warn(
    	"agent JSON parse failed; requesting one repair",
    	"purpose",
    	purpose,
    	"err",
    	parseErr,
    )
    
    repairPrompt := buildAgentJSONRepairPrompt(
    	purpose,
    	prompt,
    	response,
    	parseErr,
    )
    
    repairCtx := ctx
    repairCtx = agent.WithPurpose(
    	repairCtx,
    	purpose+" JSON repair",
    )
    
    repaired, repairErr :=
    	s.LLM.Send(
    		repairCtx,
    		repairPrompt,
    	)
    
    if repairErr != nil {
    	return fmt.Errorf(
    		"%s: JSON repair failed: %w",
    		purpose,
    		repairErr,
    	)
    }
    
    if err := parseAgentJSON(
    	repaired,
    	out,
    ); err != nil {
    	return fmt.Errorf(
    		"%s: invalid JSON after repair: %w",
    		purpose,
    		err,
    	)
    }
    
    return nil
}

func buildAgentJSONRepairPrompt(
	purpose string,
	originalPrompt string,
	invalidResponse string,
	parseErr error,
) string {
	const maxPromptBytes = 14000
	const maxResponseBytes = 10000

	originalPrompt =
		textutil.TruncateStringBytes(
			originalPrompt,
			maxPromptBytes,
		)

	invalidResponse =
		textutil.TruncateStringBytes(
			invalidResponse,
			maxResponseBytes,
		)

	errText := ""
	if parseErr != nil {
		errText = parseErr.Error()
	}

	errText =
		textutil.TruncateStringBytes(
			errText,
			1000,
		)

	return fmt.Sprintf(
		`You are repairing an invalid JSON response.

Purpose:
%s

Return ONLY one valid JSON object.
Do not add markdown.
Do not add explanations.
Do not change the intended meaning.
Preserve all information that can be recovered.

Original request:
---BEGIN REQUEST---
%s
---END REQUEST---

Previous invalid response:
---BEGIN RESPONSE---
%s
---END RESPONSE---

Parser error:
%s

Return ONLY the corrected JSON object.`,
		purpose,
		originalPrompt,
		invalidResponse,
		errText,
	)
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
