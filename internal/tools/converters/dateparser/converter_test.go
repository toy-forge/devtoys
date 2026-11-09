package dateparser

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/toy-forge/devtoys/internal/toolkit"
)

func TestTimestampUnitsAndCustomEpoch(t *testing.T) {
	epoch := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		unit	timestampUnit
		input	string
		want	time.Time
	}{
		{seconds, "0", epoch}, {seconds, "-1", epoch.Add(-time.Second)},
		{milliseconds, "1", epoch.Add(time.Millisecond)}, {milliseconds, "-1", epoch.Add(-time.Millisecond)},
		{ticks, "0", time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC)},
		{ticks, "621355968000000001", time.Unix(0, 100).UTC()},
		{ticks, "3155378975999999999", time.Date(9999, 12, 31, 23, 59, 59, 999999900, time.UTC)},
	}
	for _, tc := range cases {
		got, err := timeFromTimestamp(tc.input, epoch, tc.unit)
		if err != nil || !got.Equal(tc.want) {
			t.Fatalf("%v %s: got %v, %v", tc.unit, tc.input, got, err)
		}
		back, err := timestampValue(got, epoch, tc.unit)
		if err != nil || back != tc.input {
			t.Fatalf("%v %s: roundtrip %q %v", tc.unit, tc.input, back, err)
		}
	}
	for _, value := range []string{"-1", "3155378976000000000", "9999999999999999999999999999", "x"} {
		if _, err := timeFromTimestamp(value, epoch, ticks); err == nil {
			t.Fatalf("invalid ticks accepted: %s", value)
		}
	}
	before := epoch.Add(-500 * time.Millisecond)
	if got, _ := timestampValue(before, epoch, seconds); got != "0" {
		t.Fatalf("negative fraction must truncate toward zero: %s", got)
	}
}
func TestDSTAndTimezone(t *testing.T) {
	loc, err := loadZone("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	winter, err := parseDateTime("2024-01-15", "12:34:56", loc)
	if err != nil {
		t.Fatal(err)
	}
	summer, err := parseDateTime("2024-07-15", "12:34:56", loc)
	if err != nil {
		t.Fatal(err)
	}
	if winter.IsDST() || !summer.IsDST() {
		t.Fatal("DST state incorrect")
	}
	if _, err := parseDateTime("2024-03-10", "02:30:00", loc); err == nil {
		t.Fatal("accepted non-existent local time")
	}
	if _, err := parseFlexibleDate("2024-03-10 02:30:00", loc); err == nil {
		t.Fatal("accepted non-existent local time in date parser")
	}
	if _, err := parseDateTime("2024-02-30", "00:00:00", loc); err == nil {
		t.Fatal("accepted invalid date")
	}
	localDate, err := parseFlexibleDate("2024-07-15 12:34:56", loc)
	if err != nil || !localDate.Equal(summer) {
		t.Fatalf("selected zone not applied to date input: %v %v", localDate, err)
	}
	if _, err := loadZone("Invalid/Zone"); err == nil {
		t.Fatal("accepted invalid time zone")
	}
}
func TestConverterFieldsStaySynchronized(t *testing.T) {
	c := newConverter()
	c.inputs[zoneField].SetValue("UTC")
	c.applyField(zoneField)
	c.inputs[timestampField].SetValue("1700000000")
	c.applyField(timestampField)
	if c.err != nil || c.inputs[isoField].Value() != "2023-11-14T22:13:20Z" {
		t.Fatalf("timestamp: %s %v", c.inputs[isoField].Value(), c.err)
	}
	c.inputs[isoField].SetValue("2000-01-01T00:00:01Z")
	c.applyField(isoField)
	if c.err != nil || c.inputs[timestampField].Value() != "946684801" {
		t.Fatalf("ISO: %s %v", c.inputs[timestampField].Value(), c.err)
	}
	c.toggleSetting(0)
	c.inputs[epochYearField].SetValue("2000")
	c.applyField(epochYearField)
	if c.err != nil || c.inputs[timestampField].Value() != "1" {
		t.Fatalf("custom epoch: %s %v", c.inputs[timestampField].Value(), c.err)
	}
	c.toggleSetting(2)
	if c.inputs[timestampField].Value() != "1000" {
		t.Fatalf("milliseconds: %s", c.inputs[timestampField].Value())
	}
	c.toggleSetting(2)
	if c.inputs[timestampField].Value() != "630822816010000000" {
		t.Fatalf("ticks: %s", c.inputs[timestampField].Value())
	}
	c.inputs[secondField].SetValue("0")
	c.applyField(secondField)
	c.inputs[dayField].SetValue("2")
	c.applyField(dayField)
	if c.err != nil || c.inputs[isoField].Value() != "2000-01-02T00:00:00Z" {
		t.Fatalf("date field: %s %v", c.inputs[isoField].Value(), c.err)
	}
	old := c.instant
	c.inputs[isoField].SetValue("not a date")
	c.applyField(isoField)
	if c.err == nil || !c.instant.Equal(old) {
		t.Fatal("invalid ISO changed current instant")
	}
}
func TestConverterLayoutAndMouse(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 30, Height: 7}, {Width: 20, Height: 7}} {
		c := newConverter()
		c.Update(size)
		if w, h := lipgloss.Width(c.View()), lipgloss.Height(c.View()); w > size.Width || h != size.Height {
			t.Fatalf("%v rendered %dx%d", size, w, h)
		}
		for range c.fields() {
			c.Update(tea.KeyMsg{Type: tea.KeyTab})
			// 每个字段都必须有一个单元格落在表格里（定宽输入框，无提示符）。
			found := false
			for _, row := range c.bodyRows() {
				for _, cell := range row.cells {
					found = found || cell.field == c.focus
				}
			}
			if !found {
				t.Fatalf("focused field %d has no table cell at %v", c.focus, size)
			}
		}
		c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}, Alt: true})
		if !c.useEpoch {
			t.Fatal("epoch shortcut did not work")
		}
		c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'z'}, Alt: true})
		if c.focus != zoneField {
			t.Fatal("zone shortcut did not focus field")
		}
		c.inputs[zoneField].SetValue("")
		c.Update(toolkit.FileImportMsg{Text: "Asia/Singapore\n"})
		if c.err != nil || c.zoneName != "Asia/Singapore" {
			t.Fatalf("file import: %s %v", c.zoneName, c.err)
		}
	}
}

