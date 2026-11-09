// Package cronparser previews upcoming occurrences of a cron expression.
package cronparser

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/robfig/cron/v3"
	"github.com/toy-forge/devtoys/internal/theme"
	"github.com/toy-forge/devtoys/internal/toolkit"
	"github.com/toy-forge/devtoys/internal/ui"
)

var counts = []int{5, 10, 25, 50, 100}

type model struct {
	ws		toolkit.Workspace
	header		toolkit.SettingHeader
	dateFormat	textinput.Model
	formatWidth	int	// 输出格式字段的单元格宽度（随窗口尺寸）
	seconds		bool
	count		int
	dateFocused	bool
	before		string
	width		int
	height		int
}

// countLabels 返回"下一批日期数量"下拉框的选项（与 counts 顺序一致）。
func countLabels() []string {
	labels := make([]string, 0, len(counts))
	for _, c := range counts {
		labels = append(labels, fmt.Sprint(c))
	}
	return labels
}

func newTool() *model {
	m := &model{ws: toolkit.NewWorkspace("Enter a cron expression…", ""), header: toolkit.SettingHeader{
		ContentGap:	1,
		Names:		[]string{"Include seconds", "Next scheduled dates"},
		Descriptions:	[]string{"Whether the Cron expression should include seconds in its definition", "How many scheduled dates need to be generated"},
		Icons:		[]string{"󰦝", "󰎦"},
		Switches:	[]bool{true, false},
		Options:	[][]string{nil, countLabels()},
	}, seconds:	true}
	m.dateFormat = textinput.New()
	m.dateFormat.Placeholder = "yyyy-MM-dd ddd HH:mm:ss"
	m.dateFormat.SetValue("yyyy-MM-dd ddd HH:mm:ss")
	m.dateFormat.Blur()
	m.ws.SetStacked(true)
	m.ws.SetStackedInputHeight(1)
	m.ws.SetPaneTitles("Cron expression to parse", "Next scheduled dates")
	m.ws.Input.Placeholder = "Cron expression (e.g. */5 * * * *)…"
	m.ws.Input.SetValue("* * * * * *")
	m.before = m.ws.Input.Value()
	m.resize(80, 24)
	m.recompute()
	return m
}
func init()				{ toolkit.Register(newTool()) }
func (*model) ID() string		{ return "converters.cron" }
func (*model) Title() string		{ return "Cron expression parser" }
func (*model) ShortTitle() string	{ return "Cron parser" }
func (*model) Category() string		{ return "converters" }
func (*model) Icon() string		{ return "󰃰" }
func (*model) Description() string	{ return "Parse a cron expression and preview upcoming dates" }
func (*model) Init() tea.Cmd		{ return nil }
func (m *model) actions() []string {
	seconds := "Off"
	if m.seconds {
		seconds = "On"
	}
	return []string{seconds, fmt.Sprint(counts[m.count])}
}
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.resize(size.Width, size.Height)
		return m, nil
	}
	if mouse, ok := msg.(tea.MouseMsg); ok && mouse.Action == tea.MouseActionPress && mouse.Button == tea.MouseButtonLeft && m.header.Accessory != "" {
		accessoryStart := m.header.BaseHeight(m.actions()...)
		// 附件区第 0 行是与设置条之间的留白，标题在 +1，字段在 +2。
		if mouse.Y == accessoryStart+2 {
			m.setDateFocus(true)
			// 字段内容从 X=2（缩进）之后的 Space1 列开始。
			m.dateFormat.SetCursor(max(0, mouse.X-2-ui.Space1))
			return m, nil
		}
		if mouse.Y >= accessoryStart && mouse.Y < m.header.Height(m.actions()...) {
			return m, nil
		}
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "tab":
			m.cycleFocus(1)
			return m, nil
		case "shift+tab":
			m.cycleFocus(-1)
			return m, nil
		}
	}
	changed := false
	setting, choice, handled := m.header.Pick(msg, m.actions()...)
	if handled && setting < 0 {
		// 日期数量下拉框展开/收起/选择期间，事件全部由浮层消费。
		return m, nil
	}
	if handled {
		switch {
		case setting == 0:
			m.seconds = !m.seconds
			changed = true
		case setting == 1 && choice >= 0:
			m.count = choice
			changed = true
		case setting == 1:
			// 紧凑布局的回退按钮条：点击即轮换数量。
			m.count = (m.count + 1) % len(counts)
			changed = true
		}
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "alt+s":
			m.seconds = !m.seconds
			changed = true
		case "alt+n":
			// alt+n 打开数量下拉框；设置条不可展开（紧凑布局）时退回轮换。
			if setting, _, _ := m.header.Shortcut(1, m.actions()...); setting == 1 {
				m.count = (m.count + 1) % len(counts)
				changed = true
			}
		}
	}
	if changed {
		m.header.Relayout(&m.ws, m.actions()...)
		m.recompute()
		return m, nil
	}
	var cmd tea.Cmd
	if m.dateFocused {
		old := m.dateFormat.Value()
		m.dateFormat, cmd = m.dateFormat.Update(msg)
		if old != m.dateFormat.Value() {
			m.recompute()
		}
		return m, cmd
	}
	cmd = m.ws.UpdateBelow(msg, m.header.Height(m.actions()...)+1)
	if _, ok := msg.(toolkit.FileImportMsg); ok && m.ws.Err != nil {
		return m, cmd
	}
	if input := m.ws.Input.Value(); input != m.before {
		m.before = input
		m.recompute()
	}
	return m, cmd
}

