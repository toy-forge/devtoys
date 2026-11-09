package listcompare

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/toy-forge/devtoys/internal/toolkit"
)

func TestCompareModesAndCase(t *testing.T) {
	a := "Alpha\nbeta\nbeta\nΣ\n"
	b := "BETA\ngamma\nς\n"
	cases := []struct {
		mode		int
		sensitive	bool
		want		string
	}{{0, false, "beta\nΣ"}, {1, false, "Alpha\nbeta\nΣ\ngamma"}, {2, false, "Alpha"}, {3, false, "gamma"}, {0, true, ""}, {2, true, "Alpha\nbeta\nΣ"}}
	for _, c := range cases {
		if got := compare(a, b, c.mode, c.sensitive); got != c.want {
			t.Errorf("mode=%d case=%v got %q want %q", c.mode, c.sensitive, got, c.want)
		}
	}
}
func TestThreePanesMouseAndFileImport(t *testing.T) {
	m := newTool()
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 25})
	m.Update(toolkit.FileImportMsg{Text: "a\nb"})
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m.Update(toolkit.FileImportMsg{Text: "b\nc"})
	if got := m.panes.OutputText(); got != "b" {
		t.Fatalf("intersection=%q", got)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}, Alt: true})
	if got := m.panes.OutputText(); got != "a\nb\nc" {
		t.Fatalf("union=%q", got)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "List A") || !strings.Contains(view, "List B") || lipgloss.Height(view) != 25 {
		t.Fatalf("view=%q", view)
	}
}
func TestCompactFocusCanReachBothInputs(t *testing.T) {
	m := newTool()
	m.Update(tea.WindowSizeMsg{Width: 20, Height: 7})
	m.Update(toolkit.FileImportMsg{Text: "first"})
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m.Update(toolkit.FileImportMsg{Text: "second"})
	if m.panes.A.Value() != "first" || m.panes.B.Value() != "second" || lipgloss.Height(m.View()) != 7 {
		t.Fatalf("A=%q B=%q view=%q", m.panes.A.Value(), m.panes.B.Value(), m.View())
	}
}

func TestMouseTargetsSecondInputAndClear(t *testing.T) {
	m := newTool()
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 25})
	m.Update(toolkit.FileImportMsg{Text: "apple"})
	rows := m.header.Height(m.actions()...)
	m.Update(tea.MouseMsg{X: 54, Y: rows + 1, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("apple")})
	if m.panes.B.Value() != "apple" || m.panes.OutputText() != "apple" {
		t.Fatalf("B=%q result=%q", m.panes.B.Value(), m.panes.OutputText())
	}
	view := strings.Split(ansi.Strip(m.View()), "\n")
	found := false
	for y, line := range view {
		if strings.Contains(line, "List B") {
			// 按钮为内联胶囊（无方括号字面量）。
			i := strings.LastIndex(line, "Clear")
			if i >= 0 {
				m.Update(tea.MouseMsg{X: lipgloss.Width(line[:i]) + 1, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
				found = true
				break
			}
		}
	}
	if !found || m.panes.B.Value() != "" || m.panes.OutputText() != "" {
		t.Fatalf("clear found=%v B=%q result=%q", found, m.panes.B.Value(), m.panes.OutputText())
	}
}

func TestLargeListsRunInBackgroundAndDiscardStaleResults(t *testing.T) {
	m := newTool()
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 25})
	large := strings.Repeat("alpha\n", 6000)
	_, timer := m.Update(toolkit.FileImportMsg{Text: large})
	if timer == nil || m.panes.OutputText() != "" {
		t.Fatal("large input did not schedule work")
	}
	// Execute the scheduled tick, then keep the worker result pending.
	msg := timer()
	if batch, ok := msg.(tea.BatchMsg); ok {
		if len(batch) != 1 {
			t.Fatalf("unexpected command batch: %d", len(batch))
		}
		msg = batch[0]()
	}
	due, ok := msg.(toolkit.BackgroundToolMsg)
	if !ok {
		t.Fatalf("expected background timer, got %T", msg)
	}
	_, worker := m.Update(due.Message)
	if worker == nil || !m.running {
		t.Fatal("comparison worker did not start")
	}
	// Switch to a short input while the previous worker is still running.
	m.panes.A.SetValue("fresh")
	m.panes.B.SetValue("fresh")
	m.beforeA, m.beforeB = "fresh", "fresh"
	m.recompute()
	if got := m.panes.OutputText(); got != "fresh" {
		t.Fatalf("short result=%q", got)
	}
	old := worker().(toolkit.BackgroundToolMsg)
	m.Update(old.Message)
	if got := m.panes.OutputText(); got != "fresh" {
		t.Fatalf("stale worker replaced current result: %q", got)
	}
}

func TestLargeComparisonQueuesLatestMode(t *testing.T) {
	m := newTool()
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 25})
	m.panes.A.SetValue(strings.Repeat("alpha\n", 6000))
	m.panes.B.SetValue("alpha\nbeta")
	m.recompute()
	_, first := m.Update(compareDue{revision: m.revision})
	if first == nil || !m.running {
		t.Fatal("first comparison did not start")
	}
	m.mode = 1
	m.recompute()
	_, cmd := m.Update(compareDue{revision: m.revision})
	if cmd != nil || !m.queued {
		t.Fatal("latest comparison was not queued")
	}
	old := first().(toolkit.BackgroundToolMsg)
	_, latest := m.Update(old.Message)
	if latest == nil || m.panes.OutputText() != "" {
		t.Fatal("stale result applied or queued work lost")
	}
	current := latest().(toolkit.BackgroundToolMsg)
	m.Update(current.Message)
	if got := m.panes.OutputText(); got != "alpha\nbeta" {
		t.Fatalf("latest union=%q", got)
	}
}
