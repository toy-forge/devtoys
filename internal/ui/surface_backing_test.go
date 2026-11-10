package ui

// 回归测试：Panel/Modal 整行 Surface 底。

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/toy-forge/devtoys/internal/theme"
)

const surfaceBG = "48;2;22;27;34"	// Surface #161B22 的背景 SGR 参数段

func forceTrueColor(t *testing.T) {
	t.Helper()
	old := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })
	lipgloss.SetColorProfile(termenv.TrueColor)
}

func TestPanelToolbarRowCarriesSurfaceGap(t *testing.T) {
	prev := theme.Dark()
	defer theme.SetDark(prev)
	theme.SetDark(true)
	forceTrueColor(t)
	p := NewPanel("Output")
	p.SetSize(52, 6)
	p.SetToolbar(Button{Label: "Copy"}, Button{Label: "Clear"})
	view, _ := p.Render("text")
	top := strings.Split(view, "\n")[0]
	if !strings.Contains(top, surfaceBG) {
		t.Fatalf("panel title row missing Surface bg: %q", top)
	}
	// 两颗内联按钮之间必须有 Surface 缝（reset 之后不能落回透明格）。
	if strings.Count(top, "\x1b[0m\x1b["+surfaceBG+"m") < 2 {
		t.Fatalf("panel title row bg not re-injected between toolbar buttons: %q", top)
	}
}

func TestToolbarGapCarriesSurface(t *testing.T) {
	prev := theme.Dark()
	defer theme.SetDark(prev)
	theme.SetDark(true)
	forceTrueColor(t)
	tb := NewToolbar(Button{Label: "A", Inline: true}, Button{Label: "B", Inline: true})
	tb.SetSize(24, 1)
	view := tb.View()
	if !strings.Contains(view, surfaceBG) {
		t.Fatalf("toolbar gap missing Surface bg: %q", view)
	}
}

func TestPanelRowsCarrySurfaceBackground(t *testing.T) {
	prev := theme.Dark()
	defer theme.SetDark(prev)
	theme.SetDark(true)
	forceTrueColor(t)
	p := NewPanel("Input")
	p.SetSize(40, 6)
	p.SetFocused(false)
	view, _ := p.Render("hello")
	lines := strings.Split(view, "\n")
	for i, line := range lines {
		if !strings.Contains(line, surfaceBG) {
			t.Fatalf("panel row %d missing Surface bg: %q", i, line)
		}
		// reset 之后必须重新注入底色（除行尾最后一个 reset 外）。
		idx := strings.Index(line, "\x1b[0m")
		if idx >= 0 && !strings.HasPrefix(line[idx:], "\x1b[0m\x1b["+surfaceBG+"m") {
			end := min(idx+60, len(line))
			t.Fatalf("panel row %d: bg not re-injected after reset: %q", i, line[idx:end])
		}
	}
}

func TestModalLinesCarrySurfaceBackground(t *testing.T) {
	prev := theme.Dark()
	defer theme.SetDark(prev)
	theme.SetDark(true)
	forceTrueColor(t)
	m := &Modal{Kind: KindConfirm, Title: "Delete", Body: "Remove this file?", Width: 24}
	m.SetSize(80, 24)
	box := m.View()
	lines := strings.Split(box, "\n")
	for i, line := range lines {
		if !strings.Contains(line, surfaceBG) {
			t.Fatalf("modal line %d missing Surface bg: %q", i, line)
		}
	}
	// 标题行尾：标题分段 reset 之后必须重新注入 Surface（覆盖行尾空白）。
	title := lines[1]
	idx := strings.LastIndex(title, "\x1b[0m")
	if idx < 0 || !strings.Contains(title, "\x1b[0m\x1b["+surfaceBG+"m") {
		t.Fatalf("modal title tail bg missing: %q", title)
	}
}
