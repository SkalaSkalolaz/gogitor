package app

import (
	"fmt"
	"sort"
	"strings"

	"gogitor/internal/domain"
)

func cloneRepairChanges(
	in []domain.FileChange,
) []domain.FileChange {
	if len(in) == 0 {
		return nil
	}

	out :=
		make(
			[]domain.FileChange,
			len(in),
		)

	for i, ch := range in {
		out[i] = ch
		out[i].Patches =
			append(
				[]domain.Patch(nil),
				ch.Patches...,
			)
	}

	return out
}

func validateRepairTargetContract(
	previous []domain.FileChange,
	repaired []domain.FileChange,
	errorCode domain.PatchErrorCode,
) error {
	if len(previous) == 0 {
		return nil
	}

	previousPaths :=
		make(map[string]bool)

	repairedPaths :=
		make(map[string]bool)

	previousSymbols :=
		make(
			map[string]map[string]bool,
		)

	repairedSymbols :=
		make(
			map[string]map[string]bool,
		)

	for _, ch := range previous {
		path := strings.TrimSpace(ch.Path)
		if path == "" {
			continue
		}

		previousPaths[path] = true

		if previousSymbols[path] == nil {
			previousSymbols[path] =
				make(map[string]bool)
		}

		for _, p := range ch.Patches {
			symbol :=
				strings.TrimSpace(
					p.Symbol,
				)

			if symbol == "" {
				continue
			}

			previousSymbols[path][symbol] = true
		}
	}

	for _, ch := range repaired {
		path := strings.TrimSpace(ch.Path)
		if path == "" {
			continue
		}

		repairedPaths[path] = true

		if repairedSymbols[path] == nil {
			repairedSymbols[path] =
				make(map[string]bool)
		}

		for _, p := range ch.Patches {
			symbol :=
				strings.TrimSpace(
					p.Symbol,
				)

			if symbol == "" {
				continue
			}

			repairedSymbols[path][symbol] = true
		}
	}

	var paths []string

	for path := range previousPaths {

		paths = append(
			paths,
			path,
		)
	}

	sort.Strings(paths)

	for _, path := range paths {
		if !repairedPaths[path] {
			return domain.NewPatchError(
				domain.PatchErrorRepairFileDrift,
				fmt.Sprintf(
					"repair removed original patch target file %q",
					path,
				),
			)
		}
	}

	// Только symbol_not_found разрешает заменить
	// исходный Symbol: в этом случае предыдущий Symbol
	// был признан недействительным.
	if errorCode ==
		domain.PatchErrorSymbolNotFound {

		return nil
	}

	for _, path := range paths {
		for symbol := range previousSymbols[path] {

			if !repairedSymbols[path][symbol] {
				return domain.NewPatchError(
					domain.PatchErrorRepairSymbolDrift,
					fmt.Sprintf(
						"repair changed target Symbol %q in file %q; the original Symbol must remain unchanged for this repair",
						symbol,
						path,
					),
				)
			}
		}
	}

	return nil
}

func (s *Service) repairHasExistingFullRewrite(
	changes []domain.FileChange,
) string {
	for _, ch := range changes {
		if strings.TrimSpace(ch.Path) == "" {
			continue
		}

		if len(ch.Patches) > 0 {
			continue
		}

		if s.fileExistsRoot(ch.Path) {
			return ch.Path
		}
	}

	return ""
}
