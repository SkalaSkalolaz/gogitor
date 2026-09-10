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
	"gogitor/internal/domain"
	"gogitor/internal/index"
	"gogitor/internal/security"
	"gogitor/internal/textutil"
)

const indexRefreshInterval = 2 * time.Second

type Workspace struct {
	Root string

	mu              sync.Mutex
	applyMu         sync.Mutex
	diffTraceSink   DiffTraceSink
	diffMatching    domain.DiffMatchingConfig
	idx             *index.Index
	lastRefresh     time.Time
	watcher         *fsnotify.Watcher
	watcherOnce     sync.Once
	watcherStop     chan struct{}
	watcherStopOnce sync.Once
}

func New(root string) *Workspace {
	return &Workspace{
		Root:         root,
		diffMatching: domain.DefaultDiffMatchingConfig(),
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

func (w *Workspace) ApplyChangesSmart(
	dir string,
	changes []domain.FileChange,
) error {
	// Старый API сохраняем для полной обратной совместимости.
	// Старый вызов получает balanced-поведение.
	return w.ApplyChangesSmartWithPolicy(
		dir,
		changes,
		PatchPolicyBalanced,
		0,
	)
}

func (w *Workspace) ApplyChangesSmartWithPolicy(
	dir string,
	changes []domain.FileChange,
	policy PatchPolicy,
	minConfidenceOverride float64,
) error {
	for _, ch := range changes {
		if _, _, err := validateExpectedSource(dir, ch); err != nil {
			return err
		}
	}

	for _, ch := range changes {
		target, err := security.SafeJoin(dir, ch.Path)
		if err != nil {
			return err
		}

		if len(ch.Patches) > 0 {
			original, err := os.ReadFile(target)
			if err != nil {
				return fmt.Errorf(
					"cannot read file for patch %s: %w",
					ch.Path,
					err,
				)
			}

			updated, err := applyPatchesWithPolicyCore(
				string(original),
				ch.Patches,
				policy,
				minConfidenceOverride,
				w.getDiffMatchingConfig(),
				"SANDBOX_APPLY",
				ch.Path,
				w.getDiffTraceSink(),
			)
			if err != nil {
				return fmt.Errorf(
					"cannot apply patch %s: %w",
					ch.Path,
					err,
				)
			}

			if err := os.MkdirAll(
				filepath.Dir(target),
				0o755,
			); err != nil {
				return fmt.Errorf(
					"cannot create dir for %s: %w",
					ch.Path,
					err,
				)
			}

			if err := os.WriteFile(
				target,
				[]byte(updated),
				0o644,
			); err != nil {
				return fmt.Errorf(
					"cannot write patched file %s: %w",
					ch.Path,
					err,
				)
			}

			w.diffTracef(
				"phase=SANDBOX_APPLY file=%s stage=WRITE decision=OK bytes=%d patches=%d",
				ch.Path,
				len(updated),
				len(ch.Patches),
			)
			ensureExecutablePath(target)
			continue
		}

		if err := os.MkdirAll(
			filepath.Dir(target),
			0o755,
		); err != nil {
			return fmt.Errorf(
				"cannot create dir for %s: %w",
				ch.Path,
				err,
			)
		}

		if err := os.WriteFile(
			target,
			[]byte(ch.Content),
			0o644,
		); err != nil {
			return fmt.Errorf(
				"cannot write %s: %w",
				ch.Path,
				err,
			)
		}
	}

	return nil
}

func (w *Workspace) CopyToRootSafe(sandboxDir string, changes []domain.FileChange) error {
	w.applyMu.Lock()
	defer w.applyMu.Unlock()

	// Последняя optimistic-concurrency проверка.
	// Ни одного изменения root ещё не сделано.
	for _, ch := range changes {
		if _, _, err := validateExpectedSource(w.Root, ch); err != nil {
			return err
		}
	}
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
			return fmt.Errorf(
				"cannot replace root file %s: %w",
				ch.Path,
				err,
			)
		}

		ensureExecutablePath(dst)
		if len(ch.Patches) > 0 {
			w.diffTracef(
				"phase=ROOT_APPLY file=%s stage=COPY decision=OK mode=DIFF bytes=%d",
				ch.Path,
				len(data),
			)
		}

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

// SetDiffMatchingConfig устанавливает параметры DIFF matching.
func (w *Workspace) SetDiffMatchingConfig(
	cfg domain.DiffMatchingConfig,
) {
	w.mu.Lock()
	w.diffMatching = cfg.Normalized()
	w.mu.Unlock()
}

// getDiffMatchingConfig возвращает копию текущих параметров DIFF matching.
func (w *Workspace) getDiffMatchingConfig() domain.DiffMatchingConfig {
	w.mu.Lock()
	cfg := w.diffMatching
	w.mu.Unlock()

	return cfg.Normalized()
}

// DiffMatchingConfig возвращает фактические параметры,
// используемые patch engine.
func (w *Workspace) DiffMatchingConfig() domain.DiffMatchingConfig {
	return w.getDiffMatchingConfig()
}