func TestRangeAndInvalidEpochLeaveInstantIntact(t *testing.T) {
	c := newConverter()
	c.inputs[zoneField].SetValue("UTC")
	c.applyField(zoneField)
	c.inputs[isoField].SetValue("0001-01-01T00:00:00Z")
	c.applyField(isoField)
	if c.err != nil {
		t.Fatal(c.err)
	}
	c.inputs[zoneField].SetValue("America/New_York")
	c.applyField(zoneField)
	if c.err == nil || c.zoneName != "UTC" {
		t.Fatalf("out-of-range local time accepted: %q %v", c.zoneName, c.err)
	}
	c.toggleSetting(0)
	c.inputs[epochYearField].SetValue("bad")
	c.applyField(epochYearField)
	before := c.instant
	c.inputs[timestampField].SetValue("0")
	c.applyField(timestampField)
	if c.err == nil || !c.instant.Equal(before) {
		t.Fatal("invalid custom epoch was silently used")
	}
}

func TestIndependentComponentsAndLeapYear(t *testing.T) {
	c := newConverter()
	c.inputs[zoneField].SetValue("UTC")
	c.applyField(zoneField)
	c.inputs[isoField].SetValue("2024-02-29T12:30:45.1234567Z")
	c.applyField(isoField)
	for _, tc := range []struct {
		f	field
		want	string
	}{{yearField, "2024"}, {monthField, "02"}, {dayField, "29"}, {hourField, "12"}, {minuteField, "30"}, {secondField, "45"}, {millisecondField, "123"}} {
		if got := c.inputs[tc.f].Value(); got != tc.want {
			t.Fatalf("field %s=%q, want %q", fieldNames[tc.f], got, tc.want)
		}
	}
	c.inputs[yearField].SetValue("2023")
	c.applyField(yearField)
	if c.err != nil || c.inputs[isoField].Value() != "2023-02-28T12:30:45.1234567Z" {
		t.Fatalf("year clamp %s %v", c.inputs[isoField].Value(), c.err)
	}
	c.inputs[millisecondField].SetValue("987")
	c.applyField(millisecondField)
	if c.err != nil || c.inputs[isoField].Value() != "2023-02-28T12:30:45.9874567Z" {
		t.Fatalf("fraction preservation %s %v", c.inputs[isoField].Value(), c.err)
	}
	c.setFocus(minuteField)
	c.Update(tea.KeyMsg{Type: tea.KeyUp})
	if c.err != nil || c.inputs[minuteField].Value() != "31" {
		t.Fatalf("up arrow failed: %s %v", c.inputs[minuteField].Value(), c.err)
	}
	c.toggleSetting(0)
	c.inputs[epochYearField].SetValue("2000")
	c.applyField(epochYearField)
	c.inputs[epochMonthField].SetValue("02")
	c.applyField(epochMonthField)
	c.inputs[epochDayField].SetValue("29")
	c.applyField(epochDayField)
	if c.err != nil || c.epoch.Format("2006-01-02") != "2000-02-29" {
		t.Fatalf("epoch components %v %v", c.epoch, c.err)
	}
}

