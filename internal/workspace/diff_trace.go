package workspace

import (
	"fmt"
	"strings"

	"gogitor/internal/domain"
)

// DiffTraceSink получает диагностические сообщения DIFF.
//
// Sink является только каналом диагностики.
// Он не участвует в принятии решений patch engine.
type DiffTraceSink func(string)

// SetDiffTraceSink включает или выключает DIFF trace.
func (w *Workspace) SetDiffTraceSink(
	sink DiffTraceSink,
) {
	w.mu.Lock()
	w.diffTraceSink = sink
	w.mu.Unlock()
}

// getDiffTraceSink возвращает текущий DIFF trace sink.
func (w *Workspace) getDiffTraceSink() DiffTraceSink {
	w.mu.Lock()
	sink := w.diffTraceSink
	w.mu.Unlock()
	return sink
}

// diffTracef отправляет одно диагностическое сообщение.
//
// При отключённом trace это no-op.
func (w *Workspace) diffTracef(
	format string,
	args ...any,
) {
	sink := w.getDiffTraceSink()
	if sink == nil {
		return
	}

	sink(
		"[DIFF] " +
			fmt.Sprintf(format, args...),
	)
}

// patchTrace содержит только контекст диагностики одного patch.
//
// Он НЕ хранит состояние алгоритма поиска, кроме двух результатов,
// которые нужны для последующего сообщения APPLY.
type patchTrace struct {
	sink DiffTraceSink

	phase string
	file  string

	index int
	total int

	policy PatchPolicy
	patch  domain.Patch

	method    string
	startLine int
}

// newPatchTrace ВСЕГДА возвращает non-nil объект.
//
// Это важная часть архитектуры: trace выключен — sink nil,
// но patch core всё равно получает валидный no-op trace.
// Поэтому core никогда не зависит от режима диагностики.
func newPatchTrace(
	sink DiffTraceSink,
	phase string,
	file string,
	index int,
	total int,
	policy PatchPolicy,
	patch domain.Patch,
) *patchTrace {
	return &patchTrace{
		sink:      sink,
		phase:     phase,
		file:      file,
		index:     index,
		total:     total,
		policy:    policy,
		patch:     patch,
		startLine: 0,
	}
}

// emit является no-op, когда trace отключён.
func (t *patchTrace) emit(
	stage string,
	decision string,
	format string,
	args ...any,
) {
	if t == nil || t.sink == nil {
		return
	}

	detail := ""

	if format != "" {
		detail =
			" " +
				fmt.Sprintf(
					format,
					args...,
				)
	}

	symbol :=
		strings.TrimSpace(
			t.patch.Symbol,
		)

	protocol := "SEARCH_REPLACE"

	if t.patch.ReplaceOnly {
		protocol = "REPLACE_ONLY"
	}

	message := fmt.Sprintf(
		"phase=%s file=%s patch=%d/%d stage=%s decision=%s policy=%s protocol=%s",
		t.phase,
		t.file,
		t.index,
		t.total,
		stage,
		decision,
		t.policy.String(),
		protocol,
	)

	if symbol != "" {
		message +=
			" symbol=" +
				symbol
	}

	message += detail

	t.sink(
		"[DIFF] " +
			message,
	)
}
