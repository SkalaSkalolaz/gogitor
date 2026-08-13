package app

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"time"

	"gogitor/internal/security"
)

// agentCheckpoint — снимок проекта до начала агентной сессии.
//
// Это не идеальная замена Git-ветке, но практичный универсальный механизм:
// он работает даже тогда, когда проект ещё не является Git-репозиторием.
type agentCheckpoint struct {
	Dir       string
	CreatedAt time.Time
}

// createAgentCheckpoint создаёт временную копию проекта.
//
// Использует существующий workspace.PrepareSandbox, который уже умеет
// безопасно копировать проект во временную директорию.
func (s *Service) createAgentCheckpoint(ctx context.Context) (*agentCheckpoint, error) {
	dir, err := s.WS.PrepareSandbox(ctx)
	if err != nil {
		return nil, err
	}

	return &agentCheckpoint{
		Dir:       dir,
		CreatedAt: time.Now(),
	}, nil
}

func (cp *agentCheckpoint) cleanup() {
	if cp == nil || cp.Dir == "" {
		return
	}
	_ = os.RemoveAll(cp.Dir)
}

// rollbackAgentCheckpoint откатывает изменения, сделанные агентом.
//
// Логика:
// 1. Файлы, которые были созданы агентом, удаляются.
// 2. Файлы, которые были изменены, восстанавливаются из checkpoint.
func (s *Service) rollbackAgentCheckpoint(
	cp *agentCheckpoint,
	created []string,
	modified []string,
) error {
	if cp == nil || cp.Dir == "" {
		return nil
	}

	createdSet := stringSet(created)

	// Удаляем созданные файлы.
	for _, rel := range created {
		dst, err := security.SafeJoin(s.Cfg.WorkDir, rel)
		if err != nil {
			continue
		}
		_ = os.Remove(dst)
	}

	// Восстанавливаем изменённые файлы.
	for _, rel := range modified {
		// Если файл был создан, а потом изменён, он уже удалён выше.
		if createdSet[rel] {
			continue
		}

		dst, err := security.SafeJoin(s.Cfg.WorkDir, rel)
		if err != nil {
			continue
		}

		src, err := security.SafeJoin(cp.Dir, rel)
		if err != nil {
			continue
		}

		data, err := os.ReadFile(src)
		if err != nil {
			// Если файла не было в checkpoint, значит он появился во время сессии.
			// Удаляем его из рабочего проекта.
			_ = os.Remove(dst)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			continue
		}

		tmp := dst + ".gogitor.tmp"
		if err := os.WriteFile(tmp, data, 0o644); err != nil {
			continue
		}

		_ = os.Rename(tmp, dst)
	}

	return nil
}

func stringSet(items []string) map[string]bool {
	set := make(map[string]bool, len(items))
	for _, item := range items {
		set[item] = true
	}
	return set
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}