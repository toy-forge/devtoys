package analyzer

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/toy-forge/devtoys/internal/toolkit"
)

func TestTransformations(t *testing.T) {
	for _, test := range []struct {
		input		string
		operation	int
		want		string
	}{
		{"a\r\nb\rc\n", 0, "a\nb\nc\n"}, {"a\r\nb\rc\n", 1, "a\r\nb\r\nc\r\n"},
		{"HELLO WORLD. NEXT LINE", 4, "Hello world. Next line"}, {"hELLO-wORLD", 5, "Hello-World"},
		{"HELLO world\nNEXT one", 6, "helloWorld\nnextOne"}, {"hello world\nnext one", 7, "HelloWorld\nNextOne"},
		{"hello world", 8, "hello_world"}, {"hello world", 9, "HELLO_WORLD"},
		{"hello world", 10, "hello-world"}, {"hello world", 11, "HELLO-WORLD"}, {"hello world", 12, "Hello-World"},
		{"abcd", 13, "aBcD"}, {"abcd", 14, "AbCd"},
		{"b\r\na", 16, "a\r\nb"}, {"a\nb", 17, "b\na"},
		{"first z\nsecond a", 18, "second a\nfirst z"}, {"first a\nsecond z", 19, "second z\nfirst a"},
		{"a\nb\nc", 20, "c\nb\na"},
	} {
		if got := transform(test.input, test.operation); got != test.want {
			t.Errorf("%s: got %q, want %q", operationLabels[test.operation], got, test.want)
		}
	}
}

func TestOperationsAndOriginalViaUI(t *testing.T) {
	m := &Model{ws: toolkit.NewWorkspace("text", "")}
	m.ws.SetInputControls("Original")
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 35})
	m.Update(toolkit.FileImportMsg{Text: "Hello world"})
	// 按钮为内联胶囊（无方括号字面量），点击标签即命中。
	// 窗格按钮通过 WorkspaceActionMsg 命令回投，测试需像运行时一样执行命令。
	click := func(label string) {
		t.Helper()
		for y, line := range strings.Split(ansi.Strip(m.View()), "\n") {
			if x := strings.Index(line, label); x >= 0 {
				_, cmd := m.Update(tea.MouseMsg{X: lipgloss.Width(line[:x]), Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
				if cmd != nil {
					if msg := cmd(); msg != nil {
						m.Update(msg)
					}
				}
				return
			}
		}
		t.Fatalf("button %q missing:\n%s", label, m.View())
	}
	click("UPPERCASE")
	if got := m.ws.Input.Value(); got != "HELLO WORLD" {
		t.Fatal(got)
	}
	click("snake_case")
	if got := m.ws.Input.Value(); got != "hello_world" {
		t.Fatal(got)
	}
	click("Original")
	if got := m.ws.Input.Value(); got != "Hello world" {
		t.Fatal(got)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("!")})
	newOriginal := m.ws.Input.Value()
	click("UPPERCASE")
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}, Alt: true})
	if got := m.ws.Input.Value(); got != newOriginal {
		t.Fatalf("restore %q, want %q", got, newOriginal)
	}
}

func TestStatisticsLineEndingsAndCharacters(t *testing.T) {
	s := analyze("é a\r\nA\n")
	if s.bytes != len("é a\r\nA\n") || s.lineEndings != "Mixed" || len(s.characters) == 0 {
		t.Fatalf("unexpected stats: %+v", s)
	}
	for input, want := range map[string]string{"a\r\nb": "CRLF", "a\rb": "CR", "a\nb": "LF", "a": "Unknown"} {
		if got := detectLineEndings(input); got != want {
			t.Errorf("%q: %s", input, got)
		}
	}
}

func TestShortScreenActionPaging(t *testing.T) {
	m := &Model{ws: toolkit.NewWorkspace("text", "")}
	m.Update(tea.WindowSizeMsg{Width: 20, Height: 7})
	m.Update(toolkit.FileImportMsg{Text: "Hello"})
	m.applyOperation(3)
	for i := 0; i < 30; i++ {
		view := ansi.Strip(m.View())
		if lipgloss.Height(m.View()) != 7 || lipgloss.Width(m.View()) > 20 {
			t.Fatalf("bad layout:\n%s", view)
		}
		lines := strings.Split(view, "\n")
		// 分页按钮与操作按钮均为内联胶囊（无方括号字面量）。
		if x := strings.Index(lines[1], "Original"); x >= 0 {
			m.Update(tea.MouseMsg{X: lipgloss.Width(lines[1][:x]), Y: 1, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
			if m.ws.Input.Value() != "Hello" {
				t.Fatalf("original=%q", m.ws.Input.Value())
			}
			return
		}
		x := strings.Index(lines[0], ">")
		if x < 0 {
			t.Fatalf("next page unavailable: %q", lines[0])
		}
		m.Update(tea.MouseMsg{X: lipgloss.Width(lines[0][:x]), Y: 0, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	}
	t.Fatal("original action never appeared")
}

func TestLastWordSortingIgnoresPunctuation(t *testing.T) {
	input := "first z!\nsecond a?\nthird b."
	if got := transform(input, 18); got != "second a?\nthird b.\nfirst z!" {
		t.Fatalf("sorted %q", got)
	}
	if got := lastWord("Unicode 你好！！！"); got != "你好" {
		t.Fatalf("last word %q", got)
	}
}

func TestTransformationDoesNotTruncateEditor(t *testing.T) {
	m := &Model{ws: toolkit.NewWorkspace("text", "")}
	original := strings.Repeat("x\r", 10000) + "end"
	m.ws.Input.SetValue(original)
	m.original = original
	m.last = original
	m.applyOperation(0)
	if m.ws.Input.Value() != original {
		t.Fatal("oversized conversion changed the input")
	}
	if m.ws.Err == nil || !strings.Contains(m.ws.Err.Error(), "line limit") {
		t.Fatalf("expected limit error, got %v", m.ws.Err)
	}
}

func TestDesktopLineAndCharacterCounts(t *testing.T) {
	empty := analyze("")
	if empty.lines != 1 || empty.paragraphs != 1 || empty.lineEndings != "Unknown" {
		t.Fatalf("empty statistics: %+v", empty)
	}
	for _, test := range []struct {
		text		string
		lines, chars	int
	}{
		{"a\rb", 2, 3}, {"a\r\nb\r\n", 3, 6}, {"a\n\rb", 3, 4}, {"😀!", 1, 3},
	} {
		s := analyze(test.text)
		if s.lines != test.lines || s.chars != test.chars {
			t.Errorf("%q: lines=%d chars=%d", test.text, s.lines, s.chars)
		}
	}
}
