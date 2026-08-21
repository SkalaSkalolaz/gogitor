package security

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const maxSymlinkTraversals = 40

func SafeJoin(root, path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("empty path")
	}

	if filepath.IsAbs(path) {
		return "", fmt.Errorf("absolute paths are not allowed: %s", path)
	}

	cleaned := filepath.Clean(path)

	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path traversal is not allowed: %s", path)
	}

	if strings.Contains(cleaned, "\x00") {
		return "", fmt.Errorf("invalid path: %s", path)
	}

	full := filepath.Join(root, cleaned)

	// Прежняя лексическая проверка.
	rel, err := filepath.Rel(root, full)
	if err != nil {
		return "", fmt.Errorf("invalid path relation: %w", err)
	}

	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path escapes root directory: %s", path)
	}

	// Дальше проверяем путь с учётом символьных ссылок.
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("cannot resolve root: %w", err)
	}

	realRoot, err := resolvePathAllowMissing(absRoot)
	if err != nil {
		return "", fmt.Errorf("cannot resolve root symlinks: %w", err)
	}

	resolvedFull, err := resolvePathAllowMissing(filepath.Join(realRoot, cleaned))
	if err != nil {
		return "", fmt.Errorf("cannot resolve symlinks for %s: %w", path, err)
	}

	if !isInside(realRoot, resolvedFull) {
		return "", fmt.Errorf("path escapes root directory after symlink resolution: %s", path)
	}

	return full, nil
}

// isInside проверяет, находится ли path внутри root после очистки путей.
func isInside(root, path string) bool {
	root = filepath.Clean(root)
	path = filepath.Clean(path)

	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}

	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}

	return true
}

func resolvePathAllowMissing(path string) (string, error) {
	path = filepath.Clean(path)

	if !filepath.IsAbs(path) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return "", err
		}
		path = abs
	}

	var symlinkCount int

	components := splitPath(path)
	cur := pathRoot(path)

	for i := 0; i < len(components); i++ {
		comp := components[i]

		if comp == "" || comp == "." {
			continue
		}

		if comp == ".." {
			cur = filepath.Dir(cur)
			continue
		}

		next := filepath.Join(cur, comp)

		fi, err := os.Lstat(next)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				tail := make([]string, 0, len(components)-i+1)
				tail = append(tail, comp)
				tail = append(tail, components[i+1:]...)

				return filepath.Join(append([]string{cur}, tail...)...), nil
			}

			return "", err
		}

		if fi.Mode()&fs.ModeSymlink != 0 {
			symlinkCount++
			if symlinkCount > maxSymlinkTraversals {
				return "", fmt.Errorf("too many symlinks while resolving %q", path)
			}

			target, err := os.Readlink(next)
			if err != nil {
				return "", err
			}

			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(next), target)
			}

			target = filepath.Clean(target)

			if i+1 < len(components) {
				target = filepath.Join(target, filepath.Join(components[i+1:]...))
			}

			path = target
			components = splitPath(path)
			cur = pathRoot(path)
			i = -1

			continue
		}

		if fi.IsDir() {
			cur = next
			continue
		}

		if i == len(components)-1 {
			cur = next
			continue
		}

		return "", fmt.Errorf("path component %q is not a directory", comp)
	}

	return cur, nil
}

// pathRoot возвращает корневую часть абсолютного пути.
// Для Unix это "/", для Windows — например, "C:\".
func pathRoot(p string) string {
	vol := filepath.VolumeName(p)
	if vol != "" {
		return vol + string(filepath.Separator)
	}

	return string(filepath.Separator)
}

// splitPath разбивает абсолютный путь на компоненты без корневого префикса.
func splitPath(p string) []string {
	root := pathRoot(p)
	rel := strings.TrimPrefix(p, root)

	if rel == "" {
		return nil
	}

	parts := strings.Split(rel, string(filepath.Separator))
	out := make([]string, 0, len(parts))

	for _, part := range parts {
		if part != "" && part != "." {
			out = append(out, part)
		}
	}

	return out
}