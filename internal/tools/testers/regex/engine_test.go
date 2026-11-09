package regex

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	regexp2 "github.com/dlclark/regexp2/v2"
	"github.com/toy-forge/devtoys/internal/toolkit"
)

func TestDotNetRegexFeatures(t *testing.T) {
	tests := []struct {
		name, pattern, text, want	string
		flags				regexp2.RegexOptions
		global				bool
	}{
		{"lookbehind", `(?<=item-)\d+`, "item-123", "123", 0, true},
		{"backreference", `(?<word>\w+)\s+\k<word>`, "go go", "Group 1 (word): go", 0, true},
		{"case", `hello`, "HELLO", "1 matches", regexp2.IgnoreCase, true},
		{"whitespace", `a \s+ b # comment`, "a b", "1 matches", regexp2.IgnorePatternWhitespace, true},
		{"singleline", `a.b`, "a\nb", "1 matches", regexp2.Singleline, true},
		{"multiline", `^b`, "a\nb", "1 matches", regexp2.Multiline, true},
		{"unicode indexes", `(?<=猫)\d+`, "猫12", "1..3", 0, true},
		{"supplementary index", `(?<=😀)a`, "😀a", "2..3", 0, true},
		{"literal spaces", `  `, "a  b", "1 matches", 0, true},
		{"right-to-left", `\d+`, "1 22 333", "[1] 5..8", regexp2.RightToLeft, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := matchRegex(tc.pattern, tc.text, tc.global, tc.flags)
			if r.err != nil || !strings.Contains(plain(r.text), tc.want) {
				t.Fatalf("result=%q err=%v", r.text, r.err)
			}
		})
	}
}
func TestMatchTimeoutAndLimits(t *testing.T) {
	start := time.Now()
	r := matchRegex(`(a+)+$`, strings.Repeat("a", 10000)+"!", true, 0)
	if r.err == nil || !strings.Contains(r.err.Error(), "timed out") {
		t.Fatalf("expected timeout: %v", r.err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("timeout took %v", elapsed)
	}
	if r := matchRegex(strings.Repeat("x", maxPatternBytes+1), "x", true, 0); r.err == nil {
		t.Fatal("oversized pattern accepted")
	}
	r = matchRegex(`x`, strings.Repeat("x", maxMatches+1), true, 0)
	if r.err != nil || !strings.Contains(r.text, "Results limited") {
		t.Fatalf("match cap: %v %s", r.err, r.text)
	}
}
func TestStaleBackgroundResults(t *testing.T) {
	tool := newRegexTool()
	m := tool.model
	m.pattern.SetValue("old")
	m.ws.Input.SetValue("old new")
	m.schedule()
	rev1 := m.revision
	_, first := m.Update(matchDue{revision: rev1})
	m.pattern.SetValue("new")
	m.schedule()
	rev2 := m.revision
	m.Update(matchDue{revision: rev2})
	done := first().(toolkit.BackgroundToolMsg)
	if done.Tool != tool {
		t.Fatal("result not routed to registered tool")
	}
	_, next := m.Update(done.Message)
	if next == nil {
		t.Fatal("latest request not queued")
	}
	if strings.Contains(m.ws.OutputText(), "[1]") {
		t.Fatal("stale output applied")
	}
	done = next().(toolkit.BackgroundToolMsg)
	m.Update(done.Message)
	if !strings.Contains(m.ws.OutputText(), "new") || !strings.Contains(m.ws.OutputText(), "4..7") {
		t.Fatalf("latest output missing: %s", m.ws.OutputText())
	}
}
func TestOptionToggleAndShortLayout(t *testing.T) {
	t1 := newRegexTool()
	for _, size := range []tea.WindowSizeMsg{{Width: 20, Height: 7}, {Width: 30, Height: 12}, {Width: 80, Height: 24}} {
		t1.Update(size)
		if w, h := lipgloss.Width(t1.View()), lipgloss.Height(t1.View()); w > size.Width || h != size.Height {
			t.Fatalf("%dx%d view %dx%d", size.Width, size.Height, w, h)
		}
	}
	t1.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}, Alt: true})
	if t1.flags&regexp2.IgnoreCase == 0 {
		t.Fatal("ignore case not enabled")
	}
	t1.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}, Alt: true})
	t1.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'7'}, Alt: true})
	if t1.flags&regexp2.ECMAScript == 0 || t1.flags&regexp2.IgnorePatternWhitespace != 0 {
		t.Fatal("ECMAScript option constraints not applied")
	}
}
func TestControlCharactersInDisplay(t *testing.T) {
	r := matchRegex("a", "a\tb\r", true, 0)
	if r.err != nil || strings.ContainsAny(r.display, "\t\r") || !strings.Contains(r.text, "\t") {
		t.Fatalf("raw=%q display=%q err=%v", r.text, r.display, r.err)
	}
}
