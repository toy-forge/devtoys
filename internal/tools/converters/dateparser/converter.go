package dateparser

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

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
	timestampField	field	= iota
	isoField
	yearField
	monthField
	dayField
	hourField
	minuteField
	secondField
	millisecondField
	epochYearField
	epochMonthField
	epochDayField
	epochHourField
	epochMinuteField
	epochSecondField
	epochMillisecondField
	zoneField
	fieldCount
)

var fieldNames = [fieldCount]string{"Date", "ISO 8601", "Year", "Month", "Day", "Hour", "Minute", "Second", "Millisecond", "Epoch year", "Epoch month", "Epoch day", "Epoch hour", "Epoch minute", "Epoch second", "Epoch ms", "Time zone"}
var dateFields = []field{yearField, monthField, dayField, hourField, minuteField, secondField, millisecondField}
var epochFields = []field{epochYearField, epochMonthField, epochDayField, epochHourField, epochMinuteField, epochSecondField, epochMillisecondField}

type converter struct {
	header					toolkit.SettingHeader
	inputs					[fieldCount]textinput.Model
	focus					field
	instant, epoch				time.Time
	loc					*time.Location
	zoneName				string
	useEpoch				bool
	unit					timestampUnit
	width, height, headerHeight, offset	int
	err, epochErr				error
	copied					bool
}

func newConverter() *converter {
	c := &converter{header: toolkit.SettingHeader{
		Names:		[]string{"Default Epoch", "Time zone", "Format"},
		Descriptions:	[]string{"Use a user defined epoch", "", ""},
		Icons:		[]string{"󰃰", "󰖟", "󰉶"},
		Switches:	[]bool{true, false, false},
		Options:	[][]string{nil, zoneIDs(), unitLabels},
		HideTitle:	true,
	}, instant:	time.Now(), epoch: time.Unix(0, 0).UTC(), loc: time.Local, zoneName: "Local", width: 80, height: 24}
	for i := range c.inputs {
		c.inputs[i] = textinput.New()
		c.inputs[i].Prompt = "┃ "
		c.inputs[i].CharLimit = 1024
		if field(i) >= yearField && field(i) <= epochMillisecondField {
			c.inputs[i].CharLimit = 4
		}
		c.inputs[i].TextStyle = lipgloss.NewStyle().Foreground(theme.Text)
		c.inputs[i].PlaceholderStyle = lipgloss.NewStyle().Foreground(theme.Muted)
	}
	c.inputs[zoneField].SetValue("Local")
	c.inputs[timestampField].Focus()
	c.syncFields(fieldCount)
	c.syncEpoch(fieldCount)
	c.resize()
	return c
}
func (*converter) ID() string		{ return "converters.dateparser" }
func (*converter) Title() string	{ return "Date & Time" }
func (*converter) Category() string	{ return "converters" }
func (*converter) Icon() string		{ return "󰃰" }
func (*converter) Description() string	{ return "Convert timestamps and dates between time zones" }
func (*converter) Init() tea.Cmd	{ return nil }

// zoneIDs 返回时区下拉框的选项（顺序与 zones 一致，选项即时区标识）。
func zoneIDs() []string {
	ids := make([]string, 0, len(zones))
	for _, z := range zones {
		ids = append(ids, z.id)
	}
	return ids
}

func (c *converter) values() []string {
	epoch := "No"
	if c.useEpoch {
		epoch = "Yes"
	}
	return []string{epoch, c.zoneName, unitLabels[c.unit]}
}
func (c *converter) resize() {
	if c.height <= 8 {
		c.headerHeight = 0
	} else {
		c.headerHeight = c.header.SetSize(tea.WindowSizeMsg{Width: c.width, Height: c.height}, c.values()...)
		if c.desktopSettings() {
			c.headerHeight++	// Configuration heading
		}
	}
	c.ensureVisible()
}
func (c *converter) desktopSettings() bool {
	return c.width >= 35 && c.header.Expanded(c.values()...)
}
func (c *converter) fields() []field {
	f := []field{timestampField, isoField}
	f = append(f, dateFields...)
	if c.useEpoch {
		f = append(f, epochFields...)
	}
	return append(f, zoneField)
}
func (c *converter) statusHeight() int {
	if c.err != nil {
		return 1
	}
	return 0
}

