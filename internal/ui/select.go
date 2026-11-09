// select.go 是自研下拉框。
package ui

import (
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/toy-forge/devtoys/internal/theme"
)

// Select 是下拉框控件。
//
// 关闭态：带边框的值 chip + ▾（单行，渲染在宿主给定的位置）。
// 打开态：浮动列表弹层，宿主用 OverlayRect 求得放置位置、
// 用 Modal/Overlay 帮手把 OpenView 叠加到界面上。
// 键盘 ↑↓/enter/esc，鼠标点击 chip 开合、点击选项选择、滚轮滚动。
type Select struct {
	// Options 是全部可选项（显示文本即取值）。
	Options	[]string
	// Index 是当前选中项。
	Index	int
	// Open 报告下拉列表是否展开。
	Open	bool
	// Cursor 是展开时高亮的选项（默认跟随 Index）。
	Cursor	int
	// Width 是关闭态 chip 的最小宽度；0 表示按最长选项自适应。
	Width	int
	// ListHeight 是展开时最多显示的选项行数；默认 8。
	ListHeight	int
	// Disabled 变暗并忽略输入。
	Disabled	bool
	// Focused/Hover 是键盘焦点与指针悬停（Focused 优先）。
	Focused	bool
	Hover	bool

	offset	int
}

// Selected 返回当前选中的文本；无选项时返回空串。
func (s *Select) Selected() string {
	if s.Index < 0 || s.Index >= len(s.Options) {
		return ""
	}
	return s.Options[s.Index]
}

// SetValue 按文本设置选中项；不存在时不动。
func (s *Select) SetValue(value string) {
	for i, option := range s.Options {
		if option == value {
			s.Index = i
			s.Cursor = i
			return
		}
	}
}

func (s *Select) listHeight() int {
	if s.ListHeight > 0 {
		return s.ListHeight
	}
	return 8
}

// chipWidth 计算关闭态 chip 的宽度。
func (s *Select) chipWidth() int {
	longest := 0
	for _, option := range s.Options {
		if w := Width(option); w > longest {
			longest = w
		}
	}
	width := max(longest, s.Width) + Width(" ▾") + 2	// 两侧各 1 列内边距
	return max(3, width)
}

// View 渲染关闭态 chip（单行）。chipWidth 公式冻结（宿主命中依赖），
// 分两段着色：值段 fg=Text，"▾" 段 fg=Muted；hover 抬升 ControlHover，
// focused 整块 Accent 填充并加 bold。
func (s *Select) View() string {
	value := s.Selected()
	if len(s.Options) == 0 {
		value = "—"
	}
	inner := max(1, s.chipWidth()-4)	// 去掉 " " + " ▾ " 前后装饰后的正文宽度
	value = ansi.Truncate(value, inner, "…")
	if pad := inner - Width(value); pad > 0 {
		value += strings.Repeat(" ", pad)
	}
	if s.Disabled {
		body := " " + value + " ▾ "
		return lipgloss.NewStyle().Foreground(theme.Muted).Background(theme.Surface).Faint(true).Render(body)
	}
	// 控件填充（Control）是可交互的视觉信号；hover 抬一档（ControlHover）；
	// 键盘焦点与按钮的 active 态一致，整块 Accent 反色填充（唯一最重状态）。
	bg := theme.Control
	valueFg, arrowFg := theme.Text, theme.Muted
	switch {
	case s.Focused:
		bg, valueFg, arrowFg = theme.Accent, theme.CursorText, theme.CursorText
	case s.Hover:
		bg = theme.ControlHover
	}
	valueSeg := lipgloss.NewStyle().
		Foreground(valueFg).Bold(s.Focused).
		Background(bg).
		Render(" " + value + " ")
	arrowSeg := lipgloss.NewStyle().
		Foreground(arrowFg).
		Background(bg).
		Render("▾ ")
	return valueSeg + arrowSeg
}

// ChipRect 返回 chip 绘制在 (offsetX, offsetY) 时的矩形。
func (s *Select) ChipRect(offsetX, offsetY int) Rect {
	return Rect{X: offsetX, Y: offsetY, W: s.chipWidth(), H: 1}
}

// OpenView 渲染展开的浮层列表（带边框圆角盒子，含边框共 2 列开销）。
// 菜单是 E2 浮层：border=BorderStrong（焦点环只属于
// 焦点容器）。选项数超过可视行数时，右侧加 2 列滚动指示
// （1 空列 + 轨道列：│=轨道、▐=thumb，fg=Border/Muted），
// 加宽经由 OpenView()→OverlayRect()→ListHit() 同一份推导，无外部命中依赖。
func (s *Select) OpenView() string {
	if !s.Open || len(s.Options) == 0 {
		return ""
	}
	visible := min(len(s.Options), s.listHeight())
	if s.Cursor < s.offset {
		s.offset = s.Cursor
	}
	if s.Cursor >= s.offset+visible {
		s.offset = s.Cursor - visible + 1
	}
	width := 0
	for _, option := range s.Options {
		if w := Width(option); w > width {
			width = w
		}
	}
	inner := width + 4	// 两列缩进 + "· " 光标标记
	scroll := len(s.Options) > visible
	thumbY := s.offset + min(visible-1, s.offset*visible/max(1, len(s.Options)))
	body := make([]string, 0, visible)
	for i := s.offset; i < s.offset+visible; i++ {
		line := "  " + s.Options[i]
		var main string
		if i == s.Cursor {
			line = "· " + s.Options[i]
			main = lipgloss.NewStyle().Foreground(theme.CursorText).Background(theme.Accent).Bold(true).Width(inner).Render(line)
		} else {
			main = lipgloss.NewStyle().Foreground(theme.Text).Width(inner).Render(line)
		}
		if scroll {
			ch, fg := "│", theme.Border
			if i == thumbY {
				ch, fg = "▐", theme.Muted
			}
			sep := lipgloss.NewStyle().Background(theme.SurfaceHigh).Render(" ")
			ind := lipgloss.NewStyle().Foreground(fg).Background(theme.SurfaceHigh).Render(ch)
			body = append(body, main+sep+ind)
			continue
		}
		body = append(body, main)
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.BorderStrong).
		Background(theme.SurfaceHigh).
		Render(strings.Join(body, "\n"))
}

