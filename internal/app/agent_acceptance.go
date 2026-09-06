package app

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gogitor/internal/domain"
)

type agentAcceptanceBaseline struct {
	TestFiles map[string]map[string]bool
	Routes    map[string]bool
}

type agentAcceptanceProfile struct {
	PreserveTests         bool
	PreserveRoutes        bool
	RequireServiceLayer   bool
	RequireServiceTests   bool
	ArchitectureTask      bool
	RequireVerifierChecks bool
}

type agentAcceptanceResult struct {
	Passed   bool
	Blocking []string
	Warnings []string
	Evidence []string
	FixTask  string
}

func (r agentAcceptanceResult) String() string {
	var b strings.Builder

	b.WriteString("DETERMINISTIC ACCEPTANCE CHECKS:\n")

	if len(r.Blocking) == 0 &&
		len(r.Evidence) == 0 {
		b.WriteString("- no blocking deterministic failures\n")
	} else {
		for _, item := range r.Evidence {
			b.WriteString("- PASS: ")
			b.WriteString(item)
			b.WriteByte('\n')
		}
		for _, item := range r.Blocking {
			b.WriteString("- FAIL: ")
			b.WriteString(item)
			b.WriteByte('\n')
		}
	}

	for _, item := range r.Warnings {
		b.WriteString("- WARNING: ")
		b.WriteString(item)
		b.WriteByte('\n')
	}

	if r.FixTask != "" {
		b.WriteString("\nREQUIRED FILE-ONLY FIX:\n")
		b.WriteString(r.FixTask)
		b.WriteByte('\n')
	}

	return strings.TrimSpace(b.String())
}

func inferAgentAcceptanceProfile(
	task string,
	plan *fullPlan,
) agentAcceptanceProfile {
	var b strings.Builder

	b.WriteString(task)
	b.WriteByte('\n')

	if plan != nil {
		b.WriteString(plan.Goal)
		b.WriteByte('\n')

		for _, item := range plan.Acceptance {
			b.WriteString(item)
			b.WriteByte('\n')
		}

		for _, sub := range plan.Subtasks {
			b.WriteString(sub.Task)
			b.WriteByte('\n')

			for _, item := range sub.Acceptance {
				b.WriteString(item)
				b.WriteByte('\n')
			}
		}
	}

	lower := strings.ToLower(
		b.String(),
	)

	profile := agentAcceptanceProfile{}

	profile.ArchitectureTask =
		containsAny(
			lower,
			[]string{
				"refactor",
				"refactoring",
				"архитект",
				"рефактор",
				"service layer",
				"service-слой",
				"business logic",
				"бизнес-логик",
				"handlers should",
				"handlers must",
				"обработчики должны",
				"repository should",
				"repository must",
				"репозиторий должен",
			},
		)

	profile.PreserveTests =
		isPreservationRequest(
			lower,
			[]string{
				"test",
				"тест",
				"tests",
				"тесты",
				"тестов",
				"тесты",
			},
		)

	profile.PreserveRoutes =
		isPreservationRequest(
			lower,
			[]string{
				"endpoint",
				"endpoints",
				"route",
				"routes",
				"api",
				"маршрут",
				"маршруты",
				"эндпоинт",
				"эндпоинты",
			},
		)

	hasServiceKeyword :=
		containsAny(
			lower,
			[]string{
				"service layer",
				"service-слой",
				"слой service",
				"бизнес-логика ... service",
				"business logic ... service",
			},
		)

	profile.RequireServiceLayer =
		profile.ArchitectureTask &&
			hasServiceKeyword

	profile.RequireServiceTests =
		profile.RequireServiceLayer &&
			containsAny(
				lower,
				[]string{
					"test",
					"tests",
					"тест",
					"тесты",
				},
			)

	profile.RequireVerifierChecks =
		profile.ArchitectureTask ||
			profile.PreserveTests ||
			profile.PreserveRoutes

	return profile
}

