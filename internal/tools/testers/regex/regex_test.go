package regex

import (
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const sampleText = `邮箱: alice@example.com 与 bob@test.org；电话: 12345678。`

// ansiRe 匹配 ANSI 转义序列。着色输出依赖 lipgloss 对终端能力的检测，
// 在 CLICOLOR_FORCE=1 等环境下高亮会带转义序列，断言前先剥离，
// 使测试不因运行环境有无 TTY 而结果不同。
var ansiRe = regexp.MustCompile("\x1b(?:\\[[0-9;:]*[ -/]*[@-~]|\\][^\x07\x1b]*(?:\x07|\x1b\\\\))")

func plain(s string) string	{ return ansiRe.ReplaceAllString(s, "") }

func newTestModel(t *testing.T, pattern string) *model {
	t.Helper()
	m := New()
	m.ws.Input.SetValue(sampleText)
	m.pattern.SetValue(pattern)
	m.recompute()
	return m
}

func TestMatchAndHighlight(t *testing.T) {
	m := newTestModel(t, `\w+@\w+\.\w+`)
	out := plain(m.ws.OutputText())
	if m.ws.Err != nil {
		t.Fatalf("意外错误: %v", m.ws.Err)
	}
	if !strings.Contains(out, "2 matches") {
		t.Fatalf("期望 2 处匹配，得到: %q", out)
	}
	if !strings.Contains(out, "alice@example.com") || !strings.Contains(out, "bob@test.org") {
		t.Fatalf("缺少匹配内容: %q", out)
	}
	if !strings.Contains(out, "4..21") {
		t.Fatalf("期望 UTF-16 匹配区间信息: %q", out)
	}
}

func TestGroupCapture(t *testing.T) {
	m := newTestModel(t, `(\w+)@(\w+)\.com`)
	out := plain(m.ws.OutputText())
	if !strings.Contains(out, "Group 1: alice") || !strings.Contains(out, "Group 2: example") {
		t.Fatalf("期望分组捕获输出: %q", out)
	}
}

func TestGlobalOff(t *testing.T) {
	m := newTestModel(t, `\w+@\w+\.\w+`)
	m.global = false
	m.recompute()
	if !strings.Contains(plain(m.ws.OutputText()), "1 matches") {
		t.Fatalf("关闭全局后应只匹配第一处: %q", m.ws.OutputText())
	}
}

func TestNamedGroup(t *testing.T) {
	m := newTestModel(t, `(?<user>\w+)@\w+`)
	out := plain(m.ws.OutputText())
	if !strings.Contains(out, "Group 1 (user): alice") {
		t.Fatalf("期望命名分组输出: %q", out)
	}
}

func TestInvalidPattern(t *testing.T) {
	m := newTestModel(t, `[a-`)
	if m.ws.Err == nil || !strings.Contains(m.ws.Err.Error(), "invalid regular expression") {
		t.Fatalf("期望正则无效错误，得到: %v", m.ws.Err)
	}
}

func TestNoMatch(t *testing.T) {
	m := newTestModel(t, `xyz`)
	if !strings.Contains(plain(m.ws.OutputText()), "No matches.") {
		t.Fatalf("期望无匹配提示: %q", m.ws.OutputText())
	}
}

func TestEmptyPattern(t *testing.T) {
	m := newTestModel(t, "")
	if m.ws.Err != nil || !strings.Contains(plain(m.ws.OutputText()), "Enter a regular expression to match") {
		t.Fatalf("期望空模式提示: %q / %v", m.ws.OutputText(), m.ws.Err)
	}
}

func TestMouseTogglesGlobal(t *testing.T) {
	m := New()
	m.Update(tea.WindowSizeMsg{Width: 68, Height: 20})
	x := lipgloss.Width("Regex (.NET)  ") + 1
	m.Update(tea.MouseMsg{X: x, Y: 0, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if m.global {
		t.Fatal("global should be off")
	}
}

func TestShortScreenGlobalAndExpression(t *testing.T) {
	m := New()
	m.Update(tea.WindowSizeMsg{Width: 30, Height: 6})
	if got := lipgloss.Height(m.View()); got != 6 {
		t.Fatalf("height=%d", got)
	}
	m.Update(tea.MouseMsg{X: 1, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.global {
		t.Fatal("global toggle did not respond")
	}
	m.pattern.SetValue("abcd")
	m.Update(tea.MouseMsg{X: m.expressionPrefix() + 3, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'X'}})
	if got := m.pattern.Value(); got != "aXbcd" {
		t.Fatalf("pattern=%q", got)
	}
}
