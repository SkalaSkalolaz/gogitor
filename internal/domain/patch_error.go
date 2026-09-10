package domain

import (
	"errors"
	"strings"
)

// PatchErrorCode идентифицирует тип ошибки,
// возникшей при генерации, валидации или применении patch.
type PatchErrorCode string

const (
	// PatchErrorDuplicateFileChange означает,
	// что LLM вернула несколько FileChange
	// для одного и того же файла.
	PatchErrorDuplicateFileChange PatchErrorCode = "duplicate_file_change"

	// PatchErrorStrictSymbolRequired означает,
	// что strict policy требует Symbol для SEARCH-блока.
	PatchErrorStrictSymbolRequired PatchErrorCode = "strict_symbol_required"

	// PatchErrorStrictSearchTooLarge означает,
	// что SEARCH-блок превышает strict-лимит.
	PatchErrorStrictSearchTooLarge PatchErrorCode = "strict_search_too_large"

	PatchErrorSymbolNotFound PatchErrorCode = "symbol_not_found"

	// PatchErrorSearchOutsideSymbol означает, что SEARCH существует
	// в исходнике, но находится в другой декларации относительно Symbol.
	PatchErrorSearchOutsideSymbol PatchErrorCode = "search_outside_symbol"

	// PatchErrorSearchNotFoundInsideSymbol означает, что SEARCH
	// не удалось безопасно найти внутри указанного Symbol,
	// но принадлежность к другой декларации не доказана.
	PatchErrorSearchNotFoundInsideSymbol PatchErrorCode = "search_not_found_inside_symbol"

	// PatchErrorSearchCrossesSymbolBoundary означает, что SEARCH
	// пересекает границы двух или более Go declarations.
	PatchErrorSearchCrossesSymbolBoundary PatchErrorCode = "search_crosses_symbol_boundary"

	// PatchErrorAmbiguousSearch означает, что SEARCH имеет
	// несколько допустимых совпадений.
	PatchErrorAmbiguousSearch PatchErrorCode = "ambiguous_search"

	// PatchErrorRepairSymbolDrift означает, что repair попытался
	// изменить зафиксированный Symbol исходного rejected patch.
	PatchErrorRepairSymbolDrift PatchErrorCode = "repair_symbol_drift"

	// PatchErrorRepairFileDrift означает, что repair попытался
	// убрать исходный целевой файл из rejected patch.
	PatchErrorRepairFileDrift PatchErrorCode = "repair_file_drift"

	// PatchErrorRepairProtocolDrift означает, что во время
	// patch-repair LLM вернула full-file change вместо patch.
	PatchErrorRepairProtocolDrift PatchErrorCode = "repair_protocol_drift"

	// PatchErrorSourceChanged означает, что source hash устарел.
	PatchErrorSourceChanged PatchErrorCode = "source_changed_since_patch_generation"

	// PatchErrorStaleSymbol означает, что fingerprint Symbol устарел.
	PatchErrorStaleSymbol PatchErrorCode = "stale_symbol"

	// PatchErrorNoOpPatch означает, что SEARCH/REPLACE
	// формально сопоставились, но итоговый файл фактически
	// не изменился.
	PatchErrorNoOpPatch PatchErrorCode = "no_op_patch"

	// PatchErrorModuleImportMismatch означает,
	// что новый Go import не соответствует module path
	// проекта и не является объявленной зависимостью.
	PatchErrorModuleImportMismatch PatchErrorCode = "module_import_mismatch"

	// PatchErrorTaskEffectiveness означает, что patch формально
	// применился, но ожидаемый результат задачи не был достигнут.
	PatchErrorTaskEffectiveness PatchErrorCode = "task_effectiveness_failed"
	PatchErrorSemanticScope     PatchErrorCode = "semantic_scope"
	PatchErrorPublicAPIGuard    PatchErrorCode = "public_api_guard"
	PatchErrorImportGuard       PatchErrorCode = "import_guard"
	PatchErrorGoModGuard        PatchErrorCode = "gomod_guard"
	PatchErrorPatchTooLarge     PatchErrorCode = "patch_too_large"
)

// PatchError — структурированная ошибка patch pipeline.
//
// Error() сохраняет человекочитаемое сообщение,
// но одновременно содержит стабильный машинный код.
type PatchError struct {
	Code    PatchErrorCode
	Message string
}

