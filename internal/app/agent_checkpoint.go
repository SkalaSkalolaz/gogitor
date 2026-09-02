package app

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

type checkpointEntry struct {
	Mode  os.FileMode
	Size  int64
	Hash  [sha256.Size]byte
	IsDir bool
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

func snapshotCheckpointTree(
	root string,
) (map[string]checkpointEntry, error) {
	entries := make(map[string]checkpointEntry)

	err := filepath.Walk(
		root,
		func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}

			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}

			if rel == "." {
				return nil
			}

			if checkpointExcludedPath(
				rel,
				info.IsDir(),
			) {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			if info.IsDir() {
				entries[rel] = checkpointEntry{
					Mode:  info.Mode(),
					IsDir: true,
				}
				return nil
			}

			if !info.Mode().IsRegular() {
				return nil
			}

			hash, err := hashCheckpointFile(path)
			if err != nil {
				return fmt.Errorf(
					"hash %s: %w",
					rel,
					err,
				)
			}

			entries[rel] = checkpointEntry{
				Mode:  info.Mode(),
				Size:  info.Size(),
				Hash:  hash,
				IsDir: false,
			}

			return nil
		},
	)

	if err != nil {
		return nil, err
	}

	return entries, nil
}

func hashCheckpointFile(
	path string,
) ([sha256.Size]byte, error) {
	var zero [sha256.Size]byte

	file, err := os.Open(path)
	if err != nil {
		return zero, err
	}
	defer file.Close()

	h := sha256.New()

	if _, err := io.Copy(h, file); err != nil {
		return zero, err
	}

	var result [sha256.Size]byte
	copy(result[:], h.Sum(nil))

	return result, nil
}

func checkpointExcludedPath(
	rel string,
	isDir bool,
) bool {
	clean := filepath.Clean(rel)

	parts := strings.Split(
		clean,
		string(os.PathSeparator),
	)

	if len(parts) > 0 {
		switch parts[0] {
		case ".git",
			".gogitor",
			".idea",
			".vscode",
			"node_modules":
			return true
		}
	}

	if !isDir {
		base := filepath.Base(clean)

		if base == ".DS_Store" ||
			strings.HasSuffix(
				base,
				".gogitor.bak",
			) ||
			strings.HasSuffix(
				base,
				".gogitor.tmp",
			) {
			return true
		}
	}

	return false
}

func (cp *agentCheckpoint) cleanup() {
	if cp == nil || cp.Dir == "" {
		return
	}
	_ = os.RemoveAll(cp.Dir)
}