// HandlesEscape 日期数量下拉浮层展开时 esc 只收起浮层。
func (m *model) HandlesEscape() bool	{ return m.header.MenuOpen() }

// View 渲染设置条与工作区；下拉浮层叠加在完整视图之上。
func (m *model) View() string {
	m.layoutAccessory()
	return m.header.Overlay(m.header.View(m.actions()...) + "\n" + m.ws.View())
}

// layoutAccessory 刷新日期格式附件行，并在放不下时隐藏：
// 矮终端若装不下"设置条 + 附件 + 最小工作区"，View 行数会超过终端高度。
func (m *model) layoutAccessory() {
	m.setFormatAccessory()
	if m.height-m.header.BaseHeight(m.actions()...)-lipgloss.Height(m.header.Accessory) < toolkit.WorkspaceMinHeight {
		m.header.Accessory = ""
		// 附件行隐藏后日期格式输入框不可见，焦点交回 cron 表达式。
		if m.dateFocused {
			m.setDateFocus(false)
		}
	}
}

func (m *model) resize(width, height int) {
	size := tea.WindowSizeMsg{Width: max(1, width), Height: max(1, height)}
	m.width, m.height = size.Width, size.Height
	m.formatWidth = max(1, size.Width-2)
	m.layoutAccessory()
	headerHeight := m.header.SetSize(size, m.actions()...)
	workspaceHeight := max(1, size.Height-headerHeight)
	m.ws.Update(tea.WindowSizeMsg{Width: size.Width, Height: workspaceHeight})
	// The C# page uses a single-line expression field above the full-height schedule.
	m.ws.Input.SetWidth(max(1, size.Width-2))
	m.ws.Input.SetHeight(1)
	m.ws.Output.Width = max(1, size.Width-2)
}

func (m *model) setFormatAccessory() {
	title := lipgloss.NewStyle().Foreground(theme.BlueBright).Bold(true).Render("󰃰 Output format")
	description := lipgloss.NewStyle().Foreground(theme.Muted).Render("Date/time format for upcoming dates")
	m.header.Accessory = "\n" + title + "\n" + ui.FieldView(&m.dateFormat, m.formatWidth, m.dateFormat.Focused()) + "\n" + description
}

func (m *model) setDateFocus(focused bool) {
	m.dateFocused = focused
	if focused {
		m.dateFormat.Focus()
		m.ws.Input.Blur()
		m.ws.Focus = toolkit.FocusInput
		return
	}
	m.dateFormat.Blur()
	m.ws.Input.FocusPreservingScroll()
	m.ws.Focus = toolkit.FocusInput
}

func (m *model) cycleFocus(direction int) {
	current := 1
	if m.dateFocused {
		current = 0
	} else if m.ws.Focus == toolkit.FocusOutput {
		current = 2
	}
	current = (current + direction + 3) % 3
	switch current {
	case 0:
		m.setDateFocus(true)
	case 1:
		m.setDateFocus(false)
	case 2:
		m.setDateFocus(false)
		m.ws.Input.Blur()
		m.ws.Focus = toolkit.FocusOutput
	}
}

