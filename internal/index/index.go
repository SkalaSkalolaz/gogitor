package index

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode"
)

const indexCacheVersion = 3

// Index — AST-индекс проекта.
type Index struct {
	mu         sync.RWMutex
	root       string
	modulePath string

	files map[string]*FileInfo

	importGraph map[string][]string
	callGraph   map[string][]string

	importedBy map[string][]string
	calledFrom map[string][]string

	centrality map[string]float64

	stats *corpusStats

	ready     bool
	cachePath string
}

type corpusStats struct {
	termFreq map[string]map[string]int
	docFreq  map[string]int
	docLen   map[string]int
	avgDL    float64
	docs     int
}

type indexCache struct {
	Version     int                  `json:"version"`
	Root        string               `json:"root"`
	ModulePath  string               `json:"module_path"`
	Files       map[string]*FileInfo `json:"files"`
	ImportGraph map[string][]string  `json:"import_graph"`
	CallGraph   map[string][]string  `json:"call_graph"`
	ImportedBy  map[string][]string  `json:"imported_by"`
	CalledFrom  map[string][]string  `json:"called_from"`
	Centrality  map[string]float64   `json:"centrality"`
}

// RankedFileV2 — файл с оценкой релевантности.
type RankedFileV2 struct {
	Path  string
	Score float64
}

// New создаёт индекс для проекта.
func New(root string) *Index {
	return &Index{
		root:        root,
		files:       make(map[string]*FileInfo),
		importGraph: make(map[string][]string),
		callGraph:   make(map[string][]string),
		importedBy:  make(map[string][]string),
		calledFrom:  make(map[string][]string),
		centrality:  make(map[string]float64),
		cachePath:   indexCachePath(root),
	}
}

// Build выполняет построение индекса.
//
// Сначала пытается загрузить кеш, затем делает инкрементальный refresh.
// Если кеш невалиден, выполняется полная сборка.
func (idx *Index) Build() error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if idx.loadCacheLocked() {
		return idx.refreshLocked()
	}

	return idx.buildLocked()
}

// Refresh — инкрементальное обновление.
func (idx *Index) Refresh() error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if !idx.ready {
		return idx.buildLocked()
	}

	return idx.refreshLocked()
}

// Ready возвращает true, если индекс построен.
func (idx *Index) Ready() bool {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	return idx.ready
}

// FileCount возвращает количество проиндексированных файлов.
func (idx *Index) FileCount() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	return len(idx.files)
}

func (idx *Index) buildLocked() error {
	idx.modulePath = detectModulePath(idx.root)
	idx.files = make(map[string]*FileInfo)

	for _, rel := range walkGoFiles(idx.root) {
		abs := filepath.Join(idx.root, rel)

		fi, err := parseFile(abs, rel)
		if err != nil {
			continue
		}

		idx.files[rel] = fi
	}

	idx.buildGraphsLocked()
	idx.ready = true
	idx.saveCacheLocked()

	return nil
}

func (idx *Index) refreshLocked() error {
	currentFiles := walkGoFiles(idx.root)
	currentSet := make(map[string]bool, len(currentFiles))

	changedCount := 0

	for _, rel := range currentFiles {
		currentSet[rel] = true

		abs := filepath.Join(idx.root, rel)
		fi, err := os.Stat(abs)
		if err != nil {
			continue
		}

		cached, exists := idx.files[rel]
		if !exists {
			parsed, err := parseFile(abs, rel)
			if err != nil {
				continue
			}

			idx.files[rel] = parsed
			changedCount++
			continue
		}

		if fi.ModTime().UnixNano() != cached.ModTime || fi.Size() != cached.Size {
			parsed, err := parseFile(abs, rel)
			if err != nil {
				delete(idx.files, rel)
				changedCount++
				continue
			}

			if parsed.Hash != cached.Hash {
				idx.files[rel] = parsed
				changedCount++
			} else {
				cached.ModTime = fi.ModTime().UnixNano()
				cached.Size = fi.Size()
			}
		}
	}

	for rel := range idx.files {
		if !currentSet[rel] {
			delete(idx.files, rel)
			changedCount++
		}
	}

	if changedCount == 0 {
		return nil
	}

	// Если изменений много, проще и надёжнее пересобрать индекс целиком.
	if changedCount > 32 || changedCount*2 > len(idx.files) {
		return idx.buildLocked()
	}

	idx.buildGraphsLocked()
	idx.saveCacheLocked()

	return nil
}

