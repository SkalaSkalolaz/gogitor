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
    PatchErrorSymbolNotFound PatchErrorCode = "symbol_not_found"

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
//
// Это нужно для repair pipeline, где ошибки
// передаются LLM как строка.
func PatchErrorCodeFromText(
	text string,
) PatchErrorCode {
	const prefix = "patch_error_code="

	idx := strings.Index(
		text,
		prefix,
	)

	if idx == -1 {
		return ""
	}

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

	default:
		return ""
	}
}
