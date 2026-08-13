package workspace

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"gogitor/internal/textutil"
	"gogitor/internal/domain"
	"gogitor/internal/index"
	"gogitor/internal/security"

    "github.com/sergi/go-diff/diffmatchpatch"
)

const indexRefreshInterval = 2 * time.Second

type Workspace struct {
	Root string

	mu          sync.Mutex
	idx         *index.Index
	lastRefresh time.Time

	watcher         *fsnotify.Watcher
	watcherOnce     sync.Once
	watcherStop     chan struct{}
	watcherStopOnce sync.Once
}

func New(root string) *Workspace {
	return &Workspace{
		Root: root,
	}
}

func (w *Workspace) Close() error {
	w.mu.Lock()
	watcher := w.watcher
	stop := w.watcherStop
	w.watcher = nil
	w.watcherStop = nil
	w.mu.Unlock()

	if stop != nil {
		w.watcherStopOnce.Do(func() {
			close(stop)
		})
	}
	if watcher != nil {
		return watcher.Close()
	}
	return nil
}

func (w *Workspace) PrepareSandbox(ctx context.Context) (string, error) {
	tmp, err := os.MkdirTemp("", "gogitor-sandbox-")
	if err != nil {
		return "", fmt.Errorf("cannot create temp dir: %w", err)
	}
	if err := copyDir(ctx, w.Root, tmp); err != nil {
		_ = os.RemoveAll(tmp)
		return "", fmt.Errorf("cannot copy project to sandbox: %w", err)
	}
	return tmp, nil
}

func (w *Workspace) BuildModificationContext(targetFiles []string, maxFiles, maxBytes int) string {
	if maxFiles <= 0 {
		maxFiles = 20
	}
	if maxBytes <= 0 {
		maxBytes = 200000
	}

	var selected []string
	seen := make(map[string]bool)

	for _, rel := range w.ExistingFiles(targetFiles) {
		if !seen[rel] {
			seen[rel] = true
			selected = append(selected, rel)
		}
	}

	if len(selected) < maxFiles {
		for _, rel := range w.GoFiles(maxFiles) {
			if seen[rel] {
				continue
			}
			seen[rel] = true
			selected = append(selected, rel)
			if len(selected) >= maxFiles {
				break
			}
		}
	}

	return w.buildContextFromPaths(selected, maxBytes)
}

// ExistingFiles возвращает только существующие файлы из списка.
func (w *Workspace) ExistingFiles(paths []string) []string {
	var out []string
	seen := make(map[string]bool)
	for _, p := range paths {
		full, err := security.SafeJoin(w.Root, p)
		if err != nil {
			continue
		}
		info, err := os.Stat(full)
		if err != nil || info.IsDir() {
			continue
		}
		rel, err := filepath.Rel(w.Root, full)
		if err != nil {
			rel = p
		}
		if !seen[rel] {
			seen[rel] = true
			out = append(out, rel)
		}
	}
	return out
}

// HasGoFiles возвращает true, если в проекте есть хотя бы один Go-файл.
func (w *Workspace) HasGoFiles() bool {
	return len(w.GoFiles(1)) > 0
}

// GoFiles возвращает относительные пути Go-файлов проекта без _test.go.
func (w *Workspace) GoFiles(max int) []string {
	if max <= 0 {
		max = 10
	}
	var files []string
	_ = filepath.Walk(w.Root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if shouldSkipDir(name) {
				return filepath.SkipDir
			}
			return nil
		}
		if len(files) >= max {
			return nil
		}
		name := info.Name()
		if strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") && !shouldSkipFile(name) {
			rel, err := filepath.Rel(w.Root, path)
			if err == nil {
				files = append(files, rel)
			}
		}
		return nil
	})
	return files
}

func (w *Workspace) buildContextFromPaths(paths []string, maxBytes int) string {
	if maxBytes <= 0 {
		maxBytes = 120000
	}
	var b strings.Builder
	total := 0
	for _, p := range paths {
		if total >= maxBytes {
			break
		}
		full, err := security.SafeJoin(w.Root, p)
		if err != nil {
			continue
		}
		data, err := os.ReadFile(full)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(w.Root, full)
		if err != nil {
			rel = p
		}
		header := fmt.Sprintf("\n--- File: %s ---\n", rel)
		b.WriteString(header)
		total += len(header)

		remaining := maxBytes - total
		if remaining <= 0 {
			break
		}
		if len(data) > remaining {
			safe := textutil.TruncateBytes(data, remaining)
			b.Write(safe)
			b.WriteString("\n... truncated ...\n")
			total = maxBytes
			break
		}
		b.Write(data)
		b.WriteByte('\n')
		total += len(data) + 1
	}
	return b.String()
}