func (idx *Index) loadCacheLocked() bool {
	if idx.cachePath == "" {
		return false
	}

	data, err := os.ReadFile(idx.cachePath)
	if err != nil {
		return false
	}

	var cache indexCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return false
	}

	if cache.Version != indexCacheVersion || cache.Root != idx.root {
		return false
	}

	if len(cache.Files) == 0 {
		return false
	}

	idx.modulePath = cache.ModulePath
	idx.files = cache.Files

	idx.importGraph = nonNilStringSliceMap(cache.ImportGraph)
	idx.callGraph = nonNilStringSliceMap(cache.CallGraph)
	idx.importedBy = nonNilStringSliceMap(cache.ImportedBy)
	idx.calledFrom = nonNilStringSliceMap(cache.CalledFrom)
	idx.centrality = cache.Centrality

	if idx.centrality == nil {
		idx.centrality = make(map[string]float64)
		idx.computeCentralityLocked()
	}

	idx.ready = true
	idx.buildStatsLocked()

	return true
}

func (idx *Index) saveCacheLocked() {
	if idx.cachePath == "" || len(idx.files) == 0 {
		return
	}

	cache := indexCache{
		Version:     indexCacheVersion,
		Root:        idx.root,
		ModulePath:  idx.modulePath,
		Files:       idx.files,
		ImportGraph: idx.importGraph,
		CallGraph:   idx.callGraph,
		ImportedBy:  idx.importedBy,
		CalledFrom:  idx.calledFrom,
		Centrality:  idx.centrality,
	}

	data, err := json.Marshal(cache)
	if err != nil {
		return
	}

	dir := filepath.Dir(idx.cachePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}

	tmp := idx.cachePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}

	_ = os.Rename(tmp, idx.cachePath)
}

func (idx *Index) buildGraphsLocked() {
	idx.importGraph = make(map[string][]string)
	idx.callGraph = make(map[string][]string)
	idx.importedBy = make(map[string][]string)
	idx.calledFrom = make(map[string][]string)

	rels := make([]string, 0, len(idx.files))
	for rel := range idx.files {
		rels = append(rels, rel)
	}
	sort.Strings(rels)

	pkgToFiles := make(map[string][]string)
	filePkgPath := make(map[string]string, len(rels))
	pkgNameToPaths := make(map[string][]string)
	pkgBaseToPaths := make(map[string][]string)

	for _, rel := range rels {
		dir := filepath.ToSlash(filepath.Dir(rel))

		var pkgPath string
		if idx.modulePath != "" {
			if dir == "." {
				pkgPath = idx.modulePath
			} else {
				pkgPath = idx.modulePath + "/" + dir
			}
		} else {
			pkgPath = dir
		}

		filePkgPath[rel] = pkgPath
		pkgToFiles[pkgPath] = append(pkgToFiles[pkgPath], rel)

		fi := idx.files[rel]
		addUniqueString(pkgNameToPaths, fi.Package, pkgPath)

		base := filepath.Base(pkgPath)
		if base == "." || base == string(filepath.Separator) || base == "" {
			base = filepath.Base(idx.root)
		}

		addUniqueString(pkgBaseToPaths, base, pkgPath)
	}

	pkgSymbolToFile := make(map[string]string)
	localSymbolToFile := make(map[string]string)

	addFirst := func(m map[string]string, key, value string) {
		if key == "" || value == "" {
			return
		}
		if _, ok := m[key]; !ok {
			m[key] = value
		}
	}

	for _, rel := range rels {
		fi := idx.files[rel]
		pkgPath := filePkgPath[rel]

		for _, sym := range fi.Symbols {
			addFirst(pkgSymbolToFile, pkgPath+"."+sym.Name, rel)

			if sym.Receiver != "" {
				addFirst(pkgSymbolToFile, pkgPath+"."+sym.Receiver+"."+sym.Name, rel)
			}

			if sym.Kind == KindFunc {
				addFirst(localSymbolToFile, pkgPath+"::"+sym.Name, rel)
			}
		}
	}

	// Граф импортов.
	for _, rel := range rels {
		fi := idx.files[rel]
		seen := make(map[string]bool)

		for _, imp := range fi.Imports {
			pkgPaths := pkgPathsForImport(pkgToFiles, imp)

			for _, pkgPath := range pkgPaths {
				targets := pkgToFiles[pkgPath]

				for _, target := range targets {
					if target == rel || seen[target] {
						continue
					}

					seen[target] = true

					idx.importGraph[rel] = append(idx.importGraph[rel], target)
					idx.importedBy[target] = append(idx.importedBy[target], rel)
				}
			}
		}
	}

	// Граф вызовов.
	for _, rel := range rels {
		fi := idx.files[rel]
		pkgPath := filePkgPath[rel]

		seen := make(map[string]bool)

		for _, call := range fi.Calls {
			target := resolveCallTargetV2(
				fi,
				pkgPath,
				call.Callee,
				pkgToFiles,
				pkgNameToPaths,
				pkgBaseToPaths,
				pkgSymbolToFile,
				localSymbolToFile,
			)

			if target != "" && target != rel && !seen[target] {
				seen[target] = true

				idx.callGraph[rel] = append(idx.callGraph[rel], target)
				idx.calledFrom[target] = append(idx.calledFrom[target], rel)
			}
		}
	}

	idx.computeCentralityLocked()
	idx.buildStatsLocked()
}

