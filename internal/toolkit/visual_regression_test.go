package toolkit

// 回归测试：单焦点环、组标题、设置行整行 Surface。

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/toy-forge/devtoys/internal/theme"
)

const (
	accentSGR	= "38;2;47;129;247"	// Accent #2F81F7
	borderSGR	= "38;2;48;54;60"	// Border #30363D（termenv 渲染实际值，蓝通道 -1）
	textSGR		= "38;2;230;237;243"	// Text #E6EDF3
	blueBrightSGR	= "38;2;121;192;255"	// BlueBright #79C0FF
	surfaceBGSGR	= "48;2;22;27;34"	// Surface #161B22
)

func forceTrueColor(t *testing.T) {
	t.Helper()
	old := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })
	lipgloss.SetColorProfile(termenv.TrueColor)
}

func TestSingleFocusRingWhileInputFocused(t *testing.T) {
	prev := theme.Dark()
	defer theme.SetDark(prev)
	theme.SetDark(true)
	forceTrueColor(t)
	w := NewWorkspace("", "out")
	w.Update(tea.WindowSizeMsg{Width: 80, Height: 16})
	if w.Focus != FocusInput {
		t.Fatalf("focus = %v", w.Focus)
	}
	if strings.Contains(w.renderOutputPane(), accentSGR) {
		t.Fatal("output pane must not carry the Accent focus ring while input holds focus")
	}
	if !strings.Contains(w.renderInputPane(), accentSGR) {
		t.Fatal("input pane must carry the Accent focus ring")
	}
	// 切到输出后：单环换边。
	w.toggleFocus()
	if !strings.Contains(w.renderOutputPane(), accentSGR) {
		t.Fatal("output pane must carry the ring when focused")
	}
	if strings.Contains(w.renderInputPane(), accentSGR) {
		t.Fatal("input pane must lose the ring when output focused")
	}
}

func TestSettingsGroupTitleUsesTitleStyleAndHairlineCorner(t *testing.T) {
	prev := theme.Dark()
	defer theme.SetDark(prev)
	theme.SetDark(true)
	forceTrueColor(t)
	var h SettingHeader
	h.Names = []string{"Hashing Algorithm", "Uppercase"}
	h.Descriptions = []string{"Select which hashing algorithm you want to use"}
	h.Icons = []string{"x"}
	h.Switches = []bool{false, true}
	h.Groups = []SettingGroup{{Title: "Hashing Algorithm", Icon: "x", Primary: 0, Children: []int{1}}}
	w := NewWorkspace("", "")
	h.Resize(tea.WindowSizeMsg{Width: 60, Height: 20}, &w, "MD5", "Off")
	view := h.View("MD5", "Off")
	if strings.Contains(view, blueBrightSGR) {
		t.Fatal("group title must not use BlueBright")
	}
	if !strings.Contains(view, textSGR) {
		t.Fatal("group title must be Text (TitleStyle)")
	}
	// 设置组不画边框，而是缩进+标题样式；设置行整行铺 Surface 底。
	if !strings.Contains(view, surfaceBGSGR) {
		t.Fatal("setting rows must carry Surface bg")
	}
}