// topHeight 返回错误行 + 设置条占用的高度；.epoch 组件行在可滚动的
// bodyRows 内渲染。
func (c *converter) topHeight() int {
	return c.statusHeight() + c.headerHeight
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
func (c *converter) syncFields(except field) {
	if except != timestampField {
		v, err := timestampValue(c.instant, c.activeEpoch(), c.unit)
		if err == nil {
			c.inputs[timestampField].SetValue(v)
		}
	}
	local := c.instant.In(c.loc)
	if except != isoField {
		c.inputs[isoField].SetValue(local.Format(time.RFC3339Nano))
	}
	setComponents(&c.inputs, dateFields, local, except)

}
func (c *converter) activeEpoch() time.Time {
	if c.useEpoch {
		return c.epoch
	}
	return time.Unix(0, 0).UTC()
}
func (c *converter) syncEpoch(except field) {
	setComponents(&c.inputs, epochFields, c.epoch.UTC(), except)
}
func (c *converter) applyField(f field) {
	c.applyFieldValues(f)
	// 出错时顶部插入错误行、内容区变矮：重新滚动，保证焦点行不被挤出视口。
	c.ensureVisible()
}

func (c *converter) applyFieldValues(f field) {
	c.err = nil
	c.copied = false
	if c.useEpoch && c.unit != ticks && !isEpochField(f) && c.epochErr != nil {
		c.err = c.epochErr
		return
	}
	if isEpochField(f) {
		value, err := replaceComponent(c.epoch.UTC(), f-epochYearField, c.inputs[f].Value(), time.UTC)
		c.epochErr = err
		c.err = err
		if err == nil {
			c.epoch = value
			c.syncEpoch(f)
			c.syncFields(fieldCount)
		}
		return
	}
	var value time.Time
	switch {
	case f == timestampField:
		value, c.err = timeFromTimestamp(c.inputs[f].Value(), c.activeEpoch(), c.unit)
	case f == isoField:
		value, c.err = parseFlexibleDate(c.inputs[f].Value(), c.loc)
	case f >= yearField && f <= millisecondField:
		value, c.err = replaceComponent(c.instant.In(c.loc), f-yearField, c.inputs[f].Value(), c.loc)
	case f == zoneField:
		var loc *time.Location
		loc, c.err = loadZone(c.inputs[f].Value())
		if c.err == nil {
			c.err = validateInZone(c.instant, loc)
		}
		if c.err == nil {
			c.loc = loc
			c.zoneName = strings.TrimSpace(c.inputs[f].Value())
			c.syncFields(f)
			c.resize()
		}
		return
	}
	if c.err == nil {
		c.err = validateInZone(value, c.loc)
	}
	if c.err == nil {
		c.instant = value
		c.syncFields(f)
	}
}
func (c *converter) cycleZone(dir int) {
	idx := 0
	for i, z := range zones {
		if z.id == c.zoneName {
			idx = i
			break
		}
	}
	idx = (idx + dir + len(zones)) % len(zones)
	c.inputs[zoneField].SetValue(zones[idx].id)
	c.applyField(zoneField)
}

// chooseSetting 处理设置条的一次交互：下拉框选中（choice>=0）直接设值，
// 其余（开关、紧凑布局的回退按钮条）沿用轮换/切换语义。
func (c *converter) chooseSetting(setting, choice int) {
	switch {
	case setting == 1 && choice >= 0:
		c.setZone(choice)
	case setting == 2 && choice >= 0:
		c.setUnit(timestampUnit(choice))
	default:
		c.toggleSetting(setting)
	}
}

// setZone 把时区设为 zones[index]。
func (c *converter) setZone(index int) {
	c.inputs[zoneField].SetValue(zones[index].id)
	c.applyField(zoneField)
	c.resize()
}

// setUnit 设置时间单位，并在 epoch 模式下校验单位是否可用。
func (c *converter) setUnit(unit timestampUnit) {
	c.unit = unit
	c.err = nil
	if c.useEpoch && c.unit != ticks {
		c.err = c.epochErr
	}
	if c.err == nil {
		c.syncFields(fieldCount)
	}
	c.resize()
}

func (c *converter) toggleSetting(i int) {
	switch i {
	case 0:
		c.useEpoch = !c.useEpoch
		if !c.useEpoch && isEpochField(c.focus) {
			c.setFocus(timestampField)
		}
		if c.useEpoch {
			c.err = c.epochErr
			if c.err == nil {
				c.syncFields(fieldCount)
			}
		} else {
			c.err = nil
			c.syncFields(fieldCount)
		}
	case 1:
		c.cycleZone(1)
	case 2:
		c.setUnit((c.unit + 1) % timestampUnit(len(unitLabels)))
		return
	}
	c.resize()
}
func (c *converter) now() {
	c.instant = time.Now()
	c.err = nil
	if c.useEpoch && c.unit != ticks {
		c.err = c.epochErr
	}
	if c.err != nil {
		c.syncFields(timestampField)
	} else {
		c.syncFields(fieldCount)
	}
}
func (c *converter) copy() {
	if err := clipboard.WriteAll(c.inputs[c.focus].Value()); err != nil {
		c.err = fmt.Errorf("could not copy: %w", err)
		return
	}
	c.err = nil
	c.copied = true
}
func (c *converter) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		c.width, c.height = size.Width, size.Height
		c.resize()
		return c, nil
	}
	if mouse, ok := msg.(tea.MouseMsg); ok && c.statusHeight() > 0 {
		if mouse.Y == 0 {
			return c, nil
		}
		mouse.Y--
		msg = mouse
	}
	if c.headerHeight > 0 {
		headerMsg := msg
		if mouse, ok := msg.(tea.MouseMsg); ok && c.desktopSettings() {
			if mouse.Y == 0 {
				return c, nil	// Configuration 标题行
			}
			mouse.Y--
			headerMsg = mouse
		}
		setting, choice, handled := c.header.Pick(headerMsg, c.values()...)
		if handled && setting < 0 {
			// 时区/单位下拉框展开、收起或选择期间，事件全部由浮层消费。
			return c, nil
		}
		if handled {
			c.chooseSetting(setting, choice)
			return c, nil
		}
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "tab":
			return c, c.cycleFocus(1)
		case "shift+tab":
			return c, c.cycleFocus(-1)
		case "alt+e":
			c.toggleSetting(0)
			return c, nil
		case "alt+f":
			c.toggleSetting(2)
			return c, nil
		case "alt+[":
			c.cycleZone(-1)
			c.resize()
			return c, nil
		case "alt+]":
			c.cycleZone(1)
			c.resize()
			return c, nil
		case "alt+z":
			return c, c.setFocus(zoneField)
		case "alt+n":
			c.now()
			return c, nil
		case "ctrl+y":
			c.copy()
			return c, nil
		case "up", "down":
			if c.focus >= yearField && c.focus <= epochMillisecondField {
				n, err := strconv.Atoi(c.inputs[c.focus].Value())
				if err != nil {
					c.err = fmt.Errorf("enter a valid number")
					return c, nil
				}
				if key.String() == "up" {
					n++
				} else {
					n--
				}
				base, part, loc := c.instant.In(c.loc), c.focus-yearField, c.loc
				if isEpochField(c.focus) {
					base, part, loc = c.epoch.UTC(), c.focus-epochYearField, time.UTC
				}
				if _, err = replaceComponent(base, part, strconv.Itoa(n), loc); err != nil {
					c.err = err
					return c, nil
				}
				c.inputs[c.focus].SetValue(strconv.Itoa(n))
				c.applyField(c.focus)
				return c, nil
			}
		}
	}
	if file, ok := msg.(toolkit.FileImportMsg); ok {
		text := strings.TrimSpace(file.Text)
		limit := c.inputs[c.focus].CharLimit
		if (limit > 0 && len([]rune(text))+len([]rune(c.inputs[c.focus].Value())) > limit) || strings.IndexFunc(text, unicode.IsControl) >= 0 {
			c.err = fmt.Errorf("file must contain one short line of text")
			return c, nil
		}
		input := &c.inputs[c.focus]
		value := []rune(input.Value())
		pos := input.Position()
		next := append(append(append([]rune{}, value[:pos]...), []rune(text)...), value[pos:]...)
		input.SetValue(string(next))
		input.SetCursor(pos + len([]rune(text)))
		c.applyField(c.focus)
		return c, nil
	}
	if mouse, ok := msg.(tea.MouseMsg); ok {
		if mouse.Y < c.topHeight() {
			// 错误行与设置条区域：桌面式布局的设置行由 header.Index 处理。
			return c, nil
		}
		if mouse.Action != tea.MouseActionPress {
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
		if mouse.Y >= c.height-1-c.statusHeight() {
			return c, nil
		}
		row := mouse.Y - c.topHeight() + c.offset
		rows := c.bodyRows()
		if row >= 0 && row < len(rows) {
			for _, cell := range rows[row].cells {
				if mouse.X >= cell.start && mouse.X < cell.end {
					if cell.actionEnd > cell.actionStart && mouse.X >= cell.actionStart && mouse.X < cell.actionEnd {
						c.now()
						return c, nil
					}
					cmd := c.setFocus(cell.field)
					if mouse.X >= cell.inputStart {
						c.inputs[cell.field].SetCursor(mouse.X - cell.inputStart)
					}
					return c, cmd
				}
			}
		}
		return c, nil
	}
	before := c.inputs[c.focus].Value()
	var cmd tea.Cmd
	c.inputs[c.focus], cmd = c.inputs[c.focus].Update(msg)
	if c.inputs[c.focus].Value() != before {
		c.applyField(c.focus)
	}
	return c, cmd
}
func (c *converter) rowCount() int	{ return len(c.bodyRows()) }
func (c *converter) bodyHeight() int	{ return max(1, c.height-c.topHeight()-1) }
func (c *converter) maxOffset() int	{ return max(0, c.rowCount()-c.bodyHeight()) }
func (c *converter) ensureVisible() {
	row := 0
	for i, r := range c.bodyRows() {
		for _, cell := range r.cells {
			if cell.field == c.focus {
				row = i
			}
		}
	}
	if row < c.offset {
		c.offset = row
	}
	if row >= c.offset+c.bodyHeight() {
		c.offset = row - c.bodyHeight() + 1
	}
	c.offset = max(0, min(c.offset, c.maxOffset()))
}
func (c *converter) View() string {
	prefix := ""
	if c.statusHeight() > 0 {
		prefix = lipgloss.NewStyle().Foreground(theme.Red).Render("󰅚  "+ansi.Truncate(c.err.Error(), max(1, c.width-3), "…")) + "\n"
	}
	if c.headerHeight > 0 {
		if c.desktopSettings() {
			settings := strings.Split(c.header.View(c.values()...), "\n")
			prefix += theme.Eyebrow("Configuration") + "\n" + settings[0] + "\n"
			if len(settings) > 1 {
				prefix += strings.Join(settings[1:], "\n") + "\n"
			}
		} else {
			prefix += c.header.View(c.values()...) + "\n"
		}
	}
	rows := c.bodyRows()
	body := make([]string, c.bodyHeight())
	for i := range body {
		j := c.offset + i
		if j < len(rows) {
			body[i] = ansi.Truncate(rows[j].text, c.width, "")
		} else {
			body[i] = ""
		}
	}
	// 页脚：ui.StatusBar——左句式 + 右按键提示（超宽由 StatusBar 右侧
	// 截断加 …）；单位由设置条 Format 行承载，页脚不重复。
	footer := c.footerView()
	// 下拉浮层相对设置条定位：设置条之上是状态行（如有）与 Configuration 标题（桌面布局）。
	top := c.statusHeight()
	if c.headerHeight > 0 && c.desktopSettings() {
		top++
	}
	return c.header.OverlayAt(prefix+strings.Join(body, "\n")+"\n"+footer, top)
}

