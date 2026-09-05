package workspace

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gogitor/internal/domain"
	"gogitor/internal/security"
)

type taskEffectivenessDecision string

const (
	taskEffectivenessOK taskEffectivenessDecision = "OK"

	taskEffectivenessReject taskEffectivenessDecision = "REJECT"

	taskEffectivenessSkip taskEffectivenessDecision = "SKIP"

	taskEffectivenessAlreadySatisfied taskEffectivenessDecision = "ALREADY_SATISFIED"
)

type taskEffectivenessContract struct {
	kind   string
	source string
	target string
}

func (c taskEffectivenessContract) String() string {
	switch c.kind {
	case "extract":
		return "extract:" + c.source + "->" + c.target

	case "rename":
		return "rename:" + c.source + "->" + c.target

	case "add":
		return "add:" + c.target

	default:
		return c.kind
	}
}

type taskEffectivenessResult struct {
	Decision taskEffectivenessDecision
	Contract string
	Reason   string
}

// extractTaskEffectivenessContract извлекает только те требования,
// которые можно безопасно проверить детерминированно по AST.
//
// Если формулировка задачи неоднозначна, возвращается false,
// и TASK_EFFECTIVENESS не блокирует patch.
func extractTaskEffectivenessContract(
	task string,
) (taskEffectivenessContract, bool) {
	task = strings.TrimSpace(task)
	if task == "" {
		return taskEffectivenessContract{}, false
	}

	// ------------------------------------------------------------
	// EXTRACT: Russian
	//
	// Примеры:
	//   "Вынеси регистрацию ... из main() в отдельную функцию registerRoutes()."
	//   "Вынести ... из main() в новую функцию registerRoutes()."
	//
	// Не используем \b перед кириллическими словами:
	// Go regexp трактует \b как ASCII word boundary.
	// ------------------------------------------------------------
	reExtractRU := regexp.MustCompile(
		`(?is)из\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(\s*\)[\s\S]*?в\s+(?:[^\s,()]+\s+){0,4}([A-Za-z_][A-Za-z0-9_]*)\s*\(`,
	)

	if m := reExtractRU.FindStringSubmatch(task); len(m) == 3 {
		return taskEffectivenessContract{
			kind:   "extract",
			source: m[1],
			target: m[2],
		}, true
	}

	// ------------------------------------------------------------
	// EXTRACT: English
	//
	// Example:
	//   "Extract routes from main() into a separate registerRoutes() function."
	// ------------------------------------------------------------
	reExtractEN := regexp.MustCompile(
		`(?is)\bfrom\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(\s*\)[\s\S]*?\binto\s+(?:[A-Za-z0-9_-]+\s+){0,4}([A-Za-z_][A-Za-z0-9_]*)\s*\(`,
	)

	if m := reExtractEN.FindStringSubmatch(task); len(m) == 3 {
		return taskEffectivenessContract{
			kind:   "extract",
			source: m[1],
			target: m[2],
		}, true
	}

	// ------------------------------------------------------------
	// RENAME
	// ------------------------------------------------------------
	reRename := regexp.MustCompile(
		`(?is)(?:rename|переименуй|переименовать)\s+(?:function\s+|функци\w*\s+)?([A-Za-z_][A-Za-z0-9_]*)\s+(?:to|в)\s+([A-Za-z_][A-Za-z0-9_]*)`,
	)

	if m := reRename.FindStringSubmatch(task); len(m) == 3 {
		return taskEffectivenessContract{
			kind:   "rename",
			source: m[1],
			target: m[2],
		}, true
	}

	// ------------------------------------------------------------
	// ADD / CREATE FUNCTION
	// ------------------------------------------------------------
	reAdd := regexp.MustCompile(
		`(?is)(?:add|create|добавь|создай|добавить|создать)\s+(?:a\s+)?(?:function|функци\w*)\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`,
	)

	if m := reAdd.FindStringSubmatch(task); len(m) == 2 {
		return taskEffectivenessContract{
			kind:   "add",
			target: m[1],
		}, true
	}

	return taskEffectivenessContract{}, false
}

