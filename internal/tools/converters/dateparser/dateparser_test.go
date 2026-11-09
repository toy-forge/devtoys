package dateparser

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/toy-forge/devtoys/internal/toolkit"
)

// TestParseTimestamps 验证纯数字输入按位数识别 秒/毫秒/微秒/纳秒。
func TestParseTimestamps(t *testing.T) {
	cases := []struct {
		in	string
		want	time.Time
		check	func(a, b time.Time) bool
	}{
		{"1700000000", time.Unix(1700000000, 0), time.Time.Equal},			// 秒
		{"1700000000000", time.UnixMilli(1700000000000), time.Time.Equal},		// 毫秒
		{"1700000000000000", time.UnixMicro(1700000000000000), time.Time.Equal},	// 微秒
		{"1700000000000000000", time.Unix(0, 1700000000000000000), time.Time.Equal},	// 纳秒
		{"-86400", time.Unix(-86400, 0), time.Time.Equal},				// 负时间戳
	}
	for _, c := range cases {
		got, err := parseInput(c.in)
		if err != nil {
			t.Fatalf("parseInput(%q) 不应报错: %v", c.in, err)
		}
		if !c.check(got, c.want) {
			t.Fatalf("parseInput(%q) = %v, want %v", c.in, got.UTC(), c.want.UTC())
		}
	}
}

// TestParseLayouts 验证常见日期布局的解析。
func TestParseLayouts(t *testing.T) {
	cases := []struct {
		in	string
		want	time.Time
	}{
		{"2026-01-02T03:04:05Z", time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)},
		{"2026-01-02 03:04:05", time.Date(2026, 1, 2, 3, 4, 5, 0, time.Local)},
		{"2026-01-02", time.Date(2026, 1, 2, 0, 0, 0, 0, time.Local)},
		{"2026/01/02", time.Date(2026, 1, 2, 0, 0, 0, 0, time.Local)},
		// 8 位无符号数字优先按紧凑日期 YYYYMMDD 解析，
		// 不是合法日期的 8 位数字仍按 Unix 时间戳处理。
		{"20260102", time.Date(2026, 1, 2, 0, 0, 0, 0, time.Local)},
		{"99991399", time.Unix(99991399, 0)},
	}
	for _, c := range cases {
		got, err := parseInput(c.in)
		if err != nil {
			t.Fatalf("parseInput(%q) 不应报错: %v", c.in, err)
		}
		if !got.Equal(c.want) {
			t.Fatalf("parseInput(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// TestParseInvalid 非法输入应返回带建议的中文错误。
func TestParseInvalid(t *testing.T) {
	for _, in := range []string{"abc", "2026-13-99", "12时34分"} {
		_, err := parseInput(in)
		if err == nil {
			t.Fatalf("parseInput(%q) 应报错", in)
		}
		if !strings.Contains(err.Error(), "cannot parse") {
			t.Fatalf("错误信息应友好，得到: %v", err)
		}
	}
	// 超出 int64 范围的数字应提示超出范围。
	if _, err := parseInput("99999999999999999999999"); err == nil ||
		!strings.Contains(err.Error(), "out of range") {
		t.Fatalf("超长时间戳应报范围错误，得到: %v", err)
	}
}

// TestUnixTimePrecision 直接验证位数到精度的映射边界。
func TestUnixTimePrecision(t *testing.T) {
	// 12 位仍按秒。
	if got := unixTime(123456789012, 12); got.Unix() != 123456789012 {
		t.Fatalf("12 位应按秒解析，得到 %v", got)
	}
	// 13 位按毫秒。
	if got := unixTime(1700000000000, 13); got.UnixMilli() != 1700000000000 {
		t.Fatalf("13 位应按毫秒解析，得到 %v", got)
	}
	// 15 位按毫秒。
	if got := unixTime(170000000000000, 15); got.UnixMilli() != 170000000000000 {
		t.Fatalf("15 位应按毫秒解析，得到 %v", got)
	}
	// 16 位按微秒。
	if got := unixTime(1700000000000000, 16); got.UnixMicro() != 1700000000000000 {
		t.Fatalf("16 位应按微秒解析，得到 %v", got)
	}
	// 19 位按纳秒。
	if got := unixTime(1700000000000000000, 19); got.UnixNano() != 1700000000000000000 {
		t.Fatalf("19 位应按纳秒解析，得到 %v", got)
	}
}

// TestFormatAll 验证多格式输出包含关键行且时区正确。
func TestFormatAll(t *testing.T) {
	tt := time.Unix(1700000000, 0).UTC()	// 2023-11-14 22:13:20 UTC，星期二
	out := formatAll(tt, zones[1])		// UTC
	for _, want := range []string{
		"UTC",
		"2023-11-14T22:13:20Z",
		"Unix sec    : 1700000000",
		"Unix ms     : 1700000000000",
		"Weekday     : Tuesday",
		"Day of year : 318",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("输出缺少 %q:\n%s", want, out)
		}
	}

	// 固定时区：上海比 UTC 快 8 小时。
	out = formatAll(tt, zone{id: "Asia/Shanghai", display: "China Standard Time"})
	if !strings.Contains(out, "2023-11-15T06:13:20+08:00") {
		t.Fatalf("上海时区输出不正确:\n%s", out)
	}
}

// TestWeekdayCN 验证星期中文名映射。
func TestWeekdayCN(t *testing.T) {
	want := []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
	for d := time.Sunday; d <= time.Saturday; d++ {
		if got := weekdayCN(d); got != want[d] {
			t.Fatalf("weekdayCN(%d) = %q, want %q", d, got, want[d])
		}
	}
}

// TestRecomputeViaModel 验证工具模型：空输入提示、正常输出与错误信息条。
func TestRecomputeViaModel(t *testing.T) {
	m := &model{ws: toolkit.NewWorkspace("test", "")}

	m.recompute("")
	if m.ws.Err != nil {
		t.Fatalf("空输入不应报错: %v", m.ws.Err)
	}
	if m.ws.OutputText() != "Waiting for input…" {
		t.Fatalf("空输入应提示等待，得到 %q", m.ws.OutputText())
	}

	m.recompute("1700000000")
	if m.ws.Err != nil {
		t.Fatalf("有效时间戳不应报错: %v", m.ws.Err)
	}
	if !strings.Contains(m.ws.OutputText(), "1700000000") {
		t.Fatalf("输出应包含格式化结果:\n%s", m.ws.OutputText())
	}

	m.recompute("not a date")
	if m.ws.Err == nil {
		t.Fatal("非法输入应在信息条报错")
	}
}

func TestMouseTimezoneAndLiteralBrackets(t *testing.T) {
	m := &model{ws: toolkit.NewWorkspace("", "")}
	m.Update(tea.WindowSizeMsg{Width: 68, Height: 20})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	if m.ws.Input.Value() != "[" || m.zoneIdx != 0 {
		t.Fatalf("input=%q zone=%d", m.ws.Input.Value(), m.zoneIdx)
	}
	m.Update(tea.MouseMsg{X: 1, Y: 0, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if m.zoneIdx != len(zones)-1 {
		t.Fatalf("zone=%d", m.zoneIdx)
	}
}
