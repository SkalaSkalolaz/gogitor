package app

import (
	"os"
	"path/filepath"
	"strings"

	"gogitor/internal/textutil"
)

const maxProjectInstructionsBytes = 16000

// projectInstructionsPath возвращает путь к локальным правилам проекта.
func (s *Service) projectInstructionsPath() string {
	return filepath.Join(
		s.Cfg.WorkDir,
		".gogitor.md",
	)
}

// projectInstructions загружает постоянные инструкции проекта.
//
// Файл является необязательным. Если файла нет или он пуст,
// возвращается пустая строка.
func (s *Service) projectInstructions() string {
	path := s.projectInstructionsPath()

	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	text := strings.TrimSpace(string(data))
	if text == "" {
		return ""
	}

	if len(text) > maxProjectInstructionsBytes {
		text = textutil.TruncateStringBytes(
			text,
			maxProjectInstructionsBytes,
		)

		text += "\n\n[Project instructions truncated by Gogitor.]"
	}

	return text
}

// appendProjectInstructions добавляет инструкции проекта
// к существующему LLM prompt.
func (s *Service) appendProjectInstructions(
	prompt string,
) string {
	instructions := s.projectInstructions()
	if instructions == "" {
		return prompt
	}

	return strings.TrimSpace(prompt) +
		"\n\n" +
		"=== TRUSTED PROJECT INSTRUCTIONS (.gogitor.md) ===\n" +
		instructions +
		"\n=== END PROJECT INSTRUCTIONS ===\n"
}