func (idx *Index) computeCentralityLocked() {
	files := make([]string, 0, len(idx.files))
	for rel := range idx.files {
		files = append(files, rel)
	}
	sort.Strings(files)

	n := len(files)
	idx.centrality = make(map[string]float64, n)

	if n == 0 {
		return
	}

	out := make(map[string][]string, n)

	for _, rel := range files {
		seen := make(map[string]bool)

		add := func(target string) {
			if target == "" || target == rel || seen[target] {
				return
			}
			seen[target] = true
			out[rel] = append(out[rel], target)
		}

		for _, target := range idx.importGraph[rel] {
			add(target)
		}

		for _, target := range idx.callGraph[rel] {
			add(target)
		}
	}

	pr := make(map[string]float64, n)
	for _, rel := range files {
		pr[rel] = 1.0 / float64(n)
	}

	const damping = 0.85
	const iterations = 20

	for i := 0; i < iterations; i++ {
		next := make(map[string]float64, n)

		dangling := 0.0
		for _, rel := range files {
			if len(out[rel]) == 0 {
				dangling += pr[rel]
			}
		}

		base := (1.0-damping)/float64(n) + damping*dangling/float64(n)
		for _, rel := range files {
			next[rel] = base
		}

		for _, rel := range files {
			if len(out[rel]) == 0 {
				continue
			}

			share := damping * pr[rel] / float64(len(out[rel]))
			for _, target := range out[rel] {
				next[target] += share
			}
		}

		pr = next
	}

	max := 0.0
	for _, v := range pr {
		if v > max {
			max = v
		}
	}

	if max > 0 {
		for k, v := range pr {
			pr[k] = v / max
		}
	}

	idx.centrality = pr
}

func (idx *Index) buildStatsLocked() {
	stats := &corpusStats{
		termFreq: make(map[string]map[string]int),
		docFreq:  make(map[string]int),
		docLen:   make(map[string]int),
	}

	rels := make([]string, 0, len(idx.files))
	for rel := range idx.files {
		rels = append(rels, rel)
	}
	sort.Strings(rels)

	totalLen := 0

	for _, rel := range rels {
		terms := fileTerms(rel, idx.files[rel])
		freq := make(map[string]int, len(terms))

		for _, term := range terms {
			freq[term]++
		}

		stats.termFreq[rel] = freq
		stats.docLen[rel] = len(terms)
		totalLen += len(terms)

		for term := range freq {
			stats.docFreq[term]++
		}
	}

	stats.docs = len(rels)
	if stats.docs > 0 {
		stats.avgDL = float64(totalLen) / float64(stats.docs)
	} else {
		stats.avgDL = 1
	}

	idx.stats = stats
}

