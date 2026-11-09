// switch.go 是 pill 式开关控件。
//
// 状态矩阵：on = Accent 填充 regular（焦点时加 bold），
// off = Control 底（hover/focused 抬升 ControlHover，焦点加 bold）。
// 字形与宽度公式冻结：两态等宽、含 ●/○。
package ui

import (
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/toy-forge/devtoys/internal/theme"
)

// Switch 是 pill 式开关：On 为 Accent 填充 + ●，Off 为 Control + ○。
// 两种状态的渲染宽度一致，切换不会引起布局抖动。
type Switch struct {
	// On 是当前状态。
	On	bool
	// Disabled 变暗并忽略命中。
	Disabled	bool
	// Focused 表示键盘焦点（on：同色加粗；off：抬升 + 加粗）。
	Focused	bool
	// Hover 表示指针悬停（仅 off 态抬升 ControlHover；Focused 优先）。
	Hover	bool
	// OnLabel/OffLabel 可定制文案，默认 "On"/"Off"。
	OnLabel		string
	OffLabel	string
}

// Toggle 切换状态并返回新状态。
func (s *Switch) Toggle() bool {
	s.On = !s.On
	return s.On
}

// SetOn 直接设置状态。
func (s *Switch) SetOn(on bool)	{ s.On = on }

func (s Switch) labels() (on, off string) {
	on, off = s.OnLabel, s.OffLabel
	if on == "" {
		on = "On"
	}
	if off == "" {
		off = "Off"
	}
	return on, off
}

// View 渲染开关（单行 pill）。
func (s Switch) View() string {
	on, off := s.labels()
	width := max(Width(" ● "+on+" "), Width(" ○ "+off+" "))
	mark, label := "●", on
	if !s.On {
		mark, label = "○", off
	}
	if pad := width - Width(" ● "+on+" "); pad > 0 && s.On {
		label += strings.Repeat(" ", pad)
	}
	body := " " + mark + " " + label + " "
	if pad := width - Width(body); pad > 0 {
		body += strings.Repeat(" ", pad)
	}
	style := lipgloss.NewStyle().Width(width)
	switch {
	case s.Disabled:
		style = theme.DisabledStyle().Background(theme.Surface).Width(width)
	case s.On:
		// on：Accent 填充 regular；焦点用字重表达（终端唯一可靠手段）。
		style = style.Foreground(theme.CursorText).Background(theme.Accent).Bold(s.Focused)
	default:
		// off：Control 填充（可交互）；hover/focused 抬升 ControlHover，焦点加 bold。
		bg := theme.Control
		if s.Hover || s.Focused {
			bg = theme.ControlHover
		}
		style = style.Foreground(theme.Text).Background(bg).Bold(s.Focused)
	}
	return style.Render(body)
}

// Width 返回开关的显示宽度（与状态无关）。
func (s Switch) Width() int {
	on, off := s.labels()
	return max(Width(" ● "+on+" "), Width(" ○ "+off+" "))
}

// Rect 返回开关绘制在 (offsetX, offsetY) 时的矩形。
func (s Switch) Rect(offsetX, offsetY int) Rect {
	return Rect{X: offsetX, Y: offsetY, W: s.Width(), H: 1}
}

// Hit 判定一次点击是否点中绘制在 (offsetX, offsetY) 的开关。
func (s *Switch) Hit(msg tea.Msg, offsetX, offsetY int) bool {
	if s.Disabled {
		return false
	}
	return HitIn(msg, s.Rect(offsetX, offsetY))
}