func copyDir(ctx context.Context, src, dst string) error {
	if _, err := os.Stat(src); err != nil {
		return nil
	}
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return nil
		}
		if rel == "." {
			return nil
		}
		name := info.Name()
		if info.IsDir() {
			if shouldSkipDir(name) {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), info.Mode().Perm())
		}
		if shouldSkipFile(name) {
			return nil
		}
		return copyFile(path, filepath.Join(dst, rel), info)
	})
}

func copyFile(src, dst string, info os.FileInfo) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func shouldSkipDir(name string) bool {
	switch name {
	case ".git", ".gogitor", ".idea", ".vscode", "node_modules":
		return true
	}
	return false
}

func shouldWatchSkipDir(name string) bool {
	if shouldSkipDir(name) {
		return true
	}
	return name == "vendor"
}

func shouldSkipFile(name string) bool {
	if name == ".DS_Store" {
		return true
	}
	if strings.HasSuffix(name, ".gogitor.bak") {
		return true
	}
	if strings.HasSuffix(name, ".gogitor.tmp") {
		return true
	}
	return false
}

func (w *Workspace) ApplyChangesSmart(dir string, changes []domain.FileChange) error {
	for _, ch := range changes {
		target, err := security.SafeJoin(dir, ch.Path)
		if err != nil {
			return err
		}

    	if len(ch.Patches) > 0 {
    		original, err := os.ReadFile(target)
    		if err != nil {
    			return fmt.Errorf("cannot read file for patch %s: %w", ch.Path, err)
    		}
    		// Логируем low-confidence патчи для диагностики.
    		for pi, p := range ch.Patches {
    			conf := PatchConfidence(string(original), p.Search)
    			if conf > 0 && conf < 0.9 {
    				fmt.Fprintf(os.Stderr,
    					"[warn] patch %d for %s applied with low confidence %.2f\n",
    					pi+1, ch.Path, conf)
    			}
    		}
    		updated, err := applyPatches(string(original), ch.Patches)
    		if err != nil {
    			return fmt.Errorf("cannot apply patch %s: %w", ch.Path, err)
    		}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("cannot create dir for %s: %w", ch.Path, err)
			}
			if err := os.WriteFile(target, []byte(updated), 0o644); err != nil {
				return fmt.Errorf("cannot write patched file %s: %w", ch.Path, err)
			}
			ensureExecutablePath(target)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("cannot create dir for %s: %w", ch.Path, err)
		}
		if err := os.WriteFile(target, []byte(ch.Content), 0o644); err != nil {
			return fmt.Errorf("cannot write %s: %w", ch.Path, err)
		}
	}
	return nil
}

func applyPatches(content string, patches []domain.Patch) (string, error) {
	content = normalizeNewlines(content)
	for i, p := range patches {
		updated, err := applyOnePatch(content, p)
		if err != nil {
			return "", fmt.Errorf("patch %d: %w", i+1, err)
		}
		content = updated
	}
	return content, nil
}

// ─── Нормализация для сравнения ─────────────────────────────────────────

// normalizeLineForCompare приводит строку к канонической форме для сравнения:
// табы → 4 пробела, trim leading/trailing whitespace.
func normalizeLineForCompare(s string) string {
	s = strings.ReplaceAll(s, "\t", "    ")
	s = strings.TrimLeft(s, " ")
	s = strings.TrimRight(s, " ")
	return s
}

// leadingIndent возвращает количество пробелов отступа (табы считаются как 4).
func leadingIndent(s string) int {
	count := 0
	for _, ch := range s {
		if ch == ' ' {
			count++
		} else if ch == '\t' {
			count += 4
		} else {
			break
		}
	}
	return count
}

// ─── Улучшенное сравнение строк ─────────────────────────────────────────

