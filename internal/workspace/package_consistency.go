package workspace

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"gogitor/internal/domain"
	"gogitor/internal/security"
)

// validateGoPackageConsistency проверяет, что ни один Go-файл из changes
// не создаёт конфликт package с существующими Go-файлами в той же директории.
//
// Go не допускает двух разных package в одной директории (за исключением
// _test.go, объявляющего package X_test). Если LLM создаёт fetcher.go с
// package weather рядом с main.go с package main, go build детерминированно
// падает с "found packages ...". Такой патч отклоняется ДО компиляции.
//
// dir — корень проекта (или песочницы), из которого читаются существующие файлы.
//
// Проверяются только полные перезаписи и новые файлы (ch.Patches == nil).
// Патч обычно не меняет package clause, и восстанавливать результат патча
// только ради этого pre-check не имеет смысла.
func validateGoPackageConsistency(
	dir string,
	changes []domain.FileChange,
) error {
	type claim struct {
		Path    string
		Package string
	}

	byDir := make(map[string][]claim)
	claimed := make(map[string]map[string]bool)

	for _, ch := range changes {
		if !isGoPath(ch.Path) || len(ch.Patches) > 0 {
			continue
		}

		pkg, err := extractPackageName(ch.Content)
		if err != nil || pkg == "" {
			continue
		}

		d := filepath.ToSlash(filepath.Dir(filepath.Clean(ch.Path)))
		if d == "" || d == "." {
			d = "."
		}

		byDir[d] = append(byDir[d], claim{
			Path:    ch.Path,
			Package: pkg,
		})

		if claimed[d] == nil {
			claimed[d] = make(map[string]bool)
		}
		claimed[d][filepath.Base(ch.Path)] = true
	}

	for dirPath, claims := range byDir {
		existingPkg, err := existingPackageInDir(dir, dirPath, claimed[dirPath])
		if err != nil {
			return err
		}
		if existingPkg == "" {
			continue
		}

		for _, c := range claims {
			base := filepath.Base(c.Path)
			if goPackageNamesCompatible(existingPkg, c.Package, base) {
				continue
			}

			return domain.NewPatchError(
				domain.PatchErrorGoPackageMismatch,
				fmt.Sprintf(
					"package mismatch in directory %q: existing files declare package %q, but %q declares package %q; "+
						"either move %q into a dedicated subdirectory with its own package, or change its package clause to %q",
					dirPath,
					existingPkg,
					c.Path,
					c.Package,
					filepath.Base(c.Path),
					existingPkg,
				),
			)
		}
	}

	return nil
}

// existingPackageInDir возвращает имя package из первого .go-файла
// в dirPath, который не указан в excludedBases.
//
// Файлы _test.go с package X_test не являются носителями основного
// имени пакета и пропускаются.
func existingPackageInDir(
	root, dirPath string,
	excludedBases map[string]bool,
) (string, error) {
	fullDir, err := security.SafeJoin(root, dirPath)
	if err != nil {
		return "", err
	}

	entries, err := os.ReadDir(fullDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		if excludedBases[name] {
			continue
		}

		pkg, err := packageNameFromPath(filepath.Join(fullDir, name))
		if err != nil || pkg == "" {
			continue
		}

		if strings.HasSuffix(name, "_test.go") &&
			strings.HasSuffix(pkg, "_test") {
			continue
		}

		return pkg, nil
	}

	return "", nil
}

func packageNameFromPath(path string) (string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.PackageClauseOnly)
	if err != nil {
		return "", err
	}
	if file.Name == nil {
		return "", fmt.Errorf("no package clause in %s", path)
	}
	return file.Name.Name, nil
}

func extractPackageName(content string) (string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(
		fset,
		"__package_check__.go",
		[]byte(content),
		parser.PackageClauseOnly,
	)
	if err != nil {
		return "", err
	}
	if file.Name == nil {
		return "", fmt.Errorf("no package clause")
	}
	return file.Name.Name, nil
}

func goPackageNamesCompatible(existing, candidate, filename string) bool {
	if existing == candidate {
		return true
	}
	if strings.HasSuffix(filename, "_test.go") &&
		candidate == existing+"_test" {
		return true
	}
	return false
}