// HandlesEscape 时区/单位下拉浮层展开时 esc 只收起浮层。
func (c *converter) HandlesEscape() bool	{ return c.header.MenuOpen() }

// footerView 渲染底行：ui.StatusBar——右侧只保留 Copy；快捷键由外壳状态栏
// 统一展示，避免同屏两行提示。
func (c *converter) footerView() string {
	bar := ui.NewStatusBar()
	bar.SetWidth(max(1, c.width))
	bar.SetLeftTokens(nil)
	if c.copied {
		bar.SetToast(ui.Toast{Level: ui.LevelSuccess, Message: "Copied to clipboard"})
	}
	return bar.View()
}

// PaneStatusHints 供外壳状态栏合并展示本工具独有的键。
func (c *converter) PaneStatusHints() []ui.StatusHint {
	return []ui.StatusHint{
		{Key: "click", Desc: "edit"},
		{Key: "scroll", Desc: "browse"},
		{Key: "alt+n", Desc: "now"},
		{Key: "alt+e", Desc: "epoch"},
		{Key: "alt+[", Desc: "zone"},
	}
}

func (c *converter) bodyRows() []formRow {
	form := c.formRows()
	epoch := c.epochFormRows()
	local := c.instant.In(c.loc)
	name, offset := local.Zone()
	dst := "Standard time"
	if local.IsDST() {
		dst = "Daylight saving time"
	}
	if ambiguousLocal(local) {
		dst = "Ambiguous local time · " + dst
	}
	sign := "+"
	if offset < 0 {
		sign = "-"
		offset = -offset
	}
	info := fmt.Sprintf("%s · %s %s%02d:%02d", dst, name, sign, offset/3600, (offset%3600)/60)
	ticksValue, _ := ticksOf(c.instant)
	muted := lipgloss.NewStyle().Foreground(theme.Muted)
	value := lipgloss.NewStyle().Foreground(theme.Text)
	// 统计表：2×2 网格（标签 / 数值 交替四列）；放不下时退化为每行一对（标签 / 数值）的竖排。
	offsetText := fmt.Sprintf("%s%02d:%02d", sign, offset/3600, (offset%3600)/60)
	ticksText := strconv.FormatInt(ticksValue, 10)
	localText := local.Format("2006-01-02 15:04:05")
	utcText := c.instant.UTC().Format("2006-01-02 15:04:05")
	labelA, labelB := ui.LabelWidth(0, "UTC offset", "Local"), ui.LabelWidth(0, "UTC ticks", "UTC")
	valueA, valueB := max(lipgloss.Width(offsetText), lipgloss.Width(localText)), max(lipgloss.Width(ticksText), lipgloss.Width(utcText))
	detailLines := []string{"◷  " + value.Render(info)}
	if gridWidth := labelA + valueA + labelB + valueB + 3*ui.Space2; gridWidth <= c.width-8 {
		stats := ui.Table{Gap: ui.Space2, Columns: []ui.Column{{Width: labelA}, {Width: valueA}, {Width: labelB}, {Width: valueB}}}
		_, offsetRow := stats.Row(muted.Render("UTC offset"), value.Render(offsetText), muted.Render("UTC ticks"), value.Render(ticksText))
		_, localRow := stats.Row(muted.Render("Local"), value.Render(localText), muted.Render("UTC"), value.Render(utcText))
		detailLines = append(detailLines, offsetRow, localRow)
	} else {
		stats := ui.Table{Gap: ui.Space2, Columns: []ui.Column{{Width: labelA}, {Width: max(1, c.width-8-labelA-ui.Space2)}}}
		for _, pair := range [][2]string{{"UTC offset", offsetText}, {"UTC ticks", ticksText}, {"Local", localText}, {"UTC", utcText}} {
			_, line := stats.Row(muted.Render(pair[0]), value.Render(pair[1]))
			detailLines = append(detailLines, line)
		}
	}
	detailsCard := lipgloss.NewStyle().Width(max(1, c.width-4)).Padding(0, 1).
		Border(lipgloss.RoundedBorder()).BorderForeground(theme.Border).
		Render(strings.Join(detailLines, "\n"))
	details := make([]formRow, 0, lipgloss.Height(detailsCard))
	for _, line := range strings.Split(detailsCard, "\n") {
		details = append(details, formRow{text: line})
	}
	result := make([]formRow, 0, len(epoch)+len(details)+len(form)+3)
	// 设置条与正文之间留一行空白；Epoch 网格始终位于转换详情之前（不随 headerHeight 改变归属）；
	// 各区块（epoch 网格 / 转换详情 / 字段表单）之间也留一行空白。
	result = append(result, spacerRow())
	result = append(result, epoch...)
	if len(epoch) > 0 {
		result = append(result, spacerRow())
	}
	result = append(result, details...)
	result = append(result, spacerRow())
	result = append(result, form...)
	return result
}
