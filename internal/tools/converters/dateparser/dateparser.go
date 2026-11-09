// Package dateparser 实现日期时间解析工具。
// 输入日期字符串或 Unix 时间戳（自动识别 秒/毫秒/微秒/纳秒），
// 即时输出 RFC3339、ISO8601、本地时间、Unix 秒/毫秒等常见格式；
// 附加按键 alt+[ / alt+] 循环切换输出时区。出错显示在信息条。
package dateparser

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"	// 内嵌时区数据，保证无系统 tzdata 的环境也能切换时区
	"unicode"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/toy-forge/devtoys/internal/toolkit"
)

// zone 是可切换的输出时区项。
type zone struct {
	id	string	// IANA 时区 ID；"Local" 表示系统本地时区
	display	string	// 显示名
}

// zones 是时区循环切换的顺序。
var zones = []zone{
	{"Local", "Local time"},
	{"UTC", "UTC"},
	{"Asia/Shanghai", "China Standard Time"},
	{"Asia/Tokyo", "Japan Standard Time"},
	{"America/New_York", "US Eastern Time"},
	{"America/Los_Angeles", "US Pacific Time"},
	{"Europe/London", "UK Time"},
}

// layouts 是按优先级尝试的日期解析格式。
// 不带时区的格式会按本地时区解析。
var layouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02T15:04",
	"2006-01-02 15:04:05.999999999 -07:00 MST",
	"2006-01-02 15:04:05 -07:00 MST",
	"2006-01-02 15:04:05 -07:00",
	"2006-01-02 15:04:05",
	"2006-01-02 15:04",
	"2006-01-02",
	"2006/01/02 15:04:05",
	"2006/01/02",
	time.RFC1123Z,
	time.RFC1123,
	time.RFC822Z,
	time.RFC822,
	time.ANSIC,
	time.UnixDate,
	time.RubyDate,
	"20060102",
	"01/02/2006",
}

// model 持有公共工作区、时区选择与上一次的输入内容。
type model struct {
	header		toolkit.ActionHeader
	ws		toolkit.Workspace
	zoneIdx		int	// 当前时区在 zones 中的下标
	before		string	// 上一次触发重算时的输入内容
	lastTime	time.Time
}

func init() {
	toolkit.Register(newConverter())
}

func (m *model) ID() string		{ return "converters.dateparser" }
func (m *model) Title() string		{ return "Date Converter" }
func (m *model) ShortTitle() string	{ return "Date" }
func (m *model) Category() string	{ return "converters" }
func (m *model) Icon() string		{ return "[T]" }
func (m *model) Description() string {
	return "Parse dates and timestamps in multiple time zones"
}

func (m *model) Init() tea.Cmd	{ return nil }

// Update 拦截时区切换按键，其余转发给工作区；输入变化时即时重算。
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.header.Index(msg, m.actions()...) {
	case 0:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}, Alt: true}
	case 1:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}, Alt: true}
	}
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		return m, m.header.Resize(sz, &m.ws, m.actions()...)
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "alt+[":
			m.zoneIdx = (m.zoneIdx - 1 + len(zones)) % len(zones)
			m.header.Relayout(&m.ws, m.actions()...)
			m.recompute(m.ws.Input.Value())
			return m, nil
		case "alt+]":
			m.zoneIdx = (m.zoneIdx + 1) % len(zones)
			m.header.Relayout(&m.ws, m.actions()...)
			m.recompute(m.ws.Input.Value())
			return m, nil
		}
	}

	cmd := m.ws.UpdateBelow(msg, m.header.Height(m.actions()...))
	if cur := m.ws.Input.Value(); cur != m.before {
		m.before = cur
		m.recompute(cur)
	}
	return m, cmd
}

// View 渲染时区选择头部与工作区。
func (m *model) actions() []string	{ return []string{"< Time zone", zones[m.zoneIdx].display + " >"} }
func (m *model) View() string		{ return m.header.View(m.actions()...) + "\n" + m.ws.View() }

