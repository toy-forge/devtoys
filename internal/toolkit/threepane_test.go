package toolkit

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// 三个窗格的输出/结果区在空内容时必须有空态提示，不能是一块空白面板
//（与 Workspace 的输出区保持同一语言）。
func TestThreePaneEmptyOutputShowsPlaceholder(t *testing.T) {
	p := NewThreePane("A", "B")
	p.SetSize(80, 20)
	if view := ansi.Strip(p.View()); !strings.Contains(view, "Results appear here.") {
		t.Fatalf("empty result pane has no placeholder:\n%s", view)
	}
	p.SetOutput("value")
	if view := ansi.Strip(p.View()); strings.Contains(view, "Results appear here.") {
		t.Fatalf("placeholder remained after output was set:\n%s", view)
	}
}

func TestThreePaneOutputFirstLayoutAndFocus(t *testing.T) {
	p := NewThreePane("Header", "Payload")
	p.Titles[2] = "Token"
	p.SetOutputFirst(true)
	p.SetSize(80, 18)
	if p.rects[2].Y != 0 || p.rects[0].Y <= p.rects[2].Y || p.rects[1].Y != p.rects[0].Y {
		t.Fatalf("wrong pane positions: %+v", p.rects)
	}
	p.SetOutput("signed-token")
	view := ansi.Strip(p.View())
	if !strings.Contains(view, "Token") || strings.Index(view, "Header") < strings.Index(view, "Token") || strings.Index(view, "Payload") < strings.Index(view, "Token") {
		t.Fatalf("wrong visual order:\n%s", view)
	}
	payload := p.rects[1]
	p.Update(tea.MouseMsg{X: payload.X + 2, Y: payload.Y + 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("data")})
	if p.B.Value() != "data" || p.Focus != 1 {
		t.Fatalf("payload focus=%d value=%q", p.Focus, p.B.Value())
	}
	p.Update(tea.MouseMsg{X: 2, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if p.Focus != 2 {
		t.Fatal("output pane was not clickable")
	}
}

func TestThreePaneHeightContract(t *testing.T) {
	for _, size := range [][2]int{{80, 18}, {30, 7}, {20, 7}, {60, 5}} {
		p := NewThreePane("A", "B")
		p.SetSize(size[0], size[1])
		if got := lipgloss.Height(p.View()); got != size[1] {
			t.Fatalf("size %v: height=%d view=%q", size, got, p.View())
		}
		p.SetResultBar(true)
		if got := lipgloss.Height(p.View()); got != size[1] {
			t.Fatalf("resultBar %v: height=%d", size, got)
		}
	}
}