func TestComponentMouseAndScrollableLayout(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 69, Height: 12}, {Width: 20, Height: 7}} {
		c := newConverter()
		c.Update(size)
		c.toggleSetting(0)
		for _, f := range []field{yearField, dayField, millisecondField, epochYearField, epochMillisecondField, zoneField} {
			c.setFocus(f)
			if w, h := lipgloss.Width(c.View()), lipgloss.Height(c.View()); w > size.Width || h != size.Height {
				t.Fatalf("%v %v rendered %dx%d", size, f, w, h)
			}
			found := false
			for row, layout := range c.bodyRows() {
				for _, cell := range layout.cells {
					if cell.field == f {
						y := c.headerHeight + row - c.offset
						if y < c.headerHeight || y >= size.Height-1 {
							t.Fatalf("%v %v not visible: row %d", size, f, y)
						}
						c.Update(tea.MouseMsg{X: cell.inputStart, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
						if c.focus != f {
							t.Fatalf("%v click focused %v, wanted %v", size, c.focus, f)
						}
						found = true
					}
				}
			}
			if !found {
				t.Fatalf("missing %v at %v", f, size)
			}
		}
	}
}
func TestInvalidFileDoesNotTruncateComponent(t *testing.T) {
	c := newConverter()
	c.setFocus(yearField)
	before := c.inputs[yearField].Value()
	c.Update(toolkit.FileImportMsg{Text: "2345"})
	if c.err == nil || c.inputs[yearField].Value() != before {
		t.Fatal("file was silently truncated")
	}
}

func TestAmbiguousLocalAndIncrementBounds(t *testing.T) {
	loc, err := loadZone("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := parseDateTime("2024-11-03", "01:30:00", loc)
	if err != nil || !ambiguousLocal(repeated) {
		t.Fatalf("missing ambiguous local time: %v %v", repeated, err)
	}
	normal, err := parseDateTime("2024-11-03", "03:30:00", loc)
	if err != nil || ambiguousLocal(normal) {
		t.Fatalf("incorrect ambiguity: %v %v", normal, err)
	}
	c := newConverter()
	c.inputs[zoneField].SetValue("UTC")
	c.applyField(zoneField)
	c.inputs[isoField].SetValue("9999-01-01T00:00:00Z")
	c.applyField(isoField)
	before := c.instant
	c.setFocus(yearField)
	c.Update(tea.KeyMsg{Type: tea.KeyUp})
	if c.err == nil || !c.instant.Equal(before) || c.inputs[yearField].Value() != "9999" {
		t.Fatalf("increment overflow changed year: %s %v", c.inputs[yearField].Value(), c.err)
	}
}

func TestEpochFieldsPrecedeConversionDetails(t *testing.T) {
	c := newConverter()
	c.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	c.toggleSetting(0)
	// 第 0 行是设置条与正文之间的留白；第 1 行是 epoch 网格表头，第 2、3 行是 Epoch date / Epoch time。
	rows := c.bodyRows()
	if !strings.Contains(rows[2].text, "Epoch date") || !strings.Contains(rows[3].text, "Epoch time") {
		t.Fatalf("epoch controls are not above conversion details: %q / %q", rows[2].text, rows[3].text)
	}
	// epoch 网格之后是一行留白，再是详情卡片（圆角边框行）。
	if rows[4].text != "" || !strings.Contains(rows[5].text, "╭") {
		t.Fatalf("conversion details should follow the epoch grid after a spacer: %q / %q", rows[4].text, rows[5].text)
	}
	if len(rows[2].cells) != 3 || len(rows[3].cells) != 4 {
		t.Fatalf("unexpected epoch controls: %d / %d", len(rows[2].cells), len(rows[3].cells))
	}
}
