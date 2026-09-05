package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gogitor/internal/domain"
)

type goModImportContext struct {
	HasModule      bool
	ModulePaths    map[string]bool
	RequiredModule map[string]bool
}

// loadGoModImportContext читает исходный go.mod и дополнительно
// собирает module/require значения из изменений, которые LLM
// одновременно предлагает внести в go.mod.
//
// Это позволяет легально добавить новую зависимость в той же
// операции, в которой появляется новый import.
func loadGoModImportContext(
	root string,
	changes []domain.FileChange,
) (goModImportContext, error) {
	ctx := goModImportContext{
		ModulePaths:    make(map[string]bool),
		RequiredModule: make(map[string]bool),
	}

	goModPath := filepath.Join(root, "go.mod")

	data, err := os.ReadFile(goModPath)
	if err == nil {
		parseGoModImportText(
			string(data),
			&ctx,
		)
	} else if !os.IsNotExist(err) {
		return ctx, fmt.Errorf(
			"cannot read go.mod: %w",
			err,
		)
	}

	// Учитываем go.mod, который LLM создаёт или изменяет
	// в том же наборе FileChange.
	for _, ch := range changes {
		if filepath.ToSlash(
			filepath.Clean(ch.Path),
		) != "go.mod" {
			continue
		}

		if strings.TrimSpace(ch.Content) != "" {
			parseGoModImportText(
				ch.Content,
				&ctx,
			)
		}

		for _, p := range ch.Patches {
			parseGoModImportText(
				p.Search,
				&ctx,
			)
			parseGoModImportText(
				p.Replace,
				&ctx,
			)
		}
	}

	// Если go.mod вообще не существует и набор изменений
	// не содержит module directive, guard ничего не делает.
	if !ctx.HasModule {
		return ctx, nil
	}

	return ctx, nil
}

func parseGoModImportText(
	content string,
	ctx *goModImportContext,
) {
	lines := strings.Split(
		strings.ReplaceAll(
			content,
			"\r\n",
			"\n",
		),
		"\n",
	)

	inRequireBlock := false

	for _, raw := range lines {
		line := strings.TrimSpace(raw)

		if line == "" ||
			strings.HasPrefix(line, "//") {
			continue
		}

		if strings.HasPrefix(
			line,
			"module ",
		) {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				modulePath := strings.TrimSpace(
					fields[1],
				)

				if modulePath != "" {
					ctx.HasModule = true
					ctx.ModulePaths[modulePath] = true
				}
			}

			continue
		}

		if strings.HasPrefix(
			line,
			"require ",
		) {
			rest := strings.TrimSpace(
				strings.TrimPrefix(
					line,
					"require",
				),
			)

			if rest == "(" {
				inRequireBlock = true
				continue
			}

			fields := strings.Fields(rest)
			if len(fields) >= 1 &&
				fields[0] != "" {

				ctx.RequiredModule[
					fields[0],
				] = true
			}

			continue
		}

		if inRequireBlock {
			if line == ")" {
				inRequireBlock = false
				continue
			}

			fields := strings.Fields(line)
			if len(fields) >= 1 &&
				fields[0] != "" {

				ctx.RequiredModule[
					fields[0],
				] = true
			}
		}
	}
}

func addedGoImports(
	before,
	after string,
) ([]string, error) {
	beforeImports, err := goImports(before)
	if err != nil {
		return nil, fmt.Errorf(
			"parse before imports: %w",
			err,
		)
	}

	afterImports, err := goImports(after)
	if err != nil {
		return nil, fmt.Errorf(
			"parse after imports: %w",
			err,
		)
	}

	beforeSet := make(map[string]bool, len(beforeImports))
	for _, imp := range beforeImports {
		beforeSet[imp] = true
	}

	var added []string

	for _, imp := range afterImports {
		if !beforeSet[imp] {
			added = append(
				added,
				imp,
			)
		}
	}

	return added, nil
}

func moduleImportAllowed(
	importPath string,
	ctx goModImportContext,
) bool {
	importPath = strings.TrimSpace(importPath)

	if importPath == "" {
		return true
	}

	// Relative imports недопустимы в module mode.
	if strings.HasPrefix(importPath, "./") ||
		strings.HasPrefix(importPath, "../") ||
		strings.HasPrefix(importPath, "/") {
		return false
	}

	// Bare internal/... никогда не является корректным
	// импортом целевого модуля.
	if strings.HasPrefix(
		importPath,
		"internal/",
	) {
		return false
	}

	// Специальные стандартные пакеты.
	switch importPath {
	case "C", "builtin", "unsafe":
		return true
	}

	// Стандартная библиотека определяется по GOROOT,
	// а не по эвристике "в имени нет точки".
	//
	// Это важно для пакетов вроде net/http.
	if isStdlibImport(importPath) {
		return true
	}

	// Локальный пакет проекта.
	for modulePath := range ctx.ModulePaths {
		if importPath == modulePath ||
			strings.HasPrefix(
				importPath,
				modulePath+"/",
			) {
			return true
		}
	}

	// Уже объявленная или одновременно добавляемая
	// внешняя зависимость.
	for modulePath := range ctx.RequiredModule {
		if importPath == modulePath ||
			strings.HasPrefix(
				importPath,
				modulePath+"/",
			) {
			return true
		}
	}

	return false
}

func isStdlibImport(importPath string) bool {
	if strings.TrimSpace(importPath) == "" {
		return false
	}

	// Не разрешаем внутренние import paths GOROOT:
	// это не делает их допустимыми для пользовательского проекта.
	if strings.HasPrefix(
		importPath,
		"internal/",
	) {
		return false
	}

	goroot := runtime.GOROOT()
	if goroot == "" {
		return false
	}

	full := filepath.Join(
		goroot,
		"src",
		filepath.FromSlash(importPath),
	)

	info, err := os.Stat(full)
	if err != nil {
		return false
	}

	return info.IsDir()
}

// validateModuleImportGuard проверяет только НОВЫЕ imports.
//
// Уже существующий сомнительный import не блокируется:
// guard не должен превращаться в проверку всего legacy-кода.
// Он контролирует именно изменения, внесённые текущим patch.
func validateModuleImportGuard(
	before,
	after string,
	patches []domain.Patch,
	path string,
	modCtx goModImportContext,
) error {
	if len(patches) == 0 ||
		!isGoPath(path) ||
		!modCtx.HasModule {
		return nil
	}

	added, err := addedGoImports(
		before,
		after,
	)
	if err != nil {
		return fmt.Errorf(
			"module import guard %s: %w",
			path,
			err,
		)
	}

	for _, importPath := range added {
		if moduleImportAllowed(
			importPath,
			modCtx,
		) {
			continue
		}

		modulePath := ""
		for candidate := range modCtx.ModulePaths {
			modulePath = candidate
			break
		}

		return domain.NewPatchError(
			domain.PatchErrorModuleImportMismatch,
			fmt.Sprintf(
				`new import %q in %q does not match project module %q and is not a declared dependency`,
				importPath,
				path,
				modulePath,
			),
		)
	}

	return nil
}