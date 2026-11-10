package optbar

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func press(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
}

func TestMouseControlsMatchWrappedLayout(t *testing.T) {
	n := &Number{Label: "Count", Value: 1, Min: 1, Max: 2}
	toggle := &Toggle{Label: "Uppercase"}
	b := Bar{Numbers: []*Number{n}, Toggles: []*Toggle{toggle}}
	b.SetWidth(12)
	l := b.layout()
	if len(l.lines) < 2 {
		t.Fatal("expected wrapped controls")
	}
	if lipgloss.Height(b.View()) != b.Height() {
		t.Fatalf("view height %d differs from Height %d", lipgloss.Height(b.View()), b.Height())
	}
	if dec, inc, ok := b.StepRects(0); !ok {
		t.Fatal("stepper rects missing")
	} else if changed, _ := b.HandleMouse(press(inc.X, inc.Y)); !changed || n.Value != 2 {
		t.Fatalf("inc click did not step: %v %d", changed, n.Value)
	} else if changed, _ := b.HandleMouse(press(dec.X, dec.Y)); !changed || n.Value != 1 {
		t.Fatalf("dec click did not step back: %v %d", changed, n.Value)
	}
	if r, ok := b.ControlRect(ToggleOption, 0); !ok {
		t.Fatal("switch rect missing")
	} else if changed, _ := b.HandleMouse(press(r.X, r.Y)); !changed || !toggle.On {
		t.Fatalf("switch click did not toggle: %v %v", changed, toggle.On)
	}
}

