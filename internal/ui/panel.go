// panel.go 是带标题的圆角边框窗格：rest 发丝线 Border，focused 时边框
// 变 BorderFocus（唯一焦点环），标题随焦点换色（focused fg=Text bold，
// 否则 fg=Muted bold）。内容区左右各 Space1 列水平内边距
// （文本区宽 = 总宽-4），标题行右端可嵌 Toolbar 按钮。渲染后返回盒子矩形。
package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/toy-forge/devtoys/internal/theme"
)

// Panel 是带标题的窗格。
type Panel struct {
	// Title 是标题（渲染为 " Title " 嵌入上边框）。
	Title	string
	// Focused 决定是否画焦点环（BorderFocus 边框 + 正文色标题）。
	Focused	bool

	toolbar	*Toolbar
	width	int
	height	int
}

// NewPanel 创建窗格。
func NewPanel(title string) *Panel {
	return &Panel{Title: title}
}

// SetSize 记录窗格总尺寸（含边框）。height 为 0 时按内容自适应。
func (p *Panel) SetSize(width, height int) {
	p.width, p.height = max(0, width), max(0, height)
}

// SetTitle 更新标题。
func (p *Panel) SetTitle(title string)	{ p.Title = title }

// SetFocused 更新焦点态。
func (p *Panel) SetFocused(focused bool)	{ p.Focused = focused }

// SetToolbar 设置标题行右端的按钮（自动转为内联形态）。
func (p *Panel) SetToolbar(buttons ...Button) {
	if p.toolbar == nil {
		p.toolbar = NewToolbar(buttons...)
		return
	}
	p.toolbar.SetButtons(buttons...)
}

// Toolbar 返回内嵌按钮条（可为 nil）。
func (p *Panel) Toolbar() *Toolbar	{ return p.toolbar }

// ContentWidth 返回文本区宽度（总宽-4：边框 2 列 + 左右各 1 列内边距）。
// 宿主（workspace/threepane）据此推导 textarea/viewport 宽度，
// 与 Render 的 Fill 预算共享同一算术。
func (p *Panel) ContentWidth() int {
	return max(1, p.outerWidth()-4)
}

func (p *Panel) outerWidth() int {
	if p.width > 0 {
		return p.width
	}
	return 80
}

// ToolbarRect 返回标题行按钮区的矩形（局部坐标，Y=0）。
// 宿主用它把全局鼠标坐标平移到按钮条局部坐标后调 Toolbar.Index。
// 空矩形表示标题行放不下按钮（与 toolbarStart<0 同语义）。
func (p *Panel) ToolbarRect() Rect {
	if p.toolbar == nil || len(p.toolbar.Buttons) == 0 {
		return Rect{}
	}
	total := 0
	for i, b := range p.toolbar.Buttons {
		if i > 0 {
			total += max(0, p.toolbar.Gap)
		}
		total += b.Width()
	}
	if total <= 0 {
		return Rect{}
	}
	x := p.outerWidth() - 1 - total	// 紧贴右上角（右角字符占 1 列）
	if label := Width(" " + p.Title + " "); x < 2+label {
		return Rect{}
	}
	return Rect{X: x, Y: 0, W: total, H: 1}
}

// Render 渲染窗格并返回盒子矩形。
// content 按文本区（宽 -4、高 -2 行）裁剪/补齐，左右各补 1 列内边距，
// 输出行数严格等于窗格高度（SetSize 的 height；height 为 0 时按内容行数
// 自适应）。
func (p *Panel) Render(content string) (string, Rect) {
	width := p.outerWidth()
	inner := max(1, width-2)
	innerH := max(1, p.height-2)
	if p.height <= 0 {
		innerH = max(1, Height(content))
	}
	// 内容水平内边距 1 列/侧：文本区 = 总宽-4；极窄时退化为
	// 无内边距，行宽始终精确等于 inner。
	textW := max(1, inner-2*Space1)
	padTotal := inner - textW
	padL := min(Space1, padTotal)
	body := Fill(content, textW, innerH)
	pad := func(n int) string { return strings.Repeat(" ", n) }

	rawTop := "╭" + strings.Repeat("─", inner) + "╮"
	borderColor := theme.Border
	if p.Focused {
		borderColor = theme.BorderFocus
	}
	corner := lipgloss.NewStyle().Foreground(borderColor).Render

	title := ""
	if p.Title != "" {
		title = " " + p.Title + " "
		if p.Focused {
			title = lipgloss.NewStyle().Foreground(theme.Text).Bold(true).Render(title)
		} else {
			title = lipgloss.NewStyle().Foreground(theme.Muted).Bold(true).Render(title)
		}
	}

	top := corner(rawTop)
	if title != "" && Width(title) <= inner {
		top = corner("╭") + title + corner(skipCells(rawTop, 1+Width(title)))
	}
	if p.toolbar != nil && len(p.toolbar.Buttons) > 0 {
		if r := p.ToolbarRect(); r.W > 0 {
			p.toolbar.SetSize(r.W, 1)
			view := p.toolbar.View()
			prefix := ansi.Cut(top, 0, r.X)
			top = prefix + view + corner(skipCells(rawTop, r.X+Width(view)))
		}
	}
	top = ansi.Truncate(top, width, "")

	rows := make([]string, 0, innerH+2)
	rows = append(rows, top)
	for _, line := range strings.Split(body, "\n") {
		rows = append(rows, corner("│")+pad(padL)+line+pad(padTotal-padL)+corner("│"))
	}
	rows = append(rows, corner("╰"+strings.Repeat("─", inner)+"╯"))
	// E1 海拔：面板逐行铺 Surface 底。行内分段（正文样式、嵌入的
	// Toolbar 按钮）自带 reset，外层组合的底色会被取消——与 statusbar
	// 同一问题域，用 withBackground 在行首与每个 reset 后重注入。
	for i, line := range rows {
		rows[i] = withBackground(line, theme.Surface)
	}
	return strings.Join(rows, "\n"), Rect{X: 0, Y: 0, W: width, H: len(rows)}
}

// Hit 判定一次点击是否落在窗格内。
func (p *Panel) Hit(msg tea.Msg) bool {
	return HitIn(msg, Rect{X: 0, Y: 0, W: p.outerWidth(), H: p.height})
}