func taskFunctionKey(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}

	return declarationKey(name)
}

func taskDeclarationExists(
	content,
	name string,
) bool {
	if strings.TrimSpace(name) == "" {
		return false
	}

	decls, err := goDeclarationFingerprints(content)
	if err != nil {
		return false
	}

	key := taskFunctionKey(name)
	if key == "" {
		return false
	}

	_, ok := decls[key]
	return ok
}

func taskDeclarationChanged(
	before,
	after,
	name string,
) bool {
	key := taskFunctionKey(name)
	if key == "" {
		return false
	}

	beforeDecls, err := goDeclarationFingerprints(before)
	if err != nil {
		return false
	}

	afterDecls, err := goDeclarationFingerprints(after)
	if err != nil {
		return false
	}

	return beforeDecls[key] !=
		afterDecls[key]
}

func taskFunctionCalls(
	content,
	functionName,
	targetName string,
) bool {
	if strings.TrimSpace(functionName) == "" ||
		strings.TrimSpace(targetName) == "" {
		return false
	}

	calls, err := goDeclarationCalls(content)
	if err != nil {
		return false
	}

	key := taskFunctionKey(functionName)
	if key == "" {
		return false
	}

	return calls[key][targetName]
}

func evaluateTaskEffectiveness(
	task string,
	before,
	after string,
) taskEffectivenessResult {
	contract, ok :=
		extractTaskEffectivenessContract(task)

	if !ok {
		return taskEffectivenessResult{
			Decision: taskEffectivenessSkip,
			Reason:   "no deterministic task contract recognized",
		}
	}

	switch contract.kind {
	case "extract":
		return evaluateExtractContract(
			contract,
			before,
			after,
		)

	case "rename":
		return evaluateRenameContract(
			contract,
			before,
			after,
		)

	case "add":
		return evaluateAddContract(
			contract,
			before,
			after,
		)

	default:
		return taskEffectivenessResult{
			Decision: taskEffectivenessSkip,
			Contract: contract.String(),
			Reason:   "unsupported deterministic task contract",
		}
	}
}

