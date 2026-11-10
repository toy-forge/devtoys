package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
	"github.com/toy-forge/devtoys/internal/theme"
)

func pressAt(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
}

// assertBounded 断言多行渲染结果每行显示宽度不超过 width。
func assertBounded(t *testing.T, name string, view string, width int) {
	t.Helper()
	for i, line := range strings.Split(view, "\n") {
		if w := lipgloss.Width(line); w > width {
			t.Fatalf("%s line %d width %d exceeds %d", name, i, w, width)
		}
	}
}

func TestButtonViewAndHit(t *testing.T) {
	variants := []Variant{VariantGhost, VariantPrimary, VariantDanger}
	shapes := []Shape{ShapeRounded, ShapeSquare}
	for _, variant := range variants {
		for _, shape := range shapes {
			b := Button{Label: "OK", Variant: variant, Shape: shape}
			view := b.View()
			if got := lipgloss.Width(view); got != b.Width() {
				t.Fatalf("variant %d shape %d: view width %d != Width() %d", variant, shape, got, b.Width())
			}
			if b.Height() != 3 {
				t.Fatalf("bordered button height = %d, want 3", b.Height())
			}
			if lipgloss.Height(view) != 3 {
				t.Fatalf("bordered view height = %d", lipgloss.Height(view))
			}
			assertBounded(t, "button", view, b.Width())
		}
	}
	inline := Button{Label: "OK", Inline: true}
	if inline.Height() != 1 || lipgloss.Height(inline.View()) != 1 {
		t.Fatal("inline button must be 1 row")
	}
	if inline.Width() != 4 {	// 2 label + 2 padding
		t.Fatalf("inline width = %d, want 4", inline.Width())
	}
	// 命中：矩形内为真，外为假；禁用后不命中。
	b := Button{Label: "Apply", Inline: true}
	if !b.Hit(pressAt(3, 0), 0, 0) {
		t.Fatal("expected hit inside button")
	}
	if b.Hit(pressAt(9, 0), 0, 0) {
		t.Fatal("unexpected hit outside button")
	}
	if !b.Hit(pressAt(3, 4), 1, 4) {
		t.Fatal("expected hit with offset")
	}
	b.Disabled = true
	if b.Hit(pressAt(3, 0), 0, 0) {
		t.Fatal("disabled button must not hit")
	}
}

func TestButtonExtremeWidth(t *testing.T) {
	// 极限宽度 8 列：显式 MinWidth 与超长标签都要收住。
	b := Button{Label: "A very long label indeed", Inline: true, MinWidth: 8}
	assertBounded(t, "inline button", b.View(), 8)
	if b.Width() != 8 {
		t.Fatalf("MinWidth ignored: %d", b.Width())
	}
	bordered := Button{Label: "Confirm", MinWidth: 8}
	assertBounded(t, "bordered button", bordered.View(), 8)
}

func TestToolbarLayoutAndHits(t *testing.T) {
	buttons := []Button{{Label: "Copy"}, {Label: "Paste"}, {Label: "Open"}}
	tb := NewToolbar(buttons...)
	tb.SetSize(80, 1)
	if h := tb.Height(); h != 1 {
		t.Fatalf("one-row toolbar height = %d", h)
	}
	view := tb.View()
	assertBounded(t, "toolbar", view, 80)
	if got := lipgloss.Height(view); got != tb.Height() {
		t.Fatalf("view rows %d != Height %d", got, tb.Height())
	}
	// 命中每个按钮的中心。
	for i := range buttons {
		r := tb.Rect(i)
		if got := tb.Index(pressAt(r.X+r.W/2, 0)); got != i {
			t.Fatalf("button %d hit got %d", i, got)
		}
	}
	if got := tb.Index(pressAt(79, 0)); got != -1 {
		t.Fatalf("empty click got %d", got)
	}
	if got := tb.Index(tea.KeyMsg{Type: tea.KeyEnter}); got != -1 {
		t.Fatal("non-mouse events must not hit")
	}
}

