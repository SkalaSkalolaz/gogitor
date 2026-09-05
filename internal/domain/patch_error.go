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

	// PatchErrorNoOpPatch означает, что SEARCH/REPLACE
	// формально сопоставились, но итоговый файл фактически
	// не изменился.
	PatchErrorNoOpPatch PatchErrorCode = "no_op_patch"

    // PatchErrorModuleImportMismatch означает,
    // что новый Go import не соответствует module path
    // проекта и не является объявленной зависимостью.
    PatchErrorModuleImportMismatch PatchErrorCode =  "module_import_mismatch"

	// PatchErrorTaskEffectiveness означает, что patch формально
	// применился, но ожидаемый результат задачи не был достигнут.
	PatchErrorTaskEffectiveness PatchErrorCode = "task_effectiveness_failed"
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

	default:
		return ""
	}
}