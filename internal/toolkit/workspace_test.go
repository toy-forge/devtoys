package toolkit

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// TestStripANSI 验证复制输出时剥离 ANSI 转义序列。
func TestStripANSI(t *testing.T) {
	cases := []struct{ in, want string }{
		{"\x1b[31m红色\x1b[0m", "红色"},
		{"\x1b[1;32;40m粗体+色\x1b[m", "粗体+色"},
		{"plain text", "plain text"},
		{"\x1b]0;title\x07rest", "rest"},
		{"a\x1b[38;5;208mmb", "amb"},
	}
	for _, c := range cases {
		if got := stripANSI(c.in); got != c.want {
			t.Fatalf("stripANSI(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestWorkspaceMouseFocusAndScroll(t *testing.T) {
	w := NewWorkspace("", strings.Repeat("line\n", 30))
	w.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	w.Update(tea.MouseMsg{X: w.outputRect.X, Y: 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if w.Focus != FocusOutput {
		t.Fatal("output click did not focus output")
	}
	w.Update(tea.MouseMsg{X: w.outputRect.X, Y: 2, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	if w.Output.YOffset == 0 {
		t.Fatal("wheel did not scroll output")
	}
	w.Update(tea.MouseMsg{X: 2, Y: 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if w.Focus != FocusInput {
		t.Fatal("input click did not focus input")
	}
}

func TestWorkspaceBelowIgnoresHeaderMouse(t *testing.T) {
	w := NewWorkspace("", "")
	w.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
	w.UpdateBelow(tea.MouseMsg{X: w.outputRect.X, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}, 1)
	if w.Focus != FocusInput {
		t.Fatal("header click focused workspace")
	}
	w.UpdateBelow(tea.MouseMsg{X: w.outputRect.X, Y: 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}, 1)
	if w.Focus != FocusOutput {
		t.Fatal("workspace click did not focus output")
	}
}

func TestStackedWorkspaceMouseAndScroll(t *testing.T) {
	w := NewWorkspace("", strings.Repeat("line\n", 30))
	w.Update(tea.WindowSizeMsg{Width: 30, Height: 16})
	if !w.stacked {
		t.Fatal("expected vertical layout")
	}
	if got := lipgloss.Height(w.View()); got != 16 {
		t.Fatalf("height=%d", got)
	}
	if got := lipgloss.Width(w.View()); got > 30 {
		t.Fatalf("width=%d", got)
	}
	w.Update(tea.MouseMsg{X: 2, Y: w.outputRect.Y + 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if w.Focus != FocusOutput {
		t.Fatal("output click did not focus output")
	}
	w.Update(tea.MouseMsg{X: 2, Y: w.outputRect.Y + 1, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	if w.Output.YOffset == 0 {
		t.Fatal("wheel did not scroll output")
	}
	w.Update(tea.MouseMsg{X: 2, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if w.Focus != FocusInput {
		t.Fatal("input click did not focus input")
	}
}

func TestOutputOffsetClampsAfterResize(t *testing.T) {
	w := NewWorkspace("", strings.Repeat("line\n", 20))
	w.Update(tea.WindowSizeMsg{Width: 30, Height: 10})
	w.Output.GotoBottom()
	before := w.Output.YOffset
	w.Update(tea.WindowSizeMsg{Width: 80, Height: 25})
	if before == 0 || w.Output.YOffset != 0 {
		t.Fatalf("offset before=%d after=%d", before, w.Output.YOffset)
	}
}

func TestMousePlacesInputCursorInBothLayouts(t *testing.T) {
	for _, width := range []int{30, 80} {
		w := NewWorkspace("", "")
		w.Update(tea.WindowSizeMsg{Width: width, Height: 16})
		w.Input.SetValue("abc\ndef")
		// 点击 X=5：Panel 水平内边距后文本起点 = 边框内 2 列，落在第 3 列。
		w.Update(tea.MouseMsg{X: 5, Y: 1, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
		w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'X'}})
		if got := w.Input.Value(); got != "aXbc\ndef" {
			t.Fatalf("width %d: %q", width, got)
		}
	}
}

func TestMouseWheelInputPreservesOutputFocusAndCaret(t *testing.T) {
	w := NewWorkspace("", "")
	w.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
	w.Input.SetValue("one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine\nten")
	originalLine := w.Input.Line()
	w.Update(tea.MouseMsg{X: w.outputRect.X, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	for range 5 {
		w.Update(tea.MouseMsg{X: 4, Y: 1, Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress})
	}
	if w.Focus != FocusOutput || w.Input.Line() != originalLine {
		t.Fatal("wheel changed focus or caret")
	}
	if !strings.Contains(w.Input.View(), "one") {
		t.Fatalf("input did not scroll: %q", w.Input.View())
	}
	// 点击 X=5：Panel 水平内边距后文本起点 = 边框内 2 列，落在第 3 列。
	w.Update(tea.MouseMsg{X: 5, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'X'}})
	if !strings.HasPrefix(w.Input.Value(), "oXne") {
		t.Fatalf("wrong insertion: %q", w.Input.Value())
	}
}

func TestMouseScrollsOutputHorizontally(t *testing.T) {
	w := NewWorkspace("", "abcdefghijklmnopqrstuvwxyz0123456789")
	w.Update(tea.WindowSizeMsg{Width: 40, Height: 10})
	before := w.Output.View()
	bar := w.footerBarRect()
	// 按钮顺序：Copy / < / >，各按钮宽 = 标签+2，间隔 1。
	prevX := bar.X + (len("Copy") + 2) + 1 + 1
	nextX := bar.X + (len("Copy") + 2) + 1 + (len("<") + 2) + 1 + 1
	w.Update(tea.MouseMsg{X: nextX, Y: bar.Y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	after := w.Output.View()
	if before == after {
		t.Fatal("right button did not scroll output")
	}
	w.Update(tea.MouseMsg{X: prevX, Y: bar.Y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if got := w.Output.View(); got != before {
		t.Fatalf("left button did not restore output: %q", got)
	}
}

func TestShortWorkspaceTabsKeepBothPanesReachable(t *testing.T) {
	w := NewWorkspace("", strings.Repeat("line\n", 10))
	w.Update(tea.WindowSizeMsg{Width: 30, Height: 5})
	if !w.compact || lipgloss.Height(w.View()) != 5 {
		t.Fatalf("compact=%v height=%d", w.compact, lipgloss.Height(w.View()))
	}
	outputX := lipgloss.Width(ansi.Strip(toolkitTabLabel(w.tabs()[0]))) + 3	// 按钮宽 = 标签+2，间隔 1
	w.Update(tea.MouseMsg{X: outputX, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if w.Focus != FocusOutput || !strings.Contains(w.View(), "line") {
		t.Fatal("output tab unavailable")
	}
	w.Update(tea.MouseMsg{X: 1, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	w.Update(tea.MouseMsg{X: 4, Y: 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'X'}})
	if w.Focus != FocusInput || w.Input.Value() != "X" {
		t.Fatalf("input focus=%v value=%q", w.Focus, w.Input.Value())
	}
}

// toolkitTabLabel 原样返回页签标签（页签不加前缀）。
func toolkitTabLabel(label string) string	{ return label }

func TestWorkspaceInputToolbarClearAndOpen(t *testing.T) {
	w := NewWorkspace("", "")
	w.SetStacked(true)
	w.Update(tea.WindowSizeMsg{Width: 80, Height: 14})
	w.Input.SetValue("content")
	view := ansi.Strip(w.View())
	line := strings.Split(view, "\n")[0]
	clearX := strings.Index(line, "Clear")
	openX := strings.Index(line, "Open")
	if clearX < 0 || openX < 0 {
		t.Fatalf("toolbar missing: %q", line)
	}
	// strings.Index 是字节下标，按钮坐标是显示列。
	clearCol := lipgloss.Width(line[:clearX])
	openCol := lipgloss.Width(line[:openX])
	w.Update(tea.MouseMsg{X: clearCol, Y: 0, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if w.Input.Value() != "" {
		t.Fatalf("clear left %q", w.Input.Value())
	}
	cmd := w.Update(tea.MouseMsg{X: openCol, Y: 0, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if cmd == nil {
		t.Fatal("open did not produce command")
	}
	if _, ok := cmd().(OpenFileRequestMsg); !ok {
		t.Fatal("wrong open command")
	}
}

func TestOutputOnlyWorkspaceUsesAllAvailableSpace(t *testing.T) {
	w := NewWorkspace("not shown", strings.Repeat("line\n", 30))
	w.SetOutputOnly(true)
	for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 15}, {Width: 20, Height: 5}} {
		w.Update(size)
		view := w.View()
		if strings.Contains(view, "not shown") || lipgloss.Height(view) != size.Height || lipgloss.Width(view) > size.Width {
			t.Fatalf("size=%+v view=%q", size, view)
		}
		w.Update(tea.KeyMsg{Type: tea.KeyTab})
		if w.Focus != FocusOutput {
			t.Fatal("tab focused hidden input")
		}
		w.Update(tea.MouseMsg{X: 2, Y: 1, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
		if w.Output.YOffset == 0 {
			t.Fatal("wheel did not scroll output")
		}
	}
}
