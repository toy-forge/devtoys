// Package listcompare compares two lists of text lines.
package listcompare

import (
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/toy-forge/devtoys/internal/toolkit"
)

type tool struct {
	panes			*toolkit.ThreePane
	header			toolkit.SettingHeader
	width, height		int
	mode			int
	sensitive		bool
	beforeA, beforeB	string
	revision		uint64
	running, queued		bool
}

type compareDue struct{ revision uint64 }
type compareDone struct {
	revision	uint64
	output		string
}

const backgroundThreshold = 32 << 10

var modes = []string{"A ∩ B", "A ∪ B", "A Only", "B Only"}

func newTool() *tool {
	t := &tool{
		panes:	toolkit.NewThreePane("List A", "List B"),
		header: toolkit.SettingHeader{
			Names:		[]string{"Case sensitive comparison", "Comparison mode"},
			Descriptions:	[]string{"", "Please specify comparison mode (A ∩ B, A ∪ B, A Only, B Only)"},
			Icons:		[]string{"󰈈", "󰒓"},
			Switches:	[]bool{true, false},
			Options:	[][]string{nil, modes},
		},
	}
	t.panes.SetExtendable(2)
	t.panes.Titles[2] = modes[0]
	return t
}
func init()				{ toolkit.Register(newTool()) }
func (*tool) ID() string		{ return "text.listcompare" }
func (*tool) Title() string		{ return "List Comparer" }
func (*tool) ShortTitle() string	{ return "List Compare" }
func (*tool) Category() string		{ return "text" }
func (*tool) Icon() string		{ return "󰕚" }
func (*tool) Description() string	{ return "Compare, combine and find differences between two lists" }
func (*tool) Init() tea.Cmd		{ return nil }
func (t *tool) actions() []string {
	s := "Off"
	if t.sensitive {
		s = "On"
	}
	return []string{s, modes[t.mode]}
}
func (t *tool) resize() {
	if t.width <= 0 {
		return
	}
	rows := t.header.SetSize(tea.WindowSizeMsg{Width: t.width, Height: t.height}, t.actions()...)
	t.panes.SetSize(t.width, max(1, t.height-rows))
}
func (t *tool) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch event := msg.(type) {
	case compareDue:
		if event.revision != t.revision {
			return t, nil
		}
		if t.running {
			t.queued = true
			return t, nil
		}
		return t, t.startCompare(event.revision)
	case compareDone:
		t.running = false
		if event.revision == t.revision {
			t.panes.SetOutput(event.output)
		}
		if t.queued {
			t.queued = false
			return t, t.startCompare(t.revision)
		}
		return t, nil
	}
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		t.width, t.height = sz.Width, sz.Height
		t.resize()
		return t, nil
	}
	changed := false
	setting, choice, handled := t.header.Pick(msg, t.actions()...)
	if handled && setting < 0 {
		// 比较模式下拉框展开/收起/选择期间，事件全部由浮层消费。
		return t, nil
	}
	if handled {
		switch {
		case setting == 0:
			t.sensitive = !t.sensitive
			changed = true
		case setting == 1 && choice >= 0:
			t.mode = choice
			changed = true
		case setting == 1:
			// 紧凑布局的回退按钮条：点击即轮换比较模式。
			t.mode = (t.mode + 1) % len(modes)
			changed = true
		}
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "alt+c":
			t.sensitive = !t.sensitive
			changed = true
		case "alt+m":
			t.mode = (t.mode + 1) % len(modes)
			changed = true
		}
	}
	if changed {
		t.resize()
		return t, t.recompute()
	}
	if mouse, ok := msg.(tea.MouseMsg); ok {
		rows := t.header.Height(t.actions()...)
		if mouse.Y < rows {
			return t, nil
		}
		mouse.Y -= rows
		msg = mouse
	}
	cmd := t.panes.Update(msg)
	a, b := t.panes.A.Value(), t.panes.B.Value()
	if a != t.beforeA || b != t.beforeB {
		t.beforeA, t.beforeB = a, b
		cmd = tea.Batch(cmd, t.recompute())
	}
	return t, cmd
}
func (t *tool) recompute() tea.Cmd {
	t.revision++
	t.queued = false
	t.panes.Titles[2] = modes[t.mode]
	a, b := t.panes.A.Value(), t.panes.B.Value()
	if len(a)+len(b) < backgroundThreshold {
		t.panes.SetOutput(compare(a, b, t.mode, t.sensitive))
		return nil
	}
	t.panes.SetOutput("")
	t.panes.Output.SetContent("Comparing…")
	revision := t.revision
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg {
		return toolkit.BackgroundToolMsg{Tool: t, Message: compareDue{revision: revision}}
	})
}

func (t *tool) startCompare(revision uint64) tea.Cmd {
	t.running = true
	a, b, mode, sensitive := t.panes.A.Value(), t.panes.B.Value(), t.mode, t.sensitive
	return func() tea.Msg {
		result := compare(a, b, mode, sensitive)
		return toolkit.BackgroundToolMsg{Tool: t, Message: compareDone{revision: revision, output: result}}
	}
}

// HandlesEscape 比较模式下拉浮层展开时 esc 只收起浮层。
func (t *tool) HandlesEscape() bool	{ return t.header.MenuOpen() }

// View 渲染设置条与对比窗格；下拉浮层叠加在完整视图之上。
func (t *tool) View() string {
	return t.header.Overlay(t.header.View(t.actions()...) + "\n" + t.panes.View())
}
func lines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	result := strings.Split(s, "\n")
	if len(result) > 0 && result[len(result)-1] == "" {
		result = result[:len(result)-1]
	}
	return result
}
func fold(s string) string {
	return strings.Map(func(r rune) rune {
		small := r
		for x := unicode.SimpleFold(r); x != r; x = unicode.SimpleFold(x) {
			if x < small {
				small = x
			}
		}
		return small
	}, s)
}
func compare(a, b string, mode int, sensitive bool) string {
	left, right := lines(a), lines(b)
	key := func(s string) string {
		if sensitive {
			return s
		}
		return fold(s)
	}
	leftSet, rightSet, seen := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, v := range left {
		leftSet[key(v)] = true
	}
	for _, v := range right {
		rightSet[key(v)] = true
	}
	var out []string
	appendMatches := func(values []string, filter func(string) bool) {
		for _, v := range values {
			k := key(v)
			if !seen[k] && filter(k) {
				seen[k] = true
				out = append(out, v)
			}
		}
	}
	switch mode {
	case 0:
		appendMatches(left, func(k string) bool { return rightSet[k] })
	case 1:
		appendMatches(left, func(string) bool { return true })
		appendMatches(right, func(string) bool { return true })
	case 2:
		appendMatches(left, func(k string) bool { return !rightSet[k] })
	case 3:
		appendMatches(right, func(k string) bool { return !leftSet[k] })
	}
	return strings.Join(out, "\n")
}