// ExpandContextV2 расширяет набор файлов через графы.
func (idx *Index) ExpandContextV2(seeds []string, depth int) []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if depth <= 0 {
		depth = 1
	}

	visited := make(map[string]bool)
	queue := make([]string, len(seeds))
	copy(queue, seeds)

	for _, seed := range seeds {
		visited[seed] = true
	}

	for d := 0; d < depth && len(queue) > 0; d++ {
		var next []string

		for _, file := range queue {
			for _, dep := range idx.importGraph[file] {
				if !visited[dep] {
					visited[dep] = true
					next = append(next, dep)
				}
			}

			for _, dep := range idx.importedBy[file] {
				if !visited[dep] {
					visited[dep] = true
					next = append(next, dep)
				}
			}

			for _, dep := range idx.callGraph[file] {
				if !visited[dep] {
					visited[dep] = true
					next = append(next, dep)
				}
			}

			for _, dep := range idx.calledFrom[file] {
				if !visited[dep] {
					visited[dep] = true
					next = append(next, dep)
				}
			}
		}

		queue = next
	}

	result := make([]string, 0, len(visited))
	for f := range visited {
		result = append(result, f)
	}

	return result
}

// SelectRelevantFilesV2 выбирает релевантные файлы по задаче.
func (idx *Index) SelectRelevantFilesV2(task string, maxFiles int) []string {
	if maxFiles <= 0 {
		maxFiles = 20
	}

	ranked := idx.rankFilesV2(task)
	if len(ranked) == 0 {
		return nil
	}

	seedCount := 5
	if seedCount > len(ranked) {
		seedCount = len(ranked)
	}
	if seedCount > maxFiles {
		seedCount = maxFiles
	}

	seeds := make([]string, seedCount)
	for i := 0; i < seedCount; i++ {
		seeds[i] = ranked[i].Path
	}

	expanded := idx.ExpandContextV2(seeds, 1)

	scoreMap := make(map[string]float64, len(ranked))
	for _, r := range ranked {
		scoreMap[r.Path] = r.Score
	}

	seedSet := make(map[string]bool, len(seeds))
	for _, seed := range seeds {
		seedSet[seed] = true
	}

	type fileScore struct {
		path  string
		score float64
	}

	var all []fileScore
	seen := make(map[string]bool)

	for _, f := range expanded {
		if seen[f] {
			continue
		}
		seen[f] = true

		score := scoreMap[f]
		if seedSet[f] {
			score += 100
		} else if score == 0 {
			score = 0.01
		}

		all = append(all, fileScore{path: f, score: score})
	}

	sort.Slice(all, func(i, j int) bool {
		if all[i].score != all[j].score {
			return all[i].score > all[j].score
		}
		return all[i].path < all[j].path
	})

	result := make([]string, 0, maxFiles)
	for _, fs := range all {
		if len(result) >= maxFiles {
			break
		}
		result = append(result, fs.path)
	}

	return result
}

func (idx *Index) rankFilesV2(task string) []RankedFileV2 {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if !idx.ready || idx.stats == nil || len(idx.files) == 0 {
		return nil
	}

	keywords := extractKeywordsV2(task)
	if len(keywords) == 0 {
		return nil
	}

	testIntent := detectTestIntentV2(task)

	var scored []RankedFileV2

	for rel, fi := range idx.files {
		score := idx.bm25ScoreLocked(rel, keywords)

		pathLower := strings.ToLower(rel)
		dirLower := strings.ToLower(filepath.Dir(rel))
		baseLower := strings.ToLower(strings.TrimSuffix(filepath.Base(rel), ".go"))

		for _, kw := range keywords {
			if strings.Contains(baseLower, kw) {
				score += 3.0
			}
			if strings.Contains(dirLower, kw) {
				score += 2.0
			}
			if strings.Contains(pathLower, kw) {
				score += 1.0
			}
		}

		if fi.IsTest {
			if testIntent {
				score += 2.0
				score *= 1.25
			} else {
				score *= 0.45
			}
		}

		if c := idx.centrality[rel]; c > 0 {
			score += c * 0.20
		}

		if score > 0 {
			scored = append(scored, RankedFileV2{
				Path:  rel,
				Score: score,
			})
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		if scored[i].Score != scored[j].Score {
			return scored[i].Score > scored[j].Score
		}
		return scored[i].Path < scored[j].Path
	})

	return scored
}

