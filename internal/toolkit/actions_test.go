package toolkit

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	textarea "github.com/toy-forge/devtoys/internal/toolkit/editor"
)

func TestActionsClickUsesCellWidths(t *testing.T) {
	labels := []string{"Encode", "URL-safe"}
	if got := ActionAt(tea.MouseMsg{X: 9, Y: 0, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}, labels...); got != 1 {
		t.Fatalf("action=%d", got)
	}
	if got := ActionAt(tea.MouseMsg{X: 8, Y: 0, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}, labels...); got != -1 {
		t.Fatalf("gap action=%d", got)
	}
}
func TestImportRejectsTruncation(t *testing.T) {
	input := textarea.New()
	input.SetValue("existing")
	if err := InsertFileText(&input, strings.Repeat("x\n", 10000)); err == nil {
		t.Fatal("expected line limit error")
	}
	if input.Value() != "existing" {
		t.Fatal("input changed on rejection")
	}
}

func TestImportRejectsContentTextareaWouldChange(t *testing.T) {
	for _, content := range []string{"a\x1bb", "a\x00b"} {
		input := textarea.New()
		input.SetValue("existing")
		if err := InsertFileText(&input, content); err == nil {
			t.Fatalf("accepted %q", content)
		}
		if input.Value() != "existing" {
			t.Fatalf("changed input for %q", content)
		}
	}
}

// actionHeaderRows 返回渲染后的按钮行（剥离 ANSI）。
func actionHeaderRows(view string) []string	{ return strings.Split(ansi.Strip(view), "\n") }

// actionHeaderClick 按显示文本定位按钮并点击，返回命中的下标。
func actionHeaderClick(t *testing.T, header *ActionHeader, labels []string, label string) int {
	t.Helper()
	rows := actionHeaderRows(header.View(labels...))
	for y, row := range rows {
		if pos := strings.Index(row, label); pos >= 0 {
			return header.Index(tea.MouseMsg{X: pos, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}, labels...)
		}
	}
	t.Fatalf("label %q missing from %q", label, header.View(labels...))
	return -1
}

func TestActionHeaderWrapsAndHitTests(t *testing.T) {
	var header ActionHeader
	w := NewWorkspace("", "")
	labels := []string{"Encode", "URL-safe", "Padding on"}
	header.Resize(tea.WindowSizeMsg{Width: 15, Height: 16}, &w, labels...)
	rows := actionHeaderRows(header.View(labels...))
	if len(rows) < 2 {
		t.Fatal("expected wrapped actions")
	}
	for _, row := range rows {
		if lipgloss.Width(row) > 15 {
			t.Fatalf("row too wide: %q", row)
		}
	}
	for _, label := range labels {
		if got := actionHeaderClick(t, &header, labels, label); got != -1 {
			if labels[got] != label {
				t.Fatalf("label %q mapped to %d (%q)", label, got, labels[got])
			}
		}
	}
	if got := header.Index(tea.MouseMsg{X: 14, Y: len(rows), Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}, labels...); got != -1 {
		t.Fatalf("outside mapped to %d", got)
	}
}

func TestShortActionHeaderPagesWithoutInterceptingWorkspace(t *testing.T) {
	var header ActionHeader
	w := NewWorkspace("", "")
	labels := []string{"MD5", "SHA-1", "SHA-256", "SHA-512", "CRC32"}
	header.Resize(tea.WindowSizeMsg{Width: 15, Height: 2}, &w, labels...)
	if header.Height(labels...) != 2 {
		t.Fatal("expected paged header")
	}
	// 第二页按钮的期望坐标来自同一份 Toolbar 布局。
	fit := actionToolbar(1, labels, false)
	fit.SetSize(15, 0)
	want, x := -1, 0
	for i, r := range fit.Buttons {
		_ = r
		if rect := fit.Rect(i); rect.Y == 1 {
			want, x = i, rect.X
			break
		}
	}
	if want < 0 {
		t.Fatal("second page has no buttons")
	}
	before := header.View(labels...)
	header.Index(tea.MouseMsg{X: 2, Y: 1, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress}, labels...)
	if header.View(labels...) == before {
		t.Fatal("wheel did not change page")
	}
	if got := header.Index(tea.MouseMsg{X: x, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}, labels...); got != want {
		t.Fatalf("got %d want %d", got, want)
	}
}