func TestIsPreservationRequest(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		target []string
		want   bool
	}{
		{
			name:   "Russian save tests",
			text:   "сохрани все endpoint и тесты",
			target: []string{"тест", "тесты"},
			want:   true,
		},
		{
			name:   "Russian do not remove tests",
			text:   "не удаляй существующие тесты",
			target: []string{"тест"},
			want:   true,
		},
		{
			name:   "English preserve tests",
			text:   "preserve all existing tests",
			target: []string{"test"},
			want:   true,
		},
		{
			name:   "English keep routes",
			text:   "keep all existing routes",
			target: []string{"route"},
			want:   true,
		},
		{
			name:   "Adding tests is not preservation",
			text:   "добавь новые тесты",
			target: []string{"тест"},
			want:   false,
		},
		{
			name:   "Adding endpoint is not preservation",
			text:   "добавь новый endpoint",
			target: []string{"endpoint"},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isPreservationRequest(
				tt.text,
				tt.target,
			)

			if got != tt.want {
				t.Fatalf(
					"isPreservationRequest() = %v, want %v",
					got,
					tt.want,
				)
			}
		})
	}
}

func isPreservationRequest(
	text string,
	targets []string,
) bool {
	preservationWords := []string{
		"preserve",
		"preserves",
		"preserving",
		"keep",
		"kept",
		"retain",
		"do not remove",
		"do not delete",
		"don't remove",
		"don't delete",

		"сохрани",
		"сохранить",
		"сохраняй",
		"сохранять",
		"оставь",
		"оставить",
		"не удаляй",
		"не удалить",
		"не удалять",
		"не ломай",
		"не ломать",
	}

	if len(text) == 0 {
		return false
	}

	hasPreserve := containsAny(
		text,
		preservationWords,
	)

	if !hasPreserve {
		return false
	}

	for _, target := range targets {
		target = strings.ToLower(
			strings.TrimSpace(target),
		)

		if target == "" {
			continue
		}

		if strings.Contains(text, target) {
			return true
		}
	}

	return false
}

func captureAgentAcceptanceBaseline(
	root string,
) (agentAcceptanceBaseline, error) {
	tests, err :=
		collectAgentTestInventory(root)

	if err != nil {
		return agentAcceptanceBaseline{}, err
	}

	routes, err :=
		collectAgentHTTPRoutes(root)

	if err != nil {
		return agentAcceptanceBaseline{}, err
	}

	return agentAcceptanceBaseline{
		TestFiles: tests,
		Routes:    routes,
	}, nil
}

func collectAgentTestInventory(
	root string,
) (map[string]map[string]bool, error) {
	result :=
		make(map[string]map[string]bool)

	err := filepath.Walk(
		root,
		func(
			path string,
			info os.FileInfo,
			err error,
		) error {
			if err != nil {
				return err
			}

			if info.IsDir() {
				if shouldSkipAgentAcceptanceDir(info.Name()) {
					return filepath.SkipDir
				}
				return nil
			}

			if !strings.HasSuffix(
				info.Name(),
				"_test.go",
			) {
				return nil
			}

			rel, err :=
				filepath.Rel(
					root,
					path,
				)

			if err != nil {
				return err
			}

			rel =
				filepath.ToSlash(rel)

			file, err :=
				parser.ParseFile(
					token.NewFileSet(),
					path,
					nil,
					0,
				)

			if err != nil {
				// Повреждённый исходник до начала Agent —
				// не повод ломать всю сессию.
				// Такой файл будет оставлен на LLM/verifier.
				return nil
			}

			names :=
				make(map[string]bool)

			for _, decl := range file.Decls {
				fn, ok :=
					decl.(*ast.FuncDecl)

				if !ok ||
					fn.Recv != nil ||
					fn.Name == nil {
					continue
				}

				name := fn.Name.Name

				if strings.HasPrefix(name, "Test") ||
					strings.HasPrefix(name, "Benchmark") ||
					strings.HasPrefix(name, "Fuzz") ||
					strings.HasPrefix(name, "Example") {

					names[name] = true
				}
			}

			result[rel] = names
			return nil
		},
	)

	if err != nil {
		return nil, err
	}

	return result, nil
}

