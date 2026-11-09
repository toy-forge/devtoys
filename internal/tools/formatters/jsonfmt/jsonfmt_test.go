package jsonfmt

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/toy-forge/devtoys/internal/toolkit"
)

// TestFormatIndent 验证 2/4 空格缩进格式化。
func TestFormatIndent(t *testing.T) {
	const src = `{"a":1,"b":[true,null]}`

	got, err := FormatJSON(src, 2, false)
	if err != nil {
		t.Fatalf("格式化不应报错: %v", err)
	}
	want := "{\n  \"a\": 1,\n  \"b\": [\n    true,\n    null\n  ]\n}"
	if got != want {
		t.Fatalf("2 空格缩进不正确:\ngot:\n%s\nwant:\n%s", got, want)
	}

	got, err = FormatJSON(`{"a":1}`, 4, false)
	if err != nil {
		t.Fatalf("格式化不应报错: %v", err)
	}
	if got != "{\n    \"a\": 1\n}" {
		t.Fatalf("4 空格缩进不正确: %q", got)
	}
}

// TestFormatMinify 验证压缩输出。
func TestFormatMinify(t *testing.T) {
	got, err := FormatJSON("{\n  \"a\": 1,\n  \"b\": \"x y\"\n}", 2, true)
	if err != nil {
		t.Fatalf("压缩不应报错: %v", err)
	}
	if got != `{"a":1,"b":"x y"}` {
		t.Fatalf("got %q", got)
	}
}

// TestFormatUnicodeEscape 验证已转义字符不被二次改写。
func TestFormatUnicodeEscape(t *testing.T) {
	got, err := FormatJSON(`{"s":"a\"b"}`, 2, false)
	if err != nil {
		t.Fatalf("格式化不应报错: %v", err)
	}
	if !strings.Contains(got, `"a\"b"`) {
		t.Fatalf("字符串转义应原样保留: %q", got)
	}
}

// TestFormatErrors 非法 JSON 应返回带行列位置的中文错误。
func TestFormatErrors(t *testing.T) {
	got, err := FormatJSON("{\n  a: 1\n}", 2, false)
	if err == nil {
		t.Fatal("非法 JSON 应报错")
	}
	if !strings.Contains(err.Error(), "JSON syntax error") || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("错误应带行列位置，得到: %v", err)
	}
	if got != "" {
		t.Fatalf("出错时输出应为空，得到 %q", got)
	}
}

// TestLineCol 验证字节偏移到行列号的换算与越界保护。
func TestLineCol(t *testing.T) {
	cases := []struct {
		src		string
		offset		int64
		line, col	int
	}{
		{"abc", 0, 1, 1},
		{"a\nbc", 3, 2, 2},
		{"a\nbc", 99, 2, 3},	// 越界截断到文本长度
		{"abc", -5, 1, 1},	// 负偏移归零
		{"中文a", 3, 1, 4},	// 按字节计数（一个中文字符占 3 列）
	}
	for _, c := range cases {
		line, col := lineCol(c.src, c.offset)
		if line != c.line || col != c.col {
			t.Fatalf("lineCol(%q, %d) = (%d, %d), want (%d, %d)",
				c.src, c.offset, line, col, c.line, c.col)
		}
	}
}

// TestHighlight 高亮应产生 ANSI 输出，非法 JSON 时回退为空串。
func TestHighlight(t *testing.T) {
	if got := highlight("{\n  \"a\": 1\n}"); got == "" {
		t.Fatal("合法 JSON 应产生高亮输出")
	} else if !strings.Contains(got, "\"a\"") {
		t.Fatalf("高亮输出应包含原文内容: %q", got)
	}
}

// TestRenderViaTool 验证工具模型：空输入、合法输入与错误信息条。
func TestRenderViaTool(t *testing.T) {
	tl := newTool()

	tl.render()
	if tl.ws.Err != nil || tl.ws.OutputText() != "" {
		t.Fatalf("空输入应输出空串且不报错: %q %v", tl.ws.OutputText(), tl.ws.Err)
	}

	tl.ws.Input.SetValue(`{"a":1}`)
	tl.render()
	if tl.ws.Err != nil {
		t.Fatalf("合法 JSON 不应报错: %v", tl.ws.Err)
	}
	if !strings.Contains(tl.ws.OutputText(), "\"a\": 1") {
		t.Fatalf("输出应包含格式化结果: %q", tl.ws.OutputText())
	}

	tl.ws.Input.SetValue("{bad")
	tl.render()
	if tl.ws.Err == nil || !strings.Contains(tl.ws.Err.Error(), "JSON syntax error") {
		t.Fatalf("非法 JSON 应在信息条报错: %v", tl.ws.Err)
	}
}