// relaxedLineEqual сравнивает две строки с допуском на:
// - разницу в trailing whitespace (до maxDiff символов)
// - разницу в leading whitespace (отступах) до maxIndentDiff
// - табы vs пробелы
func relaxedLineEqual(a, b string, maxTrailingDiff, maxIndentDiff int) bool {
	// Быстрый путь: полное совпадение.
	if a == b {
		return true
	}

	// Нормализуем табы → пробелы для обеих строк.
	aNorm := strings.ReplaceAll(a, "\t", "    ")
	bNorm := strings.ReplaceAll(b, "\t", "    ")

	if aNorm == bNorm {
		return true
	}

	// Сравниваем контент без leading/trailing whitespace.
	aContent := strings.TrimSpace(aNorm)
	bContent := strings.TrimSpace(bNorm)

	// Если контент (без отступов) не совпадает — строки разные.
	if aContent != bContent {
		return false
	}

	// Контент совпадает, проверяем допустимость разницы в отступах.
	aIndent := len(aNorm) - len(strings.TrimLeft(aNorm, " "))
	bIndent := len(bNorm) - len(strings.TrimLeft(bNorm, " "))
	indentDiff := aIndent - bIndent
	if indentDiff < 0 {
		indentDiff = -indentDiff
	}
	if indentDiff > maxIndentDiff {
		return false
	}

	// Проверяем trailing whitespace.
	aTrail := len(aNorm) - len(strings.TrimRight(aNorm, " "))
	bTrail := len(bNorm) - len(strings.TrimRight(bNorm, " "))
	trailDiff := aTrail - bTrail
	if trailDiff < 0 {
		trailDiff = -trailDiff
	}
	return trailDiff <= maxTrailingDiff
}

// ─── Similarity для fuzzy-поиска ────────────────────────────────────────

// lineSimilarity возвращает долю совпадающих строк (0.0–1.0)
// между двумя наборами строк после нормализации.
func lineSimilarity(aLines, bLines []string) float64 {
	if len(aLines) == 0 && len(bLines) == 0 {
		return 1.0
	}
	if len(aLines) != len(bLines) {
		return 0.0
	}
	matchCount := 0
	for i := range aLines {
		aNorm := normalizeLineForCompare(aLines[i])
		bNorm := normalizeLineForCompare(bLines[i])
		if aNorm == bNorm {
			matchCount++
		}
	}
	return float64(matchCount) / float64(len(aLines))
}

// ─── Поиск ближайшего блока ─────────────────────────────────────────────

type fuzzyMatch struct {
	StartLine  int
	Similarity float64
}

// findClosestBlock ищет в origLines блок, наиболее похожий на searchLines.
// Возвращает лучший результат с similarity >= threshold.
func findClosestBlock(origLines, searchLines []string, threshold float64) *fuzzyMatch {
	if len(searchLines) == 0 || len(searchLines) > len(origLines) {
		return nil
	}

	var best *fuzzyMatch
	for i := 0; i+len(searchLines) <= len(origLines); i++ {
		candidate := origLines[i : i+len(searchLines)]
		sim := lineSimilarity(candidate, searchLines)
		if sim >= threshold {
			if best == nil || sim > best.Similarity {
				best = &fuzzyMatch{StartLine: i, Similarity: sim}
			}
			// Если нашли идеальное совпадение, дальше не ищем.
			if sim >= 1.0 {
				break
			}
		}
	}
	return best
}

// ─── Основная функция применения патча ──────────────────────────────────

