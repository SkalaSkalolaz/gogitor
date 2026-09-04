package workspace

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"gogitor/internal/domain"
	"gogitor/internal/security"
)

// PatchProtocol determines which patch representation Gogitor asks a model to emit.
type PatchProtocol int

const (
	PatchProtocolSearchReplace PatchProtocol = iota
	PatchProtocolReplaceOnly
)

func (p PatchProtocol) String() string {
	if p == PatchProtocolReplaceOnly {
		return "replace_only"
	}
	return "search_replace"
}

func ParsePatchProtocol(s string) (PatchProtocol, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "replace_only", "replace-only":
		return PatchProtocolReplaceOnly, true
	case "search_replace", "search-replace", "searchreplace":
		return PatchProtocolSearchReplace, true
	default:
		return PatchProtocolSearchReplace, false
	}
}

// PatchProtocolForModel keeps the existing policy split intact while choosing a
// simpler output protocol for models that are more likely to make SEARCH mistakes.
func PatchProtocolForModel(provider, model, override string) PatchProtocol {
	if p, ok := ParsePatchProtocol(override); ok {
		return p
	}

	p := strings.ToLower(strings.TrimSpace(provider))
	m := strings.ToLower(strings.TrimSpace(model))

	// Remote / OpenAI-compatible endpoints are normally strong enough to keep
	// the explicit SEARCH/REPLACE protocol.
	if strings.HasPrefix(p, "openai+") ||
		strings.HasPrefix(p, "openai-compatible+") ||
		strings.Contains(m, "cloud") {
		return PatchProtocolSearchReplace
	}

	if size := modelParameterCountB(m); size > 0 && size <= 12 {
		return PatchProtocolReplaceOnly
	}

	return PatchProtocolSearchReplace
}

type PatchPreflightReport struct {
	Files        int
	PatchFiles   int
	PatchBlocks  int
	ChangedLines int
	ChangedBytes int
}

