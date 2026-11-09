package comparer

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TestBuildRowsReplace 验证替换行被配对成 kindMod，并带正确的全局行号。
func TestBuildRowsReplace(t *testing.T) {
	a, b := "hello\nworld\n", "hello\nthere\n"
	rows, added, removed := buildRows(diffSegments(a, b))
	if added != 1 || removed != 1 {
		t.Fatalf("added=%d removed=%d, want 1/1", added, removed)
	}
	if len(rows) != 2 {
		t.Fatalf("rows=%d, want 2", len(rows))
	}
	if rows[0].kind != kindEqual || rows[0].left != "hello\n" {
		t.Fatalf("rows[0]=%+v", rows[0])
	}
	if rows[1].kind != kindMod || rows[1].left != "world\n" || rows[1].right != "there\n" {
		t.Fatalf("rows[1]=%+v", rows[1])
	}
	if rows[1].leftNum != "2" || rows[1].rightNum != "2" {
		t.Fatalf("行号错误: %+v", rows[1])
	}
}

// TestBuildRowsInsertDelete 验证纯增/纯删行只出现在对应一侧。
func TestBuildRowsInsertDelete(t *testing.T) {
	a, b := "a\nb\nc\n", "a\nc\nd\n"
	rows, added, removed := buildRows(diffSegments(a, b))
	if added != 1 || removed != 1 {
		t.Fatalf("added=%d removed=%d, want 1/1", added, removed)
	}
	hasDel, hasAdd := false, false
	for _, r := range rows {
		if r.kind == kindDel && strings.TrimSpace(r.left) == "b" {
			hasDel = true
			if r.right != "" || r.rightNum != "" {
				t.Fatalf("删除行右侧应为空: %+v", r)
			}
		}
		if r.kind == kindAdd && strings.TrimSpace(r.right) == "d" {
			hasAdd = true
			if r.left != "" || r.leftNum != "" {
				t.Fatalf("新增行左侧应为空: %+v", r)
			}
		}
	}
	if !hasDel || !hasAdd {
		t.Fatalf("未找到增删行: %+v", rows)
	}
}

// TestDiffIdentical 验证相同文本不产生差异。
func TestDiffIdentical(t *testing.T) {
	rows, added, removed := buildRows(diffSegments("a\nb\n", "a\nb\n"))
	if added != 0 || removed != 0 {
		t.Fatalf("added=%d removed=%d, want 0/0", added, removed)
	}
	for _, r := range rows {
		if r.kind != kindEqual {
			t.Fatalf("不应存在差异行: %+v", r)
		}
	}
}