func applyOnePatch(content string, p domain.Patch) (string, error) {
    search := trimPatchLines(normalizeNewlines(p.Search))
    replace := trimPatchLines(normalizeNewlines(p.Replace))

    if search == "" {
        return "", fmt.Errorf("empty SEARCH block")
    }

    // Шаг 1: точное совпадение
    count := strings.Count(content, search)
    if count == 1 {
        return strings.Replace(content, search, replace, 1), nil
    }
    if count > 1 {
        return "", fmt.Errorf("SEARCH block is ambiguous (%d matches)", count)
    }

    // Шаг 2: расслабленное совпадение
    const maxTrailingWsDiff = 8
    const maxIndentDiff = 12
    origLines := strings.Split(content, "\n")
    searchLines := strings.Split(search, "\n")
    var matches []int
    for i := 0; i+len(searchLines) <= len(origLines); i++ {
        ok := true
        for j, searchLine := range searchLines {
            origLine := origLines[i+j]
            if !relaxedLineEqual(origLine, searchLine, maxTrailingWsDiff, maxIndentDiff) {
                ok = false
                break
            }
        }
        if ok {
            matches = append(matches, i)
        }
    }
    if len(matches) == 1 {
        idx := matches[0]
        var replaceLines []string
        if replace != "" {
            replaceLines = strings.Split(replace, "\n")
        }
        newLines := make([]string, 0, len(origLines)-len(searchLines)+len(replaceLines))
        newLines = append(newLines, origLines[:idx]...)
        newLines = append(newLines, replaceLines...)
        newLines = append(newLines, origLines[idx+len(searchLines):]...)
        return strings.Join(newLines, "\n"), nil
    }
    if len(matches) > 1 {
        return "", fmt.Errorf("SEARCH block is ambiguous (%d relaxed matches)", len(matches))
    }

    // Шаг 3: fuzzy-поиск по сходству строк (line similarity)
    const fuzzyThreshold = 0.80
    fuzzy := findClosestBlock(origLines, searchLines, fuzzyThreshold)
    if fuzzy != nil {
        idx := fuzzy.StartLine
        var replaceLines []string
        if replace != "" {
            replaceLines = strings.Split(replace, "\n")
        }
        newLines := make([]string, 0, len(origLines)-len(searchLines)+len(replaceLines))
        newLines = append(newLines, origLines[:idx]...)
        newLines = append(newLines, replaceLines...)
        newLines = append(newLines, origLines[idx+len(searchLines):]...)
        return strings.Join(newLines, "\n"), nil
    }

    // Шаг 4: поиск по нормализованному контенту (без отступов)
    normalizedSearch := make([]string, len(searchLines))
    for i, l := range searchLines {
        normalizedSearch[i] = normalizeLineForCompare(l)
    }
    var normMatches []int
    for i := 0; i+len(normalizedSearch) <= len(origLines); i++ {
        ok := true
        for j, searchNorm := range normalizedSearch {
            origNorm := normalizeLineForCompare(origLines[i+j])
            if origNorm != searchNorm {
                ok = false
                break
            }
        }
        if ok {
            normMatches = append(normMatches, i)
        }
    }
    if len(normMatches) == 1 {
        idx := normMatches[0]
        var replaceLines []string
        if replace != "" {
            replaceLines = strings.Split(replace, "\n")
        }
        newLines := make([]string, 0, len(origLines)-len(searchLines)+len(replaceLines))
        newLines = append(newLines, origLines[:idx]...)
        newLines = append(newLines, replaceLines...)
        newLines = append(newLines, origLines[idx+len(searchLines):]...)
        return strings.Join(newLines, "\n"), nil
    }
    if len(normMatches) > 1 {
        return "", fmt.Errorf("SEARCH block is ambiguous (%d normalized matches)", len(normMatches))
    }

    // ========== НОВЫЙ ШАГ 5: нечёткий поиск через diffmatchpatch ==========
    pos := findFuzzyMatchDMP(content, search)
    if pos != -1 {
        // Заменяем найденный фрагмент, считая, что длина search соответствует найденному блоку.
        // Это допустимо, так как различия обычно в пробелах/табах, а длина остаётся той же.
        newContent := content[:pos] + replace + content[pos+len(search):]
        return newContent, nil
    }

    return "", fmt.Errorf("SEARCH block not found")
}

