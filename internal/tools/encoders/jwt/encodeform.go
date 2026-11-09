package jwt

import (
	"fmt"
	"strconv"
	"strings"
	"time"
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

type encodeField int

const (
	encodeIssuer	encodeField	= iota
	encodeAudience
	encodeYear
	encodeMonth
	encodeDay
	encodeHour
	encodeMinute
	encodeFieldCount
)

var encodeLabels = [encodeFieldCount]string{"Issuer", "Audience", "Year", "Month", "Day", "Hour", "Minute"}

type encodeCell struct {
	field			encodeField
	start, end, inputStart	int
}
type encodeRow struct {
	text	string
	cells	[]encodeCell
}
type encodeClaimForm struct {
	inputs			[encodeFieldCount]textinput.Model
	focus			encodeField
	width, height, offset	int
	err			error
	statusErr		error
}

func newEncodeClaimForm() *encodeClaimForm {
	f := &encodeClaimForm{width: 80, height: 12}
	for i := range f.inputs {
		f.inputs[i] = textinput.New()
		f.inputs[i].Prompt = "┃ "
		f.inputs[i].CharLimit = 1024
	}
	f.inputs[encodeIssuer].Placeholder = "Issuer"
	f.inputs[encodeAudience].Placeholder = "Audience"
	for i := encodeYear; i < encodeFieldCount; i++ {
		f.inputs[i].CharLimit = 4
	}
	now := time.Now().UTC()
	for i, v := range []int{now.Year(), int(now.Month()), now.Day(), now.Hour(), now.Minute()} {
		index := encodeYear + encodeField(i)
		width := 2
		if index == encodeYear {
			width = 4
		}
		f.inputs[index].SetValue(fmt.Sprintf("%0*d", width, v))
	}
	f.resize(80, 12)
	return f
}
func (f *encodeClaimForm) resize(w, h int) {
	f.width, f.height = max(1, w), max(1, h)
	for i := range f.inputs {
		f.inputs[i].Width = max(1, f.width-16)
		if f.width >= 65 && encodeField(i) >= encodeYear {
			f.inputs[i].Width = 2
			if encodeField(i) == encodeYear {
				f.inputs[i].Width = 4
			}
		}
	}
	f.reveal()
}
func (f *encodeClaimForm) setFocus(index encodeField) tea.Cmd {
	for i := range f.inputs {
		f.inputs[i].Blur()
	}
	f.focus = index
	f.reveal()
	return f.inputs[index].Focus()
}

// encodeGridFields 是日期/时间网格的列顺序：年、月、日（过期日期）与时、分（过期时间）。
var encodeGridFields = []encodeField{encodeYear, encodeMonth, encodeDay, encodeHour, encodeMinute}

// encodeLabelWidth 是表单标签列宽（Issuer / Audience / Expiry 行共用，保证输入框左缘对齐）。
const encodeLabelWidth = 14

// rows 渲染声明表单：Issuer / Audience 为"标签 + 定宽输入框"；过期时间为一张表：
// 表头列出年/月/日/时/分，"Expiry date"、"Expiry UTC" 两行把字段落在对应列上。
// 窄屏（网格放不下）退回逐行表单。渲染与鼠标命中共用 ui.Table / ui.FormRow 的矩形。
func (f *encodeClaimForm) rows() []encodeRow {
	single := func(index encodeField) encodeRow {
		control := ui.FieldView(&f.inputs[index], max(1, f.width-encodeLabelWidth-ui.Space2), f.focus == index)
		inputStart := encodeLabelWidth + ui.Space2 + ui.Space1
		return encodeRow{
			text:	ui.FormRow(encodeLabels[index], encodeLabelWidth, control, f.width),
			cells:	[]encodeCell{{field: index, start: 0, end: f.width, inputStart: inputStart}},
		}
	}
	rows := []encodeRow{single(encodeIssuer), single(encodeAudience)}
	columns := []ui.Column{{Width: encodeLabelWidth}}
	for _, index := range encodeGridFields {
		columns = append(columns, ui.Column{Title: encodeLabels[index], Width: encodeColumnWidth(index)})
	}
	table := ui.Table{Gap: ui.Space2, Columns: columns}
	if f.width < encodeGridWidth(table) {
		rows = rows[:0]
		for _, index := range []encodeField{encodeIssuer, encodeAudience, encodeYear, encodeMonth, encodeDay, encodeHour, encodeMinute} {
			rows = append(rows, single(index))
		}
		return rows
	}
	rows = append(rows, encodeRow{text: table.Header()})
	groups := []struct {
		label	string
		fields	[]encodeField
		first	int	// 字段在网格中的起始列
	}{
		{"Expiry date", encodeGridFields[:3], 0},
		{"Expiry UTC", encodeGridFields[3:], 3},
	}
	for _, group := range groups {
		cells := make([]string, len(columns))
		cells[0] = theme.LabelStyle().Render(group.label)
		for j, index := range group.fields {
			cells[group.first+j+1] = ui.FieldView(&f.inputs[index], columns[group.first+j+1].Width, f.focus == index)
		}
		rects, line := table.Row(cells...)
		row := encodeRow{text: line}
		for j, index := range group.fields {
			r := rects[group.first+j+1]
			row.cells = append(row.cells, encodeCell{field: index, start: r.X, end: r.Right(), inputStart: r.X + ui.Space1})
		}
		rows = append(rows, row)
	}
	return rows
}

// encodeColumnWidth 返回网格列宽：表头与输入框（定宽 6 列）取较大者。
func encodeColumnWidth(index encodeField) int {
	return max(lipgloss.Width(encodeLabels[index]), 6)
}

// encodeGridWidth 是网格（标签列 + 各字段列 + 间距）的总宽度。
func encodeGridWidth(table ui.Table) int {
	w := 0
	for i, col := range table.Columns {
		w += col.Width
		if i > 0 {
			w += table.Gap
		}
	}
	return w
}

func (f *encodeClaimForm) bodyHeight() int {
	rows := f.height - 2
	if f.err != nil || f.statusErr != nil {
		rows--
	}
	return max(1, rows)
}
func (f *encodeClaimForm) maxOffset() int	{ return max(0, len(f.rows())-f.bodyHeight()) }
func (f *encodeClaimForm) reveal() {
	row := 0
	for i, r := range f.rows() {
		for _, cell := range r.cells {
			if cell.field == f.focus {
				row = i
			}
		}
	}
	if row < f.offset {
		f.offset = row
	}
	if row >= f.offset+f.bodyHeight() {
		f.offset = row - f.bodyHeight() + 1
	}
	f.offset = max(0, min(f.offset, f.maxOffset()))
}
func (f *encodeClaimForm) options(base encoderClaims) (encoderClaims, error) {
	base.issuer = f.inputs[encodeIssuer].Value()
	base.audience = f.inputs[encodeAudience].Value()
	if !base.expirationEnabled {
		return base, nil
	}
	var v [5]int
	for i := range v {
		n, err := strconv.Atoi(strings.TrimSpace(f.inputs[encodeYear+encodeField(i)].Value()))
		if err != nil {
			return base, fmt.Errorf("enter a valid UTC expiration")
		}
		v[i] = n
	}
	if v[0] < 1 || v[0] > 9999 || v[1] < 1 || v[1] > 12 || v[2] < 1 || v[2] > 31 || v[3] < 0 || v[3] > 23 || v[4] < 0 || v[4] > 59 {
		return base, fmt.Errorf("enter a valid UTC expiration")
	}
	date := time.Date(v[0], time.Month(v[1]), v[2], v[3], v[4], 0, 0, time.UTC)
	if date.Year() != v[0] || int(date.Month()) != v[1] || date.Day() != v[2] {
		return base, fmt.Errorf("enter a valid UTC expiration")
	}
	base.expiration = date
	base.expirationSet = true
	return base, nil
}
func (f *encodeClaimForm) Update(msg tea.Msg) (bool, tea.Cmd) {
	switch event := msg.(type) {
	case tea.KeyMsg:
		switch event.String() {
		case "tab":
			return false, f.setFocus((f.focus + 1) % encodeFieldCount)
		case "shift+tab":
			return false, f.setFocus((f.focus + encodeFieldCount - 1) % encodeFieldCount)
		case "up", "down":
			if f.focus >= encodeYear {
				value, err := strconv.Atoi(strings.TrimSpace(f.inputs[f.focus].Value()))
				if err != nil {
					f.err = fmt.Errorf("enter a valid number")
					return false, nil
				}
				if event.String() == "up" {
					value++
				} else {
					value--
				}
				lower, upper := 0, 59
				switch f.focus {
				case encodeYear:
					lower, upper = 1, 9999
				case encodeMonth:
					lower, upper = 1, 12
				case encodeDay:
					lower, upper = 1, 31
				case encodeHour:
					upper = 23
				}
				if value < lower || value > upper {
					f.err = fmt.Errorf("%s is out of range", encodeLabels[f.focus])
					return false, nil
				}
				width := 2
				if f.focus == encodeYear {
					width = 4
				}
				f.inputs[f.focus].SetValue(fmt.Sprintf("%0*d", width, value))
				f.err = nil
				return true, nil
			}
		case "ctrl+y":
			if err := clipboard.WriteAll(f.inputs[f.focus].Value()); err != nil {
				f.err = fmt.Errorf("could not copy: %w", err)
			} else {
				f.err = nil
			}
			return false, nil
		}
	case toolkit.FileImportMsg:
		text := strings.TrimSpace(event.Text)
		input := &f.inputs[f.focus]
		if !utf8.ValidString(text) || strings.IndexFunc(text, unicode.IsControl) >= 0 || utf8.RuneCountInString(text)+utf8.RuneCountInString(input.Value()) > input.CharLimit {
			f.err = fmt.Errorf("file must contain one short line of text")
			return false, nil
		}
		old := []rune(input.Value())
		pos := input.Position()
		combined := append(append(append([]rune{}, old[:pos]...), []rune(text)...), old[pos:]...)
		input.SetValue(string(combined))
		input.SetCursor(pos + utf8.RuneCountInString(text))
		f.err = nil
		return true, nil
	case tea.MouseMsg:
		if event.Action != tea.MouseActionPress {
			return false, nil
		}
		if event.Button == tea.MouseButtonWheelUp {
			f.offset = max(0, f.offset-2)
			return false, nil
		}
		if event.Button == tea.MouseButtonWheelDown {
			f.offset = min(f.maxOffset(), f.offset+2)
			return false, nil
		}
		rowStart := 1
		if f.err != nil || f.statusErr != nil {
			rowStart++
		}
		if event.Button != tea.MouseButtonLeft || event.Y < rowStart || event.Y >= f.height-1 {
			return false, nil
		}
		row := event.Y - rowStart + f.offset
		rows := f.rows()
		if row < 0 || row >= len(rows) {
			return false, nil
		}
		for _, cell := range rows[row].cells {
			if event.X >= cell.start && event.X < cell.end {
				cmd := f.setFocus(cell.field)
				if event.X >= cell.inputStart {
					f.inputs[cell.field].SetCursor(event.X - cell.inputStart)
				}
				return false, cmd
			}
		}
		return false, nil
	}
	before := f.inputs[f.focus].Value()
	var cmd tea.Cmd
	f.inputs[f.focus], cmd = f.inputs[f.focus].Update(msg)
	changed := before != f.inputs[f.focus].Value()
	if changed {
		f.err = nil
	}
	return changed, cmd
}
func (f *encodeClaimForm) View(encodingErr error) string {
	f.statusErr = encodingErr
	lines := []string{ansi.Truncate("Claim values · UTC · Alt+V back", f.width, "")}
	statusErr := f.err
	if statusErr == nil {
		statusErr = encodingErr
	}
	if statusErr != nil {
		status := lipgloss.NewStyle().Foreground(theme.Red).Render("󰅚  " + statusErr.Error())
		lines = append(lines, ansi.Truncate(status, f.width, ""))
	}
	rows := f.rows()
	for i := range f.bodyHeight() {
		j := f.offset + i
		if j < len(rows) {
			lines = append(lines, ansi.Truncate(rows[j].text, f.width, ""))
		} else {
			lines = append(lines, "")
		}
	}
	footer := "Tab focus · Alt+I issuer · Alt+U audience · Alt+E expiry"
	lines = append(lines, ansi.Truncate(footer, f.width, ""))
	return strings.Join(lines, "\n")
}
