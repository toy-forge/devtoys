// Package editor wraps the terminal textarea with lossless text handling.
package editor

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rivo/uniseg"

	"github.com/toy-forge/devtoys/internal/theme"
	"github.com/toy-forge/devtoys/internal/toolkit/editor/textarea"
)

// Control pictures are printable, so they cannot move the terminal cursor.
// Literal occurrences are escaped to keep Value reversible.
const (
	tabMark		= '␉'
	crMark		= '␍'
	replacementMark	= '␚'
	escapeMark	= '␛'
)

// Model is a textarea that preserves tabs and carriage returns. It displays
// those characters as control pictures while exposing their original values.
type Model struct {
	textarea.Model
	hasEscapes	bool
}

func New() Model {
	input := textarea.New()
	input.FocusedStyle.Text = lipgloss.NewStyle().Foreground(theme.Text)
	input.FocusedStyle.CursorLine = lipgloss.NewStyle().Foreground(theme.Text).Background(theme.Surface)
	input.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(theme.Accent)
	input.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(theme.Muted)
	input.FocusedStyle.LineNumber = lipgloss.NewStyle().Foreground(theme.Muted)
	input.FocusedStyle.CursorLineNumber = lipgloss.NewStyle().Foreground(theme.BlueBright)
	input.FocusedStyle.EndOfBuffer = lipgloss.NewStyle().Foreground(theme.Border)
	input.BlurredStyle.Text = lipgloss.NewStyle().Foreground(theme.Text)
	input.BlurredStyle.CursorLine = lipgloss.NewStyle().Foreground(theme.Text)
	input.BlurredStyle.Prompt = lipgloss.NewStyle().Foreground(theme.Border)
	input.BlurredStyle.Placeholder = lipgloss.NewStyle().Foreground(theme.Muted)
	input.BlurredStyle.LineNumber = lipgloss.NewStyle().Foreground(theme.Muted)
	input.BlurredStyle.CursorLineNumber = lipgloss.NewStyle().Foreground(theme.Muted)
	input.BlurredStyle.EndOfBuffer = lipgloss.NewStyle().Foreground(theme.Border)
	// The underlying cursor reverses its style when visible.
	input.Cursor.Style = lipgloss.NewStyle().Foreground(theme.Accent).Background(theme.CursorText)
	input.Blur()
	return Model{Model: input}
}

// Value returns the original text, including tabs and carriage returns.
func (m Model) Value() string	{ return decode(m.Model.Value()) }

// SetValue replaces the input with text.
func (m *Model) SetValue(text string) {
	encoded := encode(text)
	m.hasEscapes = strings.ContainsRune(encoded, escapeMark)
	m.Model.SetValue(encoded)
	m.syncViewport()
}

// InsertString inserts text at the current cursor position.
func (m *Model) InsertString(text string) {
	encoded := encode(text)
	m.hasEscapes = m.hasEscapes || strings.ContainsRune(encoded, escapeMark)
	m.Model.InsertString(encoded)
	m.syncViewport()
}

// Focus and sizing keep the underlying viewport aligned with the caret.
func (m *Model) Focus() tea.Cmd {
	cmd := m.Model.Focus()
	m.syncViewport()
	return cmd
}

// FocusPreservingScroll focuses the input without moving its visible viewport.
func (m *Model) FocusPreservingScroll() tea.Cmd	{ return m.Model.Focus() }

func (m *Model) SetWidth(width int)	{ m.Model.SetWidth(width); m.syncViewport() }
func (m *Model) SetHeight(height int)	{ m.Model.SetHeight(height); m.syncViewport() }

func (m *Model) syncViewport() {
	if !m.Focused() {
		return
	}
	// The textarea scroll range is derived from the most recently rendered
	// content; refresh it before asking the model to reveal its current caret.
	_ = m.Model.View()
	m.Model, _ = m.Model.Update(tea.WindowSizeMsg{})
}

// CheckInsert ensures the underlying textarea will not truncate or sanitize text.
func (m *Model) CheckInsert(text string) error {
	const maxLines = 10000
	if !utf8.ValidString(text) {
		return fmt.Errorf("input is not valid UTF-8")
	}
	for _, r := range text {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return fmt.Errorf("input contains unsupported control characters")
		}
	}
	if m.LineCount()+strings.Count(text, "\n") > maxLines {
		return fmt.Errorf("input exceeds the %d-line limit", maxLines)
	}
	if m.CharLimit > 0 {
		encoded := encode(text)
		length := strings.Count(encoded, "\n")
		for _, line := range strings.Split(encoded, "\n") {
			length += uniseg.StringWidth(line)
		}
		if m.Model.Length()+length > m.CharLimit {
			return fmt.Errorf("input exceeds the character limit")
		}
	}
	return nil
}