func hashBytes(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// CaptureProjectSnapshot hashes the whole working tree that Gogitor may touch.
// It is intentionally broader than targetFiles: the model can name another file
// in its response, so concurrency protection must cover the full tree.
func (w *Workspace) CaptureProjectSnapshot() (map[string]string, error) {
	out := make(map[string]string)

	err := filepath.Walk(w.Root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if shouldSkipDir(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if shouldSkipFile(info.Name()) {
			return nil
		}

		rel, err := filepath.Rel(w.Root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("snapshot %s: %w", rel, err)
		}
		out[filepath.Clean(rel)] = hashBytes(data)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return out, nil
}

// BindSourceSnapshot copies trusted snapshot information into changes produced by the LLM.
// The LLM never controls these fields.
func (w *Workspace) BindSourceSnapshot(
	changes []domain.FileChange,
	snapshot map[string]string,
) []domain.FileChange {
	out := cloneFileChanges(changes)
	for i := range out {
		path := filepath.Clean(strings.TrimSpace(out[i].Path))
		out[i].Path = path

		hash, exists := snapshot[path]
		out[i].ExpectedPresent = exists
		out[i].ExpectedAbsent = !exists
		if exists {
			out[i].SourceHash = hash
		}

		if len(out[i].Patches) == 0 || !exists {
			continue
		}

		full, err := security.SafeJoin(w.Root, path)
		if err != nil {
			continue
		}
		data, err := os.ReadFile(full)
		if err != nil || hashBytes(data) != hash {
			continue
		}

		for j := range out[i].Patches {
			// Один и тот же hash относится ко всему исходному
			// FileChange и не меняется после применения предыдущих
			// patch-блоков.
			out[i].Patches[j].ExpectedSourceHash = hash

			symbol := strings.TrimSpace(
				out[i].Patches[j].Symbol,
			)

			if symbol == "" {
				continue
			}

			fp, err := SymbolFingerprint(
				string(data),
				symbol,
			)

			if err == nil {
				out[i].Patches[j].ExpectedSymbolFingerprint = fp
			}
		}
	}
	return out
}

func cloneFileChanges(changes []domain.FileChange) []domain.FileChange {
	out := make([]domain.FileChange, len(changes))
	copy(out, changes)
	for i := range out {
		if len(changes[i].Patches) > 0 {
			out[i].Patches = append([]domain.Patch(nil), changes[i].Patches...)
		}
	}
	return out
}

func validateExpectedSource(root string, ch domain.FileChange) ([]byte, bool, error) {
	full, err := security.SafeJoin(root, ch.Path)
	if err != nil {
		return nil, false, err
	}

	data, err := os.ReadFile(full)
	if err != nil {
		if os.IsNotExist(err) {
			if ch.ExpectedPresent {
				return nil, false, fmt.Errorf(
					"stale change %s: file disappeared after source snapshot",
					ch.Path,
				)
			}
			if ch.ExpectedAbsent {
				return nil, false, nil
			}
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("cannot read expected source %s: %w", ch.Path, err)
	}

	if ch.ExpectedAbsent {
		return nil, true, fmt.Errorf(
			"stale change %s: file appeared after source snapshot",
			ch.Path,
		)
	}

	if ch.SourceHash != "" && hashBytes(data) != ch.SourceHash {
		return nil, true, fmt.Errorf(
			"stale change %s: source file changed after snapshot",
			ch.Path,
		)
	}

	return data, true, nil
}

// PreflightChanges validates and resolves all changes without writing files.
func (w *Workspace) PreflightChanges(
	dir string,
	changes []domain.FileChange,
	policy PatchPolicy,
	minConfidenceOverride float64,
) ([]domain.FileChange, *PatchPreflightReport, error) {
	if len(changes) == 0 {
		return nil, nil, fmt.Errorf("no changes to preflight")
	}

	prepared := cloneFileChanges(changes)
	report := &PatchPreflightReport{Files: len(prepared)}

	for i := range prepared {
		ch := &prepared[i]
		if strings.TrimSpace(ch.Path) == "" {
			return nil, nil, fmt.Errorf("preflight file %d has empty path", i+1)
		}
		if _, err := security.SafeJoin(dir, ch.Path); err != nil {
			return nil, nil, fmt.Errorf("preflight %s: invalid path: %w", ch.Path, err)
		}

		original, exists, err := validateExpectedSource(dir, *ch)
		if err != nil {
			return nil, nil, err
		}

		if len(ch.Patches) == 0 {
			if strings.TrimSpace(ch.Content) == "" {
				return nil, nil, fmt.Errorf("preflight %s: empty file content", ch.Path)
			}
			continue
		}

		if !exists {
			return nil, nil, fmt.Errorf("preflight %s: patch target file does not exist", ch.Path)
		}

		report.PatchFiles++
		report.PatchBlocks += len(ch.Patches)

		before := string(original)
		current := before
		for pi := range ch.Patches {
			if ch.Patches[pi].ExpectedSymbolFingerprint != "" && strings.TrimSpace(ch.Patches[pi].Symbol) != "" {
				fp, fpErr := SymbolFingerprint(before, ch.Patches[pi].Symbol)
				if fpErr != nil {
					return nil, nil, fmt.Errorf("preflight %s patch %d: %w", ch.Path, pi+1, fpErr)
				}
				if fp != ch.Patches[pi].ExpectedSymbolFingerprint {
					return nil, nil, fmt.Errorf(
						"preflight %s patch %d: symbol %q changed since generation",
						ch.Path, pi+1, ch.Patches[pi].Symbol,
					)
				}
			}

			p := ch.Patches[pi]
			resolved, resolveErr := preparePatch(current, p)
			if resolveErr != nil {
				return nil, nil, fmt.Errorf("preflight %s patch %d: %w", ch.Path, pi+1, resolveErr)
			}
			ch.Patches[pi] = resolved
			updated, err := applyOnePatchWithPolicyCoreChecked(
				current,
				resolved,
				policy,
				minConfidenceOverride,
				w.getDiffMatchingConfig(),
				pi == 0,
				newPatchTrace(
					w.getDiffTraceSink(),
					"PREFLIGHT",
					ch.Path,
					pi+1,
					len(ch.Patches),
					policy,
					resolved,
				),
			)
			if err != nil {
				return nil, nil, fmt.Errorf("preflight %s patch %d: %w", ch.Path, pi+1, err)
			}
			current = updated
		}

		if err := validateSemanticScope(before, current, ch.Patches, ch.Path); err != nil {
			return nil, nil, err
		}
		if err := validatePublicAPIGuard(before, current, ch.Patches, ch.Path); err != nil {
			return nil, nil, err
		}
		if err := validateImportGuard(before, current, ch.Patches, ch.Path); err != nil {
			return nil, nil, err
		}
		if err := validateGoModGuard(before, current, ch.Patches, ch.Path); err != nil {
			return nil, nil, err
		}

		maxBlocks, maxLines, maxBytes := patchLimits(policy)

		changedLines := estimateChangedLines(before, current)
		changedBytes := len(current) - len(before)
		if changedBytes < 0 {
			changedBytes = -changedBytes
		}

		if len(ch.Patches) > maxBlocks {
			return nil, nil, fmt.Errorf(
				"preflight %s: too many patch blocks (%d, max %d)",
				ch.Path, len(ch.Patches), maxBlocks,
			)
		}
		if changedLines > maxLines {
			return nil, nil, fmt.Errorf(
				"preflight %s: change is too large (%d lines, max %d)",
				ch.Path, changedLines, maxLines,
			)
		}
		if changedBytes > maxBytes {
			return nil, nil, fmt.Errorf(
				"preflight %s: change is too large (%d bytes, max %d)",
				ch.Path, changedBytes, maxBytes,
			)
		}

		report.ChangedLines += changedLines
		report.ChangedBytes += changedBytes
		w.diffTracef(
			"phase=PREFLIGHT file=%s stage=SUMMARY decision=OK patch_blocks=%d changed_lines=%d changed_bytes=%d",
			ch.Path,
			len(ch.Patches),
			changedLines,
			changedBytes,
		)
	}

	return prepared, report, nil
}

func preparePatch(content string, p domain.Patch) (domain.Patch, error) {
	if !p.ReplaceOnly {
		return p, nil
	}
	if strings.TrimSpace(p.Symbol) == "" {
		return domain.Patch{}, fmt.Errorf("REPLACE_ONLY patch requires Symbol")
	}
	if strings.TrimSpace(p.Search) != "" {
		return domain.Patch{}, fmt.Errorf("REPLACE_ONLY patch must not contain SEARCH")
	}
	if strings.TrimSpace(p.Replace) == "" {
		return domain.Patch{}, fmt.Errorf("REPLACE_ONLY patch has empty replacement")
	}
	start, end, err := findSymbolRange(content, p.Symbol)
	if err != nil {
		return domain.Patch{}, err
	}
	search := content[start:end]
	if err := validateReplacementForSymbol(p.Replace, p.Symbol); err != nil {
		return domain.Patch{}, err
	}
	p.Search = search
	return p, nil
}

func SymbolFingerprint(content, symbol string) (string, error) {
	start, end, err := findSymbolRange(content, symbol)
	if err != nil {
		return "", err
	}
	return hashBytes([]byte(normalizeSourceForFingerprint(content[start:end]))), nil
}

func normalizeSourceForFingerprint(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, "\r\n", "\n")), " ")
}

func validateReplacementForSymbol(replacement, symbol string) error {
	replacement = strings.TrimSpace(replacement)
	if replacement == "" {
		return fmt.Errorf("empty replacement")
	}

	file, err := parser.ParseFile(
		token.NewFileSet(),
		"replace-only.go",
		[]byte("package patchcheck\n\n"+replacement+"\n"),
		parser.ParseComments,
	)
	if err != nil {
		return fmt.Errorf("invalid REPLACE_ONLY Go declaration: %w", err)
	}

	if len(file.Decls) != 1 {
		return fmt.Errorf("REPLACE_ONLY must contain exactly one top-level declaration")
	}

	fn, ok := file.Decls[0].(*ast.FuncDecl)
	if !ok || fn.Name == nil {
		return fmt.Errorf("REPLACE_ONLY currently supports functions and methods only")
	}

	name := fn.Name.Name
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		if recv := receiverTypeName(fn.Recv.List[0].Type); recv != "" {
			name = recv + "." + name
		}
	}

	if normalizePatchSymbol(symbol) != name {
		return fmt.Errorf(
			"replacement Symbol %q does not match target Symbol %q",
			name, normalizePatchSymbol(symbol),
		)
	}

	return nil
}