func normalizeNewlines(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

func trimPatchLines(s string) string {
	lines := strings.Split(s, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

func (w *Workspace) CopyToRootSafe(sandboxDir string, changes []domain.FileChange) error {
	type backupEntry struct {
		path    string
		data    []byte
		existed bool
		mode    os.FileMode
	}
	var backups []backupEntry

	restore := func() {
		for i := len(backups) - 1; i >= 0; i-- {
			b := backups[i]
			if b.existed {
				mode := b.mode
				if mode == 0 {
					mode = 0o644
				}
				_ = os.WriteFile(b.path, b.data, mode)
				_ = os.Chmod(b.path, mode)
			} else {
				_ = os.Remove(b.path)
			}
		}
	}

	for _, ch := range changes {
		src, err := security.SafeJoin(sandboxDir, ch.Path)
		if err != nil {
			restore()
			return err
		}
		dst, err := security.SafeJoin(w.Root, ch.Path)
		if err != nil {
			restore()
			return err
		}

		data, err := os.ReadFile(src)
		if err != nil {
			restore()
			return fmt.Errorf("cannot read sandbox file %s: %w", ch.Path, err)
		}

		var oldData []byte
		existed := false
		var oldMode os.FileMode = 0o644
		if old, err := os.ReadFile(dst); err == nil {
			oldData = old
			existed = true
			if fi, err := os.Stat(dst); err == nil {
				oldMode = fi.Mode().Perm()
			}
		}

		backups = append(backups, backupEntry{
			path:    dst,
			data:    oldData,
			existed: existed,
			mode:    oldMode,
		})

		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			restore()
			return fmt.Errorf("cannot create dir for %s: %w", ch.Path, err)
		}

		tmp := dst + ".gogitor.tmp"
		if err := os.WriteFile(tmp, data, 0o644); err != nil {
			restore()
			return fmt.Errorf("cannot write temp file %s: %w", ch.Path, err)
		}
		if err := os.Rename(tmp, dst); err != nil {
			_ = os.Remove(tmp)
			restore()
			return fmt.Errorf("cannot replace root file %s: %w", ch.Path, err)
		}
		ensureExecutablePath(dst)
	}
	for _, ch := range changes {
		dst, err := security.SafeJoin(w.Root, ch.Path)
		if err != nil {
			restore()
			return fmt.Errorf("post-copy verification failed for %s: %w", ch.Path, err)
		}
		if _, err := os.Stat(dst); err != nil {
			restore()
			return fmt.Errorf("post-copy verification: file %s missing after write: %w", ch.Path, err)
		}
	}
	return nil
}

func (w *Workspace) Index() *index.Index {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.idx
}

// ExistingIndex — алиас для Index.
func (w *Workspace) ExistingIndex() *index.Index {
	return w.Index()
}

// RefreshIndex обновляет индекс после изменений файлов.
func (w *Workspace) RefreshIndex() {
	w.mu.Lock()
	if w.idx == nil {
		_ = w.indexLocked()
	} else {
		if err := w.idx.Refresh(); err == nil {
			w.lastRefresh = time.Now()
		}
	}
	w.mu.Unlock()

	if w.ExistingIndex() != nil {
		w.ensureWatcher()
	}
}

func (w *Workspace) indexLocked() *index.Index {
	if w.idx != nil {
		return w.idx
	}
	if len(w.GoFiles(1)) == 0 {
		return nil
	}
	idx := index.New(w.Root)
	if err := idx.Build(); err != nil {
		return nil
	}
	w.idx = idx
	w.lastRefresh = time.Now()
	return w.idx
}

func (w *Workspace) ensureFreshIndexLocked() *index.Index {
	idx := w.indexLocked()
	if idx == nil {
		return nil
	}
	if time.Since(w.lastRefresh) > indexRefreshInterval {
		if err := idx.Refresh(); err == nil {
			w.lastRefresh = time.Now()
		}
	}
	return idx
}

func (w *Workspace) ensureWatcher() {
	w.watcherOnce.Do(func() {
		go w.initWatcher()
	})
}

func (w *Workspace) initWatcher() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}

	w.mu.Lock()
	w.watcher = watcher
	w.watcherStop = make(chan struct{})
	root := w.Root
	w.mu.Unlock()

	w.addWatchDirs(root)
	go w.watchLoop()
}

func (w *Workspace) addWatchDirs(root string) {
	w.mu.Lock()
	watcher := w.watcher
	w.mu.Unlock()

	if watcher == nil {
		return
	}
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if shouldWatchSkipDir(info.Name()) {
				return filepath.SkipDir
			}
			_ = watcher.Add(path)
		}
		return nil
	})
}

func (w *Workspace) watchLoop() {
	w.mu.Lock()
	watcher := w.watcher
	stop := w.watcherStop
	w.mu.Unlock()

	if watcher == nil || stop == nil {
		return
	}

	var timer *time.Timer
	debounce := func() {
		if timer != nil {
			timer.Stop()
		}
		timer = time.AfterFunc(800*time.Millisecond, func() {
			w.refreshFromWatcher()
		})
	}

	for {
		select {
		case <-stop:
			if timer != nil {
				timer.Stop()
			}
			return
		case ev, ok := <-watcher.Events:
			if !ok {
				return
			}
			if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) == 0 {
				continue
			}
			if ev.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
					if !shouldWatchSkipDir(info.Name()) {
						_ = watcher.Add(ev.Name)
					}
				}
			}
			debounce()
		case _, ok := <-watcher.Errors:
			if !ok {
				return
			}
		}
	}
}

