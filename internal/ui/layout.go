// Package ui 是 DevToys TUI 的组件库。
//
// 组件遵循仓库既有约定：
//
//   - 布局：宿主通过 SetSize/SetWidth 下发尺寸，组件在 View 时布局；
//   - 鼠标：宿主先把全局 tea.MouseMsg 平移成组件局部坐标，
//     再调用组件的 Hit/Index/Update 做命中测试；
//   - 渲染与命中共享同一份 Rect 布局，禁止两套手算算术。
//
// 不新增重型依赖，仅用 lipgloss / bubbletea / x-ansi / go-runewidth。
package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/toy-forge/devtoys/internal/theme"
)

// 间距刻度：布局算术一律引用 Space*，不写裸数字 gap/padding。
const (
	// Space1 是组内缝：Stepper 三格 gap、图标与文字之间、垂直行距、
	// 弹窗正文与按钮行之间的空行。
	Space1	= 1
	// Space2 是默认缝：状态栏 token 间距、弹窗按钮间隔、
	// 内容区左右边距、弹窗/面板内容水平内边距（每侧）。
	Space2	= 2
	// Space3 是章节缝：设置区与工作区之间的空行等章节级间隔。
	Space3	= 3
)

// IconCell 让图标占满 slot 列：不足 slot 列右补空格、超出截断。
// Nerd Font 字形宽度 1..2 不定，统一占格后标题起始列才可对齐。
func IconCell(icon string, slot int) string {
	if slot < 1 {
		slot = 1
	}
	icon = ansi.Truncate(icon, slot, "")
	if pad := slot - Width(icon); pad > 0 {
		icon += strings.Repeat(" ", pad)
	}
	return icon
}

// DimLine 压暗一行：弹窗遮罩的唯一合法通道（Modal.Render 使用）。
//
// 实现契约：背景行内嵌大量 "\x1b[0m"，只在行首注入一次
// faint+Overlay 底的话，第一个 reset 之后全部恢复原样、遮罩只剩半行。
// 必须在每个 reset 后重新注入：
//
//	DimLine(line) = prefix + strings.ReplaceAll(line, "\x1b[0m", "\x1b[0m"+prefix) + "\x1b[0m"
//	prefix        = "\x1b[2m" + theme.OverlaySGR()   // faint + Overlay 真彩底
//
// 已知限制：行内自带背景色的分段（Surface 填充等）不会被 Overlay 底色覆盖——
// 「整块底色落到 Overlay」只对无背景样式的段成立。
func DimLine(line string) string {
	prefix := "\x1b[2m" + theme.OverlaySGR()
	return prefix + strings.ReplaceAll(line, "\x1b[0m", "\x1b[0m"+prefix) + "\x1b[0m"
}

// bgSGR 返回主题色的真彩背景 SGR 序列；非十六进制色返回空串。
func bgSGR(c theme.Color) string {
	s := strings.TrimPrefix(string(c), "#")
	if len(s) != 6 {
		return ""
	}
	var r, g, b int
	if _, err := fmt.Sscanf(s, "%02x%02x%02x", &r, &g, &b); err != nil {
		return ""
	}
	return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r, g, b)
}

// withBackground 把整行铺上 bg 底色：在行首与每个 "\x1b[0m" 后重新注入
// 背景 SGR（与 DimLine 同构）。行内分段自带前景样式时，lipgloss 外层
// Background 会被内部分段的 reset 取消，因此组合行的
// 底色必须走本帮手，而不是外层样式。
func withBackground(line string, bg theme.Color) string {
	prefix := bgSGR(bg)
	if prefix == "" {
		return line
	}
	return prefix + strings.ReplaceAll(line, "\x1b[0m", "\x1b[0m"+prefix) + "\x1b[0m"
}

// WithBackground 是 withBackground 的导出形式：组件库之外的包
// （toolkit/app 的组合行，如设置行、侧栏行、工具 footer）同样受
// 「内层 reset 取消外层背景」问题域影响，用本帮手注入主题底色。
func WithBackground(line string, bg theme.Color) string	{ return withBackground(line, bg) }

// Rect 是组件坐标体系里的矩形（含 W/H 本身）。
type Rect struct {
	X, Y, W, H int
}

// Contains 报告 (x, y) 是否落在矩形内。
func (r Rect) Contains(x, y int) bool {
	return x >= r.X && y >= r.Y && x < r.X+r.W && y < r.Y+r.H
}

// Right 返回右边界外一列（X+W）。
func (r Rect) Right() int	{ return r.X + r.W }

// Bottom 返回下边界外一行（Y+H）。
func (r Rect) Bottom() int	{ return r.Y + r.H }

// Width 返回字符串的显示宽度（ANSI 感知）。
func Width(s string) int	{ return lipgloss.Width(s) }

// Height 返回字符串的行数。
func Height(s string) int	{ return lipgloss.Height(s) }