func evaluateExtractContract(
	contract taskEffectivenessContract,
	before,
	after string,
) taskEffectivenessResult {
	sourceBefore :=
		taskDeclarationExists(
			before,
			contract.source,
		)

	sourceAfter :=
		taskDeclarationExists(
			after,
			contract.source,
		)

	targetBefore :=
		taskDeclarationExists(
			before,
			contract.target,
		)

	targetAfter :=
		taskDeclarationExists(
			after,
			contract.target,
		)

	afterCallsTarget :=
		taskFunctionCalls(
			after,
			contract.source,
			contract.target,
		)

	beforeCallsTarget :=
		taskFunctionCalls(
			before,
			contract.source,
			contract.target,
		)

	// Если задача уже была выполнена до patch:
	//
	//   main() -> registerRoutes()
	//   registerRoutes() существует
	//
	// то новый patch не должен изменять код только ради
	// формального "успеха".
	if sourceBefore &&
		targetBefore &&
		beforeCallsTarget {

		return taskEffectivenessResult{
			Decision: taskEffectivenessAlreadySatisfied,
			Contract: contract.String(),
			Reason: fmt.Sprintf(
				"task already satisfied before patch: %s() calls %s()",
				contract.source,
				contract.target,
			),
		}
	}

	if !sourceAfter {
		return taskEffectivenessResult{
			Decision: taskEffectivenessReject,
			Contract: contract.String(),
			Reason: fmt.Sprintf(
				"required source function %q is missing after patch",
				contract.source,
			),
		}
	}

	if !targetAfter {
		return taskEffectivenessResult{
			Decision: taskEffectivenessReject,
			Contract: contract.String(),
			Reason: fmt.Sprintf(
				"required extracted function %q is missing after patch",
				contract.target,
			),
		}
	}

	if !afterCallsTarget {
		return taskEffectivenessResult{
			Decision: taskEffectivenessReject,
			Contract: contract.String(),
			Reason: fmt.Sprintf(
				"%s() does not call required extracted function %s()",
				contract.source,
				contract.target,
			),
		}
	}

	sourceChanged :=
		taskDeclarationChanged(
			before,
			after,
			contract.source,
		)

	targetChanged :=
		taskDeclarationChanged(
			before,
			after,
			contract.target,
		)

	// Мы не принимаем patch только потому,
	// что файл изменился. Хотя бы одна из
	// относящихся к контракту деклараций должна
	// реально измениться.
	if !sourceChanged &&
		!targetChanged &&
		beforeCallsTarget == afterCallsTarget {
		return taskEffectivenessResult{
			Decision: taskEffectivenessReject,
			Contract: contract.String(),
			Reason:   "required extraction produced no semantic declaration change",
		}
	}

	// Для настоящего extract:
	// до patch source не должен уже содержать
	// нужную связь source -> target.
	if !beforeCallsTarget &&
		afterCallsTarget {
		return taskEffectivenessResult{
			Decision: taskEffectivenessOK,
			Contract: contract.String(),
			Reason: fmt.Sprintf(
				"%s() now calls %s()",
				contract.source,
				contract.target,
			),
		}
	}

	// Если target появился именно в результате patch,
	// это также валидный extract.
	if !targetBefore &&
		targetAfter &&
		sourceChanged {
		return taskEffectivenessResult{
			Decision: taskEffectivenessOK,
			Contract: contract.String(),
			Reason: fmt.Sprintf(
				"new extracted function %s() is present and called",
				contract.target,
			),
		}
	}

	return taskEffectivenessResult{
		Decision: taskEffectivenessReject,
		Contract: contract.String(),
		Reason:   "required extraction postcondition was not established",
	}
}

func evaluateRenameContract(
	contract taskEffectivenessContract,
	before,
	after string,
) taskEffectivenessResult {
	oldBefore :=
		taskDeclarationExists(
			before,
			contract.source,
		)

	newBefore :=
		taskDeclarationExists(
			before,
			contract.target,
		)

	oldAfter :=
		taskDeclarationExists(
			after,
			contract.source,
		)

	newAfter :=
		taskDeclarationExists(
			after,
			contract.target,
		)

	if !oldBefore && newBefore {
		return taskEffectivenessResult{
			Decision: taskEffectivenessAlreadySatisfied,
			Contract: contract.String(),
			Reason: fmt.Sprintf(
				"rename already satisfied: %s() is absent and %s() exists",
				contract.source,
				contract.target,
			),
		}
	}

	if !oldAfter && newAfter {
		return taskEffectivenessResult{
			Decision: taskEffectivenessOK,
			Contract: contract.String(),
			Reason: fmt.Sprintf(
				"function renamed from %s() to %s()",
				contract.source,
				contract.target,
			),
		}
	}

	return taskEffectivenessResult{
		Decision: taskEffectivenessReject,
		Contract: contract.String(),
		Reason: fmt.Sprintf(
			"rename postcondition not satisfied: expected %s() -> %s()",
			contract.source,
			contract.target,
		),
	}
}

