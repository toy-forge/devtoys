package editor

import tea "github.com/charmbracelet/bubbletea"

// PlaceCursor moves the caret to a visible textarea cell, excluding its border.
func (m *Model) PlaceCursor(x, y int) tea.Cmd {
	if x < 0 || y < 0 || y >= m.Height() {
		return nil
	}
	m.Model.CursorAtCell(x, y)
	_, _, middle := m.escapePairAtCursor()
	if middle {
		info := m.Model.LineInfo()
		m.Model.SetCursor(info.StartColumn + info.ColumnOffset - 1)
	}
	return m.Model.Focus()
}

// Scroll moves the visible text without moving the caret or changing focus.
func (m *Model) Scroll(lines int)	{ m.Model.ScrollLines(lines) }
