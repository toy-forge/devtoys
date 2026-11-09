package regex

import (
	tea "github.com/charmbracelet/bubbletea"
	regexp2 "github.com/dlclark/regexp2/v2"
	"github.com/toy-forge/devtoys/internal/toolkit"
)

type regexTool struct {
	*model
	header				toolkit.SettingHeader
	width, height, headerHeight	int
}

func newRegexTool() *regexTool {
	t := &regexTool{model: New(), header: toolkit.SettingHeader{
		Names:		[]string{"All matches", "ECMA Script", "Culture Invariant", "Ignore Case", "Ignore Whitespace", "Singleline", "Multiline", "Right To Left"},
		Descriptions:	[]string{"Find every match or stop after the first match.", "Enable ECMAScript-compatible expression behavior.", "Culture handling is fixed by the Go regular expression engine.", "Match without considering letter case.", "Ignore unescaped pattern whitespace and allow comments.", "Make dot match newline characters.", "Make start and end anchors match each line.", "Search from right to left."},
		Switches:	[]bool{true, true, true, true, true, true, true, true},
		Groups:		[]toolkit.SettingGroup{{Title: "Options", Icon: "󰒓", Primary: -1, Children: []int{0, 1, 2, 3, 4, 5, 6, 7}, HeaderOnly: true}},
	}, width:	80, height: 24}
	t.model.owner = t
	t.resize()
	return t
}
func (t *regexTool) values() []string {
	states := []string{"Off", "Off", "On", "Off", "Off", "Off", "Off", "Off"}
	if t.global {
		states[0] = "On"
	}
	if t.flags&regexp2.ECMAScript != 0 {
		states[1] = "On"
	}
	if t.flags&regexp2.IgnoreCase != 0 {
		states[3] = "On"
	}
	if t.flags&regexp2.IgnorePatternWhitespace != 0 {
		states[4] = "On"
	}
	if t.flags&regexp2.Singleline != 0 {
		states[5] = "On"
	}
	if t.flags&regexp2.Multiline != 0 {
		states[6] = "On"
	}
	if t.flags&regexp2.RightToLeft != 0 {
		states[7] = "On"
	}
	values := make([]string, len(states))
	for i, state := range states {
		values[i] = state
	}
	return values
}
func (t *regexTool) resize() tea.Cmd {
	// 选项设置条只在桌面式展开布局放得下时显示；其余尺寸通过 Alt+1..8
	// 快捷键切换，保证表达式行始终位于视图首行。
	if t.header.Expanded(t.values()...) {
		t.headerHeight = t.header.SetSize(tea.WindowSizeMsg{Width: t.width, Height: t.height}, t.values()...)
	} else {
		t.headerHeight = 0
	}
	_, cmd := t.model.Update(tea.WindowSizeMsg{Width: t.width, Height: max(1, t.height-t.headerHeight)})
	return cmd
}
func (t *regexTool) toggle(i int) tea.Cmd {
	switch i {
	case 0:
		t.global = !t.global
	case 1:
		t.flags ^= regexp2.ECMAScript
		if t.flags&regexp2.ECMAScript != 0 {
			t.flags &^= regexp2.IgnorePatternWhitespace | regexp2.Singleline | regexp2.RightToLeft
		}
	case 2:
		return nil
	case 3:
		t.flags ^= regexp2.IgnoreCase
	case 4:
		if t.flags&regexp2.ECMAScript == 0 {
			t.flags ^= regexp2.IgnorePatternWhitespace
		}
	case 5:
		if t.flags&regexp2.ECMAScript == 0 {
			t.flags ^= regexp2.Singleline
		}
	case 6:
		t.flags ^= regexp2.Multiline
	case 7:
		if t.flags&regexp2.ECMAScript == 0 {
			t.flags ^= regexp2.RightToLeft
		}
	}
	t.resize()
	return t.schedule()
}
func (t *regexTool) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		t.width, t.height = sz.Width, sz.Height
		return t, t.resize()
	}
	option := -1
	if t.headerHeight > 0 {
		option = t.header.Index(msg, t.values()...)
	}
	if key, ok := msg.(tea.KeyMsg); ok && key.Alt && len(key.Runes) == 1 && key.Runes[0] >= '1' && key.Runes[0] <= '8' {
		// Alt+1..8 依常用度排列：1 全局匹配、2 忽略大小写、3 ECMAScript、
		// 4 区域不变、5 忽略空白、6 Singleline、7 Multiline、8 从右向左。
		option = [8]int{0, 3, 1, 2, 4, 5, 6, 7}[key.Runes[0]-'1']
	}
	if option >= 0 {
		return t, t.toggle(option)
	}
	if mouse, ok := msg.(tea.MouseMsg); ok {
		if mouse.Y < t.headerHeight {
			return t, nil
		}
		mouse.Y -= t.headerHeight
		_, cmd := t.model.Update(mouse)
		return t, cmd
	}
	_, cmd := t.model.Update(msg)
	return t, cmd
}
func (t *regexTool) View() string {
	if t.headerHeight == 0 {
		return t.model.View()
	}
	return t.header.View(t.values()...) + "\n" + t.model.View()
}