func (idx *Index) bm25ScoreLocked(rel string, keywords []string) float64 {
	if idx.stats == nil {
		return 0
	}

	freq := idx.stats.termFreq[rel]
	if len(freq) == 0 {
		return 0
	}

	const (
		k1 = 1.2
		b  = 0.75
	)

	avgDL := idx.stats.avgDL
	if avgDL <= 0 {
		avgDL = 1
	}

	dl := float64(idx.stats.docLen[rel])
	n := float64(idx.stats.docs)

	var score float64

	for _, term := range keywords {
		df := float64(idx.stats.docFreq[term])
		if df == 0 {
			continue
		}

		tf := float64(freq[term])
		if tf == 0 {
			continue
		}

		idf := math.Log(1 + (n-df+0.5)/(df+0.5))
		denom := tf + k1*(1-b+b*dl/avgDL)
		score += idf * tf * (k1 + 1) / denom
	}

	return score
}

func detectModulePath(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return ""
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}

	return ""
}

// walkGoFiles возвращает относительные пути всех .go файлов, включая _test.go.
func walkGoFiles(root string) []string {
	var files []string

	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			name := info.Name()
			if name == ".git" ||
				name == ".gogitor" ||
				name == "node_modules" ||
				name == "vendor" ||
				name == ".idea" ||
				name == ".vscode" {
				return filepath.SkipDir
			}

			return nil
		}

		name := info.Name()
		if strings.HasSuffix(name, ".go") {
			rel, err := filepath.Rel(root, path)
			if err == nil {
				files = append(files, rel)
			}
		}

		return nil
	})

	sort.Strings(files)

	return files
}

func indexCachePath(root string) string {
	h := sha1.Sum([]byte(root))
	name := hex.EncodeToString(h[:]) + ".json"

	if dir, err := os.UserCacheDir(); err == nil {
		return filepath.Join(dir, "gogitor", "index", name)
	}

	return filepath.Join(os.TempDir(), "gogitor-index", name)
}

func nonNilStringSliceMap(m map[string][]string) map[string][]string {
	if m == nil {
		return make(map[string][]string)
	}
	return m
}

func addUniqueString(m map[string][]string, key, value string) {
	if key == "" || value == "" {
		return
	}

	for _, existing := range m[key] {
		if existing == value {
			return
		}
	}

	m[key] = append(m[key], value)
}

func pkgPathsForImport(pkgToFiles map[string][]string, importPath string) []string {
	importPath = filepath.ToSlash(importPath)

	if _, ok := pkgToFiles[importPath]; ok {
		return []string{importPath}
	}

	var out []string

	for pkgPath := range pkgToFiles {
		if pkgPath == importPath || strings.HasSuffix(importPath, "/"+pkgPath) {
			out = append(out, pkgPath)
		}
	}

	sort.Strings(out)

	return out
}

func resolveImportPkgPathStrict(pkgToFiles map[string][]string, importPath string) string {
	importPath = filepath.ToSlash(importPath)

	if _, ok := pkgToFiles[importPath]; ok {
		return importPath
	}

	best := ""
	bestLen := 0
	ambiguous := false

	for pkgPath := range pkgToFiles {
		if pkgPath == "" || pkgPath == "." {
			continue
		}

		if !strings.HasSuffix(importPath, "/"+pkgPath) {
			continue
		}

		if len(pkgPath) > bestLen {
			best = pkgPath
			bestLen = len(pkgPath)
			ambiguous = false
			continue
		}

		if len(pkgPath) == bestLen {
			ambiguous = true
		}
	}

	if ambiguous {
		return ""
	}

	return best
}

func resolveCallTargetV2(
	fi *FileInfo,
	pkgPath string,
	callee string,
	pkgToFiles map[string][]string,
	pkgNameToPaths map[string][]string,
	pkgBaseToPaths map[string][]string,
	pkgSymbolToFile map[string]string,
	localSymbolToFile map[string]string,
) string {
	if callee == "" {
		return ""
	}

	if !strings.Contains(callee, ".") {
		return localSymbolToFile[pkgPath+"::"+callee]
	}

	parts := strings.SplitN(callee, ".", 2)
	if len(parts) != 2 {
		return ""
	}

	prefix := parts[0]
	name := parts[1]

	if prefix == "" || name == "" {
		return ""
	}

	if fi.ImportNames != nil {
		if importPath, ok := fi.ImportNames[prefix]; ok {
			candidatePkg := resolveImportPkgPathStrict(pkgToFiles, importPath)
			if candidatePkg == "" {
				return ""
			}

			return pkgSymbolToFile[candidatePkg+"."+name]
		}
	}

	if candidates := pkgNameToPaths[prefix]; len(candidates) == 1 {
		if target := pkgSymbolToFile[candidates[0]+"."+name]; target != "" {
			return target
		}
	}

	if candidates := pkgBaseToPaths[prefix]; len(candidates) == 1 {
		if target := pkgSymbolToFile[candidates[0]+"."+name]; target != "" {
			return target
		}
	}

	if target := pkgSymbolToFile[pkgPath+"."+prefix+"."+name]; target != "" {
		return target
	}

	return ""
}

