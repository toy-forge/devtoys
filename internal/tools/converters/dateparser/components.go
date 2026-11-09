package dateparser

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"

	"github.com/toy-forge/devtoys/internal/theme"
	"github.com/toy-forge/devtoys/internal/ui"
)

func isEpochField(f field) bool	{ return f >= epochYearField && f <= epochMillisecondField }
func componentWidth(f field) int {
	if f >= epochYearField {
		f = f - epochYearField + yearField
	}
	switch f {
	case yearField:
		return 4
	case millisecondField:
		return 3
	default:
		return 2
	}
}
func setComponents(inputs *[fieldCount]textinput.Model, fields []field, t time.Time, except field) {
	values := []int{t.Year(), int(t.Month()), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond() / 1000000}
	for i, f := range fields {
		if f == except {
			continue
		}
		inputs[f].SetValue(fmt.Sprintf("%0*d", componentWidth(f), values[i]))
	}
}

// replaceComponent changes one part without discarding sub-millisecond ticks.
// Year and month changes clamp the day, like DateTimeOffset.AddYears/AddMonths.
func replaceComponent(base time.Time, part field, raw string, loc *time.Location) (time.Time, error) {
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return time.Time{}, fmt.Errorf("enter a valid number")
	}
	y, mon, d := base.Date()
	h, minu, sec := base.Clock()
	ns := base.Nanosecond()
	switch part {
	case 0:
		if v < 1 || v > 9999 {
			return time.Time{}, fmt.Errorf("year must be 1–9999")
		}
		y = v
	case 1:
		if v < 1 || v > 12 {
			return time.Time{}, fmt.Errorf("month must be 1–12")
		}
		mon = time.Month(v)
	case 2:
		if v < 1 || v > 31 {
			return time.Time{}, fmt.Errorf("day must be 1–31")
		}
		d = v
	case 3:
		if v < 0 || v > 23 {
			return time.Time{}, fmt.Errorf("hour must be 0–23")
		}
		h = v
	case 4:
		if v < 0 || v > 59 {
			return time.Time{}, fmt.Errorf("minute must be 0–59")
		}
		minu = v
	case 5:
		if v < 0 || v > 59 {
			return time.Time{}, fmt.Errorf("second must be 0–59")
		}
		sec = v
	case 6:
		if v < 0 || v > 999 {
			return time.Time{}, fmt.Errorf("millisecond must be 0–999")
		}
		ns = v*1000000 + ns%1000000
	default:
		return time.Time{}, fmt.Errorf("invalid date component")
	}
	lastDay := time.Date(y, mon+1, 0, 0, 0, 0, 0, time.UTC).Day()
	if d > lastDay {
		if part == 0 || part == 1 {
			d = lastDay
		} else {
			return time.Time{}, fmt.Errorf("day is invalid for this month")
		}
	}
	t := time.Date(y, mon, d, h, minu, sec, ns, loc)
	if t.Year() != y || t.Month() != mon || t.Day() != d || t.Hour() != h || t.Minute() != minu || t.Second() != sec {
		return time.Time{}, fmt.Errorf("local time does not exist in this time zone")
	}
	if err := validateInZone(t, loc); err != nil {
		return time.Time{}, err
	}
	return t, nil
}

type formCell struct {
	field			field
	start, end, inputStart	int
	actionStart, actionEnd	int
}
type formRow struct {
	text	string
	cells	[]formCell
	actions	bool
}

// 表单布局常量：标签列宽、列间距、动作按钮宽，以及网格所需的最小宽度。
const (
	formLabelWidth		= 14
	formGap			= ui.Space2
	formActionWidth		= 7	// "󰥔 Now" 内联按钮宽（含内边距）
	formNowLabel		= "󰥔 Now"
	formStackedLabel	= 14
)

// gridColumnWidth 返回字段在网格中的列宽：表头与输入框（取值宽 + 两列内边距）取较大者，
// 保证表头对齐且任何取值都不会撑开列。
func gridColumnWidth(f field) int {
	return max(lipgloss.Width(fieldNames[f]), componentWidth(f)+2*ui.Space1)
}

// gridWidth 是网格（标签列 + 日期/时间七列 + 列间距）的总宽度；宽度不足时退回逐行表单。
func gridWidth() int {
	w := formLabelWidth + formGap*len(dateFields)
	for _, f := range dateFields {
		w += gridColumnWidth(f)
	}
	return w
}