// PlaceRow 把一行单元格按 gap 列间隔水平拼接（顶部对齐），
// 返回每个单元格的矩形（局部坐标）与合成结果。
// 渲染与命中测试都应使用这份布局。
func PlaceRow(cells []string, gap int) ([]Rect, string) {
	if len(cells) == 0 {
		return nil, ""
	}
	if gap < 0 {
		gap = 0
	}
	widths := make([]int, len(cells))
	height := 1
	for i, cell := range cells {
		widths[i] = Width(cell)
		if h := Height(cell); h > height {
			height = h
		}
	}
	rects := make([]Rect, len(cells))
	rows := make([]strings.Builder, height)
	x := 0
	for i, cell := range cells {
		lines := strings.Split(cell, "\n")
		for y := range rows {
			line := ""
			if y < len(lines) {
				line = lines[y]
			}
			if pad := widths[i] - Width(line); pad > 0 {
				line += strings.Repeat(" ", pad)
			}
			rows[y].WriteString(line)
		}
		rects[i] = Rect{X: x, Y: 0, W: widths[i], H: height}
		x += widths[i]
		if i < len(cells)-1 {
			for y := range rows {
				rows[y].WriteString(strings.Repeat(" ", gap))
			}
			x += gap
		}
	}
	out := make([]string, height)
	for y := range rows {
		out[y] = rows[y].String()
	}
	return rects, strings.Join(out, "\n")
}

// JoinRow 是只关心渲染结果的 PlaceRow。
func JoinRow(cells []string, gap int) string {
	_, s := PlaceRow(cells, gap)
	return s
}

// PlaceCol 把一列单元格按 gap 行间隔垂直拼接（左对齐），
// 返回每个单元格的矩形与合成结果。
func PlaceCol(cells []string, gap int) ([]Rect, string) {
	if len(cells) == 0 {
		return nil, ""
	}
	if gap < 0 {
		gap = 0
	}
	width := 1
	offsets := make([]int, len(cells))
	heights := make([]int, len(cells))
	y := 0
	for i, cell := range cells {
		offsets[i] = y
		heights[i] = Height(cell)
		if w := Width(cell); w > width {
			width = w
		}
		y += heights[i]
		if i < len(cells)-1 {
			y += gap
		}
	}
	rects := make([]Rect, len(cells))
	rows := make([]string, 0, y)
	for i, cell := range cells {
		for _, line := range strings.Split(cell, "\n") {
			if pad := width - Width(line); pad > 0 {
				line += strings.Repeat(" ", pad)
			}
			rows = append(rows, line)
		}
		rects[i] = Rect{X: 0, Y: offsets[i], W: width, H: heights[i]}
		if i < len(cells)-1 {
			for range gap {
				rows = append(rows, strings.Repeat(" ", width))
			}
		}
	}
	return rects, strings.Join(rows, "\n")
}

// JoinCol 是只关心渲染结果的 PlaceCol。
func JoinCol(cells []string, gap int) string {
	_, s := PlaceCol(cells, gap)
	return s
}

// Fill 把内容补齐到 width×height：宽度不足补空格，高度不足补空行，
// 超出宽度按显示宽度截断。
func Fill(s string, width, height int) string {
	if width < 0 || height < 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if w := Width(line); w > width {
			lines[i] = ansi.Truncate(line, width, "")
		} else if pad := width - w; pad > 0 {
			lines[i] = line + strings.Repeat(" ", pad)
		}
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", max(width, 0)))
	}
	return strings.Join(lines[:max(0, min(len(lines), height))], "\n")
}

// Center 计算把 w×h 的盒子放进 areaW×areaH 区域时的居中偏移，
// 盒子大于区域时钳制到 0（贴左上角）。
func Center(w, h, areaW, areaH int) (x, y int) {
	if areaW > w {
		x = (areaW - w) / 2
	}
	if areaH > h {
		y = (areaH - h) / 2
	}
	return max(0, x), max(0, y)
}

// isPressLeft 统一"左键按下"的判定。
func isPressLeft(msg tea.Msg) (tea.MouseMsg, bool) {
	mouse, ok := msg.(tea.MouseMsg)
	if !ok {
		return mouse, false
	}
	return mouse, mouse.Action == tea.MouseActionPress && mouse.Button == tea.MouseButtonLeft
}

// skipCells 跳过字符串前 n 个显示单元格（ANSI 感知）。
func skipCells(s string, n int) string {
	if n <= 0 {
		return s
	}
	return ansi.TruncateLeft(s, n, "")
}

// HitIn 判定一次左键按下是否落在 rect（已平移到组件局部坐标）内。
func HitIn(msg tea.Msg, r Rect) bool {
	mouse, ok := isPressLeft(msg)
	return ok && r.Contains(mouse.X, mouse.Y)
}