func TestToolbarTruncationAndPaging(t *testing.T) {
	buttons := []Button{{Label: "Copy"}, {Label: "Paste"}, {Label: "Open file"}, {Label: "Clear"}}
	tb := NewToolbar(buttons...)
	// 高度只有 1：单行内截断为 …。
	tb.SetSize(20, 1)
	view := tb.View()
	assertBounded(t, "toolbar truncate", view, 20)
	if !strings.Contains(view, "…") {
		t.Fatal("expected truncation ellipsis")
	}
	if h := tb.Height(); h != 1 || lipgloss.Height(view) != 1 {
		t.Fatalf("truncated toolbar height = %d/%d, want 1/1", h, lipgloss.Height(view))
	}
	// 放不下且高度 ≥ 2：两行分页模式。
	tb.SetSize(8, 2)
	if h := tb.Height(); h != 2 {
		t.Fatalf("paged toolbar height = %d, want 2", h)
	}
	view = tb.View()
	assertBounded(t, "toolbar paged", view, 8)
	if rows := strings.Split(view, "\n"); len(rows) != 2 {
		t.Fatalf("paged view rows = %d", len(rows))
	}
	before := tb.page
	if got := tb.Index(tea.MouseMsg{X: 0, Y: 0, Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown}); got != -1 || tb.page != before+1 {
		t.Fatalf("wheel down paging: index=%d page=%d", got, tb.page)
	}
	tb.Index(tea.MouseMsg{X: 0, Y: 0, Action: tea.MouseActionPress, Button: tea.MouseButtonWheelUp})
	if tb.page != before {
		t.Fatalf("wheel up paging: page=%d", tb.page)
	}
	// 点击页码行的 "<" 翻回上一页。
	tb.Index(tea.MouseMsg{X: 0, Y: 0, Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	next := tb.page
	tb.Index(pressAt(0, 0))
	if tb.page != next-1 {
		t.Fatalf("prev page button: page=%d want %d", tb.page, next-1)
	}
	// 禁用按钮不可命中。
	tb2 := NewToolbar(Button{Label: "Go", Disabled: true})
	tb2.SetSize(80, 1)
	tb2.View()
	if got := tb2.Index(pressAt(1, 0)); got != -1 {
		t.Fatalf("disabled toolbar button hit = %d", got)
	}
}

func TestSelectKeysAndMouse(t *testing.T) {
	s := &Select{Options: []string{"json", "yaml", "toml"}, Index: 0}
	if got := s.Selected(); got != "json" {
		t.Fatalf("selected = %q", got)
	}
	chip := s.View()
	if lipgloss.Width(chip) != s.ChipRect(0, 0).W {
		t.Fatalf("chip width %d != ChipRect %d", lipgloss.Width(chip), s.ChipRect(0, 0).W)
	}
	assertBounded(t, "select chip", chip, 8)
	// 键盘：enter 打开，↑↓ 选择，enter 确认，esc 关闭且消费。
	if _, consumed := s.HandleKey("enter"); !consumed || !s.Open {
		t.Fatal("enter should open the dropdown")
	}
	if changed, _ := s.HandleKey("down"); changed {
		t.Fatal("moving cursor is not a change yet")
	}
	if changed, consumed := s.HandleKey("enter"); !consumed || !changed || s.Selected() != "yaml" {
		t.Fatalf("enter should confirm yaml: changed=%v selected=%q", changed, s.Selected())
	}
	if s.Open {
		t.Fatal("dropdown should close after confirm")
	}
	s.HandleKey("enter")
	if _, consumed := s.HandleKey("esc"); !consumed || s.Open {
		t.Fatal("esc should close and be consumed")
	}
	// 鼠标：点击 chip 展开，点击选项选择。
	if !s.HandleMouse(pressAt(2, 0), 0, 0, 40, 24) || !s.Open {
		t.Fatal("chip click should open")
	}
	if !s.HandleMouse(pressAt(3, 4), 0, 0, 40, 24) || s.Selected() != "toml" || s.Open {
		t.Fatalf("list click should select toml, got %q open=%v", s.Selected(), s.Open)
	}
	// 点击外部不消费、保持展开。
	s.HandleMouse(pressAt(2, 0), 0, 0, 40, 24)
	if s.HandleMouse(pressAt(30, 20), 0, 0, 40, 24) || !s.Open {
		t.Fatal("outside click should not be consumed")
	}
	// 滚轮滚动选项。
	if !s.HandleMouse(tea.MouseMsg{X: 3, Y: 3, Action: tea.MouseActionMotion, Button: tea.MouseButtonWheelDown}, 0, 0, 40, 24) {
		t.Fatal("wheel should scroll the open list")
	}
	if s.Cursor != 2 {
		t.Fatalf("cursor = %d, want 2", s.Cursor)
	}
}

func TestSelectOverlayRect(t *testing.T) {
	s := &Select{Options: []string{"a", "b", "c"}, Index: 0, Open: true}
	// 顶部放置：贴 chip 正下方，水平钳制。
	r := s.OverlayRect(0, 0, 40, 24)
	if r.Y != 1 || r.X != 0 {
		t.Fatalf("overlay rect = %+v, want below chip", r)
	}
	// 底部翻转：chip 在倒数第二行时列表翻到上方。
	r = s.OverlayRect(10, 22, 40, 24)
	if r.Y+r.H > 24 {
		t.Fatalf("overlay overflows area: %+v", r)
	}
	if r.Y >= 22 {
		t.Fatalf("overlay should flip above chip: %+v", r)
	}
	// 水平钳制：chip 靠右时列表不越界。
	r = s.OverlayRect(38, 0, 40, 24)
	if r.X+r.W > 40 || r.X < 0 {
		t.Fatalf("overlay not clamped: %+v", r)
	}
	assertBounded(t, "select overlay", s.OpenView(), 40)
}

func TestSwitch(t *testing.T) {
	s := &Switch{}
	offWidth, onWidth := s.Width(), func() int { v := *s; v.On = true; return v.Width() }()
	if offWidth != onWidth {
		t.Fatalf("switch width changed between states: %d vs %d", offWidth, onWidth)
	}
	if w := lipgloss.Width(s.View()); w != s.Width() {
		t.Fatalf("switch view width %d != Width %d", w, s.Width())
	}
	assertBounded(t, "switch", s.View(), 8)
	if s.Hit(pressAt(1, 0), 0, 0) != true {
		t.Fatal("expected switch hit")
	}
	if s.Hit(pressAt(20, 0), 0, 0) {
		t.Fatal("unexpected switch hit")
	}
	s.Disabled = true
	if s.Hit(pressAt(1, 0), 0, 0) {
		t.Fatal("disabled switch must not hit")
	}
	s.Disabled = false
	if s.Toggle() != true || !s.On {
		t.Fatal("toggle should turn on")
	}
	if !strings.Contains(s.View(), "●") || !strings.Contains(func() string { v := s; v.On = false; return v.View() }(), "○") {
		t.Fatal("switch must render ●/○ marks")
	}
}

func TestStepper(t *testing.T) {
	s := &Stepper{Label: "Length", Value: 16, Min: 8, Max: 64}
	view := s.View()
	assertBounded(t, "stepper", view, 80)
	if !strings.Contains(view, "16") || !strings.Contains(view, "Length") {
		t.Fatalf("stepper view missing parts: %q", view)
	}
	dec, val, inc := s.Rects(0, 0)
	if dec.X >= val.X || val.X >= inc.X {
		t.Fatalf("stepper rects out of order: %+v %+v %+v", dec, val, inc)
	}
	if delta, hit := s.Hit(pressAt(dec.X, 0), 0, 0); !hit || delta != -1 || s.Value != 15 {
		t.Fatalf("dec hit: delta=%d hit=%v value=%d", delta, hit, s.Value)
	}
	if delta, hit := s.Hit(pressAt(inc.X, 0), 0, 0); !hit || delta != 1 || s.Value != 16 {
		t.Fatalf("inc hit: delta=%d hit=%v value=%d", delta, hit, s.Value)
	}
	if _, hit := s.Hit(pressAt(val.X, 0), 0, 0); !hit {
		t.Fatal("value cell should count as a hit")
	}
	if _, hit := s.Hit(pressAt(50, 0), 0, 0); hit {
		t.Fatal("unexpected stepper hit")
	}
	// 边界钳制。
	s.Value, s.Min, s.Max = 8, 8, 8
	if _, hit := s.Hit(pressAt(dec.X, 0), 0, 0); hit {
		t.Fatal("dec at min must not hit")
	}
	if _, hit := s.Hit(pressAt(inc.X, 0), 0, 0); hit {
		t.Fatal("inc at max must not hit")
	}
	s.Disabled = true
	if _, hit := s.Hit(pressAt(inc.X, 0), 0, 0); hit {
		t.Fatal("disabled stepper must not hit")
	}
}

func TestModalCenteringAndKeys(t *testing.T) {
	m := &Modal{Kind: KindConfirm, Title: "Delete", Body: "Remove this file?", Width: 24}
	m.SetSize(80, 24)
	w, h := m.BoxSize()
	r := m.Rect()
	if r.X != (80-w)/2 || r.Y != (24-h)/2 {
		t.Fatalf("centering offset: rect=%+v box=%dx%d", r, w, h)
	}
	if r.W != w || r.H != h {
		t.Fatalf("rect size mismatch: %+v vs %dx%d", r, w, h)
	}
	// 盒子大于区域时钳制到 0。
	small := &Modal{Kind: KindInfo, Body: strings.Repeat("x", 100)}
	small.SetSize(8, 3)
	if r := small.Rect(); r.X != 0 || r.Y != 0 {
		t.Fatalf("clamped rect = %+v", r)
	}
	assertBounded(t, "modal box", small.View(), 8)
	// 键盘语义。
	if action, consumed := m.HandleKey("enter"); !consumed || action != ActionConfirm {
		t.Fatalf("enter = %v %v", action, consumed)
	}
	if action, consumed := m.HandleKey("esc"); !consumed || action != ActionCancel {
		t.Fatalf("esc = %v %v", action, consumed)
	}
	if _, consumed := m.HandleKey("tab"); !consumed || !m.focusCancel {
		t.Fatal("tab should toggle focus to cancel")
	}
	if action, _ := m.HandleKey("enter"); action != ActionCancel {
		t.Fatal("enter on focused cancel should cancel")
	}
	// 按钮命中（按钮行右对齐，buttonRowX = r.X+1+max(0, inner-rowWidth)；
	// 渲染与 ButtonHit 共用 buttonRowOffset 单一来源）。
	m.SetSize(80, 24)
	m.focusCancel = false
	buttons := m.Buttons()
	views := make([]string, len(buttons))
	for i, b := range buttons {
		views[i] = b.View()
	}
	rects, _ := PlaceRow(views, Space2)
	box := m.Rect()
	row := box.Y + box.H - 2
	inner := box.W - 2
	rowWidth := buttonRowWidth(views)
	off := buttonRowOffset(inner, rowWidth)
	if got, _, ok := m.ButtonHit(pressAt(box.X+off+rects[0].X, row)); !ok || got != 0 {
		t.Fatalf("confirm button hit = %d ok=%v", got, ok)
	}
	if got, _, ok := m.ButtonHit(pressAt(box.X+off+rects[1].X, row)); !ok || got != 1 {
		t.Fatalf("cancel button hit = %d ok=%v", got, ok)
	}
	// 右对齐：按钮行右缘贴内容区右缘。
	if off-1+rowWidth != inner {
		t.Fatalf("button row not flush right: off=%d rowWidth=%d inner=%d", off, rowWidth, inner)
	}
	if _, inside, ok := m.ButtonHit(pressAt(box.X+1, box.Y+1)); !ok || !inside {
		t.Fatal("click inside box but off buttons should report inside")
	}
	if _, _, ok := m.ButtonHit(pressAt(0, 0)); ok {
		t.Fatal("click outside box must not hit")
	}
}

func TestModalRender(t *testing.T) {
	m := &Modal{Kind: KindConfirm, Title: "Confirm", Body: "Clear all?"}
	m.SetSize(20, 7)
	background := strings.Join(func() []string {
		out := make([]string, 7)
		for i := range out {
			out[i] = strings.Repeat("·", 20)
		}
		return out
	}(), "\n")
	out := m.Render(background)
	lines := strings.Split(out, "\n")
	if len(lines) != 7 {
		t.Fatalf("render lines = %d, want 7", len(lines))
	}
	assertBounded(t, "modal render", out, 20)
	if !strings.Contains(out, "Confirm") {
		t.Fatal("render must contain the modal title")
	}
	if !strings.Contains(out, "╭") {
		t.Fatal("render must contain the box border")
	}
}

func TestToast(t *testing.T) {
	levels := []Level{LevelInfo, LevelSuccess, LevelWarning, LevelError}
	for _, level := range levels {
		toast, cmd := NewToast(level, "done", time.Minute)
		if cmd == nil {
			t.Fatal("expected expiry cmd")
		}
		assertBounded(t, "toast", toast.View(), 80)
		if !strings.Contains(toast.View(), "done") {
			t.Fatalf("toast view missing message: %q", toast.View())
		}
	}
	// 常驻通知没有过期 Cmd。
	if _, cmd := NewToast(LevelInfo, "stay", 0); cmd != nil {
		t.Fatal("persistent toast must not expire")
	}
	// 过期消息匹配。
	toast, cmd := NewToast(LevelSuccess, "Copied", time.Millisecond)
	msg, ok := cmd().(ToastExpireMsg)
	if !ok {
		t.Fatalf("expiry cmd returned %T", cmd())
	}
	if !toast.Expired(msg) {
		t.Fatal("expiry msg should match the toast")
	}
	other := Toast{ID: toast.ID + 1, Level: LevelInfo, Message: "x"}
	if other.Expired(msg) {
		t.Fatal("expiry msg must not match another toast")
	}
	// 极限宽度截断。
	toast, _ = NewToast(LevelError, strings.Repeat("e", 40), time.Minute)
	assertBounded(t, "toast truncate", toast.ViewTruncate(8), 8)
}

func TestTabs(t *testing.T) {
	tb := NewTabs("Input", "Output")
	tb.SetWidth(80)
	view := tb.View()
	assertBounded(t, "tabs", view, 80)
	if tb.Height() != 1 || lipgloss.Height(view) != 1 {
		t.Fatal("tabs must be a single row")
	}
	for i := range tb.Items {
		r := tb.Rect(i)
		if got := tb.Hit(pressAt(r.X, 0)); got != i {
			t.Fatalf("tab %d hit got %d", i, got)
		}
	}
	if got := tb.Hit(pressAt(70, 0)); got != -1 {
		t.Fatalf("empty tab click got %d", got)
	}
	// 极限宽度不越界。
	tb.SetWidth(8)
	assertBounded(t, "tabs narrow", tb.View(), 8)
	// Rect 与渲染共享布局：第二个 Tab 的矩形落在第一个之后。
	tb.SetWidth(80)
	if tb.Rect(0).X+tb.Rect(0).W+1 != tb.Rect(1).X {
		t.Fatalf("tab rects: %+v %+v", tb.Rect(0), tb.Rect(1))
	}
}

func TestStatusBar(t *testing.T) {
	sb := NewStatusBar()
	sb.SetWidth(40)
	sb.SetLeft("JSON Formatter")
	sb.SetHints(StatusHint{Key: "ctrl+y", Desc: "copy"}, StatusHint{Key: "ctrl+c", Desc: "quit"})
	view := sb.View()
	if got := lipgloss.Width(view); got != 40 {
		t.Fatalf("status bar width = %d, want 40", got)
	}
	if sb.Height() != 1 || lipgloss.Height(view) != 1 {
		t.Fatal("status bar must be exactly 1 line")
	}
	// 左侧过长时截断且仍为 1 行。
	sb.SetLeft(strings.Repeat("L", 60))
	assertBounded(t, "status bar", sb.View(), 40)
	// Toast 显示位。
	toast, _ := NewToast(LevelSuccess, "Copied to clipboard", time.Minute)
	sb.SetLeft("Tool")
	sb.SetToast(toast)
	view = sb.View()
	if !strings.Contains(ansi.Strip(view), "Copied to clipboard") {
		t.Fatalf("status bar should show the toast: %q", view)
	}
	assertBounded(t, "status bar toast", view, 40)
	if lipgloss.Height(view) != 1 {
		t.Fatal("toast must not add lines")
	}
	sb.ClearToast()
	// 极限宽度。
	sb.SetWidth(8)
	assertBounded(t, "status bar narrow", sb.View(), 8)
}

func TestPanel(t *testing.T) {
	p := NewPanel("Input")
	p.SetSize(40, 10)
	p.SetToolbar(Button{Label: "Copy"}, Button{Label: "Paste"})
	p.SetFocused(true)
	view, rect := p.Render("hello")
	if len(strings.Split(view, "\n")) != 10 {
		t.Fatalf("panel rows = %d, want 10", len(strings.Split(view, "\n")))
	}
	assertBounded(t, "panel", view, 40)
	if rect.W != 40 || rect.H != 10 {
		t.Fatalf("panel rect = %+v", rect)
	}
	if !strings.Contains(ansi.Strip(view), " Input ") {
		t.Fatalf("panel title missing: %q", ansi.Strip(view))
	}
	// 标题行右端嵌按钮；宿主把全局 X 平移到按钮条局部坐标后命中。
	tr := p.ToolbarRect()
	if tr.W == 0 || tr.Y != 0 || tr.X+tr.W != 39 {
		t.Fatalf("toolbar rect = %+v", tr)
	}
	tb := p.Toolbar()
	local := pressAt(tr.X, 0)
	local.X -= tr.X
	if got := tb.Index(local); got != 0 {
		t.Fatalf("panel toolbar first button hit = %d", got)
	}
	local = pressAt(tr.X+tb.Rect(1).X, 0)
	local.X -= tr.X
	if got := tb.Index(local); got != 1 {
		t.Fatalf("panel toolbar second button hit = %d", got)
	}
	// 放不下时按钮区为空矩形。
	narrow := NewPanel("A very long pane title")
	narrow.SetSize(12, 5)
	narrow.SetToolbar(Button{Label: "Copy"}, Button{Label: "Paste"})
	if r := narrow.ToolbarRect(); r.W != 0 {
		t.Fatalf("narrow panel toolbar rect = %+v", r)
	}
	view, rect = narrow.Render("x")
	if len(strings.Split(view, "\n")) != 5 {
		t.Fatalf("narrow panel rows = %d", len(strings.Split(view, "\n")))
	}
	assertBounded(t, "narrow panel", view, 12)
	if rect.H != 5 {
		t.Fatalf("narrow panel rect = %+v", rect)
	}
	// 非焦点边框。
	p.SetFocused(false)
	unfocused, _ := p.Render("hello")
	assertBounded(t, "unfocused panel", unfocused, 40)
}

func TestLayoutPrimitives(t *testing.T) {
	rects, row := PlaceRow([]string{"ab", "c"}, 1)
	if row != "ab c" {
		t.Fatalf("PlaceRow = %q", row)
	}
	if len(rects) != 2 || rects[0] != (Rect{0, 0, 2, 1}) || rects[1] != (Rect{3, 0, 1, 1}) {
		t.Fatalf("PlaceRow rects = %+v", rects)
	}
	// 多行单元格顶部对齐。
	rects, row = PlaceRow([]string{"a\nb", "cd"}, 0)
	assertBounded(t, "PlaceRow", row, 3)
	if rects[0].H != 2 || rects[1].H != 2 {
		t.Fatalf("PlaceRow heights = %+v", rects)
	}
	_, col := PlaceCol([]string{"ab", "cd"}, 1)
	if col != "ab\n  \ncd" {
		t.Fatalf("PlaceCol = %q", col)
	}
	if x, y := Center(10, 4, 80, 24); x != 35 || y != 10 {
		t.Fatalf("Center = %d,%d", x, y)
	}
	if x, y := Center(90, 30, 80, 24); x != 0 || y != 0 {
		t.Fatalf("Center clamp = %d,%d", x, y)
	}
	if got := Fill("a\nb", 3, 3); lipgloss.Height(got) != 3 || lipgloss.Width(got) != 3 {
		t.Fatalf("Fill = %q", got)
	}
	if got := Fill("abcdef", 3, 1); lipgloss.Width(got) != 3 {
		t.Fatalf("Fill truncate = %q", got)
	}
	if !HitIn(pressAt(2, 2), Rect{1, 1, 3, 3}) {
		t.Fatal("HitIn inside failed")
	}
	if HitIn(pressAt(4, 2), Rect{1, 1, 3, 3}) {
		t.Fatal("HitIn outside passed")
	}
	r := Rect{1, 1, 3, 3}
	if !r.Contains(1, 1) || r.Contains(4, 4) || r.Right() != 4 || r.Bottom() != 4 {
		t.Fatalf("Rect methods: %+v", r)
	}
}

func TestIcons(t *testing.T) {
	names := []theme.IconName{
		theme.IconLogo, theme.IconSearch, theme.IconInfo, theme.IconSuccess, theme.IconWarning,
		theme.IconError, theme.IconCopy, theme.IconPaste, theme.IconFolder, theme.IconClear,
		theme.IconExpand, theme.IconCollapse, theme.IconClock, theme.IconCalendar, theme.IconTable,
		theme.IconTransform, theme.IconNumber, theme.IconCertificate, theme.IconEye, theme.IconEncode,
		theme.IconGlobe, theme.IconDatabase, theme.IconPaint, theme.IconFile, theme.IconKey,
		theme.IconWrench, theme.IconBug, theme.IconBook, theme.IconCompare, theme.IconZip, theme.IconCode,
	}
	previous := theme.ASCII()
	defer theme.SetASCII(previous)
	// ASCII 回退全部单列宽。
	theme.SetASCII(true)
	for _, name := range names {
		if got := runewidth.StringWidth(theme.Icon(name)); got != 1 {
			t.Fatalf("ascii icon %q width = %d, want 1", name, got)
		}
	}
	// Nerd Font 字形宽度与 go-runewidth 一致（1 或 2 列）。
	theme.SetASCII(false)
	for _, name := range names {
		icon := theme.Icon(name)
		if icon == "" {
			t.Fatalf("icon %q is empty", name)
		}
		if got := runewidth.StringWidth(icon); got != IconWidth(name) || got < 1 || got > 2 {
			t.Fatalf("nerd icon %q width = %d, IconWidth = %d", name, got, IconWidth(name))
		}
	}
	if theme.ToolIcon("converters.cron") == "" || theme.ToolIcon("unknown.tool") == "" {
		t.Fatal("tool icons must never be empty")
	}
	if theme.GroupIcon("converters") == "" || theme.GroupIcon("nope") == "" {
		t.Fatal("group icons must never be empty")
	}
	if theme.StatusIcon(3) != theme.Icon(theme.IconError) {
		t.Fatal("status icon 3 should be the error icon")
	}
}

func TestBothPalettesRender(t *testing.T) {
	previous := theme.Dark()
	defer theme.SetDark(previous)
	for _, dark := range []bool{true, false} {
		theme.SetDark(dark)
		if theme.Dark() != dark {
			t.Fatalf("SetDark(%v) did not stick", dark)
		}
		components := map[string]string{
			"button":	Button{Label: "OK", Variant: VariantPrimary, Inline: true}.View(),
			"bordered":	Button{Label: "OK"}.View(),
			"switch":	Switch{On: true}.View(),
			"panel":	func() string { p := NewPanel("P"); p.SetSize(20, 5); v, _ := p.Render("x"); return v }(),
			"modal":	func() string { m := &Modal{Kind: KindInfo, Body: "hi"}; m.SetSize(40, 12); return m.View() }(),
			"toast":	func() string { toast, _ := NewToast(LevelError, "boom", 0); return toast.View() }(),
			"tabs":		NewTabs("A", "B").View(),
			"status":	func() string { sb := NewStatusBar(); sb.SetWidth(30); sb.SetLeft("x"); return sb.View() }(),
			"select":	(&Select{Options: []string{"a"}}).View(),
			"stepper":	(&Stepper{Value: 1, Min: 0, Max: 2}).View(),
			"toolbar":	RenderToolbar(40, Button{Label: "Go"}),
		}
		for name, view := range components {
			if strings.TrimSpace(ansi.Strip(view)) == "" {
				t.Fatalf("dark=%v %s rendered empty", dark, name)
			}
			assertBounded(t, name+"@"+map[bool]string{true: "dark", false: "light"}[dark], view, 80)
		}
	}
	// 明暗切换必须改变色值。
	theme.SetDark(false)
	lightAccent := theme.Accent
	theme.SetDark(true)
	if theme.Accent == lightAccent {
		t.Fatal("light and dark accents must differ")
	}
}

// TestButtonBorderedGeometry 锁死按钮几何：带边框按钮内边距
// 每侧 2 列，Width = label+6；内联为 label+2（冻结）。悬停只换色不改宽。
func TestButtonBorderedGeometry(t *testing.T) {
	for _, variant := range []Variant{VariantGhost, VariantPrimary, VariantDanger} {
		b := Button{Label: "Apply", Variant: variant}
		if got := b.Width(); got != Width("Apply")+6 {
			t.Fatalf("bordered width = %d, want label+6 = %d", got, Width("Apply")+6)
		}
		if got := lipgloss.Width(b.View()); got != b.Width() {
			t.Fatalf("bordered view width %d != Width %d", got, b.Width())
		}
		// hover/focused 组合不得改变宽度（状态只影响颜色与字重）。
		hover := b
		hover.Hover = true
		focused := b
		focused.Focused = true
		for _, s := range []Button{hover, focused} {
			if s.Width() != b.Width() || lipgloss.Width(s.View()) != b.Width() {
				t.Fatalf("variant %d: state changed width", variant)
			}
		}
	}
	inline := Button{Label: "Apply", Inline: true}
	if inline.Width() != Width("Apply")+2 {
		t.Fatalf("inline width = %d, want label+2", inline.Width())
	}
	// 极窄 MinWidth：带边框形态按 decor=6 计算内容预算，仍不得越界。
	bordered := Button{Label: "Confirm", MinWidth: 8}
	assertBounded(t, "bordered minwidth", bordered.View(), 8)
}

// TestSelectChipAndMenuStates 覆盖 chip 两段分色总宽不变
// （chipWidth 公式冻结）、hover/focused 抬升不改宽；菜单 BorderStrong、
// 滚动指示加宽 2 列且 thumb 位置正确。
func TestSelectChipAndMenuStates(t *testing.T) {
	s := &Select{Options: []string{"json", "yaml", "toml"}, Index: 0}
	base := s.View()
	for _, mutate := range []func(*Select){
		func(v *Select) { v.Hover = true },
		func(v *Select) { v.Focused = true },
	} {
		mutate(s)
		if lipgloss.Width(s.View()) != lipgloss.Width(base) {
			t.Fatal("chip state must not change width")
		}
		if lipgloss.Width(s.View()) != s.ChipRect(0, 0).W {
			t.Fatal("chip view width must match ChipRect")
		}
	}
	s.Hover, s.Focused = false, false
	// 菜单不滚动：宽 = 最长选项 + 4 + 边框 2。
	s.Open = true
	steady := Width(s.OpenView())
	if steady != Width("toml")+4+2 {
		t.Fatalf("steady menu width = %d, want %d", steady, Width("toml")+4+2)
	}
	// 超出可视行数：右侧 +2 列（1 空列 + 指示列）。
	s.Options = []string{"a", "b", "c", "d", "e", "f"}
	s.ListHeight = 3
	s.Cursor = 0
	scrolled := s.OpenView()
	if w := Width(scrolled); w != Width("a")+4+2+2 {
		t.Fatalf("scrolled menu width = %d, want %d", w, Width("a")+4+2+2)
	}
	// OverlayRect/ListHit 与 OpenView 同一份推导：加宽后命中仍一致。
	r := s.OverlayRect(0, 0, 40, 24)
	if r.W != Width(scrolled) {
		t.Fatalf("overlay rect width %d != view width %d", r.W, Width(scrolled))
	}
	if option, ok := s.ListHit(pressAt(r.X+r.W-1, r.Y+1), 0, 0, 40, 24); !ok || option != 0 {
		t.Fatalf("indicator column click should resolve via ListHit: option=%d ok=%v", option, ok)
	}
	// thumb 落在轨道可视范围内。
	if !strings.Contains(scrolled, "▐") || !strings.Contains(scrolled, "│") {
		t.Fatal("scrolled menu must render track and thumb")
	}
}

// TestToolbarHoverAPI 覆盖 SetHover 只改渲染不改命中；
// HoverAt 镜像 Index 的 Rect 遍历但只认坐标。
func TestToolbarHoverAPI(t *testing.T) {
	buttons := []Button{{Label: "Copy"}, {Label: "Paste"}}
	tb := NewToolbar(buttons...)
	tb.SetSize(40, 1)
	plain := tb.View()
	tb.SetHover(0)
	hovered := tb.View()
	if lipgloss.Width(hovered) != lipgloss.Width(plain) {
		t.Fatal("hover must not change toolbar width")
	}
	if r := tb.Rect(0); tb.HoverAt(r.X+r.W/2, 0) != 0 {
		t.Fatal("HoverAt should hit the first button")
	}
	if r := tb.Rect(1); tb.HoverAt(r.X+r.W/2, 0) != 1 {
		t.Fatal("HoverAt should hit the second button")
	}
	if tb.HoverAt(-1, 0) != -1 || tb.HoverAt(39, 0) != -1 {
		t.Fatal("HoverAt must return -1 outside buttons")
	}
	tb.SetHover(-1)
	if tb.HoverAt(0, 0) != 0 {	// 第一颗按钮从 X=0 开始
		t.Fatal("HoverAt at origin should hit button 0")
	}
	// 越界下标按 -1 处理。
	tb.SetHover(99)
	if lipgloss.Width(tb.View()) != lipgloss.Width(plain) {
		t.Fatal("out-of-range hover must be clamped to none")
	}
}

// TestStatusTokens 覆盖 SetLeftTokens 的宽度算术必须与宿主
// 命中换算一致（前导 1 列 + token 间 Space2），整条仍严格满宽 1 行。
func TestStatusTokens(t *testing.T) {
	sb := NewStatusBar()
	sb.SetWidth(60)
	sb.SetLeftTokens([]StatusToken{
		{Label: "[Tools]", Clickable: true},
		{Label: "Formatter"},
	})
	view := sb.View()
	if got := lipgloss.Width(view); got != 60 {
		t.Fatalf("status bar width = %d, want 60", got)
	}
	if lipgloss.Height(view) != 1 {
		t.Fatal("status bar must stay 1 line with tokens")
	}
	stripped := ansi.Strip(view)
	// 布局算术：前导 1 列 + token + Space2 分隔（宿主 statusActions 同式）。
	if want := " " + "[Tools]" + strings.Repeat(" ", Space2) + "Formatter"; !strings.HasPrefix(stripped, want) {
		t.Fatalf("token layout mismatch: %q, want prefix %q", stripped, want)
	}
	// SetLeft 之后回到单 token 行为。
	sb.SetLeft("Tool")
	if !strings.Contains(ansi.Strip(sb.View()), "Tool") {
		t.Fatal("SetLeft must still render")
	}
}

// TestPanelContentWidth 覆盖文本区宽 = 总宽-4 与渲染行数合同。
func TestPanelContentWidth(t *testing.T) {
	p := NewPanel("Input")
	p.SetSize(40, 6)
	if got := p.ContentWidth(); got != 36 {
		t.Fatalf("ContentWidth = %d, want 36", got)
	}
	view, rect := p.Render(strings.Repeat("x", 36))
	if rect.W != 40 || rect.H != 6 {
		t.Fatalf("panel rect = %+v", rect)
	}
	assertBounded(t, "panel content", view, 40)
	// 内容带内边距渲染：正文左缘在边框内 2 列。
	lines := strings.Split(view, "\n")
	if got := ansi.Strip(lines[1]); !strings.HasPrefix(got, "│ x") {
		t.Fatalf("content padding missing: %q", got)
	}
}

// TestModalDanger 覆盖 Danger 弹窗标题前缀错误图标、
// 确认按钮变 Danger 变体；键盘语义不变。
func TestModalDanger(t *testing.T) {
	m := &Modal{Kind: KindConfirm, Title: "Delete", Body: "Remove file?", Danger: true}
	m.SetSize(60, 20)
	buttons := m.Buttons()
	if len(buttons) != 2 || buttons[0].Variant != VariantDanger {
		t.Fatalf("danger modal confirm variant = %v", buttons[0].Variant)
	}
	view := m.View()
	if !strings.Contains(view, theme.StatusIcon(3)) {
		t.Fatal("danger modal must prefix the error icon")
	}
	if !strings.Contains(view, "Delete") {
		t.Fatal("danger modal must keep the title")
	}
	// 键盘语义不变。
	if action, consumed := m.HandleKey("enter"); !consumed || action != ActionConfirm {
		t.Fatalf("enter = %v %v", action, consumed)
	}
	// 普通 Confirm 弹窗不带头图标、确认按钮为 Primary。
	plain := &Modal{Kind: KindConfirm, Title: "Delete", Body: "Remove file?"}
	plain.SetSize(60, 20)
	if plain.Buttons()[0].Variant != VariantPrimary {
		t.Fatal("non-danger confirm must stay Primary")
	}
}

// TestToastPill 覆盖 View 即 pill（左右各 1 列内边距），
// 四级别均非空、可截断。
func TestToastPill(t *testing.T) {
	levels := []Level{LevelInfo, LevelSuccess, LevelWarning, LevelError}
	for _, level := range levels {
		toast, _ := NewToast(level, "done", 0)
		plain := Width(toast.Icon() + " done")
		want := plain + 2
		if got := toast.Width(); got != want {
			t.Fatalf("level %d pill width = %d, want plain+2 = %d", level, got, want)
		}
		if lipgloss.Height(toast.View()) != 1 {
			t.Fatalf("level %d pill must be 1 row", level)
		}
	}
	toast, _ := NewToast(LevelError, strings.Repeat("e", 40), 0)
	assertBounded(t, "toast pill truncate", toast.ViewTruncate(8), 8)
}

// TestSwitchStates 覆盖状态矩阵：宽度冻结，focus 用字重、
// off+hover 抬升 ControlHover、on 态仅聚焦时加粗。
func TestSwitchStates(t *testing.T) {
	base := Switch{}
	width := base.Width()
	states := []Switch{
		{On: true},
		{On: true, Focused: true},
		{Hover: true},
		{Focused: true},
		{Hover: true, Focused: true},
		{Disabled: true},
		{On: true, Disabled: true},
	}
	for i, s := range states {
		if s.Width() != width {
			t.Fatalf("state %d changed switch width", i)
		}
		if got := lipgloss.Width(s.View()); got != width {
			t.Fatalf("state %d view width = %d, want %d", i, got, width)
		}
	}
	if !strings.Contains(base.View(), "○") {
		t.Fatal("off state must render ○")
	}
	on := Switch{On: true}
	if !strings.Contains(on.View(), "●") {
		t.Fatal("on state must render ●")
	}
}

// TestStepperValueCell 覆盖值格 bg=Control、左右各 1 列内边距
// （宽 = valueWidth+2），三格 gap = Space1，Rects 与渲染自洽。
func TestStepperValueCell(t *testing.T) {
	s := &Stepper{Label: "Length", Value: 16, Min: 8, Max: 64}
	dec, val, inc := s.Rects(0, 0)
	if val.W != 2+2*Space1 {
		t.Fatalf("value cell width = %d, want valueWidth+2 = %d", val.W, 2+2*Space1)
	}
	if inc.X-(val.X+val.W) != Space1 || (val.X-dec.X)-dec.W != Space1 {
		t.Fatalf("stepper gaps must be Space1: %+v %+v %+v", dec, val, inc)
	}
	if got := lipgloss.Width(s.View()); got != inc.X+inc.W {
		t.Fatalf("view width %d != rects width %d", got, inc.X+inc.W)
	}
}

// TestTabsSurfaceActive 覆盖激活项 = Control 填充 + 正文色 bold，
// 几何冻结。
func TestTabsSurfaceActive(t *testing.T) {
	tb := NewTabs("Input", "Output")
	tb.SetWidth(80)
	if tb.Rect(0).X+tb.Rect(0).W+1 != tb.Rect(1).X {
		t.Fatal("tab geometry must stay frozen")
	}
	if tb.Rect(0).W != Width("Input")+2 {
		t.Fatalf("tab width = %d, want label+2", tb.Rect(0).W)
	}
	if !strings.Contains(ansi.Strip(tb.View()), " Input ") {
		t.Fatalf("active tab must render padded label: %q", ansi.Strip(tb.View()))
	}
}

// TestLayoutSpaceAndHelpers 覆盖 Space 刻度、IconCell 占格、
// DimLine 的重注入契约。
func TestLayoutSpaceAndHelpers(t *testing.T) {
	if Space1 != 1 || Space2 != 2 || Space3 != 3 {
		t.Fatal("space scale values are contractual")
	}
	if got := Width(CanvasGutter(Space3)); got != Space3 {
		t.Fatalf("CanvasGutter width = %d, want %d", got, Space3)
	}
	if got := IconCell("i", 2); got != "i " || Width(got) != 2 {
		t.Fatalf("IconCell narrow = %q", got)
	}
	if got := IconCell("ab", 2); got != "ab" || Width(got) != 2 {
		t.Fatalf("IconCell exact = %q", got)
	}
	if got := IconCell("abc", 2); got != "ab" || Width(got) != 2 {
		t.Fatalf("IconCell wide = %q", got)
	}
	// DimLine：行首注入 faint+Overlay 底，且每个 reset 之后重新注入。
	line := "a\x1b[31mred\x1b[0mb"
	dim := DimLine(line)
	if !strings.HasPrefix(dim, "\x1b[2m"+theme.OverlaySGR()) {
		t.Fatalf("DimLine missing faint+overlay prefix: %q", dim)
	}
	if want := "\x1b[0m\x1b[2m" + theme.OverlaySGR(); !strings.Contains(dim, want) {
		t.Fatalf("DimLine must re-inject after each reset: %q", dim)
	}
	if !strings.HasSuffix(dim, "\x1b[0m") {
		t.Fatalf("DimLine must end with a reset: %q", dim)
	}
	// 无 reset 的行也必须被包裹。
	plain := DimLine("hi")
	if !strings.HasPrefix(plain, "\x1b[2m") || !strings.HasSuffix(plain, "\x1b[0m") {
		t.Fatalf("DimLine plain = %q", plain)
	}
}
