// tabs.go 是单行 Tab 条：
// 激活项 = 浏览器标签语义（Text on Control + bold），非激活 fg=Muted，
// 支持鼠标命中。
package ui

import (
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/toy-forge/devtoys/internal/theme"
)

// Tabs 是单行 Tab 条。
type Tabs struct {
	// Items 是各 Tab 的标题。
	Items	[]string
	// Index 是激活的 Tab。
	Index	int

	width	int
}

// NewTabs 创建 Tab 条。
func NewTabs(items ...string) *Tabs {
	return &Tabs{Items: items}
}

// SetWidth 记录可用宽度。
func (t *Tabs) SetWidth(width int)	{ t.width = max(0, width) }

// SetIndex 切换激活 Tab。
func (t *Tabs) SetIndex(i int)	{ t.Index = i }

// tabWidth 计算第 i 个 Tab 渲染宽度（含两侧 1 列内边距）。
func (t *Tabs) tabWidth(i int) int {
	if i < 0 || i >= len(t.Items) {
		return 0
	}
	return Width(t.Items[i]) + 2
}

// Height 返回占用行数（恒为 1，严格执行单行合同）。
func (t *Tabs) Height() int	{ return 1 }

// View 渲染 Tab 条：激活项 " Name "（Control 底 + 正文色加粗），非激活项 fg=Muted，
// 以 Border 色的 "│" 分隔。宽度不足时整行截断。
func (t *Tabs) View() string {
	var parts []string
	for i, item := range t.Items {
		if i > 0 {
			parts = append(parts, lipgloss.NewStyle().Foreground(theme.Border).Render("│"))
		}
		if i == t.Index {
			parts = append(parts, lipgloss.NewStyle().Foreground(theme.Text).Background(theme.Control).Bold(true).Padding(0, 1).Render(item))
		} else {
			parts = append(parts, lipgloss.NewStyle().Foreground(theme.Muted).Padding(0, 1).Render(item))
		}
	}
	line := strings.Join(parts, "")
	width := t.width
	if width > 0 && Width(line) > width {
		line = ansi.Truncate(line, width, "…")
	}
	return line
}

// Rect 返回第 i 个 Tab 最近一次渲染中的矩形（局部坐标，Y=0）。
// 渲染与命中共享同一份布局；View 会整行截断到 SetWidth 的宽度，
// 因此矩形同样按该宽度钳制：被截掉不可见的部分不参与命中。
func (t *Tabs) Rect(i int) Rect {
	if i < 0 || i >= len(t.Items) {
		return Rect{}
	}
	x := 0
	for j := range t.Items {
		if j > 0 {
			x++	// 分隔符 "│"
		}
		w := t.tabWidth(j)
		if j == i {
			if t.width > 0 && x >= t.width {
				return Rect{}	// 整个 Tab 已被截断，不可见也不可命中
			}
			r := Rect{X: x, Y: 0, W: w, H: 1}
			if t.width > 0 && x+w > t.width {
				r.W = t.width - x	// 只保留截断后可见的部分
			}
			return r
		}
		x += w
	}
	return Rect{}
}

// Hit 返回点击的 Tab 下标；未命中返回 -1。
func (t *Tabs) Hit(msg tea.Msg) int {
	mouse, press := isPressLeft(msg)
	if !press {
		return -1
	}
	if t.width > 0 && mouse.X >= t.width {
		return -1
	}
	for i := range t.Items {
		if t.Rect(i).Contains(mouse.X, mouse.Y) {
			return i
		}
	}
	return -1
}
