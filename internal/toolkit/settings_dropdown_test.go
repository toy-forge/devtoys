package toolkit

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// dropdownHeader 构造一个带下拉框的设置条：第 0 项为下拉框，第 1 项为开关。
func dropdownHeader(t *testing.T) (SettingHeader, []string) {
	t.Helper()
	h := SettingHeader{
		Names:		[]string{"Indentation", "Sort"},
		Switches:	[]bool{false, true},
		Options:	[][]string{{"2 spaces", "4 spaces", "Minified"}, nil},
	}
	values := []string{"2 spaces", "Off"}
	h.SetSize(tea.WindowSizeMsg{Width: 60, Height: 20}, values...)
	return h, values
}

// chipClick 返回点击下拉框 chip（第 0 行）的鼠标事件。
func chipClick(h SettingHeader, values []string) tea.MouseMsg {
	sel := h.dropdown(0, values[0])
	x := h.width - sel.ChipRect(0, 0).W
	return tea.MouseMsg{X: x + 1, Y: h.titleHeight(), Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
}

func TestDropdownOpensOnChipClickAndDoesNotCycle(t *testing.T) {
	h, values := dropdownHeader(t)
	setting, option, handled := h.Pick(chipClick(h, values), values...)
	if setting != -1 || option != -1 || !handled || !h.MenuOpen() {
		t.Fatalf("chip click: setting=%d option=%d handled=%v open=%v", setting, option, handled, h.MenuOpen())
	}
	// 真实视图还包含工作区，这里用空行模拟，保证浮层有地方绘制。
	base := h.View(values...) + strings.Repeat("\n", 10)
	if view := ansi.Strip(h.Overlay(base)); !strings.Contains(view, "4 spaces") || !strings.Contains(view, "Minified") {
		t.Fatalf("open menu does not list every option:\n%s", view)
	}
}

func TestDropdownKeyboardSelectsOption(t *testing.T) {
	h, values := dropdownHeader(t)
	h.Pick(chipClick(h, values), values...)
	// 光标从当前选项 "2 spaces" 下移两次到 "Minified"，enter 确认。
	for _, key := range []tea.KeyMsg{{Type: tea.KeyDown}, {Type: tea.KeyDown}} {
		if _, _, handled := h.Pick(key, values...); !handled {
			t.Fatal("open dropdown must consume arrow keys")
		}
	}
	setting, option, handled := h.Pick(tea.KeyMsg{Type: tea.KeyEnter}, values...)
	if setting != 0 || option != 2 || !handled || h.MenuOpen() {
		t.Fatalf("enter: setting=%d option=%d handled=%v open=%v", setting, option, handled, h.MenuOpen())
	}
}

func TestDropdownEscCollapsesWithoutChoosing(t *testing.T) {
	h, values := dropdownHeader(t)
	h.Pick(chipClick(h, values), values...)
	setting, option, handled := h.Pick(tea.KeyMsg{Type: tea.KeyEsc}, values...)
	if setting != -1 || option != -1 || !handled || h.MenuOpen() {
		t.Fatalf("esc: setting=%d option=%d handled=%v open=%v", setting, option, handled, h.MenuOpen())
	}
}

func TestDropdownMouseChoosesOptionFromOverlay(t *testing.T) {
	h, values := dropdownHeader(t)
	h.Pick(chipClick(h, values), values...)
	sel := h.menu
	// 选项行：浮层顶边框之后的第 i 行（chip 下一行起）。
	option := 1
	x, y := h.menuX+2, h.menuY+1+option+1
	if got, ok := sel.ListHit(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}, h.menuX, h.menuY, h.width, h.height); !ok || got != option {
		t.Fatalf("ListHit = %d,%v want %d", got, ok, option)
	}
	setting, chosen, handled := h.Pick(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}, values...)
	if setting != 0 || chosen != option || !handled || h.MenuOpen() {
		t.Fatalf("option click: setting=%d option=%d handled=%v open=%v", setting, chosen, handled, h.MenuOpen())
	}
}

func TestDropdownOutsideClickCollapsesAndIsConsumed(t *testing.T) {
	h, values := dropdownHeader(t)
	h.Pick(chipClick(h, values), values...)
	// 点击工作区（远离浮层）：收起并且不穿透给工具。
	setting, option, handled := h.Pick(tea.MouseMsg{X: 1, Y: 15, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}, values...)
	if setting != -1 || option != -1 || !handled || h.MenuOpen() {
		t.Fatalf("outside click: setting=%d option=%d handled=%v open=%v", setting, option, handled, h.MenuOpen())
	}
}

func TestSwitchClickStillReportsSettingAndDropdownIsNotIndexed(t *testing.T) {
	h, values := dropdownHeader(t)
	// 第 1 行是开关（Sort）：Pick 与 Index 都报告它。
	switchClick := tea.MouseMsg{X: 50, Y: h.titleHeight() + 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	if setting, option, handled := h.Pick(switchClick, values...); setting != 1 || option != -1 || !handled {
		t.Fatalf("switch click: setting=%d option=%d handled=%v", setting, option, handled)
	}
	// 下拉框行不再经由 Index 切换，避免工具把它当开关处理。
	if got := h.Index(chipClick(h, values), values...); got != -1 {
		t.Fatalf("Index on dropdown row = %d, want -1", got)
	}
}

func TestDropdownChipWidthIsStableAcrossValues(t *testing.T) {
	h, _ := dropdownHeader(t)
	shortSel := h.dropdown(0, "2 spaces")
	longSel := h.dropdown(0, "Minified")
	short := lipgloss.Width(shortSel.View())
	long := lipgloss.Width(longSel.View())
	if short != long {
		t.Fatalf("chip width changes with value: %d vs %d", short, long)
	}
}

// 紧凑布局没有展开设置条的空间：下拉框退化为按钮条，点击照常报告设置下标
// （option=-1），工具据此轮换取值，不会因为没有浮层而失去操作。
func TestCompactDropdownFallsBackToCycling(t *testing.T) {
	h := SettingHeader{
		Names:		[]string{"Indentation"},
		Options:	[][]string{{"2 spaces", "4 spaces", "1 tab"}},
	}
	values := []string{"2 spaces"}
	h.SetSize(tea.WindowSizeMsg{Width: 20, Height: 6}, values...)
	if h.Expanded(values...) {
		t.Fatal("test setup must be a compact layout")
	}
	setting, option, handled := h.Pick(tea.MouseMsg{X: 3, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}, values...)
	if setting != 0 || option != -1 || !handled || h.MenuOpen() {
		t.Fatalf("compact click: setting=%d option=%d handled=%v open=%v", setting, option, handled, h.MenuOpen())
	}
}

// 浮层展开时窗口尺寸等非输入消息必须照常交给工具（不得被下拉框吞掉）。
func TestOpenMenuDoesNotSwallowResize(t *testing.T) {
	h, values := dropdownHeader(t)
	h.Pick(chipClick(h, values), values...)
	if _, _, handled := h.Pick(tea.WindowSizeMsg{Width: 100, Height: 30}, values...); handled {
		t.Fatal("an open dropdown must not consume window size messages")
	}
	if !h.MenuOpen() {
		t.Fatal("resize must not close the dropdown")
	}
}
