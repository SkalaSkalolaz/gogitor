package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// renderByLayout renders the single supported Gogitor interface.
//
// The interface is intentionally fixed. Keeping the layout selector out of the
// configuration and command-line layers makes the TUI predictable and prevents
// presentation choices from leaking into the application domain.
func (m *model) renderByLayout() string {
	if m.width <= 0 || m.height <= 0 {
		return "Loading..."
	}
	return m.renderZenStyle()
}

// renderZenStyle is the only supported Gogitor TUI layout.
//
// It keeps the screen deliberately quiet: the output occupies the largest
// possible area, the input is directly below it, and a compact status line
// remains visible without taking attention away from the work.
func (m *model) renderZenStyle() string {
	bodyH := maxInt(4, m.height-inputTextAreaHeight-inputVerticalOverhead-2)
	outputWidth := maxInt(1, m.width-2)

	// Устанавливаем размеры напрямую (замена отсутствующего prepareViewport)
	m.viewport.Width = outputWidth
	m.viewport.Height = bodyH
	m.setWrapWidth(outputWidth)

	output := lipgloss.NewStyle().
		Width(outputWidth).
		Padding(0, 1).
		Render(m.viewport.View())

	input := lipgloss.NewStyle().
		Width(outputWidth).
		Render(m.input.View())

	statusText, style := m.statusLine()
	status := lipgloss.NewStyle().
		Width(outputWidth).
		Foreground(lipgloss.Color("242")).
		Render(style.Render(statusText))

	return lipgloss.JoinVertical(
		lipgloss.Left,
		output,
		input,
		status,
	)
}

// maxInt возвращает наибольшее из двух целых чисел.
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}