func fileTerms(rel string, fi *FileInfo) []string {
	var terms []string

	add := func(s string) {
		for _, term := range normalizeTokensV2(s) {
			if term == "" || stopWordV2(term) {
				continue
			}
			terms = append(terms, term)
		}
	}

	add(rel)
	add(strings.TrimSuffix(filepath.Base(rel), ".go"))
	add(filepath.Dir(rel))

	if fi != nil {
		add(fi.Package)

		if fi.IsTest {
			terms = append(terms, "test", "tests", "тест")
		}

		for _, sym := range fi.Symbols {
			add(sym.Name)

			if sym.Receiver != "" {
				add(sym.Receiver)
			}

			if sym.Doc != "" {
				add(sym.Doc)
			}
		}
	}

	return terms
}

func normalizeTokensV2(s string) []string {
	s = strings.ReplaceAll(s, "\\", "/")

	fields := strings.FieldsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})

	var out []string

	for _, field := range fields {
		for _, part := range splitCamelV2(field) {
			part = strings.ToLower(part)
			if len(part) >= 2 {
				out = append(out, part)
			}
		}
	}

	return out
}

func splitCamelV2(s string) []string {
	var parts []string
	var cur strings.Builder

	runes := []rune(s)

	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) {
			prev := runes[i-1]

			splitBefore := false

			if unicode.IsLower(prev) || unicode.IsDigit(prev) {
				splitBefore = true
			} else if unicode.IsUpper(prev) && i+1 < len(runes) && unicode.IsLower(runes[i+1]) {
				splitBefore = true
			}

			if splitBefore && cur.Len() > 0 {
				parts = append(parts, cur.String())
				cur.Reset()
			}
		}

		cur.WriteRune(unicode.ToLower(r))
	}

	if cur.Len() > 0 {
		parts = append(parts, cur.String())
	}

	return parts
}

func extractKeywordsV2(task string) []string {
	tokens := normalizeTokensV2(task)

	seen := make(map[string]bool)
	var keywords []string

	for _, token := range tokens {
		if len(token) < 2 || stopWordV2(token) || seen[token] {
			continue
		}

		seen[token] = true
		keywords = append(keywords, token)

		for _, stem := range stemVariantsV2(token) {
			if len(stem) >= 3 && !seen[stem] {
				seen[stem] = true
				keywords = append(keywords, stem)
			}
		}
	}

	keywords = expandSynonymsV2(keywords, seen)

	return keywords
}

func stemVariantsV2(word string) []string {
	var variants []string

	ruSuffixes := []string{
		"ация", "ирование", "ность", "тель", "ник",
		"овать", "ировать", "изировать",
		"ный", "ная", "ное", "ные",
		"ский", "ская", "ское",
		"ость", "есть",
	}

	for _, suf := range ruSuffixes {
		if strings.HasSuffix(word, suf) && len(word)-len(suf) >= 3 {
			variants = append(variants, word[:len(word)-len(suf)])
		}
	}

	enSuffixes := []string{
		"tion", "sion", "ment", "ness", "ity",
		"ing", "ated", "ized", "ised",
		"able", "ible", "ful", "less",
		"er", "or", "ar",
		"ed", "ly", "al", "ic",
	}

	for _, suf := range enSuffixes {
		if strings.HasSuffix(word, suf) && len(word)-len(suf) >= 3 {
			variants = append(variants, word[:len(word)-len(suf)])
		}
	}

	return variants
}