func (m *model) recompute() {
	expression := strings.TrimSpace(m.ws.Input.Value())
	if expression == "" {
		m.ws.SetError(nil)
		m.ws.SetStackedStatus("", toolkit.StatusInfo)
		m.ws.SetStackedDetails("Cron description", " ")
		m.ws.SetOutput("")
		return
	}
	dates, err := nextOccurrences(expression, m.seconds, counts[m.count], time.Now())
	if err != nil {
		// The desktop page places its warning directly between the expression
		// input and the description field, rather than in the page footer.
		m.ws.SetError(nil)
		m.ws.SetStackedStatus(err.Error(), toolkit.StatusWarning)
		m.ws.SetStackedDetails("Cron description", " ")
		m.ws.SetOutput("")
		return
	}
	m.ws.SetStackedStatus("", toolkit.StatusInfo)
	m.ws.SetStackedDetails("Cron description", describeExpression(expression, m.seconds))
	var out strings.Builder
	for _, date := range dates {
		out.WriteString(date.Format(goDateLayout(m.dateFormat.Value())))
		out.WriteByte('\n')
	}
	m.ws.SetError(nil)
	m.ws.SetOutput(strings.TrimSuffix(out.String(), "\n"))
}

func describeExpression(expression string, seconds bool) string {
	if strings.HasPrefix(expression, "@every ") {
		return "Runs every " + strings.TrimPrefix(expression, "@every ")
	}
	switch expression {
	case "@yearly", "@annually":
		return "Runs once a year, at midnight on January 1"
	case "@monthly":
		return "Runs at midnight on the first day of every month"
	case "@weekly":
		return "Runs at midnight every Sunday"
	case "@daily", "@midnight":
		return "Runs once a day, at midnight"
	case "@hourly":
		return "Runs at the start of every hour"
	}
	fields := strings.Fields(expression)
	if len(fields) == 5 && !seconds {
		fields = append([]string{"0"}, fields...)
	}
	if len(fields) != 6 {
		return "Upcoming run times for this cron expression"
	}
	names := []string{"second", "minute", "hour", "day of month", "month", "day of week"}
	parts := make([]string, 0, len(fields))
	for i, field := range fields {
		switch {
		case field == "*" || field == "?":
			parts = append(parts, "every "+names[i])
		case strings.HasPrefix(field, "*/"):
			unit := names[i]
			parts = append(parts, "every "+strings.TrimPrefix(field, "*/")+" "+unit+"s")
		case strings.ContainsAny(field, "*/?,"):
			return "Upcoming run times for this cron expression"
		default:
			parts = append(parts, names[i]+" "+field)
		}
	}
	return "Runs " + strings.Join(parts, ", ")
}

func goDateLayout(format string) string {
	type token struct{ csharp, goLayout string }
	tokens := [...]token{
		{"yyyy", "2006"}, {"yyy", "2006"}, {"yy", "06"}, {"y", "6"},
		{"MMMM", "January"}, {"MMM", "Jan"}, {"MM", "01"}, {"M", "1"},
		{"dddd", "Monday"}, {"ddd", "Mon"}, {"dd", "02"}, {"d", "2"},
		{"HH", "15"}, {"H", "15"}, {"hh", "03"}, {"h", "3"},
		{"mm", "04"}, {"m", "4"}, {"ss", "05"}, {"s", "5"},
		{"fff", "000"}, {"ff", "00"}, {"f", "0"},
		{"zzz", "-07:00"}, {"zz", "-07"}, {"z", "-7"},
		{"tt", "PM"}, {"t", "PM"}, {"K", "Z07:00"},
	}
	if format == "" {
		return "2006-01-02 15:04:05"
	}
	var layout strings.Builder
	for i := 0; i < len(format); {
		if format[i] == '\\' && i+1 < len(format) {
			layout.WriteByte(format[i+1])
			i += 2
			continue
		}
		matched := false
		for _, candidate := range tokens {
			if strings.HasPrefix(format[i:], candidate.csharp) {
				layout.WriteString(candidate.goLayout)
				i += len(candidate.csharp)
				matched = true
				break
			}
		}
		if !matched {
			layout.WriteByte(format[i])
			i++
		}
	}
	return layout.String()
}

func nextOccurrences(expression string, seconds bool, count int, now time.Time) ([]time.Time, error) {
	opts := cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor
	if seconds {
		opts |= cron.Second
	}
	schedule, err := cron.NewParser(opts).Parse(expression)
	if err != nil {
		return nil, fmt.Errorf("invalid cron expression: %w", err)
	}
	dates := make([]time.Time, 0, count)
	for range count {
		next := schedule.Next(now)
		if next.IsZero() {
			return nil, fmt.Errorf("no future occurrence found")
		}
		dates = append(dates, next)
		now = next
	}
	return dates, nil
}
