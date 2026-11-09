package jwt

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/toy-forge/devtoys/internal/toolkit"
	"github.com/toy-forge/devtoys/internal/ui"
)

func TestDecoderDesktopLayoutAndInteraction(t *testing.T) {
	tool := newJWTTool()
	tool.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if !tool.desktop.active {
		t.Fatal("desktop layout not active")
	}
	tool.Update(toolkit.FileImportMsg{Text: makeToken("secret")})
	view := ansi.Strip(tool.View())
	token, key := strings.Index(view, "Token"), strings.Index(view, "Verification key")
	header, payload := strings.Index(view, "Header"), strings.Index(view, "Payload")
	if token < 0 || key <= token || header <= key || payload <= header {
		t.Fatalf("unexpected desktop order:\n%s", view)
	}
	// 结果窗格视口较矮："sub" 落在 PAYLOAD 视口外，
	// 解码数据以 HEADER 的 "alg" 断言；"sub" 由下方 claims 表断言覆盖。
	if !strings.Contains(view, `"alg"`) {
		t.Fatalf("decoded panes missing data:\n%s", view)
	}
	tool.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}, Alt: true})
	// PAYLOAD 视口较矮：表头与 sub 行分两屏断言。
	view = ansi.Strip(tool.View())
	if !strings.Contains(view, "Claim") {
		t.Fatalf("claim table header missing:\n%s", view)
	}
	tool.Update(tea.MouseMsg{X: 75, Y: tool.headerHeight + tool.desktop.resultY + 1, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	view = ansi.Strip(tool.View())
	if !strings.Contains(view, "sub") {
		t.Fatalf("claim row missing:\n%s", view)
	}
	tool.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}, Alt: true})
	tool.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	tool.Update(toolkit.FileImportMsg{Text: "secret"})
	if !strings.Contains(ansi.Strip(tool.View()), "Signature verified") {
		t.Fatalf("verification not displayed:\n%s", ansi.Strip(tool.View()))
	}
	tool.Update(tea.MouseMsg{X: 75, Y: tool.headerHeight + tool.desktop.resultY + 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if tool.desktop.focus != 3 {
		t.Fatalf("payload focus = %d", tool.desktop.focus)
	}
	tool.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if tool.Input.Value() != makeToken("secret") {
		t.Fatal("read-only pane changed token")
	}
	tool.Update(tea.WindowSizeMsg{Width: 30, Height: 10})
	if tool.desktop.active {
		t.Fatal("compact fallback not active")
	}
	if tool.Input.Value() != makeToken("secret") || tool.secret.Value() != "secret" {
		t.Fatal("resize lost input")
	}
}

func TestDecoderDesktopGeometry(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 50, Height: 16}, {Width: 80, Height: 24}, {Width: 120, Height: 40}} {
		tool := newJWTTool()
		tool.Update(size)
		view := tool.View()
		if w, h := lipgloss.Width(view), lipgloss.Height(view); w > size.Width || h != size.Height {
			t.Fatalf("size %v: got %dx%d\n%s", size, w, h, ansi.Strip(view))
		}
	}
}

func TestDecoderDesktopToolbar(t *testing.T) {
	tool := newJWTTool()
	tool.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	tool.Update(toolkit.FileImportMsg{Text: makeToken("secret")})
	click := func(index, action int) tea.Cmd {
		// 纵向缝：密钥面板在令牌面板下 1 空行处；横向缝 Space2：
		// 右列窗格自 leftWidth+Space2 起。
		width, x, y := tool.desktop.width, 0, 0
		if index == 1 {
			y = tool.desktop.tokenHeight + 1
		}
		if index >= 2 {
			y = tool.desktop.resultY
			width = (tool.desktop.width - ui.Space2) / 2
			if index == 3 {
				x = width + ui.Space2
			}
		}
		start := decoderToolbarStart(width, index, tool.desktop.title(tool.tool, index))
		if start < 0 {
			t.Fatalf("toolbar %d unavailable", index)
		}
		x += start
		for _, label := range decoderActions(index)[:action] {
			x += lipgloss.Width("[" + label + "]")
		}
		_, cmd := tool.Update(tea.MouseMsg{X: x, Y: tool.headerHeight + y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		return cmd
	}
	click(3, 0)
	if !tool.desktop.showClaims || !strings.Contains(ansi.Strip(tool.View()), "Claim") {
		t.Fatal("claim button did not switch view")
	}
	cmd := click(1, 1)
	if cmd == nil {
		t.Fatal("open button returned no command")
	}
	if _, ok := cmd().(toolkit.OpenFileRequestMsg); !ok {
		t.Fatal("open button did not request file picker")
	}
	tool.Update(toolkit.FileImportMsg{Text: "secret"})
	if tool.secret.Value() != "secret" {
		t.Fatal("file picker did not target key")
	}
	click(1, 2)
	if tool.secret.Value() != "" || !strings.Contains(ansi.Strip(tool.View()), "No secret supplied") {
		t.Fatal("clear key did not update verification")
	}
	click(0, 2)
	if tool.Input.Value() != "" || strings.Contains(ansi.Strip(tool.View()), `"alg"`) {
		t.Fatal("clear token did not clear output")
	}
}