func TestSortPreservesNumbersAndTabIndentation(t *testing.T) {
	tool := newTool()
	tool.sortProperties = true
	tool.indent = 0
	tool.ws.Input.SetValue(`{"z":9007199254740993,"a":{"z":2,"a":1}}`)
	tool.render()
	out := tool.ws.OutputText()
	if tool.ws.Err != nil || !strings.Contains(out, "\n\t\"a\"") || !strings.Contains(out, "9007199254740993") || strings.Index(out, `"a"`) > strings.Index(out, `"z"`) {
		t.Fatalf("out=%q err=%v", out, tool.ws.Err)
	}
}

func TestClearButtonRefreshesOutput(t *testing.T) {
	tool := newTool()
	tool.Update(tea.WindowSizeMsg{Width: 100, Height: 25})
	tool.ws.Input.SetValue(`{"a":1}`)
	tool.render()
	lines := strings.Split(ansi.Strip(tool.View()), "\n")
	for y, line := range lines {
		if x := strings.Index(line, "Clear"); x >= 0 {
			_, cmd := tool.Update(tea.MouseMsg{X: lipgloss.Width(line[:x]), Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
			if cmd == nil {
				t.Fatal("clear did not notify tool")
			}
			tool.Update(cmd())
			if tool.ws.Input.Value() != "" || tool.ws.OutputText() != "" {
				t.Fatalf("input=%q output=%q", tool.ws.Input.Value(), tool.ws.OutputText())
			}
			return
		}
	}
	t.Fatal("clear button not visible")
}

func TestSortedPropertiesKeepFirstDuplicateAndRawLiterals(t *testing.T) {
	src := `{"z":1e+100,"a":"<tag>","z":9,"\u0061":"discarded","arr":[{"b":"\u0062","a":-0.0}]}`
	got, err := sortJSON(src)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"a":"<tag>","arr":[{"a":-0.0,"b":"\u0062"}],"z":1e+100}`
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestLargeJSONBackgroundAndStaleResult(t *testing.T) {
	m := newTool()
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 25})
	m.ws.Input.SetValue(`{"items":[` + strings.Repeat("1,", 17000) + `1]}`)
	m.scheduleFormat()
	if m.ws.OutputText() != "" {
		t.Fatal("old output was not cleared")
	}
	_, first := m.Update(formatDue{revision: m.revision})
	if first == nil || !m.running {
		t.Fatal("background formatting not started")
	}
	m.minify = true
	m.scheduleFormat()
	m.Update(formatDue{revision: m.revision})
	if !m.queued {
		t.Fatal("latest operation not queued")
	}
	_, latest := m.Update(first().(toolkit.BackgroundToolMsg).Message)
	if latest == nil || m.ws.OutputText() != "" {
		t.Fatal("stale result applied")
	}
	m.Update(latest().(toolkit.BackgroundToolMsg).Message)
	if got := m.ws.OutputText(); strings.Contains(got, "\n") || !strings.HasPrefix(got, `{"items":[`) {
		t.Fatal("latest minified result missing")
	}
	m.ws.Input.SetValue(`{"new":true}`)
	m.scheduleFormat()
	m.Update(formatDone{revision: m.revision - 1, output: "stale", display: "stale"})
	if m.ws.OutputText() != `{"new":true}` {
		t.Fatalf("stale overwrote short result: %s", m.ws.OutputText())
	}
}

func TestInvalidInputCanBeCorrected(t *testing.T) {
	m := newTool()
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.Update(toolkit.FileImportMsg{Text: "{"})
	if m.ws.Err == nil {
		t.Fatal("missing syntax error")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("}")})
	if m.ws.Err != nil || m.ws.OutputText() != "{}" {
		t.Fatalf("could not recover: %q %v", m.ws.OutputText(), m.ws.Err)
	}
}