func patchLimits(policy PatchPolicy) (maxBlocks, maxLines, maxBytes int) {
	switch policy {
	case PatchPolicyAdvanced:
		return 16, 600, 48 * 1024
	case PatchPolicyBalanced:
		return 10, 300, 24 * 1024
	default:
		return 6, 120, 12 * 1024
	}
}

func estimateChangedLines(before, after string) int {
	beforeLines := strings.Split(strings.ReplaceAll(before, "\r\n", "\n"), "\n")
	afterLines := strings.Split(strings.ReplaceAll(after, "\r\n", "\n"), "\n")

	prefix := 0
	for prefix < len(beforeLines) && prefix < len(afterLines) && beforeLines[prefix] == afterLines[prefix] {
		prefix++
	}

	bi := len(beforeLines) - 1
	ai := len(afterLines) - 1
	for bi >= prefix && ai >= prefix && beforeLines[bi] == afterLines[ai] {
		bi--
		ai--
	}

	oldMiddle := bi - prefix + 1
	newMiddle := ai - prefix + 1
	if oldMiddle < 0 {
		oldMiddle = 0
	}
	if newMiddle < 0 {
		newMiddle = 0
	}
	return oldMiddle + newMiddle
}

func validateSemanticScope(
	before,
	after string,
	patches []domain.Patch,
	path string,
) error {
	if len(patches) == 0 || !isGoPath(path) {
		return nil
	}

	allowed := make(map[string]bool)

	for _, p := range patches {
		if symbol := normalizePatchSymbol(p.Symbol); symbol != "" {
			allowed[declarationKey(symbol)] = true
		}
	}

	if len(allowed) == 0 {
		return nil
	}

	beforeDecls, err := goDeclarationFingerprints(before)
	if err != nil {
		return fmt.Errorf(
			"semantic scope %s: parse before: %w",
			path,
			err,
		)
	}

	afterDecls, err := goDeclarationFingerprints(after)
	if err != nil {
		return fmt.Errorf(
			"semantic scope %s: parse after: %w",
			path,
			err,
		)
	}

	calls, err := goDeclarationCalls(after)
	if err != nil {
		return fmt.Errorf(
			"semantic scope %s: parse calls: %w",
			path,
			err,
		)
	}

	// Разрешаем ограниченный класс новых приватных helper-функций:
	//
	//   func main() {
	//       registerRoutes()
	//   }
	//
	//   func registerRoutes() {
	//       ...
	//   }
	//
	// registerRoutes() не является исходным Symbol,
	// но является непосредственной частью требуемого
	// структурного рефакторинга.
	for key := range afterDecls {
		if newScopedFunctionAllowed(
			key,
			beforeDecls,
			afterDecls,
			allowed,
			calls,
		) {
			allowed[key] = true
		}
	}

	var unexpected []string

	all := make(map[string]bool, len(beforeDecls)+len(afterDecls))

	for key := range beforeDecls {
		all[key] = true
	}

	for key := range afterDecls {
		all[key] = true
	}

	for key := range all {
		if beforeDecls[key] == afterDecls[key] {
			continue
		}

		if !allowed[key] {
			unexpected = append(
				unexpected,
				key,
			)
		}
	}

	if len(unexpected) > 0 {
		sort.Strings(unexpected)

		return fmt.Errorf(
			"patch_error_code=semantic_scope: semantic scope %s: unrelated declarations changed: %s",
			path,
			strings.Join(unexpected, ", "),
		)
	}

	return nil
}