func compareAgentTestInventory(
	before map[string]map[string]bool,
	root string,
) ([]string, error) {
	after, err :=
		collectAgentTestInventory(root)

	if err != nil {
		return nil, err
	}

	var missing []string

	var files []string

	for path := range before {

		files = append(
			files,
			path,
		)
	}

	sort.Strings(files)

	for _, path := range files {
		beforeTests :=
			before[path]

		afterTests, exists :=
			after[path]

		if !exists {
			missing = append(
				missing,
				fmt.Sprintf(
					"test file was removed: %s",
					path,
				),
			)
			continue
		}

		var names []string

		for name := range beforeTests {

			names = append(
				names,
				name,
			)
		}

		sort.Strings(names)

		for _, name := range names {
			if !afterTests[name] {
				missing = append(
					missing,
					fmt.Sprintf(
						"test function was removed: %s:%s",
						path,
						name,
					),
				)
			}
		}
	}

	return missing, nil
}

var agentHTTPRouteRE = regexp.MustCompile(
	`(?m)\b(?:HandleFunc|Handle)\s*\(\s*"([^"]+)"`,
)

func collectAgentHTTPRoutes(
	root string,
) (map[string]bool, error) {
	result := make(map[string]bool)

	err := filepath.Walk(
		root,
		func(
			path string,
			info os.FileInfo,
			err error,
		) error {
			if err != nil {
				return err
			}

			if info.IsDir() {
				if shouldSkipAgentAcceptanceDir(info.Name()) {
					return filepath.SkipDir
				}
				return nil
			}

			if !strings.HasSuffix(
				info.Name(),
				".go",
			) ||
				strings.HasSuffix(
					info.Name(),
					"_test.go",
				) {
				return nil
			}

			data, err :=
				os.ReadFile(path)

			if err != nil {
				return err
			}

			matches :=
				agentHTTPRouteRE.FindAllStringSubmatch(
					string(data),
					-1,
				)

			for _, m := range matches {
				if len(m) == 2 &&
					strings.TrimSpace(m[1]) != "" {

					result[m[1]] = true
				}
			}

			return nil
		},
	)

	if err != nil {
		return nil, err
	}

	return result, nil
}

func shouldSkipAgentAcceptanceDir(
	name string,
) bool {
	switch name {
	case ".git",
		".gogitor",
		"node_modules",
		"vendor":
		return true
	default:
		return strings.HasPrefix(name, ".")
	}
}

func findAgentServiceRoots(
	root string,
) []string {
	var result []string
	seen := make(map[string]bool)

	err := filepath.Walk(
		root,
		func(
			path string,
			info os.FileInfo,
			err error,
		) error {
			if err != nil {
				return nil
			}

			if info.IsDir() {
				if shouldSkipAgentAcceptanceDir(info.Name()) {
					return filepath.SkipDir
				}

				lower :=
					strings.ToLower(
						info.Name(),
					)

				if lower == "service" ||
					lower == "services" {

					hasGo := false

					_ = filepath.Walk(
						path,
						func(
							p string,
							i os.FileInfo,
							e error,
						) error {
							if e != nil {
								return nil
							}

							if !i.IsDir() &&
								strings.HasSuffix(
									i.Name(),
									".go",
								) {

								hasGo = true
								return filepath.SkipAll
							}

							return nil
						},
					)

					if hasGo &&
						!seen[path] {

						seen[path] = true
						result = append(
							result,
							path,
						)
					}
				}
			}

			return nil
		},
	)

	if err != nil {
		return nil
	}

	sort.Strings(result)
	return result
}

func serviceRootHasTests(
	root string,
) bool {
	found := false

	_ = filepath.Walk(
		root,
		func(
			path string,
			info os.FileInfo,
			err error,
		) error {
			if err != nil {
				return nil
			}

			if info.IsDir() {
				return nil
			}

			if strings.HasSuffix(
				info.Name(),
				"_test.go",
			) {
				found = true
				return filepath.SkipAll
			}

			return nil
		},
	)

	return found
}