func (e *PatchError) Error() string {
	if e == nil {
		return ""
	}

	if e.Code == "" {
		return e.Message
	}

	return "patch_error_code=" +
		string(e.Code) +
		": " +
		e.Message
}

// NewPatchError создаёт структурированную patch error.
func NewPatchError(
	code PatchErrorCode,
	message string,
) error {
	return &PatchError{
		Code:    code,
		Message: message,
	}
}

// PatchErrorCodeFromError возвращает код из typed PatchError.
//
// Используется, когда ошибка ещё представлена как error.
func PatchErrorCodeFromError(
	err error,
) PatchErrorCode {
	if err == nil {
		return ""
	}

	var patchErr *PatchError

	if errors.As(err, &patchErr) &&
		patchErr != nil {
		return patchErr.Code
	}

	return PatchErrorCodeFromText(
		err.Error(),
	)
}

// PatchErrorCodeFromText извлекает код из уже
// сформированного текста ошибки.
func PatchErrorCodeFromText(
	text string,
) PatchErrorCode {
	const prefix = "patch_error_code="

	idx := strings.Index(
		text,
		prefix,
	)

	if idx >= 0 {
		value := text[idx+len(prefix):]

		if colon := strings.IndexByte(
			value,
			':',
		); colon >= 0 {
			value = value[:colon]
		}

		if space := strings.IndexAny(
			value,
			" \t\r\n",
		); space >= 0 {
			value = value[:space]
		}

		value = strings.TrimSpace(value)

		switch PatchErrorCode(value) {
		case PatchErrorSearchOutsideSymbol:
			return PatchErrorSearchOutsideSymbol

		case PatchErrorSearchNotFoundInsideSymbol:
			return PatchErrorSearchNotFoundInsideSymbol

		case PatchErrorSearchCrossesSymbolBoundary:
			return PatchErrorSearchCrossesSymbolBoundary

		case PatchErrorAmbiguousSearch:
			return PatchErrorAmbiguousSearch

		case PatchErrorRepairSymbolDrift:
			return PatchErrorRepairSymbolDrift

		case PatchErrorRepairFileDrift:
			return PatchErrorRepairFileDrift

		case PatchErrorRepairProtocolDrift:
			return PatchErrorRepairProtocolDrift

		case PatchErrorSourceChanged:
			return PatchErrorSourceChanged

		case PatchErrorStaleSymbol:
			return PatchErrorStaleSymbol
		case PatchErrorDuplicateFileChange:
			return PatchErrorDuplicateFileChange

		case PatchErrorStrictSymbolRequired:
			return PatchErrorStrictSymbolRequired

		case PatchErrorSymbolNotFound:
			return PatchErrorSymbolNotFound

		case PatchErrorModuleImportMismatch:
			return PatchErrorModuleImportMismatch
		case PatchErrorNoOpPatch:
			return PatchErrorNoOpPatch
		case PatchErrorSemanticScope:
			return PatchErrorSemanticScope

		case PatchErrorPublicAPIGuard:
			return PatchErrorPublicAPIGuard

		case PatchErrorImportGuard:
			return PatchErrorImportGuard

		case PatchErrorGoModGuard:
			return PatchErrorGoModGuard

		case PatchErrorPatchTooLarge:
			return PatchErrorPatchTooLarge

		default:
			return ""
		}
	}

	// Fallback для сырых сообщений go/build/go mod,
	// где структурированного patch_error_code ещё нет.
	lower := strings.ToLower(text)

	switch {
	case strings.Contains(
		lower,
		"no required module provides package",
	):
		return PatchErrorModuleImportMismatch

	case strings.Contains(
		lower,
		"cannot find module providing package",
	):
		return PatchErrorModuleImportMismatch

	case strings.Contains(
		lower,
		"module found, but does not contain package",
	):
		return PatchErrorModuleImportMismatch

	case strings.Contains(
		lower,
		"search crosses declaration boundary",
	):
		return PatchErrorSearchCrossesSymbolBoundary

	case strings.Contains(
		lower,
		"search block is ambiguous",
	):
		return PatchErrorAmbiguousSearch

	case strings.Contains(
		lower,
		"search block not found inside symbol",
	):
		return PatchErrorSearchNotFoundInsideSymbol

	case strings.Contains(
		lower,
		"symbol %q changed since patch generation",
	):
		return PatchErrorStaleSymbol

	default:
		return ""
	}
}
