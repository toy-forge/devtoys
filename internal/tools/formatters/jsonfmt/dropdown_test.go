package jsonfmt

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// 缩进是下拉框：点击 chip 展开列表，点击选项直接设置，而不是轮换。
func TestIndentationDropdownChoosesOption(t *testing.T) {
	m := newTool()
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	values := m.actions()
	chip := tea.MouseMsg{X: 90, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	if _, _, handled := m.header.Pick(chip, values...); !handled || !m.header.MenuOpen() {
		t.Fatal("chip click did not open the indentation dropdown")
	}
	if m.indent != 2 {
		t.Fatalf("chip click must not change the value, indent=%d", m.indent)
	}
	// 浮层第 2 个选项 "1 tab" 的位置：顶边框在 chip 下一行，选项从其后开始。
	option := tea.MouseMsg{X: 90, Y: 1 + 1 + 2 + 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	m.Update(option)
	if m.header.MenuOpen() {
		t.Fatal("menu should close after choosing an option")
	}
	if !strings.Contains(ansi.Strip(m.View()), "1 tab") {
		t.Fatalf("chosen option not shown:\n%s", ansi.Strip(m.View()))
	}
}