// PasteErrorMsg reports a clipboard failure to the surrounding workspace.
type PasteErrorMsg struct{ Err error }

// Update preserves characters in typed or pasted rune messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && m.Focused() {
		if key.String() == "ctrl+v" {
			return m, func() tea.Msg {
				text, err := clipboard.ReadAll()
				if err != nil {
					return PasteErrorMsg{Err: fmt.Errorf("could not read clipboard: %w", err)}
				}
				if !utf8.ValidString(text) {
					return PasteErrorMsg{Err: fmt.Errorf("clipboard is not valid UTF-8")}
				}
				return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text), Paste: true}
			}
		}
		if key.Type == tea.KeyRunes {
			if key.Paste {
				if err := m.CheckInsert(string(key.Runes)); err != nil {
					return m, func() tea.Msg { return PasteErrorMsg{Err: err} }
				}
			}
			encoded := encode(string(key.Runes))
			m.hasEscapes = m.hasEscapes || strings.ContainsRune(encoded, escapeMark)
			key.Runes = []rune(encoded)
			msg = key
		}
	}
	repeat := false
	if key, ok := msg.(tea.KeyMsg); ok && m.Focused() {
		start, end, _ := m.escapePairAtCursor()
		switch key.String() {
		case "left", "backspace":
			repeat = end
		case "right", "delete":
			repeat = start
		}
	}
	var cmd tea.Cmd
	m.Model, cmd = m.Model.Update(msg)
	if repeat {
		var second tea.Cmd
		m.Model, second = m.Model.Update(msg)
		cmd = tea.Batch(cmd, second)
	}
	if key, ok := msg.(tea.KeyMsg); ok && m.Focused() && (key.Type == tea.KeyRunes || key.Type == tea.KeyEnter || key.Type == tea.KeyBackspace || key.Type == tea.KeyDelete) {
		m.syncViewport()
	}
	_, _, middle := m.escapePairAtCursor()
	if middle {
		info := m.Model.LineInfo()
		m.Model.SetCursor(info.StartColumn + info.ColumnOffset - 1)
	}
	return m, cmd
}

// escapePairAtCursor identifies the two-cell representation of a literal
// control-picture rune, so ordinary arrow and delete keys treat it as one rune.
func (m Model) escapePairAtCursor() (start, end, middle bool) {
	if !m.hasEscapes {
		return false, false, false
	}
	text := m.Model.Value()
	if !strings.ContainsRune(text, escapeMark) {
		return false, false, false
	}
	lines := strings.Split(text, "\n")
	row := m.Model.Line()
	if row < 0 || row >= len(lines) {
		return false, false, false
	}
	line := []rune(lines[row])
	info := m.Model.LineInfo()
	col := info.StartColumn + info.ColumnOffset
	for i := 0; i+1 < len(line); i++ {
		if line[i] != escapeMark || !isMarker(line[i+1]) {
			continue
		}
		if col == i {
			start = true
		}
		if col == i+2 {
			end = true
		}
		if col == i+1 {
			middle = true
		}
		i++
	}
	return start, end, middle
}

func isMarker(r rune) bool {
	return r == tabMark || r == crMark || r == replacementMark || r == escapeMark
}

func encode(text string) string {
	var out strings.Builder
	for _, r := range text {
		switch r {
		case '\t':
			out.WriteRune(tabMark)
		case '\r':
			out.WriteRune(crMark)
		case '�':
			out.WriteRune(replacementMark)
		case tabMark, crMark, replacementMark, escapeMark:
			out.WriteRune(escapeMark)
			out.WriteRune(r)
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}

func decode(text string) string {
	var out strings.Builder
	escaped := false
	for _, r := range text {
		if escaped {
			if r != tabMark && r != crMark && r != replacementMark && r != escapeMark {
				out.WriteRune(escapeMark)
			}
			out.WriteRune(r)
			escaped = false
			continue
		}
		switch r {
		case escapeMark:
			escaped = true
		case tabMark:
			out.WriteRune('\t')
		case crMark:
			out.WriteRune('\r')
		case replacementMark:
			out.WriteRune('�')
		default:
			out.WriteRune(r)
		}
	}
	if escaped {
		out.WriteRune(escapeMark)
	}
	return out.String()
}