func (w *Workspace) refreshFromWatcher() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.idx == nil {
		return
	}
	if err := w.idx.Refresh(); err == nil {
		w.lastRefresh = time.Now()
	}
}

func (w *Workspace) BuildSmartContext(task string, targetFiles []string, maxFiles, maxBytes int) string {
	if maxFiles <= 0 {
		maxFiles = 20
	}
	if maxBytes <= 0 {
		maxBytes = 200000
	}

	w.mu.Lock()
	idx := w.ensureFreshIndexLocked()
	w.mu.Unlock()

	if idx != nil {
		w.ensureWatcher()
	}

	if idx == nil {
		return w.BuildModificationContext(targetFiles, maxFiles, maxBytes)
	}

	var selected []string
	seen := make(map[string]bool)

	for _, rel := range w.ExistingFiles(targetFiles) {
		if !seen[rel] {
			seen[rel] = true
			selected = append(selected, rel)
		}
	}

	if len(selected) < maxFiles {
		relevant := idx.SelectRelevantFilesV2(task, maxFiles)
		for _, rel := range relevant {
			if seen[rel] {
				continue
			}
			seen[rel] = true
			selected = append(selected, rel)
			if len(selected) >= maxFiles {
				break
			}
		}
	}

	if len(selected) < maxFiles {
		for _, rel := range w.GoFiles(maxFiles) {
			if seen[rel] {
				continue
			}
			seen[rel] = true
			selected = append(selected, rel)
			if len(selected) >= maxFiles {
				break
			}
		}
	}

	return w.buildContextFromPaths(selected, maxBytes)
}

func ensureExecutablePath(path string) {
	if isExecutableScriptPath(path) {
		_ = os.Chmod(path, 0o755)
	}
}

func isExecutableScriptPath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".sh", ".bash", ".zsh", ".fish", ".command":
		return true
	}
	return false
}

func findFuzzyMatchDMP(content, search string) int {
    dmp := diffmatchpatch.New()
    dmp.MatchThreshold = 0.5
    dmp.MatchDistance = 1000

    pos := dmp.MatchMain(content, search, 0)
    return pos
}

// PatchConfidence возвращает оценку уверенности (0.0–1.0) для fuzzy-применения патча.
// 1.0 = точное совпадение, 0.0 = SEARCH не найден.
// Используется для принятия решения об автоматическом применении.
func PatchConfidence(content string, search string) float64 {
	search = trimPatchLines(normalizeNewlines(search))
	if search == "" {
		return 0
	}
	// Шаг 1: точное совпадение
	if strings.Count(content, search) == 1 {
		return 1.0
	}
	// Шаг 2: relaxed match
	const maxTrailingWsDiff = 8
	const maxIndentDiff = 12
	origLines := strings.Split(content, "\n")
	searchLines := strings.Split(search, "\n")
	for i := 0; i+len(searchLines) <= len(origLines); i++ {
		ok := true
		for j, sl := range searchLines {
			if !relaxedLineEqual(origLines[i+j], sl, maxTrailingWsDiff, maxIndentDiff) {
				ok = false
				break
			}
		}
		if ok {
			return 0.95
		}
	}
	// Шаг 3: fuzzy по line similarity
	const threshold = 0.80
	fuzzy := findClosestBlock(origLines, searchLines, threshold)
	if fuzzy != nil {
		return fuzzy.Similarity
	}
	// Шаг 4: normalized match
	normalizedSearch := make([]string, len(searchLines))
	for i, l := range searchLines {
		normalizedSearch[i] = normalizeLineForCompare(l)
	}
	for i := 0; i+len(normalizedSearch) <= len(origLines); i++ {
		ok := true
		for j, sn := range normalizedSearch {
			if normalizeLineForCompare(origLines[i+j]) != sn {
				ok = false
				break
			}
		}
		if ok {
			return 0.85
		}
	}
	return 0
}