// rollbackAgentCheckpoint откатывает изменения, сделанные агентом.
func (s *Service) rollbackAgentCheckpoint(
	cp *agentCheckpoint,
	created []string,
	modified []string,
) error {
	if cp == nil || cp.Dir == "" {
		return nil
	}

	// Metadata Agent остаётся аргументом для совместимости,
	// но больше не является источником истины.
	_ = created
	_ = modified

	baseline, err := snapshotCheckpointTree(cp.Dir)
	if err != nil {
		return fmt.Errorf(
			"cannot snapshot checkpoint: %w",
			err,
		)
	}

	current, err :=
		snapshotCheckpointTree(s.Cfg.WorkDir)

	if err != nil {
		return fmt.Errorf(
			"cannot snapshot current project: %w",
			err,
		)
	}

	var rollbackErrors []string

	// ------------------------------------------------------------
	// 1. Удаляем всё, что появилось в проекте после checkpoint.
	// ------------------------------------------------------------

	var extra []string

	for rel := range current {
		if _, ok := baseline[rel]; !ok {
			extra = append(extra, rel)
		}
	}

	sort.Slice(
		extra,
		func(i, j int) bool {
			di := checkpointPathDepth(extra[i])
			dj := checkpointPathDepth(extra[j])

			if di != dj {
				return di > dj
			}

			return extra[i] > extra[j]
		},
	)

	for _, rel := range extra {
		dst, err := security.SafeJoin(
			s.Cfg.WorkDir,
			rel,
		)
		if err != nil {
			rollbackErrors = append(
				rollbackErrors,
				fmt.Sprintf(
					"remove %s: %v",
					rel,
					err,
				),
			)
			continue
		}

		if err := os.RemoveAll(dst); err != nil {
			rollbackErrors = append(
				rollbackErrors,
				fmt.Sprintf(
					"remove %s: %v",
					rel,
					err,
				),
			)
		}
	}

	// ------------------------------------------------------------
	// 2. Восстанавливаем весь baseline:
	//    - удалённые файлы;
	//    - изменённые файлы;
	//    - permissions;
	//    - каталоги.
	// ------------------------------------------------------------

	paths := make([]string, 0, len(baseline))

	for rel := range baseline {
		paths = append(paths, rel)
	}

	sort.Slice(
		paths,
		func(i, j int) bool {
			di := checkpointPathDepth(paths[i])
			dj := checkpointPathDepth(paths[j])

			if di != dj {
				return di < dj
			}

			return paths[i] < paths[j]
		},
	)

	for _, rel := range paths {
		entry := baseline[rel]

		dst, err := security.SafeJoin(
			s.Cfg.WorkDir,
			rel,
		)
		if err != nil {
			rollbackErrors = append(
				rollbackErrors,
				fmt.Sprintf(
					"restore %s: %v",
					rel,
					err,
				),
			)
			continue
		}

		src, err := security.SafeJoin(
			cp.Dir,
			rel,
		)
		if err != nil {
			rollbackErrors = append(
				rollbackErrors,
				fmt.Sprintf(
					"checkpoint path %s: %v",
					rel,
					err,
				),
			)
			continue
		}

		currentEntry, exists :=
			current[rel]

		// --------------------------------------------------------
		// Directory.
		// --------------------------------------------------------

		if entry.IsDir {
			if exists && !currentEntry.IsDir {
				if err := os.RemoveAll(dst); err != nil {
					rollbackErrors = append(
						rollbackErrors,
						fmt.Sprintf(
							"replace %s: %v",
							rel,
							err,
						),
					)
					continue
				}
			}

			if err := os.MkdirAll(
				dst,
				entry.Mode.Perm(),
			); err != nil {
				rollbackErrors = append(
					rollbackErrors,
					fmt.Sprintf(
						"mkdir %s: %v",
						rel,
						err,
					),
				)
				continue
			}

			if err := os.Chmod(
				dst,
				entry.Mode.Perm(),
			); err != nil {
				rollbackErrors = append(
					rollbackErrors,
					fmt.Sprintf(
						"chmod directory %s: %v",
						rel,
						err,
					),
				)
			}

			continue
		}

		// --------------------------------------------------------
		// Regular file.
		// --------------------------------------------------------

		if exists &&
			!currentEntry.IsDir &&
			currentEntry.Hash == entry.Hash &&
			currentEntry.Mode.Perm() ==
				entry.Mode.Perm() {

			continue
		}

		if exists {
			if err := os.RemoveAll(dst); err != nil {
				rollbackErrors = append(
					rollbackErrors,
					fmt.Sprintf(
						"replace %s: %v",
						rel,
						err,
					),
				)
				continue
			}
		}

		if err := os.MkdirAll(
			filepath.Dir(dst),
			0o755,
		); err != nil {
			rollbackErrors = append(
				rollbackErrors,
				fmt.Sprintf(
					"mkdir parent for %s: %v",
					rel,
					err,
				),
			)
			continue
		}

		data, err := os.ReadFile(src)
		if err != nil {
			rollbackErrors = append(
				rollbackErrors,
				fmt.Sprintf(
					"read checkpoint %s: %v",
					rel,
					err,
				),
			)
			continue
		}

		tmp := dst + ".gogitor.tmp"

		if err := os.WriteFile(
			tmp,
			data,
			entry.Mode.Perm(),
		); err != nil {
			_ = os.Remove(tmp)

			rollbackErrors = append(
				rollbackErrors,
				fmt.Sprintf(
					"write %s: %v",
					rel,
					err,
				),
			)
			continue
		}

		if err := os.Chmod(
			tmp,
			entry.Mode.Perm(),
		); err != nil {
			_ = os.Remove(tmp)

			rollbackErrors = append(
				rollbackErrors,
				fmt.Sprintf(
					"chmod %s: %v",
					rel,
					err,
				),
			)
			continue
		}

		if err := os.Rename(
			tmp,
			dst,
		); err != nil {
			_ = os.Remove(tmp)

			rollbackErrors = append(
				rollbackErrors,
				fmt.Sprintf(
					"rename %s: %v",
					rel,
					err,
				),
			)
		}
	}

	// ------------------------------------------------------------
	// 3. Финальная проверка rollback.
	// ------------------------------------------------------------

	restored, err :=
		snapshotCheckpointTree(
			s.Cfg.WorkDir,
		)

	if err != nil {
		rollbackErrors = append(
			rollbackErrors,
			fmt.Sprintf(
				"verify restored project: %v",
				err,
			),
		)
	} else if err := compareCheckpointTrees(
		baseline,
		restored,
	); err != nil {
		rollbackErrors = append(
			rollbackErrors,
			err.Error(),
		)
	}

	if len(rollbackErrors) > 0 {
		return fmt.Errorf(
			"rollback incomplete:\n- %s",
			strings.Join(
				rollbackErrors,
				"\n- ",
			),
		)
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

func checkpointPathDepth(path string) int {
	clean := filepath.Clean(path)

	if clean == "." {
		return 0
	}

	return len(
		strings.Split(
			clean,
			string(os.PathSeparator),
		),
	)
}

func compareCheckpointTrees(
	expected,
	actual map[string]checkpointEntry,
) error {
	if len(expected) != len(actual) {
		return fmt.Errorf(
			"rollback verification failed: expected %d entries, got %d",
			len(expected),
			len(actual),
		)
	}

	for path, want := range expected {
		got, ok := actual[path]
		if !ok {
			return fmt.Errorf(
				"rollback verification failed: missing %s",
				path,
			)
		}

		if want.IsDir != got.IsDir {
			return fmt.Errorf(
				"rollback verification failed: type mismatch for %s",
				path,
			)
		}

		if want.Mode.Perm() != got.Mode.Perm() {
			return fmt.Errorf(
				"rollback verification failed: permissions mismatch for %s",
				path,
			)
		}

		if !want.IsDir {
			if want.Size != got.Size ||
				want.Hash != got.Hash {
				return fmt.Errorf(
					"rollback verification failed: content mismatch for %s",
					path,
				)
			}
		}
	}

	return nil
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
