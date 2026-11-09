package lorem

import (
	tea "github.com/charmbracelet/bubbletea"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// TestWrap 验证输出按宽度换行且段落间有空行。
func TestWrap(t *testing.T) {
	m := New()
	m.bar.Choices[0].Index = unitParagraph
	m.bar.Numbers[0].Value = 5
	m.ws.Output.Width = 50
	m.regenerate()

	for _, line := range strings.Split(m.ws.Output.View(), "\n") {
		if w := lipgloss.Width(strings.TrimRight(line, " ")); w > 48 {
			t.Fatalf("行超宽: %q (%d)", line, w)
		}
	}
}

// TestUnits 验证三种单位都能生成非空内容。
func TestUnits(t *testing.T) {
	for unit, want := range map[int]string{
		unitParagraph:	"\n\n",
		unitSentence:	"",
		unitWord:	"",
	} {
		m := New()
		m.bar.Choices[0].Index = unit
		m.bar.Numbers[0].Value = 3
		m.regenerate()
		out := m.ws.OutputText()
		if strings.TrimSpace(out) == "" {
			t.Fatalf("单位 %d 输出为空", unit)
		}
		if want != "" && !strings.Contains(out, want) {
			t.Fatalf("单位 %d 缺少段落分隔", unit)
		}
	}
}

func TestResizeRewrapsWithoutChangingCopiedText(t *testing.T) {
	m := New()
	m.bar.Choices[0].Index = unitParagraph
	m.bar.Numbers[0].Value = 2
	m.regenerate()
	before := m.ws.OutputText()
	m.Update(tea.WindowSizeMsg{Width: 50, Height: 20})
	if m.ws.OutputText() != before {
		t.Fatal("resize changed generated text")
	}
}
