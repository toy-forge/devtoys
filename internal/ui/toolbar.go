// toolbar.go 是一行内联 Button 的布局器：
// 渲染与命中共享同一份 Rect 布局。
package ui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/toy-forge/devtoys/internal/theme"
)

// Toolbar 是一行 Button 的布局器。
//
// 布局策略（渲染与命中共用同一份 Rect）：
//   - 全部按钮放得下：单行排布（必要时按 SetSize 的高度换行）；
//   - 放不下且高度 ≥ 2：两行"分页模式"，上一行为 [<] n/m [>]，
//     滚轮与页码两侧按钮翻页；
//   - 放不下且高度只有 1：单行内截断为 …，放不下的按钮不可命中。
type Toolbar struct {
	// Buttons 是要布局的按钮。宿主通过 SetButtons 替换整组按钮。
	Buttons	[]Button
	// Gap 是按钮间隔列数，默认 1（冻结：workspace_test 命中算术硬编码间隔 1）。
	Gap	int

	width, height	int
	page		int
	hover		int	// 悬停按钮下标 + 1；0 表示无悬停（零值安全）
	dirty		bool
	mode		toolbarMode
	rows		[]string
	rects		[]Rect
	pagePrev	Rect
	pageNext	Rect
	pages		int
}

type toolbarMode uint8

const (
	modeFit	toolbarMode	= iota
	modePage
	modeTruncate
)

// NewToolbar 创建按钮条。
func NewToolbar(buttons ...Button) *Toolbar {
	t := &Toolbar{Buttons: buttons, Gap: 1}
	for i := range t.Buttons {
		t.Buttons[i].Inline = true
	}
	return t
}

// SetButtons 替换整组按钮（自动转为内联形态）。
func (t *Toolbar) SetButtons(buttons ...Button) {
	t.Buttons = append(t.Buttons[:0], buttons...)
	for i := range t.Buttons {
		t.Buttons[i].Inline = true
	}
	t.hover = 0	// 旧下标随按钮组一起失效
	t.dirty = true
}

// SetSize 记录可用宽高。
func (t *Toolbar) SetSize(width, height int) {
	t.width, t.height = max(0, width), max(0, height)
	t.dirty = true
	t.relayout()
}

// Width 返回已设置的可用宽度。
func (t *Toolbar) Width() int	{ return t.width }

func (t *Toolbar) gap() int {
	if t.Gap < 0 {
		return 0
	}
	return t.Gap
}

// gapFill 渲染按钮间隔：缝隙列铺 Surface，与 StatusBar.SetRight
// 对 Toolbar 统一 withBackground 的语义一致，避免非默认底色终端
// 在 panel 标题行等场景露出透明缝。
func (t *Toolbar) gapFill() string {
	if n := t.gap(); n > 0 {
		return lipgloss.NewStyle().Background(theme.Surface).Render(strings.Repeat(" ", n))
	}
	return ""
}

func toolbarSurfaceGap(n int) string {
	if n <= 0 {
		return ""
	}
	return lipgloss.NewStyle().Background(theme.Surface).Render(strings.Repeat(" ", n))
}

func (t *Toolbar) availWidth() int {
	if t.width > 0 {
		return t.width
	}
	return 80
}

// SetHover 记录悬停按钮下标（-1 清除）；越界值按 -1 处理。
// 悬停只影响渲染（hover 态），不改宽度与命中。
func (t *Toolbar) SetHover(index int) {
	if index < -1 || index >= len(t.Buttons) {
		index = -1
	}
	t.hover = index + 1
}

// buttonView 返回第 i 个按钮的渲染（含悬停态）。
func (t *Toolbar) buttonView(i int) string {
	b := t.Buttons[i]
	b.Hover = t.hover == i+1
	return b.View()
}