func validateAgentAcceptance(
	root,
	task string,
	plan *fullPlan,
	baseline agentAcceptanceBaseline,
	final domain.Result,
) agentAcceptanceResult {
	profile :=
		inferAgentAcceptanceProfile(
			task,
			plan,
		)

	result :=
		agentAcceptanceResult{
			Passed: true,
		}

	if profile.PreserveTests {
		missing, err :=
			compareAgentTestInventory(
				baseline.TestFiles,
				root,
			)

		if err != nil {
			result.Blocking = append(
				result.Blocking,
				"cannot verify preservation of existing Go tests: "+
					err.Error(),
			)
		} else if len(missing) > 0 {
			result.Blocking =
				append(
					result.Blocking,
					missing...,
				)

			result.FixTask =
				"Restore all existing test files and test functions removed by the task. " +
					"Do not add unrelated tests or features. Preserve the current production implementation."
		} else {
			result.Evidence =
				append(
					result.Evidence,
					"all baseline test files and test functions are still present",
				)
		}

		for _, path := range final.FilesFullRewritten {

			if strings.HasSuffix(
				path,
				"_test.go",
			) {
				result.Warnings =
					append(
						result.Warnings,
						"existing test file was fully rewritten: "+path+
							"; test inventory was preserved, but review should inspect the rewrite",
					)
			}
		}
	}

	if profile.PreserveRoutes {
		currentRoutes, err :=
			collectAgentHTTPRoutes(root)

		if err != nil {
			result.Warnings =
				append(
					result.Warnings,
					"could not deterministically inspect HTTP routes: "+
						err.Error(),
				)
		} else if len(baseline.Routes) == 0 {
			result.Warnings =
				append(
					result.Warnings,
					"no conventional net/http route registrations were recognized; verifier must check endpoint preservation",
				)
		} else {
			var missing []string

			for route := range baseline.Routes {

				if !currentRoutes[route] {
					missing =
						append(
							missing,
							route,
						)
				}
			}

			sort.Strings(missing)

			if len(missing) > 0 {
				result.Blocking =
					append(
						result.Blocking,
						"existing HTTP routes disappeared: "+
							strings.Join(
								missing,
								", ",
							),
					)

				if result.FixTask == "" {
					result.FixTask =
						"Restore the missing existing HTTP routes without changing their external behavior. " +
							"Do not add unrelated endpoints."
				}
			} else {
				result.Evidence =
					append(
						result.Evidence,
						"recognized baseline HTTP routes are still present",
					)
			}
		}
	}

	if profile.RequireServiceLayer {
		serviceRoots :=
			findAgentServiceRoots(root)

		if len(serviceRoots) == 0 {
			result.Blocking =
				append(
					result.Blocking,
					"required service layer was not found",
				)

			if result.FixTask == "" {
				result.FixTask =
					"Create or restore the required service layer and move the business logic required by the original task into it. Keep HTTP handlers focused on HTTP concerns and preserve repository responsibilities."
			}
		} else {
			result.Evidence =
				append(
					result.Evidence,
					"service layer exists: "+
						strings.Join(
							serviceRoots,
							", ",
						),
				)

			if profile.RequireServiceTests {
				hasTests := false

				for _, serviceRoot := range serviceRoots {

					if serviceRootHasTests(
						serviceRoot,
					) {
						hasTests = true
						break
					}
				}

				if !hasTests {
					result.Blocking =
						append(
							result.Blocking,
							"service layer exists but contains no Go tests while the task explicitly requires preserving/updating tests",
						)

					if result.FixTask == "" {
						result.FixTask =
							"Add focused Go tests for the new service-layer business logic required by the original task. Do not remove existing tests."
					}
				} else {
					result.Evidence =
						append(
							result.Evidence,
							"service layer has Go tests",
						)
				}
			}
		}
	}

	if profile.ArchitectureTask {
		result.Warnings =
			append(
				result.Warnings,
				"architectural separation must be verified against CURRENT PROJECT SOURCE by the reviewer/verifier; deterministic checks only verify structural invariants",
			)
	}

	result.Passed =
		len(result.Blocking) == 0

	if len(result.Blocking) == 0 {
		result.FixTask = ""
	}

	_ = final

	return result
}

func requiresStructuredAgentVerification(
	task string,
) bool {
	profile :=
		inferAgentAcceptanceProfile(
			task,
			nil,
		)

	return profile.RequireVerifierChecks
}
