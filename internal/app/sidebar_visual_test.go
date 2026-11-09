package app

// 回归测试：过滤输入框 theme token、侧栏与搜索行整行 Surface 底。

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"github.com/toy-forge/devtoys/internal/theme"
)

const surfaceBG = "48;2;22;27;34"	// Surface #161B22

func forceTrueColor(t *testing.T) {
	t.Helper()
	old := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })
	lipgloss.SetColorProfile(termenv.TrueColor)
}

func TestFilterInputThemeTokens(t *testing.T) {
	prev := theme.Dark()
	defer theme.SetDark(prev)
	theme.SetDark(true)
	m := New()
	if got := m.tools.FilterInput.PromptStyle.GetForeground(); got != theme.Muted {
		t.Fatalf("filter prompt fg = %v, want Muted", got)
	}
	if got := m.tools.FilterInput.TextStyle.GetForeground(); got != theme.Text {
		t.Fatalf("filter text fg = %v, want Text", got)
	}
	if got := m.tools.FilterInput.Cursor.Style.GetBackground(); got != theme.Accent {
		t.Fatalf("filter cursor bg = %v, want Accent", got)
	}
	if got := m.tools.FilterInput.Cursor.Style.GetForeground(); got != theme.CursorText {
		t.Fatalf("filter cursor fg = %v, want CursorText", got)
	}
}

func TestSidebarRowsCarrySurface(t *testing.T) {
	prev := theme.Dark()
	defer theme.SetDark(prev)
	theme.SetDark(true)
	forceTrueColor(t)
	m := New()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)
	for i, line := range strings.Split(m.sidebarView(), "\n") {
		if !strings.Contains(line, surfaceBG) {
			t.Fatalf("sidebar row %d missing Surface bg: %q", i, line)
		}
		if got := lipgloss.Width(line); got != sidebarWidth {
			t.Fatalf("sidebar row %d width = %d, want %d", i, got, sidebarWidth)
		}
	}
}

func TestSearchRowCarriesSurfaceWhileFiltering(t *testing.T) {
	prev := theme.Dark()
	defer theme.SetDark(prev)
	theme.SetDark(true)
	forceTrueColor(t)
	m := New()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = next.(Model)
	if m.tools.FilterState() != list.Filtering {
		t.Fatal("filter did not start")
	}
	row := m.searchRow(m.innerWidth())
	if !strings.Contains(row, surfaceBG) {
		t.Fatalf("filtering search row missing Surface bg: %q", row)
	}
	if !strings.Contains(ansi.Strip(row), "j") {
		t.Fatalf("filter text missing from search row: %q", ansi.Strip(row))
	}
	// bubbles 出厂色不得出现：#ECFD65（黄绿提示）与 #EE6FF8（品红光标）。
	for _, leak := range []string{"38;2;236;253;101", "38;2;238;111;248"} {
		if strings.Contains(row, leak) {
			t.Fatalf("filter row leaks bubbles default color %s: %q", leak, row)
		}
	}
}
