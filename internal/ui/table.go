// table.go 是定宽列的表格组件：无竖线，列与列之间只用 gap 分隔，表头 Muted 粗体。
//
// 渲染（Header/Row）与命中（Row 返回的 rects）共用同一份列布局（PlaceRow），
// 禁止在宿主里再手算列位置。单元格超宽按显示宽度截断为 …，不足则补空格，
// 因此任意取值都不会撑乱列对齐。
package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/toy-forge/devtoys/internal/theme"
)

// Column 是表格的一列：Width 为显示宽度，Title 为表头文字（可为空），
// Right 为 true 时单元格右对齐（数值列）。
type Column struct {
	Title	string
	Width	int
	Right	bool
}

// Table 是定宽列表格。Gap 为列间距，默认宿主应传 Space2。
type Table struct {
	Columns	[]Column
	Gap	int
}

// FitWidth 把文本截断或补齐到 width 列（ANSI 感知）。
func FitWidth(text string, width int) string {
	width = max(0, width)
	text = ansi.Truncate(text, width, "…")
	if pad := width - Width(text); pad > 0 {
		text += strings.Repeat(" ", pad)
	}
	return text
}

// Header 渲染表头行：各列标题走统一眉标角色（大写、Muted 粗体），
// 与首页类别带、工具分区标题同一份字符形态；与 Row 同列对齐。
func (t Table) Header() string {
	cells := make([]string, len(t.Columns))
	for i, col := range t.Columns {
		cells[i] = theme.Eyebrow(col.fit(col.Title))
	}
	_, line := PlaceRow(cells, t.Gap)
	return line
}

// Row 渲染一行：cells 与列一一对应（缺省的列补空格），每格按列宽截断或补齐。
// 返回值 rects 是每格在该行中的局部矩形，用于鼠标命中；与 line 同源。
func (t Table) Row(cells ...string) ([]Rect, string) {
	fitted := make([]string, len(t.Columns))
	for i, col := range t.Columns {
		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}
		fitted[i] = col.fit(cell)
	}
	return PlaceRow(fitted, t.Gap)
}

// Rule 返回与表格等宽的水平分隔线（表头下方可选用）。
func (t Table) Rule() string {
	w := 0
	for i, col := range t.Columns {
		if i > 0 {
			w += t.Gap
		}
		w += col.Width
	}
	return theme.Style(theme.KeyMuted).Render(strings.Repeat("─", max(0, w)))
}

// fit 把文本截断/补齐到列宽；右对齐列在左侧补空格。
func (c Column) fit(text string) string {
	if !c.Right {
		return FitWidth(text, c.Width)
	}
	text = ansi.Truncate(text, max(0, c.Width), "…")
	return strings.Repeat(" ", max(0, c.Width-Width(text))) + text
}