func validatePublicAPIGuard(before, after string, patches []domain.Patch, path string) error {
	if len(patches) == 0 || !isGoPath(path) {
		return nil
	}
	allowed := make(map[string]bool)
	for _, p := range patches {
		if symbol := normalizePatchSymbol(p.Symbol); symbol != "" {
			allowed[declarationKey(symbol)] = true
		}
	}
	if len(allowed) == 0 {
		return nil
	}

	beforeDecls, err := goDeclarationFingerprints(before)
	if err != nil {
		return fmt.Errorf("public API guard %s: parse before: %w", path, err)
	}
	afterDecls, err := goDeclarationFingerprints(after)
	if err != nil {
		return fmt.Errorf("public API guard %s: parse after: %w", path, err)
	}

	var unexpected []string
	all := make(map[string]bool, len(beforeDecls)+len(afterDecls))
	for k := range beforeDecls {
		all[k] = true
	}
	for k := range afterDecls {
		all[k] = true
	}
	for k := range all {
		if !isExportedDeclarationKey(k) || beforeDecls[k] == afterDecls[k] {
			continue
		}
		if !allowed[k] {
			unexpected = append(unexpected, k)
		}
	}

	if len(unexpected) > 0 {
		sort.Strings(unexpected)
		return fmt.Errorf(
			"public API guard %s: unapproved exported API changes: %s",
			path, strings.Join(unexpected, ", "),
		)
	}
	return nil
}

