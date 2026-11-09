// field.go 是定宽单行输入框组件：把 bubbles/textinput 包装成表格单元格。
//
// 输入框宽度由宿主（表格列）给定，不随取值伸缩，保证列对齐；底色 Control 标明
// 它是可编辑的字段（焦点时 ControlHover），与弱化的标签文字区分开。
package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/toy-forge/devtoys/internal/theme"
)

// FieldView 渲染定宽输入框：去掉提示符，左右各 Space1 列内边距，整格铺满底色。
// width 为单元格宽度（显示列）；返回值恰好 width 列。
func FieldView(in *textinput.Model, width int, focused bool) string {
	width = max(1, width)
	in.Prompt = ""
	in.Width = max(1, width-2*Space1)
	bg := theme.Control
	if focused {
		bg = theme.ControlHover
	}
	line := FitWidth(strings.Repeat(" ", Space1)+in.View(), width)
	return withBackground(line, bg)
}