// HoverAt 返回局部坐标 (x, y) 悬停的按钮下标；滚轮、页码行与越界
// 位置返回 -1。实现镜像 Index() 的 Rect 遍历，但只认坐标不认按下。
func (t *Toolbar) HoverAt(x, y int) int {
	t.relayout()
	var rects []Rect
	if t.mode == modePage {
		t.clampPage()
		rects = t.pageRects()
	} else {
		_, rects = t.relayout()
	}
	for i, r := range rects {
		if r.W > 0 && r.Y == y && x >= r.X && x < r.X+r.W && !t.Buttons[i].Disabled {
			return i
		}
	}
	return -1
}

// fitRows 计算不限高度时的换行布局。
func (t *Toolbar) fitRows() ([]string, []Rect) {
	rows := []string{""}
	rects := make([]Rect, 0, len(t.Buttons))
	x, y := 0, 0
	width := t.availWidth()
	for i := range t.Buttons {
		w := t.Buttons[i].Width()
		if x > 0 && x+t.gap()+w > width {
			rows = append(rows, "")
			x = 0
			y++
		}
		if x > 0 {
			rows[y] += t.gapFill()
			x += t.gap()
		}
		rects = append(rects, Rect{X: x, Y: y, W: w, H: 1})
		rows[y] += t.buttonView(i)
		x += w
	}
	return rows, rects
}

// relayout 依据宽度与高度选定布局模式并缓存。
func (t *Toolbar) relayout() ([]string, []Rect) {
	if !t.dirty && t.rows != nil {
		return t.rows, t.rects
	}
	t.dirty = false
	rows, rects := t.fitRows()
	width := t.availWidth()
	t.mode = modeFit
	if t.height > 0 && len(rows) > t.height {
		if t.height >= 2 {
			t.mode = modePage
		} else {
			t.mode = modeTruncate
			rows, rects = t.truncateLayout(width)
		}
	}
	if t.mode == modePage {
		t.pages = len(rows)
		rows = nil
	}
	t.rows, t.rects = rows, rects
	t.clampPage()
	return t.rows, t.rects
}

// truncateLayout 把按钮压进一行：放不下的按钮截断为 …，其余丢弃。
func (t *Toolbar) truncateLayout(width int) ([]string, []Rect) {
	row := ""
	rects := make([]Rect, 0, len(t.Buttons))
	x := 0
	for i := range t.Buttons {
		w := t.Buttons[i].Width()
		if x > 0 {
			if x+t.gap() >= width {
				rects = append(rects, Rect{X: width, Y: 0, W: 0, H: 1})
				continue
			}
			row += t.gapFill()
			x += t.gap()
		}
		remaining := width - x
		text := t.buttonView(i)
		if w > remaining {
			if remaining <= 1 {
				rects = append(rects, Rect{X: width, Y: 0, W: 0, H: 1})
				continue
			}
			text = ansi.Truncate(text, remaining, "…")
			w = Width(text)
		}
		rects = append(rects, Rect{X: x, Y: 0, W: w, H: 1})
		row += text
		x += w
	}
	return []string{row}, rects
}

func (t *Toolbar) clampPage() {
	if t.pages > 0 && t.page >= t.pages {
		t.page = max(0, t.pages-1)
	}
	if t.page < 0 {
		t.page = 0
	}
}

// Height 返回当前布局占用的行数（严格等于 View 的行数）。
func (t *Toolbar) Height() int {
	t.relayout()
	switch t.mode {
	case modePage:
		return 2
	case modeTruncate:
		return 1
	default:
		return len(t.rows)
	}
}

// pageRow 渲染分页行：内联 Button "<"、页码（fg=Muted）、内联 Button ">"。
func (t *Toolbar) pageRow() string {
	left := Button{Label: "<", Inline: true}.View()
	right := Button{Label: ">", Inline: true}.View()
	middle := " " + strconv.Itoa(t.page+1) + "/" + strconv.Itoa(t.pages) + " "
	middle = lipgloss.NewStyle().Foreground(theme.Muted).Render(middle)
	width := t.availWidth()
	if Width(left)+Width(middle)+Width(right) > width {
		return left + right
	}
	gap := width - Width(left) - Width(middle) - Width(right)
	return left + toolbarSurfaceGap(max(0, gap/2)) + middle + toolbarSurfaceGap(max(0, gap-gap/2)) + right
}

