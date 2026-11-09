// button.go 是真实按钮控件。
//
// 状态矩阵：语义三元组贯穿所有变体——
// 填充色 = 可交互；描边色 = 容器；Accent 反色填充 = 当前唯一最重状态。
// Focused 优先于 Hover。
package ui

import (
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/toy-forge/devtoys/internal/theme"
)

// Variant 是按钮的语义变体。
type Variant uint8

const (
	// VariantGhost 是默认的低调按钮（Control 底、BorderStrong 边框）。
	VariantGhost	Variant	= iota
	// VariantPrimary 是主要操作按钮（Accent 填充）。
	VariantPrimary
	// VariantDanger 是危险操作按钮（Danger 描边/填充）。
	VariantDanger
)

// Shape 控制带边框按钮的边框形状。
type Shape uint8

const (
	// ShapeRounded 是圆角边框（默认）。
	ShapeRounded	Shape	= iota
	// ShapeSquare 是方角边框。
	ShapeSquare
)

// Button 是按钮控件。
//
// 两种形态：
//   - 带边框（Inline=false，默认）：3 行高，圆角或方角边框，左右内边距
//     各 2 列（总宽 = label+6），用于确认弹窗等；
//   - 内联胶囊（Inline=true）：1 行高，Control 底色，总宽 = label+2（冻结），
//     用于工具栏与窗格标题行。
type Button struct {
	Label		string
	Variant		Variant
	Shape		Shape
	Focused		bool
	Disabled	bool
	// Hover 表示指针悬停（由宿主的 hover 管线写入；Focused 优先）。
	Hover	bool
	// Inline 选择 1 行胶囊形态（工具栏用）。
	Inline	bool
	// MinWidth 指定最小渲染宽度（含边框）；0 表示按内容自适应。
	MinWidth	int
}

// borderedBorder 返回按钮使用的边框。
func (b Button) borderedBorder() lipgloss.Border {
	if b.Shape == ShapeSquare {
		return lipgloss.NormalBorder()
	}
	return lipgloss.RoundedBorder()
}

