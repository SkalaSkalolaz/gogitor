package app

import (
	"context"
	"strings"
	"time"

	"gogitor/internal/agent"
	"gogitor/internal/domain"
)

type llmStreamer interface {
	Stream(ctx context.Context, prompt string, onToken func(string)) (string, error)
}

// sendLLMStreaming отправляет LLM-запрос и стримит токены в emit, если LLM это поддерживает.
// Используется только для тех режимов, где живой вывод безопасен: chat, analyze, search.
func (s *Service) sendLLMStreaming(
	ctx context.Context,
	prompt string,
	emit func(domain.Event),
	role agent.Role,
	priority agent.Priority,
	purpose string,
) (string, error) {
	ctx = agent.WithRole(ctx, role)
	ctx = agent.WithPriority(ctx, priority)
	ctx = agent.WithPurpose(ctx, purpose)

	s.emitProgressStart(emit, purpose, role, purpose, prompt, 0, 0)

	streamer, ok := s.LLM.(llmStreamer)
	if !ok {
		return s.LLM.Send(ctx, prompt)
	}

	var pending strings.Builder
	last := time.Now()

	flush := func(force bool) {
		if pending.Len() == 0 {
			return
		}

		if force || time.Since(last) >= 80*time.Millisecond || pending.Len() >= 192 {
			if emit != nil {
				emit(domain.Event{
					Type:    domain.EventToken,
					Message: pending.String(),
				})
			}

			pending.Reset()
			last = time.Now()
		}
	}

	text, err := streamer.Stream(ctx, prompt, func(token string) {
		pending.WriteString(token)
		flush(false)
	})

	flush(true)

	return text, err
}

// sendLLMStreamingWithImages — потоковый запрос с изображениями.
func (s *Service) sendLLMStreamingWithImages(
	ctx context.Context,
	prompt string,
	images [][]byte,
	emit func(domain.Event),
	role agent.Role,
	priority agent.Priority,
	purpose string,
) (string, error) {
	ctx = agent.WithRole(ctx, role)
	ctx = agent.WithPriority(ctx, priority)
	ctx = agent.WithPurpose(ctx, purpose)
	s.emitProgressStart(emit, purpose, role, purpose, prompt, 0, 0)

	// Пробуем потоковый multimodal
	type streamMultimodal interface {
		StreamWithImages(ctx context.Context, prompt string, images [][]byte, onToken func(string)) (string, error)
	}
	if sm, ok := s.LLM.(streamMultimodal); ok {
		var pending strings.Builder
		last := time.Now()
		flush := func(force bool) {
			if pending.Len() == 0 {
				return
			}
			if force || time.Since(last) >= 80*time.Millisecond || pending.Len() >= 192 {
				if emit != nil {
					emit(domain.Event{
						Type:    domain.EventToken,
						Message: pending.String(),
					})
				}
				pending.Reset()
				last = time.Now()
			}
		}
		text, err := sm.StreamWithImages(ctx, prompt, images, func(token string) {
			pending.WriteString(token)
			flush(false)
		})
		flush(true)
		return text, err
	}

	// Fallback: не-потоковый multimodal
	type multimodal interface {
		SendWithImages(ctx context.Context, prompt string, images [][]byte) (string, error)
	}
	if ml, ok := s.LLM.(multimodal); ok {
		return ml.SendWithImages(ctx, prompt, images)
	}

	// Последний fallback: обычный текстовый запрос
	return s.LLM.Send(ctx, prompt)
}

// emitProgressStart отправляет событие прогресса с оценкой ETA.
func (s *Service) emitProgressStart(
	emit func(domain.Event),
	stage string,
	role agent.Role,
	purpose, prompt string,
	itemIndex, totalItems int,
) {
	if emit == nil || s.Stats == nil {
		return
	}

	eta := s.Stats.estimate(role, purpose, prompt)

	emit(domain.Event{
		Type:    domain.EventProgress,
		Message: stage,
		Progress: &domain.ProgressUpdate{
			Stage:      stage,
			ItemIndex:  itemIndex,
			TotalItems: totalItems,
			ETASeconds: int(eta.Seconds() + 0.5),
		},
	})
}
