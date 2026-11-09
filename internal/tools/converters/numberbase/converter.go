package numberbase

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/toy-forge/devtoys/internal/theme"
	"github.com/toy-forge/devtoys/internal/toolkit"
	"github.com/toy-forge/devtoys/internal/ui"
)

type field int

const (
	hexField	field	= iota
	decimalField
	octalField
	binaryField
	inputAlphabetField
	inputField
	outputAlphabetField
	outputField
	fieldCount
)

var titles = [fieldCount]string{"Hexadecimal", "Decimal", "Octal", "Binary", "Custom format", "Input", "Custom format", "Output"}

type converter struct {
	header					toolkit.SettingHeader
	inputs					[fieldCount]textinput.Model
	focus					field
	basicSource				field
	formatted, advanced			bool
	inputAlphabet, outputAlphabet		int
	width, height, headerHeight, offset	int
	err					error
	copied					bool
}

func newConverter() *converter {
	c := &converter{header: toolkit.SettingHeader{
		ContentGap:	1,
		Names:		[]string{"Format", "Advanced mode"},
		Icons:		[]string{"󰬴", "󰒓"},
		Switches:	[]bool{true, true},
	}, formatted:	true, basicSource: decimalField, focus: hexField, width: 80, height: 24}
	for i := range c.inputs {
		c.inputs[i] = textinput.New()
		c.inputs[i].Prompt = "┃ "
		c.inputs[i].CharLimit = 65536
		c.inputs[i].TextStyle = lipgloss.NewStyle().Foreground(theme.Text)
	}
	c.inputs[inputAlphabetField].CharLimit = 256
	c.inputs[outputAlphabetField].CharLimit = 256
	c.inputs[inputAlphabetField].Placeholder = "Enter at least two distinct characters"
	c.inputs[outputAlphabetField].Placeholder = "Enter at least two distinct characters"
	c.inputs[hexField].Focus()
	c.resize()
	return c
}
func (*converter) ID() string		{ return "converters.numberbase" }
func (*converter) Title() string	{ return "Number Base Converter" }
func (*converter) Category() string	{ return "converters" }
func (*converter) Icon() string		{ return "󰎠" }
func (*converter) Description() string	{ return "Convert numbers between standard and custom bases" }
func (*converter) Init() tea.Cmd	{ return nil }
func (c *converter) values() []string {
	f := "Off"
	if c.formatted {
		f = "On"
	}
	mode := "Off"
	if c.advanced {
		mode = "On"
	}
	return []string{f, mode}
}
func (c *converter) fields() []field {
	if !c.advanced {
		return []field{hexField, decimalField, octalField, binaryField}
	}
	fields := []field{}
	if c.inputAlphabet == customAlphabet {
		fields = append(fields, inputAlphabetField)
	}
	fields = append(fields, inputField)
	if c.outputAlphabet == customAlphabet {
		fields = append(fields, outputAlphabetField)
	}
	return fields
}
func (c *converter) resize() {
	if c.height <= 8 {
		c.headerHeight = 0
	} else {
		c.headerHeight = c.header.SetSize(tea.WindowSizeMsg{Width: c.width, Height: c.height}, c.values()...)
	}
	reserve := 0
	if c.width >= 65 {
		reserve = 30
	}
	for i := range c.inputs {
		c.inputs[i].Width = max(1, c.width-16-reserve)
	}
	c.ensureVisible()
}
func (c *converter) setFocus(f field) tea.Cmd {
	for i := range c.inputs {
		c.inputs[i].Blur()
	}
	c.focus = f
	c.copied = false
	c.ensureVisible()
	return c.inputs[f].Focus()
}
func (c *converter) cycleFocus(dir int) tea.Cmd {
	fields := c.fields()
	idx := 0
	for i, f := range fields {
		if f == c.focus {
			idx = i
			break
		}
	}
	return c.setFocus(fields[(idx+dir+len(fields))%len(fields)])
}
func (c *converter) activeAlphabet(index int, f field) alphabet {
	a := alphabets[index]
	if index == customAlphabet {
		a.chars = c.inputs[f].Value()
	}
	return a
}
func (c *converter) refreshBasic(source field, rewrite bool) {
	if source > binaryField {
		source = c.basicSource
	}
	c.basicSource = source
	n, err := parseBasic(c.inputs[source].Value(), basicBases[source])
	c.err = err
	for i, base := range basicBases {
		f := field(i)
		if f == source && (!rewrite || err != nil) {
			continue
		}
		value := ""
		if err == nil {
			value = basicValue(n, base, c.formatted)
		}
		c.inputs[f].SetValue(value)
	}
}
func (c *converter) refreshAdvanced() {
	source := c.activeAlphabet(c.inputAlphabet, inputAlphabetField)
	target := c.activeAlphabet(c.outputAlphabet, outputAlphabetField)
	if c.inputAlphabet == customAlphabet && utf8.RuneCountInString(source.chars) < 2 {
		c.err = fmt.Errorf("Please set at least two characters in the custom dictionary")
		c.inputs[outputField].SetValue("")
		return
	}
	if c.outputAlphabet == customAlphabet && utf8.RuneCountInString(target.chars) < 2 {
		c.err = fmt.Errorf("Please set at least two characters in the custom dictionary")
		c.inputs[outputField].SetValue("")
		return
	}
	n, err := parseAlphabet(c.inputs[inputField].Value(), source)
	result := ""
	if err == nil {
		result, err = formatAlphabet(n, target, c.formatted)
	}
	c.err = err
	c.inputs[outputField].SetValue(result)
}
func (c *converter) refresh(f field) {
	c.copied = false
	if c.advanced {
		c.refreshAdvanced()
	} else {
		c.refreshBasic(f, false)
	}
	c.ensureVisible()
}
func (c *converter) toggle(i int) {
	if i == 0 {
		c.formatted = !c.formatted
		if c.advanced {
			c.refreshAdvanced()
		} else {
			c.refreshBasic(c.basicSource, true)
		}
	} else {
		c.advanced = !c.advanced
		c.err = nil
		c.copied = false
		if c.advanced {
			c.setFocus(inputField)
			c.refreshAdvanced()
		} else {
			c.setFocus(c.basicSource)
			c.refreshBasic(c.basicSource, false)
		}
	}
	c.resize()
}
func (c *converter) cycleAlphabet(input bool, step int) {
	if input {
		c.inputAlphabet = (c.inputAlphabet + step + len(alphabets)) % len(alphabets)
	} else {
		c.outputAlphabet = (c.outputAlphabet + step + len(alphabets)) % len(alphabets)
	}
	if c.focus == inputAlphabetField && c.inputAlphabet != customAlphabet {
		c.setFocus(inputField)
	}
	if c.focus == outputAlphabetField && c.outputAlphabet != customAlphabet {
		c.setFocus(outputField)
	}
	c.refreshAdvanced()
	c.resize()
}
func (c *converter) copy() {
	c.copyField(c.focus)
}
func (c *converter) copyField(f field) {
	if err := clipboard.WriteAll(c.inputs[f].Value()); err != nil {
		c.err = fmt.Errorf("could not copy: %w", err)
		c.ensureVisible()
		return
	}
	c.copied = true
}

