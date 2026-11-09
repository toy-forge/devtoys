package textarea

import (
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/rivo/uniseg"
)

// ScrollLines scrolls the viewport without changing the insertion caret.
func (m *Model) ScrollLines(delta int) {
	// View refreshes the viewport's line count before clamping the new offset.
	_ = m.View()
	m.viewport.SetYOffset(m.viewport.YOffset + delta)
}

// CursorAtCell positions the caret at a visible cell, relative to the rendered
// textarea and excluding any border supplied by the caller.
func (m *Model) CursorAtCell(x, y int) {
	if x < 0 || y < 0 || y >= m.height || len(m.value) == 0 {
		return
	}
	_ = m.View()
	target := m.viewport.YOffset + y
	visualRow := 0
	for row, line := range m.value {
		wraps := m.memoizedWrap(line, m.width)
		start := 0
		for _, part := range wraps {
			if visualRow == target {
				prefix := lipgloss.Width(m.getPromptString(visualRow))
				if m.ShowLineNumbers {
					prefix += lipgloss.Width(m.formatLineNumber(row + 1))
				}
				cells := max(0, x-prefix)
				begin := min(start, len(line))
				end := min(start+len(part), len(line))
				col, used := begin, 0
				graphemes := uniseg.NewGraphemes(string(line[begin:end]))
				for graphemes.Next() {
					width := graphemes.Width()
					if used+width > cells {
						break
					}
					used += width
					col += utf8.RuneCountInString(graphemes.Str())
				}
				m.row = row
				m.SetCursor(col)
				return
			}
			visualRow++
			start += len(part)
		}
	}
	// Clicks below the document place the caret at the end.
	m.row = len(m.value) - 1
	m.SetCursor(len(m.value[m.row]))
}