// recompute 解析输入并按当前时区输出常见格式。
func (m *model) recompute(input string) {
	m.ws.SetError(nil)

	raw := strings.TrimSpace(input)
	if raw == "" {
		m.ws.SetOutput("Waiting for input…")
		return
	}

	t, err := parseInput(raw)
	if err != nil {
		m.ws.SetError(err)
		return
	}
	m.lastTime = t
	m.ws.SetOutput(formatAll(t, zones[m.zoneIdx]))
}

// parseInput 解析输入：纯数字按 Unix 时间戳（自动识别精度），
// 其余按常见布局尝试；失败时返回带可读提示的错误。
func parseInput(s string) (time.Time, error) {
	// 纯数字（允许前置正负号）视为 Unix 时间戳。
	digitsOnly := true
	for i, r := range s {
		if i == 0 && (r == '-' || r == '+') {
			continue
		}
		if !unicode.IsDigit(r) {
			digitsOnly = false
			break
		}
	}
	if digitsOnly {
		// 8 位无符号数字优先按紧凑日期 YYYYMMDD 解析（如 20260102），
		// 只有当它不是合法日期时才回落到 Unix 时间戳。
		if strings.TrimLeft(s, "+-") == s && len(s) == 8 {
			if t, err := time.ParseInLocation("20060102", s, time.Local); err == nil {
				return t, nil
			}
		}
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("timestamp out of range: %s", s)
		}
		return unixTime(n, len(strings.TrimLeft(s, "+-"))), nil
	}

	// 带时区的布局用 time.Parse，不带的按本地时区解析。
	for _, layout := range layouts {
		hasZone := strings.ContainsAny(layout, "ZX")
		if hasZone {
			if t, err := time.Parse(layout, s); err == nil {
				return t, nil
			}
			continue
		}
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf(
		"cannot parse %q: enter a date such as 2026-01-02 15:04:05, RFC3339, or a Unix timestamp", s)
}

// unixTime 根据时间戳位数识别秒/毫秒/微秒/纳秒精度。
func unixTime(n int64, digits int) time.Time {
	switch {
	case digits <= 12:	// 秒（含负数年份范围的历史时间戳）
		return time.Unix(n, 0)
	case digits <= 15:	// 毫秒
		return time.UnixMilli(n)
	case digits <= 18:	// 微秒
		return time.UnixMicro(n)
	default:	// 纳秒
		return time.Unix(0, n)
	}
}

// formatAll 把解析出的时间按指定时区渲染为多行常见格式。
func formatAll(t time.Time, z zone) string {
	loc := time.Local
	if z.id != "Local" {
		loaded, err := time.LoadLocation(z.id)
		if err != nil {
			// 理论上不会发生：tzdata 已内嵌，此处兜底为 UTC。
			loaded = time.UTC
		}
		loc = loaded
	}
	in := t.In(loc)

	// 输出是可复制的纯文本：标签列宽取最长标签，冒号对齐，无需样式。
	pairs := [][2]string{
		{"Time zone", fmt.Sprintf("%s (%s)", z.display, in.Format("-07:00"))},
		{"RFC3339", in.Format(time.RFC3339)},
		{"ISO8601", in.Format("2006-01-02T15:04:05.000-0700")},
		{"Local time", t.Local().Format("2006-01-02 15:04:05 -07:00")},
		{"UTC", t.UTC().Format("2006-01-02 15:04:05 -07:00")},
		{"RFC1123", in.Format(time.RFC1123)},
		{"Unix sec", fmt.Sprint(t.Unix())},
		{"Unix ms", fmt.Sprint(t.UnixMilli())},
		{"Weekday", weekdayCN(in.Weekday())},
		{"Day of year", fmt.Sprint(in.YearDay())},
	}
	labelWidth := 0
	for _, pair := range pairs {
		labelWidth = max(labelWidth, len(pair[0]))
	}
	var b strings.Builder
	for _, pair := range pairs {
		fmt.Fprintf(&b, "%-*s : %s\n", labelWidth, pair[0], pair[1])
	}
	return b.String()
}

// weekdayCN 返回星期几的中文名称。
func weekdayCN(w time.Weekday) string {
	names := []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
	return names[int(w)]
}