type row struct {
	text		string
	field		field
	isInput		bool
	readOnly	bool
	disabled	bool
	direction	int
	fieldX		int	// 输入框在行中的起始列（光标定位用）
	actionsX	int
	controls	[]string
}

func (c *converter) rows() []row {
	var rows []row
	// 字段行：标签列 + 输入框（定宽，占满剩余宽度）+ 右侧 Copy 按钮；渲染与命中共用 ui.Table 的矩形。
	labelWidth := ui.LabelWidth(14, titles[:]...)
	copyView := toolkit.Actions("Copy")
	addInput := func(f field, disabled ...bool) {
		r := row{field: f, isInput: true, actionsX: -1}
		if len(disabled) > 0 && disabled[0] {
			r.disabled = true
			r.isInput = false
		}
		if f == outputField {
			r.readOnly = true
		}
		withCopy := c.width >= 65 && !r.disabled
		copyWidth := 0
		if withCopy {
			copyWidth = lipgloss.Width(copyView) + ui.Space2
			r.controls = []string{"Copy"}
		}
		fieldWidth := max(1, c.width-labelWidth-ui.Space2-copyWidth)
		table := ui.Table{Gap: ui.Space2, Columns: []ui.Column{{Width: labelWidth}, {Width: fieldWidth}}}
		labelStyle := theme.LabelStyle()
		field := ui.FieldView(&c.inputs[f], fieldWidth, c.focus == f)
		if r.disabled {
			labelStyle = theme.DisabledStyle()
			field = theme.DisabledStyle().Render(ui.FitWidth(c.inputs[f].Value(), fieldWidth))
		}
		cells := []string{labelStyle.Render(titles[f]), field}
		if withCopy {
			table.Columns = append(table.Columns, ui.Column{Width: lipgloss.Width(copyView)})
			cells = append(cells, copyView)
		}
		rects, line := table.Row(cells...)
		r.text = line
		r.fieldX = rects[1].X
		if withCopy {
			r.actionsX = rects[2].X
		}
		r.isInput = !r.readOnly && !r.disabled
		rows = append(rows, r)
	}
	if !c.advanced {
		for _, f := range c.fields() {
			addInput(f)
		}
		return rows
	}
	addSelector := func(input bool) {
		name, direction := alphabets[c.inputAlphabet].name, 1
		if !input {
			name, direction = alphabets[c.outputAlphabet].name, 2
		}
		// 单行选择器：Base 标签 + 翻页按钮 + 当前字典名，窄屏也能看到控件。
		label := "   Base "
		rows = append(rows, row{
			text:		"   " + lipgloss.NewStyle().Foreground(theme.BlueBright).Bold(true).Render("Base") + " " + toolkit.Actions("<", ">") + " " + name,
			direction:	direction,
			actionsX:	lipgloss.Width(label),
		})
	}
	addSelector(true)
	addInput(inputAlphabetField, c.inputAlphabet != customAlphabet)
	addInput(inputField)
	rows = append(rows, row{})
	addSelector(false)
	addInput(outputAlphabetField, c.outputAlphabet != customAlphabet)
	addInput(outputField)
	return rows
}
func (c *converter) statusHeight() int {
	if c.err != nil {
		return 1
	}
	return 0
}
func (c *converter) bodyTop() int {
	// 与 View 的前缀行数严格一致：header 行 + 可选的错误行。
	top := c.headerHeight
	if c.err != nil {
		top++
	}
	return top
}
func (c *converter) bodyHeight() int	{ return max(1, c.height-c.bodyTop()-1) }
func (c *converter) maxOffset() int	{ return max(0, len(c.rows())-c.bodyHeight()) }
func (c *converter) ensureVisible() {
	rowIndex := 0
	for i, r := range c.rows() {
		if r.isInput && r.field == c.focus {
			rowIndex = i
			break
		}
	}
	if rowIndex < c.offset {
		c.offset = rowIndex
	}
	if rowIndex >= c.offset+c.bodyHeight() {
		c.offset = rowIndex - c.bodyHeight() + 1
	}
	c.offset = max(0, min(c.offset, c.maxOffset()))
}
func (c *converter) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		c.width, c.height = sz.Width, sz.Height
		c.resize()
		return c, nil
	}
	if c.headerHeight > 0 {
		if i := c.header.Index(msg, c.values()...); i >= 0 {
			c.toggle(i)
			return c, nil
		}
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "tab":
			return c, c.cycleFocus(1)
		case "shift+tab":
			return c, c.cycleFocus(-1)
		case "alt+f":
			c.toggle(0)
			return c, nil
		case "alt+a":
			c.toggle(1)
			return c, nil
		case "alt+[":
			if c.advanced {
				c.cycleAlphabet(true, -1)
				return c, nil
			}
		case "alt+]":
			if c.advanced {
				c.cycleAlphabet(true, 1)
				return c, nil
			}
		case "alt+,":
			if c.advanced {
				c.cycleAlphabet(false, -1)
				return c, nil
			}
		case "alt+.":
			if c.advanced {
				c.cycleAlphabet(false, 1)
				return c, nil
			}
		case "ctrl+y":
			c.copy()
			return c, nil
		case "ctrl+shift+y":
			if c.advanced {
				c.copyField(outputField)
			}
			return c, nil
		}
	}
	if file, ok := msg.(toolkit.FileImportMsg); ok {
		if c.focus == outputField {
			c.setFocus(inputField)
		}
		text := strings.TrimSpace(file.Text)
		input := &c.inputs[c.focus]
		if !utf8.ValidString(text) || strings.IndexFunc(text, unicode.IsControl) >= 0 || utf8.RuneCountInString(text)+utf8.RuneCountInString(input.Value()) > input.CharLimit {
			c.err = fmt.Errorf("file must contain one short line of text")
			return c, nil
		}
		old := []rune(input.Value())
		pos := input.Position()
		v := append(append(append([]rune{}, old[:pos]...), []rune(text)...), old[pos:]...)
		input.SetValue(string(v))
		input.SetCursor(pos + utf8.RuneCountInString(text))
		c.refresh(c.focus)
		return c, nil
	}
	if mouse, ok := msg.(tea.MouseMsg); ok {
		if mouse.Y < c.headerHeight || mouse.Y >= c.height-1 || mouse.Action != tea.MouseActionPress {
			return c, nil
		}
		if c.err != nil && mouse.Y == c.headerHeight+1 {
			return c, nil
		}
		if mouse.Button == tea.MouseButtonWheelUp {
			c.offset = max(0, c.offset-2)
			return c, nil
		}
		if mouse.Button == tea.MouseButtonWheelDown {
			c.offset = min(c.maxOffset(), c.offset+2)
			return c, nil
		}
		if mouse.Button != tea.MouseButtonLeft {
			return c, nil
		}
		i := mouse.Y - c.bodyTop() + c.offset
		rows := c.rows()
		if i < 0 || i >= len(rows) {
			return c, nil
		}
		r := rows[i]
		if r.readOnly {
			if r.actionsX >= 0 && mouse.X >= r.actionsX {
				local := mouse
				local.Y = 0
				local.X -= r.actionsX
				if toolkit.ActionAt(local, r.controls...) == 0 {
					c.copyField(r.field)
				}
			}
			return c, nil
		}
		if r.direction != 0 {
			local := mouse
			local.Y = 0
			local.X -= r.actionsX
			switch toolkit.ActionAt(local, "<", ">") {
			case 0:
				c.cycleAlphabet(r.direction == 1, -1)
			case 1:
				c.cycleAlphabet(r.direction == 1, 1)
			}
			return c, nil
		}
		if !r.isInput {
			return c, nil
		}
		cmd := c.setFocus(r.field)
		if r.actionsX >= 0 && mouse.X >= r.actionsX {
			local := mouse
			local.Y = 0
			local.X -= r.actionsX
			action := toolkit.ActionAt(local, r.controls...)
			if action == 0 {
				c.copyField(r.field)
				return c, nil
			}
		}
		if mouse.X >= r.fieldX {
			c.inputs[r.field].SetCursor(mouse.X - r.fieldX - ui.Space1)
		}
		return c, cmd
	}
	if c.focus == outputField {
		return c, nil
	}
	old := c.inputs[c.focus].Value()
	var cmd tea.Cmd
	c.inputs[c.focus], cmd = c.inputs[c.focus].Update(msg)
	if old != c.inputs[c.focus].Value() {
		c.refresh(c.focus)
	}
	return c, cmd
}
func (c *converter) View() string {
	prefix := ""
	if c.headerHeight > 0 {
		prefix = c.header.View(c.values()...) + "\n"
	}
	if c.err != nil {
		prefix += lipgloss.NewStyle().Foreground(theme.Red).Render("󰀨  "+c.err.Error()) + "\n"
	}
	rows := c.rows()
	body := make([]string, c.bodyHeight())
	for i := range body {
		n := c.offset + i
		if n < len(rows) {
			body[i] = ansi.Truncate(rows[n].text, c.width, "")
		}
	}
	// 页脚：ui.StatusBar——左句式 + 右按键提示（超宽右侧
	// 截断加 …）；已复制以 Toast 呈现。
	bar := ui.NewStatusBar()
	bar.SetWidth(max(1, c.width))
	bar.SetLeftTokens([]ui.StatusToken{{Label: "click to edit · scroll to browse"}})
	if c.advanced {
		bar.SetHints(
			ui.StatusHint{Key: "alt+[ / alt+]", Desc: "input base"},
			ui.StatusHint{Key: "alt+, / alt+.", Desc: "output base"},
			ui.StatusHint{Key: "ctrl+y", Desc: "input"},
			ui.StatusHint{Key: "ctrl+shift+y", Desc: "output"},
		)
	} else {
		bar.SetHints(
			ui.StatusHint{Key: "alt+A", Desc: "mode"},
			ui.StatusHint{Key: "alt+F", Desc: "format"},
			ui.StatusHint{Key: "ctrl+y", Desc: "copy"},
		)
	}
	if c.copied {
		bar.SetToast(ui.Toast{Level: ui.LevelSuccess, Message: "Copied"})
	}
	return prefix + strings.Join(body, "\n") + "\n" + bar.View()
}
