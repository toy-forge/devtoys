package password

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/toy-forge/devtoys/internal/toolkit"
)

// TestGenerate 验证长度、字符类别保证、易混淆过滤与排除字符。
func TestGenerate(t *testing.T) {
	for i := 0; i < 500; i++ {
		s, err := generate(16, true, true, true, false, true, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(s) != 16 {
			t.Fatalf("长度错误: %q", s)
		}
		for _, r := range ambiguous {
			if strings.ContainsRune(s, r) {
				t.Fatalf("出现易混淆字符 %q: %s", r, s)
			}
		}
		// 符号未启用时不应出现符号字符。
		if strings.ContainsAny(s, symbolChars) {
			t.Fatalf("符号不应出现: %s", s)
		}
		// 大写/小写/数字三类都应至少出现一次。
		if !strings.ContainsAny(s, upperChars) || !strings.ContainsAny(s, lowerChars) || !strings.ContainsAny(s, digitChars) {
			t.Fatalf("字符类别缺失: %s", s)
		}
	}
}

// TestGenerateErrors 验证非法配置返回错误而不是崩溃。
func TestGenerateErrors(t *testing.T) {
	if _, err := generate(8, false, false, false, false, false, ""); err == nil {
		t.Fatal("未启用任何字符类型时应报错")
	}
	if _, err := generate(1, true, true, false, false, false, ""); err == nil {
		t.Fatal("长度小于启用类别数时应报错")
	}
	if _, err := generate(8, true, false, false, false, false, upperChars); err == nil {
		t.Fatal("排除字符清空字符集时应报错")
	}
}

// TestExclude 验证排除字符确实不会出现。
func TestExclude(t *testing.T) {
	s, err := generate(20, true, true, true, true, false, "abcdefABCDEF0123456789")
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(s, "abcdefABCDEF0123456789") {
		t.Fatalf("排除字符出现: %s", s)
	}
}

func TestShortScreenKeepsOptionsAndOutputReachable(t *testing.T) {
	m := New()
	m.Update(tea.WindowSizeMsg{Width: 30, Height: 7})
	if m.bar.Height() != 2 {
		t.Fatalf("bar height=%d", m.bar.Height())
	}
	if got := lipgloss.Height(m.View()); got != 7 {
		t.Fatalf("view height=%d", got)
	}
	// The compact workspace shows input/output tabs immediately below the bar.
	x := lipgloss.Width("[*Input] ") + 1
	m.Update(tea.MouseMsg{X: x, Y: m.bar.Height(), Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.ws.Focus != toolkit.FocusOutput {
		t.Fatal("output tab did not open")
	}
	// 面板只有一行可见，校验输出前缀确实渲染在视图里。
	visible := strings.TrimSpace(m.ws.OutputText())
	if len(visible) > 10 {
		visible = visible[:10]
	}
	if visible == "" || !strings.Contains(m.View(), visible) {
		t.Fatal("generated password not visible")
	}
}
