package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gogitor/internal/domain"
)

// SaveResultToFile сохраняет результат в файл.
// Тип контента определяется расширением:
//   - .json — полный Result как JSON
//   - .go   — сгенерированный код (первый OutputFile или Response)
//   - .md, .txt и всё остальное — Response как текст
func SaveResultToFile(res domain.Result, path string) error {
	ext := strings.ToLower(filepath.Ext(path))

	var content string
	switch ext {
	case ".json":
		data, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return fmt.Errorf("json marshal: %w", err)
		}
		content = string(data)
	case ".go":
		if len(res.OutputFiles) > 0 {
			content = res.OutputFiles[0].Content
		} else {
			content = res.Response
		}
	default: // .md, .txt, и всё остальное
		content = res.Response
	}

	content = strings.TrimRight(content, "\n") + "\n"

	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create dir: %w", err)
		}
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
