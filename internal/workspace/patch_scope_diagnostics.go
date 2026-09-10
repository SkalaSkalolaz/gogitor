package workspace

import (
	"fmt"
	"sort"
	"strings"

	"gogitor/internal/domain"
)

func diagnoseSymbolScopedSearchFailure(
	content string,
	symbol string,
	search string,
) error {
	symbol = normalizePatchSymbol(symbol)
	search = strings.TrimSpace(
		normalizeNewlines(search),
	)

	if symbol == "" || search == "" {
		return nil
	}

	locations :=
		findExactSearchLocations(
			content,
			search,
		)

	if len(locations) == 0 {
		// Точного совпадения нет.
		// Это может быть formatting/whitespace problem,
		// поэтому не утверждаем, что SEARCH находится вне Symbol.
		return nil
	}

	if len(locations) > 1 {
		return domain.NewPatchError(
			domain.PatchErrorAmbiguousSearch,
			fmt.Sprintf(
				"SEARCH block for symbol %q has %d exact matches in current source; narrow SEARCH with unique source context",
				symbol,
				len(locations),
			),
		)
	}

	start := locations[0]
	end := start + len(search)

	decls, err :=
		goDeclarationInfos(content)

	if err != nil {
		// Диагностика не должна сама ломать patch pipeline.
		return nil
	}

	owners :=
		declarationsOverlappingRange(
			decls,
			start,
			end,
		)

	if len(owners) == 0 {
		return nil
	}

	sort.Strings(owners)

	if len(owners) > 1 {
		return domain.NewPatchError(
			domain.PatchErrorSearchCrossesSymbolBoundary,
			fmt.Sprintf(
				"SEARCH block for symbol %q crosses declaration boundaries: %s",
				symbol,
				strings.Join(
					owners,
					", ",
				),
			),
		)
	}

	owner := owners[0]

	if declarationDisplayName(owner) ==
		symbol {

		// SEARCH действительно принадлежит Symbol.
		// Если он не применился, причина, скорее всего,
		// форматирование/нормализация.
		return nil
	}

	return domain.NewPatchError(
		domain.PatchErrorSearchOutsideSymbol,
		fmt.Sprintf(
			"SEARCH block for Symbol %q is located inside unrelated declaration %q; keep Symbol %q and rebuild SEARCH so the entire SEARCH belongs to that Symbol",
			symbol,
			owner,
			symbol,
		),
	)
}

func findExactSearchLocations(
	content string,
	search string,
) []int {
	var result []int

	offset := 0

	for offset <= len(content)-len(search) {
		relative :=
			strings.Index(
				content[offset:],
				search,
			)

		if relative < 0 {
			break
		}

		start := offset + relative

		result = append(
			result,
			start,
		)

		offset =
			start + len(search)

		if len(result) > 16 {
			break
		}
	}

	return result
}

func declarationsOverlappingRange(
	decls map[string]declarationInfo,
	start int,
	end int,
) []string {
	var result []string

	for key, info := range decls {
		if start < info.End &&
			info.Start < end {

			result = append(
				result,
				key,
			)
		}
	}

	return result
}

func declarationDisplayName(
	key string,
) string {
	switch {
	case strings.HasPrefix(
		key,
		"func:",
	):
		return strings.TrimPrefix(
			key,
			"func:",
		)

	case strings.HasPrefix(
		key,
		"type:",
	):
		return strings.TrimPrefix(
			key,
			"type:",
		)

	case strings.HasPrefix(
		key,
		"value:",
	):
		return strings.TrimPrefix(
			key,
			"value:",
		)

	default:
		return key
	}
}
