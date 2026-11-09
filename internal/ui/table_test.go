package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// 每一行都必须恰好是列宽之和（渲染与命中共用同一份布局）。
func TestTableRowsAreExactWidthAndHitRectsMatch(t *testing.T) {
	table := Table{Gap: Space2, Columns: []Column{{Title: "Name", Width: 6}, {Title: "Value", Width: 8, Right: true}}}
	rects, line := table.Row("abcdefghijk", "42")
	if got := lipgloss.Width(line); got != 6+Space2+8 {
		t.Fatalf("row width = %d, want %d: %q", got, 6+Space2+8, line)
	}
	if rects[0] != (Rect{X: 0, Y: 0, W: 6, H: 1}) || rects[1] != (Rect{X: 6 + Space2, Y: 0, W: 8, H: 1}) {
		t.Fatalf("rects = %+v", rects)
	}
	// 超长文本截断为 …，不撑开列。
	if plain := ansi.Strip(line); !strings.Contains(plain, "abcde…") {
		t.Fatalf("long cell not truncated: %q", plain)
	}
}

func TestTableRightAlignsNumericColumn(t *testing.T) {
	table := Table{Gap: Space2, Columns: []Column{{Width: 4}, {Width: 6, Right: true}}}
	_, line := table.Row("a", "7")
	if !strings.HasSuffix(line, "     7") {
		t.Fatalf("value not right aligned: %q", line)
	}
}

func TestTableHeaderMatchesRowColumns(t *testing.T) {
	table := Table{Gap: Space2, Columns: []Column{{Title: "Year", Width: 6}, {Title: "Month", Width: 5}}}
	_, row := table.Row("2026", "11")
	if lipgloss.Width(table.Header()) != lipgloss.Width(row) {
		t.Fatalf("header width %d != row width %d", lipgloss.Width(table.Header()), lipgloss.Width(row))
	}
}

func TestFieldViewIsExactWidthForAnyValue(t *testing.T) {
	in := textinput.New()
	for _, value := range []string{"", "2", "2026-11-09T16:02:22.229698+08:00 and more text"} {
		in.SetValue(value)
		for _, focused := range []bool{false, true} {
			view := FieldView(&in, 8, focused)
			if got := lipgloss.Width(view); got != 8 {
				t.Fatalf("value %q focused=%v: width %d, want 8", value, focused, got)
			}
		}
	}
}

func TestFormRowKeepsControlAlignedAcrossLabels(t *testing.T) {
	width := LabelWidth(10, "Short", "Much longer label")
	a := FormRow("Short", width, "X", 30)
	b := FormRow("Much longer label", width, "Y", 30)
	if lipgloss.Width(a) != 30 || lipgloss.Width(b) != 30 {
		t.Fatalf("widths %d / %d", lipgloss.Width(a), lipgloss.Width(b))
	}
	if strings.Index(ansi.Strip(a), "X") != strings.Index(ansi.Strip(b), "Y") {
		t.Fatalf("controls not aligned:\n%q\n%q", ansi.Strip(a), ansi.Strip(b))
	}
}

func TestFormRowRightPushesControlToEdge(t *testing.T) {
	line := FormRowRight("Mode", 4, "ON", 12)
	if !strings.HasSuffix(ansi.Strip(line), "ON") || lipgloss.Width(line) != 12 {
		t.Fatalf("right control: %q", ansi.Strip(line))
	}
}