func TestShortBarPagesAndChangesHiddenOption(t *testing.T) {
	toggles := []*Toggle{{Label: "一"}, {Label: "二"}, {Label: "三"}, {Label: "四"}}
	b := Bar{Toggles: toggles, Hint: "regenerate"}
	b.SetSize(10, 7)
	l := b.layout()
	if len(l.lines) < 3 || b.Height() != 2 {
		t.Fatalf("rows=%d height=%d", len(l.lines), b.Height())
	}
	target, ok := b.ControlRect(ToggleOption, 3)
	if !ok {
		t.Fatal("target switch rect missing")
	}
	for b.page < target.Y {
		line := b.pageLine(len(l.lines))
		x := lipgloss.Width(ansi.Strip(line)) - 2
		b.HandleMouse(tea.MouseMsg{X: x, Y: 0, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	}
	changed, _ := b.HandleMouse(press(target.X, 1))
	if !changed || !toggles[3].On {
		t.Fatal("option on later page was not changed")
	}
}

func TestExpandedSettingsMouseControls(t *testing.T) {
	number := &Number{Label: "Count", Value: 2, Min: 1, Max: 5, Description: "How many"}
	toggle := &Toggle{Label: "Uppercase", Description: "Use uppercase"}
	choice := &Choice{Label: "Mode", Options: []string{"A", "B"}, Index: 0, Description: "Pick one"}
	b := Bar{
		Numbers:	[]*Number{number},
		Choices:	[]*Choice{choice},
		Toggles:	[]*Toggle{toggle},
		Hint:		"regenerate",
	}
	b.SetSize(80, 24)
	if !b.expanded() {
		t.Fatal("expected desktop layout at 80x24")
	}
	lines := strings.Split(b.View(), "\n")
	// 分组标题的字符形态由 theme.Eyebrow 决定（大写）。
	if ansi.Strip(lines[0]) != "CONFIGURATION" || len(lines) != b.Height() {
		t.Fatalf("lines=%q height=%d", lines, b.Height())
	}
	if dec, inc, ok := b.StepRects(0); !ok {
		t.Fatal("stepper rects missing")
	} else if changed, _ := b.HandleMouse(press(inc.X, inc.Y)); !changed || number.Value != 3 {
		t.Fatalf("inc did not change: %v %d", changed, number.Value)
	} else if changed, _ := b.HandleMouse(press(dec.X, dec.Y)); !changed || number.Value != 2 {
		t.Fatalf("dec did not change back: %v %d", changed, number.Value)
	}
	if r, ok := b.ControlRect(ToggleOption, 0); !ok {
		t.Fatal("switch rect missing")
	} else if changed, _ := b.HandleMouse(press(r.X, r.Y)); !changed || !toggle.On {
		t.Fatalf("switch did not change: %v %v", changed, toggle.On)
	}
	// 下拉：点 chip 展开 → 点选项选中。
	r, ok := b.ControlRect(ChoiceOption, 0)
	if !ok {
		t.Fatal("chip rect missing")
	}
	if changed, _ := b.HandleMouse(press(r.X, r.Y)); changed {
		t.Fatal("opening chip must not change the value")
	}
	if !b.OverlayActive() {
		t.Fatal("chip click did not open the dropdown")
	}
	view := b.OverlayOn(strings.Join(lines, "\n"))
	if !strings.Contains(ansi.Strip(view), "· A") {
		t.Fatal("dropdown list not overlaid on view")
	}
	if areaH := lipgloss.Height(view); b.selects[0].ListHeight > areaH {
		t.Fatalf("list height %d exceeds view height %d", b.selects[0].ListHeight, areaH)
	}
	if option, hit := b.selects[0].ListHit(press(r.X, r.Y+3), r.X, r.Y, b.width, 24); !hit || option != 1 {
		t.Fatalf("list hit=%d ok=%v", option, hit)
	}
	if changed, _ := b.HandleMouse(press(r.X, r.Y+3)); !changed || choice.Index != 1 {
		t.Fatalf("option click did not select: %v %d", changed, choice.Index)
	}
	if b.OverlayActive() {
		t.Fatal("dropdown did not close after selection")
	}
	// 点浮层外收起。
	if _, _, ok := b.StepRects(0); !ok {
		t.Fatal("stepper rects missing")
	}
	if changed, _ := b.HandleMouse(press(2, 0)); changed {
		t.Fatal("heading click must not change values")
	}
}

func TestKeyboardFocusAndAdjust(t *testing.T) {
	number := &Number{Label: "Count", Value: 2, Min: 1, Max: 5}
	choice := &Choice{Label: "Mode", Options: []string{"A", "B", "C"}, Index: 0}
	toggle := &Toggle{Label: "Upper"}
	b := Bar{Numbers: []*Number{number}, Choices: []*Choice{choice}, Toggles: []*Toggle{toggle}}
	// 未聚焦时不消费取值键。
	if b.HandleKey("+") {
		t.Fatal("unfocused bar consumed a key")
	}
	if b.HandleKey("ctrl+o") || !b.Focused {
		t.Fatal("ctrl+o did not focus")
	}
	if !b.HandleKey("+") || number.Value != 3 {
		t.Fatalf("+ did not step active number: %d", number.Value)
	}
	if b.HandleKey("down") || !b.HandleKey("right") || choice.Index != 1 {
		t.Fatalf("right did not cycle choice: %d", choice.Index)
	}
	if b.HandleKey("down") || !b.HandleKey(" ") || !toggle.On {
		t.Fatalf("space did not toggle active switch: %v", toggle.On)
	}
	if !b.HandleKey("1") || toggle.On {
		t.Fatalf("digit key did not toggle first switch: %v", toggle.On)
	}
	// enter 展开下拉，esc 收起，tab 退出聚焦。
	if b.HandleKey("up") {
		t.Fatal("cursor move reported a value change")
	}
	if b.HandleKey("enter") || !b.OverlayActive() {
		t.Fatal("enter did not open dropdown")
	}
	if b.HandleKey("down") || choice.Index != 1 {
		t.Fatal("dropdown cursor moved without commit")
	}
	if b.HandleKey("esc") || b.OverlayActive() {
		t.Fatal("esc did not close dropdown")
	}
	if b.HandleKey("tab") || b.Focused {
		t.Fatal("tab did not blur")
	}
	if b.HandleKey("ctrl+o") || !b.HandlesKeys() {
		t.Fatal("ctrl+o did not refocus")
	}
}

func TestSelectEscConsumedWhileMouseOpened(t *testing.T) {
	choice := &Choice{Label: "Mode", Options: []string{"A", "B"}}
	b := Bar{Choices: []*Choice{choice}}
	b.SetSize(80, 24)
	r, ok := b.ControlRect(ChoiceOption, 0)
	if !ok {
		t.Fatal("chip rect missing")
	}
	b.HandleMouse(press(r.X, r.Y))
	if !b.OverlayActive() || !b.HandlesKeys() {
		t.Fatal("opened dropdown must be active and consume keys even unfocused")
	}
	if changed := b.HandleKey("esc"); changed || b.OverlayActive() || b.Focused {
		t.Fatal("esc on mouse-opened dropdown")
	}
	if b.HandlesKeys() {
		t.Fatal("bar still consumes keys after dropdown closed")
	}
}
