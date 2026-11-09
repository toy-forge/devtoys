package editor

import (
	"strings"
	"testing"

	"github.com/toy-forge/devtoys/internal/theme"

	tea "github.com/charmbracelet/bubbletea"
)

func TestValuePreservesControlPicturesAndNewlines(t *testing.T) {
	original := "first\tsecond\r\nthird ␉ ␍ ␚ ␛ �\nlast"
	input := New()
	input.SetValue(original)
	if got := input.Value(); got != original {
		t.Fatalf("round trip got %q, want %q", got, original)
	}
	if got := input.View(); strings.ContainsAny(got, "\t\r") {
		t.Fatalf("view contains terminal controls: %q", got)
	}
	input.InsertString("\tmore\r\n")
	if got := input.Value(); got != original+"\tmore\r\n" {
		t.Fatalf("insert got %q", got)
	}
}

func TestPastedRunesPreserveTabsAndCRLF(t *testing.T) {
	input := New()
	input.Focus()
	text := "a\tb\r\n␉ ␛ �"
	input, _ = input.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text), Paste: true})
	if got := input.Value(); got != text {
		t.Fatalf("got %q, want %q", got, text)
	}
}

func TestPasteRejectsUnsupportedControlsAndOversize(t *testing.T) {
	for _, text := range []string{"x\x1by", strings.Repeat("x\n", 10000)} {
		input := New()
		input.SetValue("existing")
		input.Focus()
		next, cmd := input.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text), Paste: true})
		if next.Value() != "existing" || cmd == nil {
			t.Fatalf("input changed or no error for %q", text[:min(len(text), 8)])
		}
		if _, ok := cmd().(PasteErrorMsg); !ok {
			t.Fatal("expected paste error")
		}
	}
}

func TestLiteralControlPictureIsEditedAsOneRune(t *testing.T) {
	input := New()
	input.Focus()
	input.SetValue("␉x")
	input, _ = input.Update(tea.KeyMsg{Type: tea.KeyLeft})
	input, _ = input.Update(tea.KeyMsg{Type: tea.KeyLeft})
	input, _ = input.Update(tea.KeyMsg{Type: tea.KeyRight})
	input, _ = input.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if got := input.Value(); got != "x" {
		t.Fatalf("got %q, want x", got)
	}
}

func TestPlaceCursorOnVisibleLines(t *testing.T) {
	input := New()
	input.ShowLineNumbers = false
	input.SetWidth(20)
	input.SetHeight(4)
	input.Focus()
	input.SetValue("abc\n你好de")
	input.PlaceCursor(3, 0)	// prompt occupies two cells; after 'a'
	input.InsertString("X")
	if got := input.Value(); got != "aXbc\n你好de" {
		t.Fatalf("first line: %q", got)
	}
	input.PlaceCursor(6, 1)	// after two double-width characters
	input.InsertString("Y")
	if got := input.Value(); got != "aXbc\n你好Yde" {
		t.Fatalf("wide characters: %q", got)
	}
}

func TestPlaceCursorRespectsScrollAndWrapping(t *testing.T) {
	input := New()
	input.ShowLineNumbers = false
	input.SetWidth(8)
	input.SetHeight(3)
	input.Focus()
	input.SetValue("first\nsecond\nthird\nfourth\nfifth")
	input, _ = input.Update(tea.WindowSizeMsg{})	// scroll cursor into view
	before := input.Model.Line()
	input.Scroll(-1)
	input.PlaceCursor(2, 0)
	if input.Model.Line() >= before {
		t.Fatalf("click above did not move cursor: %d -> %d", before, input.Model.Line())
	}

	input.SetValue("abcdefghij")
	input, _ = input.Update(tea.WindowSizeMsg{})
	input.PlaceCursor(2, 0)
	input.InsertString("X")
	if !strings.HasPrefix(input.Value(), "X") {
		t.Fatalf("wrapped click: %q", input.Value())
	}
}

func TestPlaceCursorKeepsGraphemeAndEscapedMarkerWhole(t *testing.T) {
	input := New()
	input.ShowLineNumbers = false
	input.SetWidth(20)
	input.SetHeight(2)
	input.Focus()
	input.SetValue("e\u0301x")
	input.PlaceCursor(3, 0)
	input.InsertString("Y")
	if got := input.Value(); got != "e\u0301Yx" {
		t.Fatalf("grapheme split: %q", got)
	}
	input.SetValue("␉z")
	input.PlaceCursor(3, 0)
	input.InsertString("Y")
	if got := input.Value(); got != "Y␉z" {
		t.Fatalf("escape pair split: %q", got)
	}
}

func TestScrollPreservesCaretAndClickUsesVisibleText(t *testing.T) {
	input := New()
	input.ShowLineNumbers = false
	input.SetWidth(20)
	input.SetHeight(3)
	input.Focus()
	input.SetValue("one\ntwo\nthree\nfour\nfive\nsix")
	beforeLine, beforeValue := input.Line(), input.Value()
	input.Scroll(-100)
	if input.Line() != beforeLine || input.Value() != beforeValue {
		t.Fatal("scroll changed caret or text")
	}
	if view := input.View(); !strings.Contains(view, "one") || strings.Contains(view, "six") {
		t.Fatalf("did not scroll to top: %q", view)
	}
	input.PlaceCursor(3, 0)
	input.InsertString("X")
	if got := input.Value(); got != "oXne\ntwo\nthree\nfour\nfive\nsix" {
		t.Fatalf("click after scroll: %q", got)
	}
}

func TestPlaceCursorOnWrappedContinuation(t *testing.T) {
	input := New()
	input.ShowLineNumbers = false
	input.SetWidth(8)
	input.SetHeight(4)
	input.Focus()
	input.SetValue("abcdefghij")
	input.PlaceCursor(4, 1)	// prompt plus the two visible cells "gh"
	input.InsertString("X")
	if got := input.Value(); got != "abcdefghXij" {
		t.Fatalf("wrapped insertion=%q", got)
	}
}

func TestEditorUsesSharedTheme(t *testing.T) {
	input := New()
	if got := input.FocusedStyle.Prompt.GetForeground(); got != theme.Accent {
		t.Fatalf("prompt=%v", got)
	}
	if got := input.FocusedStyle.CursorLine.GetBackground(); got != theme.Surface {
		t.Fatalf("cursor line=%v", got)
	}
	if got := input.Cursor.Style.GetBackground(); got != theme.CursorText {
		t.Fatalf("cursor text=%v", got)
	}
}