func evaluateAddContract(
	contract taskEffectivenessContract,
	before,
	after string,
) taskEffectivenessResult {
	beforeExists :=
		taskDeclarationExists(
			before,
			contract.target,
		)

	afterExists :=
		taskDeclarationExists(
			after,
			contract.target,
		)

	if beforeExists {
		return taskEffectivenessResult{
			Decision: taskEffectivenessAlreadySatisfied,
			Contract: contract.String(),
			Reason: fmt.Sprintf(
				"function %s() already exists before patch",
				contract.target,
			),
		}
	}

	if afterExists {
		return taskEffectivenessResult{
			Decision: taskEffectivenessOK,
			Contract: contract.String(),
			Reason: fmt.Sprintf(
				"required function %s() was added",
				contract.target,
			),
		}
	}

	return taskEffectivenessResult{
		Decision: taskEffectivenessReject,
		Contract: contract.String(),
		Reason: fmt.Sprintf(
			"required function %s() is missing after patch",
			contract.target,
		),
	}
}

// ValidateTaskEffectiveness проверяет только существующие Go-файлы,
// изменяемые patch-режимом.
//
// Проверка намеренно консервативна:
// если задача не распознана однозначно, возвращается SKIP,
// а не REJECT.
func (w *Workspace) ValidateTaskEffectiveness(
	task,
	sandbox string,
	changes []domain.FileChange,
) (taskEffectivenessResult, error) {
	contract, ok :=
		extractTaskEffectivenessContract(task)

	if !ok {
		return taskEffectivenessResult{
			Decision: taskEffectivenessSkip,
			Reason:   "no deterministic task contract recognized",
		}, nil
	}

	foundRelevantFile := false

	for _, ch := range changes {
		if len(ch.Patches) == 0 ||
			!isGoPath(ch.Path) {
			continue
		}

		beforePath, err :=
			security.SafeJoin(
				w.Root,
				ch.Path,
			)
		if err != nil {
			return taskEffectivenessResult{},
				fmt.Errorf(
					"task effectiveness %s: resolve source path: %w",
					ch.Path,
					err,
				)
		}

		afterPath, err :=
			security.SafeJoin(
				sandbox,
				ch.Path,
			)
		if err != nil {
			return taskEffectivenessResult{},
				fmt.Errorf(
					"task effectiveness %s: resolve sandbox path: %w",
					ch.Path,
					err,
				)
		}

		beforeData, err :=
			os.ReadFile(beforePath)
		if err != nil {
			return taskEffectivenessResult{},
				fmt.Errorf(
					"task effectiveness %s: read source: %w",
					ch.Path,
					err,
				)
		}

		afterData, err :=
			os.ReadFile(afterPath)
		if err != nil {
			return taskEffectivenessResult{},
				fmt.Errorf(
					"task effectiveness %s: read sandbox: %w",
					ch.Path,
					err,
				)
		}

		before := string(beforeData)
		after := string(afterData)

		// Файл считается релевантным, если в нём есть
		// исходный или целевой Symbol.
		switch contract.kind {
		case "extract":
			foundRelevantFile =
				taskDeclarationExists(
					before,
					contract.source,
				) ||
					taskDeclarationExists(
						before,
						contract.target,
					) ||
					taskDeclarationExists(
						after,
						contract.source,
					) ||
					taskDeclarationExists(
						after,
						contract.target,
					)

		case "rename":
			foundRelevantFile =
				taskDeclarationExists(
					before,
					contract.source,
				) ||
					taskDeclarationExists(
						before,
						contract.target,
					) ||
					taskDeclarationExists(
						after,
						contract.source,
					) ||
					taskDeclarationExists(
						after,
						contract.target,
					)

		case "add":
			foundRelevantFile =
				taskDeclarationExists(
					before,
					contract.target,
				) ||
					taskDeclarationExists(
						after,
						contract.target,
					)
		}

		if !foundRelevantFile {
			continue
		}

		result :=
			evaluateTaskEffectiveness(
				task,
				before,
				after,
			)

		result.Contract =
			contract.String()

		return result, nil
	}

	// Если задача явно требует изменения функции,
	// но patch изменил совсем другой Go-файл,
	// нельзя считать это успешным.
	return taskEffectivenessResult{
		Decision: taskEffectivenessReject,
		Contract: contract.String(),
		Reason:   "task contract target was not found in any patched Go file",
	}, nil
}