// OverlayRect 计算展开列表的放置矩形：优先贴在 chip 正下方，
// 底部放不下时翻转到 chip 上方，水平方向钳制在区域内。
// chipX/chipY 是 chip 的局部位置，areaW/areaH 是宿主区域大小。
func (s *Select) OverlayRect(chipX, chipY, areaW, areaH int) Rect {
	view := s.OpenView()
	w := min(Width(view), max(1, areaW))
	h := min(Height(view), max(1, areaH))
	x := max(0, min(chipX, areaW-w))
	y := chipY + 1
	if y+h > areaH && chipY-h >= 0 {
		y = chipY - h
	}
	return Rect{X: x, Y: max(0, y), W: w, H: h}
}

// ListHit 在列表展开时判定一次点击命中了哪个选项。
// 参数与 OverlayRect 一致；返回 ok=false 表示未点中任何选项。
func (s *Select) ListHit(msg tea.Msg, chipX, chipY, areaW, areaH int) (option int, ok bool) {
	mouse, press := isPressLeft(msg)
	if !press || !s.Open {
		return -1, false
	}
	r := s.OverlayRect(chipX, chipY, areaW, areaH)
	if !r.Contains(mouse.X, mouse.Y) {
		return -1, false
	}
	inner := mouse.Y - r.Y - 1	// 减去上边框
	if inner < 0 || inner >= r.H-2 {
		return -1, false
	}
	return s.offset + inner, true
}

// HandleMouse 处理鼠标事件（局部坐标）：
//   - 关闭态：点击 chip 展开并返回 true；
//   - 展开态：点击选项选中并收起返回 true；点击 chip 收起返回 true；
//     点击列表外不消费（返回 false，宿主可继续处理该事件）；
//   - 滚轮在展开时滚动选项。
func (s *Select) HandleMouse(msg tea.Msg, chipX, chipY, areaW, areaH int) bool {
	if s.Disabled {
		return false
	}
	mouse, ok := msg.(tea.MouseMsg)
	if !ok {
		return false
	}
	if s.Open {
		switch mouse.Button {
		case tea.MouseButtonWheelUp:
			s.Cursor = max(0, s.Cursor-1)
			return true
		case tea.MouseButtonWheelDown:
			s.Cursor = min(len(s.Options)-1, s.Cursor+1)
			return true
		}
		if option, hit := s.ListHit(msg, chipX, chipY, areaW, areaH); hit {
			s.Index = option
			s.Cursor = option
			s.Close()
			return true
		}
		if pressed, ok := isPressLeft(msg); ok && s.ChipRect(chipX, chipY).Contains(pressed.X, pressed.Y) {
			s.Close()
			return true
		}
		return false
	}
	if pressed, ok := isPressLeft(msg); ok && s.ChipRect(chipX, chipY).Contains(pressed.X, pressed.Y) {
		s.Cursor = s.Index
		s.Open = true
		return true
	}
	return false
}

// HandleKey 处理一个按键（key 为 tea.KeyMsg.String()）。
// 返回 (changed, consumed)：changed 表示选中项变化；consumed 表示按键已被
// 组件吃掉（宿主不应继续处理——esc 分层语义由本组件保证）。
func (s *Select) HandleKey(key string) (changed, consumed bool) {
	if s.Disabled {
		return false, false
	}
	if !s.Open {
		switch key {
		case "enter", " ", "down":
			if len(s.Options) == 0 {
				return false, false
			}
			s.Cursor = s.Index
			s.Open = true
			return false, true
		}
		return false, false
	}
	switch key {
	case "up", "k":
		s.Cursor = max(0, s.Cursor-1)
	case "down", "j":
		s.Cursor = min(len(s.Options)-1, s.Cursor+1)
	case "home":
		s.Cursor = 0
	case "end":
		s.Cursor = len(s.Options) - 1
	case "pgup":
		s.Cursor = max(0, s.Cursor-s.listHeight())
	case "pgdown":
		s.Cursor = min(len(s.Options)-1, s.Cursor+s.listHeight())
	case "enter":
		if s.Cursor != s.Index {
			s.Index = s.Cursor
			changed = true
		}
		s.Close()
	case "esc":
		s.Close()
	default:
		return false, false
	}
	return changed, true
}

// Close 收起下拉列表。
func (s *Select) Close() {
	s.Open = false
	s.offset = 0
}

// Overlay 帮手把浮层内容 content 叠加渲染在 background 的 r 位置上。
// background 应当已经按区域尺寸补齐。OpenView 与 Modal 共用此定位逻辑。
func Overlay(background, content string, r Rect) string {
	if content == "" {
		return background
	}
	lines := strings.Split(background, "\n")
	overlay := strings.Split(content, "\n")
	for y := 0; y < r.H && r.Y+y < len(lines); y++ {
		base := ansi.Truncate(lines[r.Y+y], r.X, "")
		base += strings.Repeat(" ", max(0, r.X-Width(base)))
		piece := ansi.Truncate(overlay[y], r.W, "")
		if pad := r.W - Width(piece); pad > 0 {
			piece += strings.Repeat(" ", pad)
		}
		tail := skipCells(lines[r.Y+y], r.X+r.W)
		lines[r.Y+y] = base + piece + tail
	}
	return strings.Join(lines, "\n")
}