// formRows 是主表单：时间戳、ISO 8601、日期/时间网格、时区。
func (c *converter) formRows() []formRow {
	rows := []formRow{c.singleRow(timestampField, true), c.singleRow(isoField, true), spacerRow()}
	if c.gridFits() {
		rows = append(rows, c.gridRows("Date", "Time", dateFields)...)
	} else {
		for _, f := range dateFields {
			rows = append(rows, c.stackedRow(f))
		}
	}
	return append(rows, spacerRow(), c.singleRow(zoneField, false))
}

// spacerRow 是一行留白：各区块之间的分隔，不含任何可交互单元格。
func spacerRow() formRow	{ return formRow{} }

// epochFormRows 是 epoch 网格（仅启用时出现），布局与主表单的日期/时间网格一致。
func (c *converter) epochFormRows() []formRow {
	if !c.useEpoch {
		return nil
	}
	if c.gridFits() {
		return c.gridRows("Epoch date", "Epoch time", epochFields)
	}
	rows := make([]formRow, 0, len(epochFields))
	for _, f := range epochFields {
		rows = append(rows, c.stackedRow(f))
	}
	return rows
}

// gridFits 报告当前宽度能否放下网格。
func (c *converter) gridFits() bool	{ return c.width >= gridWidth() }

// gridRows 用 ui.Table 渲染日期/时间网格：表头列出七个字段，每个分组占一行，
// 分组内的字段落在对应列上。渲染与鼠标命中共用 Table.Row 返回的矩形。
func (c *converter) gridRows(dateLabel, timeLabel string, fields []field) []formRow {
	table := ui.Table{Gap: formGap, Columns: []ui.Column{{Width: formLabelWidth}}}
	for _, f := range dateFields {
		table.Columns = append(table.Columns, ui.Column{Title: fieldNames[f], Width: gridColumnWidth(f)})
	}
	rows := []formRow{{text: table.Header()}}
	// 日期占前三列（年/月/日），时间从第四列（时）开始，二者共用同一套列。
	groups := []struct {
		label		string
		fields		[]field
		firstCol	int
	}{{dateLabel, fields[:3], 0}, {timeLabel, fields[3:], 3}}
	for _, group := range groups {
		cells := make([]string, len(table.Columns))
		cells[0] = theme.LabelStyle().Render(group.label)
		for j, f := range group.fields {
			col := group.firstCol + j
			cells[col+1] = ui.FieldView(&c.inputs[f], table.Columns[col+1].Width, c.focus == f)
		}
		rects, line := table.Row(cells...)
		row := formRow{text: line}
		for j, f := range group.fields {
			r := rects[group.firstCol+j+1]
			row.cells = append(row.cells, formCell{field: f, start: r.X, end: r.Right(), inputStart: r.X + ui.Space1})
		}
		rows = append(rows, row)
	}
	return rows
}

// singleRow 渲染单值行：标签 + 定宽输入框（与网格的字段列左缘对齐）；
// action 为 true 时在输入框右侧放一个"Now"内联按钮。
func (c *converter) singleRow(f field, action bool) formRow {
	avail := c.width - formLabelWidth - formGap
	actionW := 0
	if action {
		actionW = formActionWidth + formGap
	}
	inputW := max(1, min(gridWidth()-formLabelWidth-formGap, avail-actionW))
	table := ui.Table{Gap: formGap, Columns: []ui.Column{{Width: formLabelWidth}, {Width: inputW}}}
	cells := []string{theme.LabelStyle().Render(fieldNames[f]), ui.FieldView(&c.inputs[f], inputW, c.focus == f)}
	if action {
		table.Columns = append(table.Columns, ui.Column{Width: formActionWidth})
		cells = append(cells, ui.Button{Label: formNowLabel, Inline: true}.View())
	}
	rects, line := table.Row(cells...)
	cell := formCell{field: f, start: rects[1].X, end: rects[1].Right(), inputStart: rects[1].X + ui.Space1}
	if action {
		cell.end = rects[2].Right()
		cell.actionStart, cell.actionEnd = rects[2].X, rects[2].Right()
	}
	return formRow{text: line, cells: []formCell{cell}}
}

// stackedRow 是窄屏的逐行表单：每个字段独占一行，标签在左、输入框铺满剩余宽度。
func (c *converter) stackedRow(f field) formRow {
	table := ui.Table{Gap: formGap, Columns: []ui.Column{{Width: formStackedLabel}, {Width: max(1, c.width-formStackedLabel-formGap)}}}
	rects, line := table.Row(theme.LabelStyle().Render(fieldNames[f]), ui.FieldView(&c.inputs[f], table.Columns[1].Width, c.focus == f))
	return formRow{text: line, cells: []formCell{{field: f, start: rects[1].X, end: rects[1].Right(), inputStart: rects[1].X + ui.Space1}}}
}