var synonymGroupsV2 = [][]string{
	{"auth", "авториз", "login", "вход", "signin", "token", "токен", "jwt", "session", "сесси", "password", "пароль", "credential"},
	{"http", "handler", "хендлер", "endpoint", "роут", "route", "api", "server", "сервер", "request", "запрос", "response", "ответ", "middleware"},
	{"db", "database", "база", "sql", "postgres", "mysql", "mongo", "storage", "хранилищ", "repository", "репозитор"},
	{"cache", "кэш", "redis", "memcache", "кэшир"},
	{"test", "тест", "mock", "stub", "assert", "проверк"},
	{"config", "конфиг", "настройк", "env", "settings", "параметр"},
	{"error", "ошибк", "err", "panic", "recover", "обработк"},
	{"log", "лог", "logger", "slog", "zap", "логир"},
	{"user", "пользовател", "account", "аккаунт", "profile", "профил"},
	{"parse", "парс", "разбор", "unmarshal", "decode", "декод"},
	{"render", "рендер", "шаблон", "template", "html", "view", "представлен"},
	{"worker", "воркер", "queue", "очередь", "job", "задач", "task", "background", "фон"},
	{"crypto", "крипт", "encrypt", "шифр", "hash", "хеш", "sign", "подпис"},
	{"file", "файл", "upload", "загрузк", "download", "скачиван", "read", "write", "чтен", "запис"},
	{"network", "сеть", "tcp", "udp", "socket", "connection", "соединен", "client", "клиент"},
}

func expandSynonymsV2(keywords []string, seen map[string]bool) []string {
	for _, group := range synonymGroupsV2 {
		matched := false

		for _, kw := range keywords {
			for _, syn := range group {
				if strings.Contains(kw, syn) || strings.Contains(syn, kw) {
					matched = true
					break
				}
			}

			if matched {
				break
			}
		}

		if matched {
			for _, syn := range group {
				if !seen[syn] {
					seen[syn] = true
					keywords = append(keywords, syn)
				}
			}
		}
	}

	return keywords
}

func detectTestIntentV2(task string) bool {
	lower := strings.ToLower(task)

	testMarkers := []string{
		"test",
		"tests",
		"тест",
		"тесты",
		"тестов",
		"тестирован",
		"coverage",
		"покрыти",
		"assert",
		"mock",
		"stub",
		"failed test",
		"падает тест",
		"упал тест",
		"не проходит тест",
	}

	for _, marker := range testMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}

	return false
}

func stopWordV2(s string) bool {
	switch s {
	case
		"и", "в", "на", "с", "по", "для", "из", "от", "до", "за",
		"как", "что", "это", "или", "но", "не", "да", "бы", "ли", "же",
		"то", "все", "весь", "при", "без", "над", "под", "между",
		"после", "перед", "если", "когда", "где", "который",
		"надо", "нужно", "можно", "будет", "есть", "был", "была", "было",
		"этот", "эта", "эти", "тот", "мой", "свой", "наш", "ваш",
		"один", "два", "три", "первый", "очень", "просто", "только",
		"еще", "уже", "тут", "там", "здесь", "так", "вот", "ведь", "ни",
		"код", "файл", "файлы", "файла", "программа", "программы", "программу",
		"функция", "функции", "функцию", "проект", "проекта", "проекте",
		"напиши", "создай", "сделай", "добавь", "исправь", "измени",
		"обнови", "удали", "покажи", "объясни", "найди",
		"go", "golang", "main",
		"the", "a", "an", "is", "are", "was", "were", "be", "been", "being",
		"have", "has", "had", "do", "does", "did", "will", "would", "could",
		"should", "may", "might", "shall", "can",
		"to", "of", "in", "for", "on", "with", "at", "by", "from", "as",
		"into", "through", "during", "before", "after", "above", "below",
		"between", "and", "but", "or", "nor", "not", "so", "yet",
		"both", "either", "neither", "each", "every", "all", "any",
		"few", "more", "most", "other", "some", "such", "no", "only",
		"own", "same", "than", "too", "very", "just", "because",
		"if", "when", "where", "how", "what", "which", "who", "whom",
		"this", "that", "these", "those", "it", "its",
		"i", "me", "my", "we", "our", "you", "your",
		"he", "him", "his", "she", "her", "they", "them", "their",
		"code", "file", "files", "program", "function",
		"write", "create", "make", "add", "fix", "change", "update",
		"remove", "delete", "show", "explain", "find",
		"please", "help", "need", "want", "using", "use", "new":
		return true
	}

	return false
}