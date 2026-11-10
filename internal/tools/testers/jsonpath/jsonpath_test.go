package jsonpath

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
)

const sampleJSON = `{"store":{"book":[{"title":"Go 语言","price":30},{"title":"Rust 语言","price":45}]}}`

func outputOf(t *testing.T, expr string) (string, error) {
	t.Helper()
	m := New()
	m.ws.Input.SetValue(sampleJSON)
	m.expr.SetValue(expr)
	m.recompute()
	return m.ws.OutputText(), m.ws.Err
}

func TestEmptyStateTeachesInsteadOfShowingNothing(t *testing.T) {
	m := New()
	if out := m.ws.OutputText(); !strings.Contains(out, "Results appear here.") {
		t.Fatalf("initial result pane is blank: %q", out)
	}
}

func TestSimpleQuery(t *testing.T) {
	out, err := outputOf(t, "$.store.book[*].title")
	if err != nil {
		t.Fatalf("查询出错: %v", err)
	}
	if !strings.Contains(out, "Go 语言") || !strings.Contains(out, "Rust 语言") {
		t.Fatalf("意外输出: %q", out)
	}
	if !strings.Contains(out, "2 results") {
		t.Fatalf("期望结果计数，得到: %q", out)
	}
}

func TestFilterQuery(t *testing.T) {
	out, err := outputOf(t, "$.store.book[?(@.price > 40)].title")
	if err != nil {
		t.Fatalf("查询出错: %v", err)
	}
	if !strings.Contains(out, "Rust 语言") || strings.Contains(out, "Go 语言") {
		t.Fatalf("意外输出: %q", out)
	}
}

func TestObjectResultPrettyJSON(t *testing.T) {
	out, err := outputOf(t, "$.store.book[0]")
	if err != nil {
		t.Fatalf("查询出错: %v", err)
	}
	if !strings.Contains(out, `"title"`) || !strings.Contains(out, "Go 语言") {
		t.Fatalf("期望缩进 JSON 输出: %q", out)
	}
}

func TestInvalidExpression(t *testing.T) {
	_, err := outputOf(t, "$.store.[")
	if err == nil || !strings.Contains(err.Error(), "invalid JSONPath expression") {
		t.Fatalf("期望表达式无效错误，得到: %v", err)
	}
}

func TestInvalidJSON(t *testing.T) {
	m := New()
	m.ws.Input.SetValue("{oops")
	m.expr.SetValue("$.store")
	m.recompute()
	if m.ws.Err == nil || !strings.Contains(m.ws.Err.Error(), "JSON parse failed") {
		t.Fatalf("期望 JSON 解析失败错误，得到: %v", m.ws.Err)
	}
}

func TestNoMatch(t *testing.T) {
	out, err := outputOf(t, "$.store.nothere")
	if err != nil {
		t.Fatalf("查询出错: %v", err)
	}
	if !strings.Contains(out, "No results") {
		t.Fatalf("期望无匹配提示，得到: %q", out)
	}
}

func TestMouseChangesFocus(t *testing.T) {
	m := New()
	m.Update(tea.WindowSizeMsg{Width: 68, Height: 29})
	// 纵向缝：表达式与 JSON 窗格之间留 1 空行，JSON 窗格从 Y=4 起。
	m.Update(tea.MouseMsg{X: 3, Y: 4, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.focus != focusJSON {
		t.Fatalf("input focus = %v", m.focus)
	}
	m.Update(tea.MouseMsg{X: 50, Y: 3, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.focus != focusOutput {
		t.Fatalf("output focus = %v", m.focus)
	}
	m.Update(tea.MouseMsg{X: 3, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.focus != focusExpr {
		t.Fatalf("expression focus = %v", m.focus)
	}
}

func TestClickPositionsExpressionCursor(t *testing.T) {
	m := New()
	m.Update(tea.WindowSizeMsg{Width: 68, Height: 20})
	m.expr.SetValue("abcd")
	// 面板内边距 2 列 + 字段内边距 1 列：文本从 X=3 开始，X=4 落在第 1 个字符之后。
	m.Update(tea.MouseMsg{X: 4, Y: 1, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'X'}})
	if got := m.expr.Value(); got != "aXbcd" {
		t.Fatalf("expression = %q", got)
	}
}

func TestShortScreenExpressionAndOutputTabs(t *testing.T) {
	m := New()
	m.Update(tea.WindowSizeMsg{Width: 30, Height: 6})
	if got := lipgloss.Height(m.View()); got != 6 {
		t.Fatalf("height=%d", got)
	}
	m.expr.SetValue("abcd")
	// 标签列之后的字段内边距 1 列：X = prefix+2 落在第 1 个字符之后。
	m.Update(tea.MouseMsg{X: m.expressionPrefix() + 2, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'X'}})
	if got := m.expr.Value(); got != "aXbcd" {
		t.Fatalf("expression=%q", got)
	}
	// The workspace starts on row 1; its output tab is clickable.
	x := lipgloss.Width("[*输入] ") + 1
	m.Update(tea.MouseMsg{X: x, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.focus != focusOutput {
		t.Fatal("output tab did not open")
	}
}