func validateImportGuard(before, after string, patches []domain.Patch, path string) error {
	if len(patches) == 0 || !isGoPath(path) {
		return nil
	}
	beforeImports, err := goImports(before)
	if err != nil {
		return fmt.Errorf("import guard %s: parse before: %w", path, err)
	}
	afterImports, err := goImports(after)
	if err != nil {
		return fmt.Errorf("import guard %s: parse after: %w", path, err)
	}
	if equalStringSet(beforeImports, afterImports) {
		return nil
	}

	for _, p := range patches {
		if patchContainsImportDeclaration(p.Search) || patchContainsImportDeclaration(p.Replace) {
			return nil
		}
	}
	return fmt.Errorf("import guard %s: import set changed outside an explicit import patch", path)
}

func patchContainsImportDeclaration(s string) bool {
	for _, line := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "import ") || trimmed == "import(" || trimmed == "import (" {
			return true
		}
	}
	return false
}

func validateGoModGuard(before, after string, patches []domain.Patch, path string) error {
	if len(patches) == 0 || filepath.ToSlash(filepath.Clean(path)) != "go.mod" {
		return nil
	}
	beforeModule := directiveValue(before, "module")
	afterModule := directiveValue(after, "module")
	if beforeModule == afterModule {
		return nil
	}
	for _, p := range patches {
		if strings.Contains(strings.TrimSpace(p.Search), "module ") ||
			strings.Contains(strings.TrimSpace(p.Replace), "module ") {
			return nil
		}
	}
	return fmt.Errorf("go.mod guard: module path changed without an explicit module patch")
}

func directiveValue(content, directive string) string {
	for _, line := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") || trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, directive+" ") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, directive))
		}
	}
	return ""
}

func goImports(content string) ([]string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), "imports.go", []byte(content), 0)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(file.Imports))
	for _, spec := range file.Imports {
		if spec.Path == nil {
			continue
		}
		out = append(out, strings.Trim(spec.Path.Value, `"`))
	}
	sort.Strings(out)
	return out, nil
}

func equalStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func goDeclarationFingerprints(content string) (map[string]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "fingerprint.go", []byte(content), parser.ParseComments)
	if err != nil {
		return nil, err
	}

	out := make(map[string]string)
	for _, decl := range file.Decls {
		switch d := decl.(type) {

		case *ast.FuncDecl:
			key := goFuncDeclarationKey(d)
			if key == "" {
				continue
			}

			out[key] = nodeFingerprint(fset, d)
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					out["type:"+s.Name.Name] = nodeFingerprint(fset, s)
				case *ast.ValueSpec:
					var names []string
					for _, n := range s.Names {
						if n != nil {
							names = append(names, n.Name)
						}
					}
					sort.Strings(names)
					out["value:"+strings.Join(names, ",")] = nodeFingerprint(fset, s)
				}
			}
		}
	}
	return out, nil
}

func goFuncDeclarationKey(d *ast.FuncDecl) string {
	if d == nil || d.Name == nil {
		return ""
	}

	name := d.Name.Name

	if d.Recv != nil && len(d.Recv.List) > 0 {
		if recv := receiverTypeName(d.Recv.List[0].Type); recv != "" {
			name = recv + "." + name
		}
	}

	return "func:" + name
}

func goDeclarationCalls(content string) (map[string]map[string]bool, error) {
	fset := token.NewFileSet()

	file, err := parser.ParseFile(
		fset,
		"calls.go",
		[]byte(content),
		parser.ParseComments,
	)
	if err != nil {
		return nil, err
	}

	out := make(map[string]map[string]bool)

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil || fn.Body == nil {
			continue
		}

		key := goFuncDeclarationKey(fn)
		if key == "" {
			continue
		}

		called := make(map[string]bool)

		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			switch expr := call.Fun.(type) {
			case *ast.Ident:
				if expr.Name != "" {
					called[expr.Name] = true
				}

			case *ast.SelectorExpr:
				if expr.Sel != nil && expr.Sel.Name != "" {
					called[expr.Sel.Name] = true
				}
			}

			return true
		})

		out[key] = called
	}

	return out, nil
}

func newScopedFunctionAllowed(
	key string,
	beforeDecls map[string]string,
	afterDecls map[string]string,
	allowed map[string]bool,
	calls map[string]map[string]bool,
) bool {
	if !strings.HasPrefix(key, "func:") {
		return false
	}

	// Функция должна быть действительно новой.
	if _, existedBefore := beforeDecls[key]; existedBefore {
		return false
	}

	if _, existsAfter := afterDecls[key]; !existsAfter {
		return false
	}

	// Новая публичная функция автоматически запрещена.
	if isExportedDeclarationKey(key) {
		return false
	}

	name := strings.TrimPrefix(key, "func:")
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		name = name[idx+1:]
	}

	if name == "" {
		return false
	}

	// Разрешаем только тогда, когда новая функция реально
	// вызывается одним из уже разрешённых Symbol.
	for parent := range allowed {
		if calls[parent] != nil && calls[parent][name] {
			return true
		}
	}

	return false
}

func nodeFingerprint(fset *token.FileSet, node ast.Node) string {
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, node); err != nil {
		return ""
	}
	return hashBytes([]byte(normalizeSourceForFingerprint(buf.String())))
}

func declarationKey(symbol string) string {
	return "func:" + normalizePatchSymbol(symbol)
}

func isExportedDeclarationKey(key string) bool {
	name := key
	if idx := strings.Index(name, ":"); idx >= 0 {
		name = name[idx+1:]
	}
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		name = name[idx+1:]
	}
	if name == "" {
		return false
	}
	return unicode.IsUpper([]rune(name)[0])
}

func isGoPath(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".go")
}