// TestUnifiedPlainLines 验证复制用的统一 diff 纯文本格式。
func TestUnifiedPlainLines(t *testing.T) {
	rows, _, _ := buildRows(diffSegments("a\nb\n", "a\nc\n"))
	got := strings.Join(unifiedPlainLines(rows), "\n")
	want := "  a\n- b\n+ c"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestTextCell 验证超宽内容截断加省略号、窄内容补空格。
func TestTextCell(t *testing.T) {
	if got := textCell("hello", 10); got != "hello     " {
		t.Fatalf("got %q", got)
	}
	if got := textCell("hello", 3); got != "he…" {
		t.Fatalf("got %q", got)
	}
}

func TestMouseChangesDiffView(t *testing.T) {
	m := newModel()
	m.Update(tea.WindowSizeMsg{Width: 68, Height: 20})
	// 双栏/统一切换在设置条「Inline mode」开关上（页脚右端只有 Copy < >）。
	m.Update(tea.MouseMsg{X: 2, Y: 0, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if !m.unified {
		t.Fatal("view was not changed")
	}
}

func TestClickPositionsBothInputs(t *testing.T) {
	m := newModel()
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	m.orig.SetValue("abcd")
	m.mod.SetValue("efgh")
	// 面板内边距：文本起点 = 边框内 2 列，点击坐标 X 4 → 5。
	m.Update(tea.MouseMsg{X: 5, Y: 2, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'X'}})
	if got := m.orig.Value(); got != "aXbcd" {
		t.Fatalf("original = %q", got)
	}
	m.Update(tea.MouseMsg{X: m.modRect.x + 5, Y: m.modRect.y + 1, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Y'}})
	if got := m.mod.Value(); got != "eYfgh" {
		t.Fatalf("modified = %q", got)
	}
}

func TestNarrowLayoutKeepsAllPanesInteractive(t *testing.T) {
	m := newModel()
	m.Update(tea.WindowSizeMsg{Width: 30, Height: 18})
	if !m.stacked || m.compact {
		t.Fatal("expected stacked layout")
	}
	if view := m.View(); lipgloss.Width(view) > 30 || lipgloss.Height(view) != 18 {
		t.Fatalf("view is %dx%d", lipgloss.Width(view), lipgloss.Height(view))
	}
	m.orig.SetValue("abcd")
	m.mod.SetValue("efgh")
	// 面板内边距：点击坐标 X 4 → 5。
	m.Update(tea.MouseMsg{X: 5, Y: m.origRect.y + 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'X'}})
	if got := m.orig.Value(); got != "aXbcd" {
		t.Fatalf("original=%q", got)
	}
	m.Update(tea.MouseMsg{X: 5, Y: m.modRect.y + 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Y'}})
	if got := m.mod.Value(); got != "eYfgh" {
		t.Fatalf("modified=%q", got)
	}
	m.orig.SetValue(strings.Repeat("one\n", 20))
	m.mod.SetValue(strings.Repeat("two\n", 20))
	m.recompute(true)
	m.Update(tea.MouseMsg{X: 1, Y: m.diffRect.y + 1, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	if m.diff.YOffset == 0 {
		t.Fatal("diff did not scroll")
	}
	m.Update(tea.MouseMsg{X: 1, Y: m.diffRect.y + 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.focus != focusDiff {
		t.Fatal("diff was not focused")
	}
}

func TestCompactLayoutSwitchesBetweenPanes(t *testing.T) {
	m := newModel()
	m.Update(tea.WindowSizeMsg{Width: 30, Height: 10})
	if !m.compact {
		t.Fatal("expected compact tabs")
	}
	second := lipgloss.Width("["+m.tabs()[0]+"] ") + 1
	m.Update(tea.MouseMsg{X: second, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.focus != focusNew || m.modRect.width == 0 {
		t.Fatal("modified tab did not open")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("updated")})
	if got := m.mod.Value(); got != "updated" {
		t.Fatalf("modified=%q", got)
	}
	third := lipgloss.Width("["+m.tabs()[0]+"] ["+m.tabs()[1]+"] ") + 1
	m.Update(tea.MouseMsg{X: third, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.focus != focusDiff || m.diffRect.width == 0 {
		t.Fatal("diff tab did not open")
	}
	if view := m.View(); lipgloss.Width(view) > 30 || lipgloss.Height(view) != 10 {
		t.Fatalf("view is %dx%d", lipgloss.Width(view), lipgloss.Height(view))
	}
}

func TestVeryNarrowFooterKeepsHorizontalControls(t *testing.T) {
	m := newModel()
	m.Update(tea.WindowSizeMsg{Width: 20, Height: 7})
	line := strings.Split(m.View(), "\n")[6]
	// 水平滚动是内联按钮胶囊（无方括号字面量）。
	if !strings.Contains(line, "<") || !strings.Contains(line, ">") {
		t.Fatalf("missing controls: %q", line)
	}
	m.diff.SetContent(strings.Repeat("abcdefghijklmnopqrstuvwxyz", 4))
	before := m.diff.View()
	x := lipgloss.Width("[双栏][复制][<]") + 1
	m.Update(tea.MouseMsg{X: x, Y: 6, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if got := m.diff.View(); got == before {
		t.Fatal("right arrow did not scroll")
	}
}

func TestWideLayoutMatchesDesktopCompareArrangement(t *testing.T) {
	m := newModel()
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 25})
	if m.origRect.y != m.modRect.y || m.origRect.x >= m.modRect.x || m.diffRect.y <= m.origRect.y || m.diffRect.width != 100 {
		t.Fatalf("rects: original=%+v modified=%+v diff=%+v", m.origRect, m.modRect, m.diffRect)
	}
	if view := m.View(); lipgloss.Height(view) != 25 || lipgloss.Width(view) > 100 {
		t.Fatalf("view dimensions=%dx%d", lipgloss.Width(view), lipgloss.Height(view))
	}
}
