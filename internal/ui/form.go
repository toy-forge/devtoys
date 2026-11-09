// form.go 是"标签 + 控件"表单行：标签列定宽，控件（FieldView、Button 等定宽控件）
// 占满剩余宽度。同一组表单行共用同一个标签列宽（LabelWidth），控件才能对齐。
package ui

import "github.com/toy-forge/devtoys/internal/theme"

// FormRow 渲染一行"标签 + 控件"：标签弱化为正文色、定宽截断；标签与控件之间
// Space2 间距；返回值恰好 width 列。
func FormRow(label string, labelWidth int, control string, width int) string {
	width = max(0, width)
	labelW := max(0, min(labelWidth, width))
	t := Table{Gap: Space2, Columns: []Column{{Width: labelW}, {Width: max(0, width-labelW-Space2)}}}
	_, line := t.Row(theme.LabelStyle().Render(label), control)
	return line
}

// LabelWidth 返回一组标签共用的标签列宽：不小于 minWidth，且能容纳最长的标签。
func LabelWidth(minWidth int, labels ...string) int {
	w := minWidth
	for _, label := range labels {
		w = max(w, Width(label))
	}
	return w
}

// FormRowRight 与 FormRow 相同，但控件右对齐（如开关、chip 这类定宽控件靠右放置）。
func FormRowRight(label string, labelWidth int, control string, width int) string {
	width = max(0, width)
	labelW := max(0, min(labelWidth, width))
	t := Table{Gap: Space2, Columns: []Column{{Width: labelW}, {Width: max(0, width-labelW-Space2), Right: true}}}
	_, line := t.Row(theme.LabelStyle().Render(label), control)
	return line
}