// AffectedPackageDirs returns unique package directories for changed Go files.
func (w *Workspace) AffectedPackageDirs(dir string, changes []domain.FileChange) []string {
	seen := make(map[string]bool)
	for _, ch := range changes {
		if !isGoPath(ch.Path) {
			continue
		}
		pkgDir := filepath.ToSlash(filepath.Dir(filepath.Clean(ch.Path)))
		if pkgDir == "" {
			pkgDir = "."
		}
		seen[pkgDir] = true
	}

	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// findRebasedBlock relocates an exact patch block by stable unique anchors.
//
// REBASE не является fuzzy matching.
//
// Его задача — найти ТОТ ЖЕ САМЫЙ SEARCH-блок,
// который был перемещён в другое место файла.
//
// Допускаются только различия, уже предусмотренные
// normalizeLineForCompare(), например:
//   - tabs vs spaces;
//   - лишние пробелы в начале/конце строки.
//
// Содержательные отличия строк REBASE не допускает.
// Если содержимое изменилось, управление должно перейти
// в обычный fuzzy matcher, где действуют его собственные
// confidence и ambiguity thresholds.
func findRebasedBlock(
	origLines,
	searchLines []string,
) *fuzzyMatch {
	if len(searchLines) == 0 ||
		len(searchLines) > len(origLines) {
		return nil
	}

	// Нормализованная строка -> все её позиции.
	frequency := make(map[string][]int)

	for i, line := range origLines {
		key := normalizeLineForCompare(line)

		if strings.TrimSpace(key) == "" {
			continue
		}

		frequency[key] = append(
			frequency[key],
			i,
		)
	}

	type candidate struct {
		start int
		sim   float64
	}

	// Несколько anchor-строк могут указывать на один
	// и тот же StartLine. Это один кандидат.
	candidatesByStart := make(map[int]candidate)

	for rel, searchLine := range searchLines {
		key := normalizeLineForCompare(searchLine)

		if strings.TrimSpace(key) == "" {
			continue
		}

		positions := frequency[key]

		// Для REBASE anchor обязан быть уникальным.
		if len(positions) != 1 {
			continue
		}

		start := positions[0] - rel

		if start < 0 ||
			start+len(searchLines) > len(origLines) {
			continue
		}

		actual := origLines[start : start+len(searchLines)]

		sim := lineSimilarity(
			actual,
			searchLines,
		)

		// КРИТИЧЕСКОЕ ПРАВИЛО:
		//
		// REBASE принимает только полностью совпадающий
		// после нормализации блок.
		//
		// Если sim = 0.67, 0.75, 0.90 и т.п.,
		// это уже НЕ REBASE.
		//
		// Такой случай должен попасть в FUZZY,
		// где Balanced потребует confidence >= 0.82.
		if sim < 1.0 {
			continue
		}

		existing, exists :=
			candidatesByStart[start]

		if !exists ||
			sim > existing.sim {

			candidatesByStart[start] =
				candidate{
					start: start,
					sim:   sim,
				}
		}
	}

	if len(candidatesByStart) == 0 {
		return nil
	}

	candidates :=
		make(
			[]candidate,
			0,
			len(candidatesByStart),
		)

	for _, c := range candidatesByStart {
		candidates = append(
			candidates,
			c,
		)
	}

	sort.Slice(
		candidates,
		func(i, j int) bool {
			if candidates[i].sim != candidates[j].sim {
				return candidates[i].sim >
					candidates[j].sim
			}

			return candidates[i].start <
				candidates[j].start
		},
	)

	best := candidates[0]

	secondBest := 0.0

	if len(candidates) > 1 {
		secondBest = candidates[1].sim

		// Два действительно разных места с одинаково
		// хорошим совпадением означают неоднозначность.
		if best.sim-secondBest < 0.10 {
			return nil
		}
	}

	return &fuzzyMatch{
		StartLine:  best.start,
		Similarity: best.sim,
		SecondBest: secondBest,
	}
}
