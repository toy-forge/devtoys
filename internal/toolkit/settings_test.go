package toolkit

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestSettingsLayoutAndMouse(t *testing.T) {
	var h SettingHeader
	h.Names = []string{"Conversion", "Encoding"}
	w := NewWorkspace("", "")
	values := []string{"Encode", "UTF-8"}
	h.Resize(tea.WindowSizeMsg{Width: 60, Height: 20}, &w, values...)
	view := h.View(values...)
	// 分组标题的字符形态由 theme.Eyebrow 决定（大写），断言用不区分大小写的形式。
	if h.Height(values...) != 3 || !strings.Contains(strings.ToUpper(view), "CONFIGURATION") || !strings.Contains(view, "Conversion") || !strings.Contains(view, "UTF-8") {
		t.Fatalf("settings=%q", view)
	}
	if got := h.Index(tea.MouseMsg{X: 40, Y: 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}, values...); got != 1 {
		t.Fatalf("clicked=%d", got)
	}
	h.Resize(tea.WindowSizeMsg{Width: 20, Height: 7}, &w, values...)
	if h.Height(values...) > 2 || lipgloss.Height(h.View(values...)) > 2 {
		t.Fatalf("compact settings=%q", h.View(values...))
	}
}

func TestForcedStackedWorkspaceAtWideSize(t *testing.T) {
	w := NewWorkspace("", "")
	w.SetStacked(true)
	w.Update(tea.WindowSizeMsg{Width: 80, Height: 18})
	if !w.stacked || lipgloss.Height(w.View()) != 18 {
		t.Fatalf("stacked=%v height=%d", w.stacked, lipgloss.Height(w.View()))
	}
	if width := lipgloss.Width(w.View()); width > 80 {
		t.Fatalf("width=%d", width)
	}
	w.Update(tea.MouseMsg{X: 2, Y: w.outputRect.Y + 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if w.Focus != FocusOutput {
		t.Fatal("output click did not focus output")
	}
}