// View 渲染按钮。宽度按 lipgloss.Width 精确计入，可用 Button.Width 复算。
// 标签超出可用宽度时截断为 …（不依赖 lipgloss 的 Width 换行）。
func (b Button) View() string {
	width := b.Width()
	decor := 2	// 内联：左右各 1 列内边距
	if !b.Inline {
		decor = 6	// 带边框：左右各 2 列内边距 + 2 列边框
	}
	content := max(1, width-decor)
	label := ansi.Truncate(b.Label, content, "…")
	if pad := content - Width(label); pad > 0 {
		label += strings.Repeat(" ", pad)
	}
	var s lipgloss.Style
	if b.Inline {
		s = lipgloss.NewStyle().Padding(0, 1)
		switch {
		case b.Disabled:
			if b.Variant == VariantPrimary {
				// Primary disabled 是灰填充（非描边），文案保持反色可读。
				s = s.Foreground(theme.CursorText).Background(theme.Muted).Faint(true)
			} else {
				s = theme.DisabledStyle().Background(theme.Surface).Padding(0, 1)
			}
		case b.Focused:	// active：反色填充（唯一最重状态）
			switch b.Variant {
			case VariantPrimary:
				s = s.Foreground(theme.CursorText).Background(theme.AccentHover).Bold(true)
			case VariantDanger:
				s = s.Foreground(theme.CursorText).Background(theme.Danger).Bold(true)
			default:
				s = s.Foreground(theme.CursorText).Background(theme.Accent).Bold(true)
			}
		case b.Hover:
			switch b.Variant {
			case VariantPrimary:
				s = s.Foreground(theme.CursorText).Background(theme.AccentHover)
			case VariantDanger:
				s = s.Foreground(theme.Danger).Background(theme.ControlHover)
			default:
				s = s.Foreground(theme.Text).Background(theme.ControlHover)
			}
		case b.Variant == VariantPrimary:
			s = s.Foreground(theme.CursorText).Background(theme.Accent)
		case b.Variant == VariantDanger:
			s = s.Foreground(theme.Danger).Background(theme.Control)
		default:
			s = s.Foreground(theme.Text).Background(theme.Control)
		}
	} else {
		border := b.borderedBorder()
		switch {
		case b.Disabled:
			s = theme.DisabledStyle().
				Border(border).BorderForeground(theme.Border)
			if b.Variant == VariantPrimary {
				s = s.Background(theme.Surface)
			}
		case b.Focused:
			switch b.Variant {
			case VariantPrimary:
				s = lipgloss.NewStyle().
					Border(border).BorderForeground(theme.AccentHover).
					Foreground(theme.CursorText).Background(theme.AccentHover).Bold(true)
			case VariantDanger:
				s = lipgloss.NewStyle().
					Border(border).BorderForeground(theme.BorderFocus).
					Foreground(theme.CursorText).Background(theme.Danger).Bold(true)
			default:
				s = lipgloss.NewStyle().
					Border(border).BorderForeground(theme.BorderFocus).
					Foreground(theme.Text).Background(theme.SurfaceHigh).Bold(true)
			}
		case b.Hover:
			switch b.Variant {
			case VariantPrimary:
				s = lipgloss.NewStyle().
					Border(border).BorderForeground(theme.AccentHover).
					Foreground(theme.CursorText).Background(theme.AccentHover).Bold(true)
			case VariantDanger:
				s = lipgloss.NewStyle().
					Border(border).BorderForeground(theme.Danger).
					Foreground(theme.Danger).Background(theme.ControlHover)
			default:
				s = lipgloss.NewStyle().
					Border(border).BorderForeground(theme.BorderStrong).
					Foreground(theme.Text).Background(theme.ControlHover)
			}
		case b.Variant == VariantPrimary:
			s = lipgloss.NewStyle().
				Border(border).BorderForeground(theme.Accent).
				Foreground(theme.CursorText).Background(theme.Accent).Bold(true)
		case b.Variant == VariantDanger:
			s = lipgloss.NewStyle().
				Border(border).BorderForeground(theme.Danger).
				Foreground(theme.Danger).Background(theme.Control)
		default:
			s = lipgloss.NewStyle().
				Border(border).BorderForeground(theme.BorderStrong).
				Foreground(theme.Text).Background(theme.Control)
		}
	}
	if !b.Inline {
		s = s.Padding(0, Space2)	// 带边框形态：每侧 2 列内边距（总宽 label+6）
	}
	return s.Render(label)
}

// Width 返回按钮渲染后的显示宽度（等价于 lipgloss.Width(b.View())，但不渲染）。
// 内联 = label+2（冻结）；带边框 = label+6（每侧 2 列内边距 + 2 列边框）。
// MinWidth > 0 时为精确宽度：标签过长会被截断为 …。
func (b Button) Width() int {
	if b.MinWidth > 0 {
		return b.MinWidth
	}
	if b.Inline {
		return Width(b.Label) + 2
	}
	return Width(b.Label) + 6
}

// Height 返回按钮渲染后的行数：内联 1，带边框 3。
func (b Button) Height() int {
	if b.Inline {
		return 1
	}
	return 3
}

// Rect 返回按钮绘制在 (offsetX, offsetY) 时的命中矩形。
func (b Button) Rect(offsetX, offsetY int) Rect {
	return Rect{X: offsetX, Y: offsetY, W: b.Width(), H: b.Height()}
}

// Hit 判定一次鼠标事件是否点中绘制在 (offsetX, offsetY) 的按钮。
// msg 应当已经是按钮所属区域的局部坐标。
func (b Button) Hit(msg tea.Msg, offsetX, offsetY int) bool {
	if b.Disabled {
		return false
	}
	return HitIn(msg, b.Rect(offsetX, offsetY))
}