// pageRects 按当前页码把换行布局的矩形映射到分页视图的命中坐标。
func (t *Toolbar) pageRects() []Rect {
	_, rects := t.fitRows()
	page := min(t.page, t.pages-1)
	for i := range rects {
		if rects[i].Y == page {
			rects[i].Y = 1
		} else {
			rects[i].Y = -1	// 非当前页按钮不可命中
		}
	}
	return rects
}

// View 渲染按钮条。
func (t *Toolbar) View() string {
	t.relayout()
	switch t.mode {
	case modePage:
		t.clampPage()
		rows, _ := t.fitRows()
		left := Width(Button{Label: "<", Inline: true}.View())
		t.pagePrev = Rect{X: 0, Y: 0, W: left, H: 1}
		t.pageNext = Rect{X: max(0, Width(t.pageRow())-left), Y: 0, W: left, H: 1}
		page := min(t.page, t.pages-1)
		t.rects = t.pageRects()
		return t.pageRow() + "\n" + rows[page]
	case modeTruncate:
		rows, _ := t.relayout()
		return rows[0]
	default:
		rows, _ := t.relayout()
		return strings.Join(rows, "\n")
	}
}

// Index 返回点击的按钮下标（局部坐标）；翻页控件与滚轮返回 -1。
// 调用前通常先调 View 使布局与命中一致。
func (t *Toolbar) Index(msg tea.Msg) int {
	mouse, ok := msg.(tea.MouseMsg)
	if !ok {
		return -1
	}
	t.relayout()
	switch t.mode {
	case modePage:
		t.clampPage()
		switch mouse.Button {
		case tea.MouseButtonWheelUp:
			t.page = max(0, t.page-1)
			return -1
		case tea.MouseButtonWheelDown:
			t.page = min(t.pages-1, t.page+1)
			return -1
		}
		if mouse.Action != tea.MouseActionPress || mouse.Button != tea.MouseButtonLeft {
			return -1
		}
		if mouse.Y == 0 {
			if mouse.X >= t.pagePrev.X && mouse.X < t.pagePrev.X+t.pagePrev.W {
				t.page = max(0, t.page-1)
			}
			if mouse.X >= t.pageNext.X && mouse.X < t.pageNext.X+t.pageNext.W {
				t.page = min(t.pages-1, t.page+1)
			}
			return -1
		}
	case modeTruncate:
		if mouse.Action != tea.MouseActionPress || mouse.Button != tea.MouseButtonLeft || mouse.Y != 0 {
			return -1
		}
	default:
		if mouse.Action != tea.MouseActionPress || mouse.Button != tea.MouseButtonLeft {
			return -1
		}
	}
	var rects []Rect
	if t.mode == modePage {
		rects = t.pageRects()
	} else {
		_, rects = t.relayout()
	}
	for i, r := range rects {
		if r.W > 0 && r.Y == mouse.Y && mouse.X >= r.X && mouse.X < r.X+r.W && !t.Buttons[i].Disabled {
			return i
		}
	}
	return -1
}

// Rect 返回第 i 个按钮最近一次布局后的矩形（局部坐标）。
func (t *Toolbar) Rect(i int) Rect {
	_, rects := t.relayout()
	if i < 0 || i >= len(rects) {
		return Rect{}
	}
	return rects[i]
}

// RenderToolbar 在给定宽度里渲染按钮条（供无状态场景一次完成）。
func RenderToolbar(width int, buttons ...Button) string {
	t := NewToolbar(buttons...)
	t.SetSize(width, 0)
	return t.View()
}
