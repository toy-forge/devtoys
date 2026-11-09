package toolkit

import (
	"fmt"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/toy-forge/devtoys/internal/theme"
	"github.com/toy-forge/devtoys/internal/toolkit/editor"
	"github.com/toy-forge/devtoys/internal/ui"
)

// StatusSeverity controls the color and icon of a result bar.
type StatusSeverity uint8

const (
	StatusInfo	StatusSeverity	= iota
	StatusSuccess
	StatusWarning
	StatusError
)

// ThreePane displays two editable inputs and an output, as in desktop compare tools.
// 窗格使用 ui.Panel（焦点环 + 标题行按钮），底部为 ui.StatusBar +
// Toast（错误/Copied），渲染与命中共用 resize() 计算的矩形。
type ThreePane struct {
	A, B		editor.Model
	Output		viewport.Model
	Titles		[3]string
	Focus		int
	Err		error
	Copied		bool
	width, height	int
	compact		bool
	resultBar	bool
	// footerGap 是正文与底部条之间的空行数（矮屏与状态行模式为 0）。
	footerGap	int
	status		StatusSeverity
	outputFirst	bool
	extendable	[3]bool
	expandKey	string
	expanded	int
	rects		[3]ui.Rect
	text		string

	panes		[3]*ui.Panel
	footer		*ui.StatusBar
	footerBar	*ui.Toolbar
	tabsBar		*ui.Toolbar
	toast		ui.Toast
	hasToast	bool
}

func NewThreePane(a, b string) *ThreePane {
	p := &ThreePane{
		A:	editor.New(), B: editor.New(), Output: viewport.New(30, 8),
		Titles:	[3]string{a, b, "Result"}, width: 80, height: 24, expanded: -1, expandKey: "alt+e",
	}
	p.A.Placeholder = "Enter " + a + "…"
	p.B.Placeholder = "Enter " + b + "…"
	p.A.ShowLineNumbers = false
	p.B.ShowLineNumbers = false
	p.A.Focus()
	p.panes = [3]*ui.Panel{ui.NewPanel(p.paneTitle(0)), ui.NewPanel(p.paneTitle(1)), ui.NewPanel(p.paneTitle(2))}
	p.footer = ui.NewStatusBar()
	p.footerBar = ui.NewToolbar(
		ui.Button{Label: "Copy"},
		ui.Button{Label: "<"},
		ui.Button{Label: ">"},
	)
	p.tabsBar = &ui.Toolbar{Gap: 1}
	p.resize()
	return p
}

// paneTitle 返回窗格标题：与 Workspace 窗格一致使用 Title Case（不做全大写）。
func (p *ThreePane) paneTitle(index int) string	{ return p.Titles[index] }

func (p *ThreePane) SetOutput(text string) {
	p.status = StatusInfo
	p.text = text
	display := strings.Map(func(r rune) rune {
		switch {
		case r == '\n':
			return r
		case r == '\t':
			return '␉'
		case r < 0x20 || r == 0x7f:
			return '�'
		default:
			return r
		}
	}, text)
	p.Output.SetContent(display)
}

// SetStatus displays a one-line result with the matching severity treatment.
func (p *ThreePane) SetStatus(text string, severity StatusSeverity) {
	p.SetOutput(text)
	p.status = severity
}
func (p *ThreePane) OutputText() string	{ return p.text }
func (p *ThreePane) SetSize(w, h int)	{ p.width, p.height = max(1, w), max(1, h); p.resize() }

// SetOutputFirst places the output above the two inputs in a wide layout.
func (p *ThreePane) SetOutputFirst(enabled bool)	{ p.outputFirst = enabled; p.resize() }

// SetExtendable marks panes that can expand to fill the detail view.
func (p *ThreePane) SetExtendable(indices ...int) {
	for _, index := range indices {
		if index >= 0 && index < len(p.extendable) {
			p.extendable[index] = true
		}
	}
}

// SetExpandKey changes the key used to expand and restore extendable panes.
func (p *ThreePane) SetExpandKey(key string)	{ p.expandKey = key }

// SetResultBar uses the desktop validator layout: two inputs and a status row.
func (p *ThreePane) SetResultBar(enabled bool) {
	p.resultBar = enabled
	if enabled && p.Focus == 2 {
		p.Focus = 0
	}
	p.resize()
}
func (p *ThreePane) resize() {
	p.compact = p.width < 40 || p.height < 11
	p.rects = [3]ui.Rect{}
	width, height := p.width, p.height
	// 底部条上方留一行空白（与 Workspace 同一版式）；状态行模式没有底部条，不留。
	p.footerGap = 0
	if !p.resultBar && height >= 8 {
		p.footerGap = ui.Space1
	}
	height -= p.footerGap
	if p.expanded >= 0 {
		// 展开态仍保留底部一行（resultBar 为状态行，其余为状态条）。
		h := max(3, height-1)
		p.rects[p.expanded] = ui.Rect{X: 0, Y: 0, W: width, H: h}
		p.setPaneSize(p.expanded, p.rects[p.expanded])
		p.layoutPanes()
		return
	}
	if p.resultBar {
		h := max(3, height-1)
		if p.compact {
			h = max(3, height-2)
			p.rects[p.Focus] = ui.Rect{X: 0, Y: 1, W: width, H: h}
		} else {
			// 双栏缝统一 Space2：中缝 2 列，两窗格各占一半。
			left := (width - ui.Space2) / 2
			right := width - left - ui.Space2
			p.rects[0] = ui.Rect{X: 0, Y: 0, W: left, H: h}
			p.rects[1] = ui.Rect{X: left + ui.Space2, Y: 0, W: right, H: h}
		}
		p.setPaneSize(0, p.rects[0])
		p.setPaneSize(1, p.rects[1])
		p.Output.Width = max(1, width-2)
		p.Output.Height = 1
		p.Output.SetYOffset(0)
		p.layoutPanes()
		return
	}
	if p.compact {
		h := max(3, height-2)
		p.rects[p.Focus] = ui.Rect{X: 0, Y: 1, W: width, H: h}
	} else {
		// 横向缝 Space2、纵向缝 1 空行：双栏中缝 2 列，
		// 输出窗格与输入窗格之间留 1 行。
		top := max(3, (height-2)/3)
		bottom := max(3, height-2-top)
		left := (width - ui.Space2) / 2
		right := width - left - ui.Space2
		if p.outputFirst {
			p.rects = [3]ui.Rect{
				{X: 0, Y: top + 1, W: left, H: bottom},
				{X: left + ui.Space2, Y: top + 1, W: right, H: bottom},
				{X: 0, Y: 0, W: width, H: top},
			}
		} else {
			p.rects = [3]ui.Rect{
				{X: 0, Y: 0, W: left, H: top},
				{X: left + ui.Space2, Y: 0, W: right, H: top},
				{X: 0, Y: top + 1, W: width, H: bottom},
			}
		}
	}
	for i := range p.rects {
		p.setPaneSize(i, p.rects[i])
	}
	p.Output.SetYOffset(p.Output.YOffset)
	p.layoutPanes()
}

// setPaneSize 把矩形同步到编辑器/视口的内部尺寸。
// 宽度从 Panel.ContentWidth() 推导：文本区 = 总宽-4
// （Panel 水平内边距 1 列/侧）；先 SetSize 再取值保证本帧一致。
func (p *ThreePane) setPaneSize(index int, r ui.Rect) {
	innerH := max(1, r.H-2)
	var innerW int
	if r.W > 0 {
		p.panes[index].SetSize(r.W, r.H)
		innerW = max(1, p.panes[index].ContentWidth())
	} else {
		innerW = 1
	}
	switch index {
	case 0:
		p.A.SetWidth(innerW)
		p.A.SetHeight(innerH)
	case 1:
		p.B.SetWidth(innerW)
		p.B.SetHeight(innerH)
	case 2:
		p.Output.Width = innerW
		p.Output.Height = innerH
	}
}

// layoutPanes 把标题、焦点与按钮同步到 ui.Panel，保证命中与渲染一致。
func (p *ThreePane) layoutPanes() {
	for i, pane := range p.panes {
		pane.SetTitle(p.paneTitle(i))
		pane.SetFocused(p.Focus == i)
		pane.SetToolbar(paneButtons(p.paneControls(i), false)...)
		if r := p.rects[i]; r.W > 0 {
			pane.SetSize(r.W, r.H)
		}
	}
	p.tabsBar.SetButtons(paneButtons(p.tabs(), false)...)
	p.tabsBar.SetSize(p.width, 1)
	p.footer.SetWidth(p.width)
}

// FocusPane focuses an input or the output, updating its editor and compact layout.
func (p *ThreePane) FocusPane(i int)	{ p.focus(i) }

func (p *ThreePane) focus(i int) {
	count := 3
	if p.resultBar {
		count = 2
	}
	p.Focus = (i + count) % count
	if p.expanded >= 0 && p.expanded != p.Focus {
		if p.extendable[p.Focus] {
			p.expanded = p.Focus
		} else {
			p.expanded = -1
		}
	}
	p.A.Blur()
	p.B.Blur()
	switch p.Focus {
	case 0:
		p.A.FocusPreservingScroll()
	case 1:
		p.B.FocusPreservingScroll()
	}
	if p.compact {
		p.resize()
	}
}
func (p *ThreePane) tabs() []string {
	labels := []string{"A", "B", "Result"}
	if p.resultBar {
		labels = labels[:2]
	}
	labels[p.Focus] = "*" + labels[p.Focus]
	return labels
}

func (p *ThreePane) paneControls(index int) []string {
	controls := []string{"Paste", "Open", "Clear", "Copy"}
	if index == 2 {
		controls = []string{"Copy"}
	}
	if p.extendable[index] || p.expanded == index {
		controls = append(controls, paneExpandControl(p.expanded == index))
	}
	return controls
}

func (p *ThreePane) copy() tea.Cmd {
	if err := clipboard.WriteAll(stripANSI(p.text)); err != nil {
		p.Err = fmt.Errorf("could not copy to clipboard: %w", err)
		p.Copied, p.hasToast = false, false
		return nil
	}
	p.Err = nil
	p.Copied = true
	toast, cmd := ui.NewToast(ui.LevelSuccess, "Copied to clipboard", copiedToastTTL)
	p.toast, p.hasToast = toast, true
	return cmd
}

func (p *ThreePane) copyInput(target *editor.Model) tea.Cmd {
	if err := clipboard.WriteAll(target.Value()); err != nil {
		p.Err = fmt.Errorf("could not copy input: %w", err)
		p.Copied, p.hasToast = false, false
		return nil
	}
	p.Err = nil
	p.Copied = true
	toast, cmd := ui.NewToast(ui.LevelSuccess, "Copied to clipboard", copiedToastTTL)
	p.toast, p.hasToast = toast, true
	return cmd
}

func (p *ThreePane) Update(msg tea.Msg) tea.Cmd {
	switch event := msg.(type) {
	case ui.ToastExpireMsg:
		if p.hasToast && p.toast.Expired(event) {
			p.hasToast = false
			p.Copied = false
		}
		return nil
	case FileImportMsg:
		if p.Focus == 2 {
			p.focus(0)
		}
		target := &p.A
		if p.Focus == 1 {
			target = &p.B
		}
		p.Err = InsertFileText(target, event.Text)
		if p.Err != nil {
			p.hasToast = false
		}
		return nil
	case tea.WindowSizeMsg:
		p.SetSize(event.Width, event.Height)
		return nil
	case tea.KeyMsg:
		if event.String() != "ctrl+y" {
			p.Copied = false
			p.hasToast = false
		}
		switch event.String() {
		case p.expandKey:
			if p.expanded >= 0 {
				p.expanded = -1
			} else if p.extendable[p.Focus] {
				p.expanded = p.Focus
			}
			p.resize()
			return nil
		case "tab":
			p.focus(p.Focus + 1)
			return nil
		case "shift+tab":
			p.focus(p.Focus - 1)
			return nil
		case "ctrl+y":
			return p.copy()
		}
		var cmd tea.Cmd
		switch p.Focus {
		case 0:
			p.A, cmd = p.A.Update(event)
		case 1:
			p.B, cmd = p.B.Update(event)
		case 2:
			p.Output, cmd = p.Output.Update(event)
		}
		return cmd
	case tea.MouseMsg:
		if event.Action != tea.MouseActionPress {
			return nil
		}
		if p.compact && event.Y == 0 && p.expanded < 0 {
			local := event
			local.Y = 0
			if i := p.tabsBar.Index(local); i >= 0 && i < len(p.tabs()) {
				p.focus(i)
			}
			return nil
		}
		if !p.resultBar && p.expanded < 0 && event.Y == p.height-1 {
			if cmd, handled := p.handleFooterMouse(event); handled {
				return cmd
			}
		}
		for i, r := range p.rects {
			if r.W == 0 || !r.Contains(event.X, event.Y) {
				continue
			}
			if event.Button == tea.MouseButtonWheelUp || event.Button == tea.MouseButtonWheelDown {
				delta := 3
				if event.Button == tea.MouseButtonWheelUp {
					delta = -3
				}
				switch i {
				case 0:
					p.A.Scroll(delta)
				case 1:
					p.B.Scroll(delta)
				case 2:
					var cmd tea.Cmd
					p.Output, cmd = p.Output.Update(event)
					return cmd
				}
				return nil
			}
			if event.Button != tea.MouseButtonLeft {
				return nil
			}
			p.focus(i)
			if cmd, handled := p.paneToolbarClick(i, r, event); handled {
				return cmd
			}
			if i < 2 && event.X > r.X && event.X < r.X+r.W-2 && event.Y > r.Y && event.Y < r.Y+r.H-1 {
				// Panel 水平内边距 1 列/侧，文本起点 = 边框内 2 列；Y 行起点 = 边框内 1 行。
				if i == 0 {
					return p.A.PlaceCursor(event.X-r.X-2, event.Y-r.Y-1)
				}
				return p.B.PlaceCursor(event.X-r.X-2, event.Y-r.Y-1)
			}
			return nil
		}
	}
	return nil
}

// paneToolbarClick 命中窗格标题行按钮并分发动作。
func (p *ThreePane) paneToolbarClick(index int, r ui.Rect, mouse tea.MouseMsg) (tea.Cmd, bool) {
	if mouse.Y != r.Y {
		return nil, false
	}
	pane := p.panes[index]
	tr := paneToolbarRect(pane)
	if tr.W == 0 {
		return nil, false
	}
	local := mouse
	local.X -= r.X + tr.X
	local.Y = 0
	action := pane.Toolbar().Index(local)
	if action < 0 {
		return nil, false
	}
	controls := p.paneControls(index)
	if action >= len(controls) {
		return nil, false
	}
	selected := controls[action]
	if (p.extendable[index] || p.expanded == index) && selected == paneExpandControl(p.expanded == index) {
		if p.expanded == index {
			p.expanded = -1
		} else {
			p.expanded = index
		}
		p.resize()
		return nil, true
	}
	if index == 2 {
		return p.copy(), true
	}
	target := &p.A
	if index == 1 {
		target = &p.B
	}
	switch action {
	case 0:
		updated, cmd := target.Update(tea.KeyMsg{Type: tea.KeyCtrlV})
		*target = updated
		return cmd, true
	case 1:
		return func() tea.Msg { return OpenFileRequestMsg{} }, true
	case 2:
		target.SetValue("")
		p.Err = nil
		p.hasToast = false
		return func() tea.Msg { return InputClearedMsg{} }, true
	case 3:
		return p.copyInput(target), true
	}
	return nil, false
}

// handleFooterMouse 处理底部状态条右端按钮（Copy / < / >）的点击。
// Toast 显示时按钮已被替换，不做命中（与 footerView 的渲染条件一致）。
func (p *ThreePane) handleFooterMouse(mouse tea.MouseMsg) (tea.Cmd, bool) {
	if mouse.Button != tea.MouseButtonLeft || p.Err != nil || p.hasToast {
		return nil, false
	}
	r := p.footerBarRect()
	if r.W == 0 || mouse.X < r.X || mouse.X >= r.X+r.W {
		return nil, false
	}
	local := mouse
	local.X -= r.X
	local.Y = 0
	switch p.footerBar.Index(local) {
	case 0:
		return p.copy(), true
	case 1:
		p.Output.ScrollLeft(max(4, p.Output.Width/2))
		return nil, true
	case 2:
		p.Output.ScrollRight(max(4, p.Output.Width/2))
		return nil, true
	}
	return nil, false
}

// footerBarRect 返回状态条右端按钮组的矩形（局部坐标）。
func (p *ThreePane) footerBarRect() ui.Rect {
	if p.footerBar == nil || len(p.footerBar.Buttons) == 0 || p.height <= 0 {
		return ui.Rect{}
	}
	total := 0
	for i, b := range p.footerBar.Buttons {
		if i > 0 {
			total += max(0, p.footerBar.Gap)
		}
		total += b.Width()
	}
	x := p.width - 1 - total
	if x < 0 {
		return ui.Rect{}
	}
	return ui.Rect{X: x, Y: p.height - 1, W: total, H: 1}
}

func (p *ThreePane) pane(index int) string {
	pane := p.panes[index]
	r := p.rects[index]
	if r.W <= 0 {
		r = ui.Rect{X: 0, Y: 0, W: p.width, H: max(3, p.height-1)}
		pane.SetSize(r.W, r.H)
	}
	pane.SetTitle(p.paneTitle(index))
	pane.SetFocused(p.Focus == index)
	pane.SetToolbar(paneButtons(p.paneControls(index), false)...)
	var content string
	switch index {
	case 0:
		content = p.A.View()
	case 1:
		content = p.B.View()
	default:
		content = p.Output.View()
	}
	view, _ := pane.Render(content)
	return view
}

func (p *ThreePane) View() string {
	// 双栏缝 Space2：横向拼接用 gap 列、纵向堆叠用 1 空行，
	// 与 resize() 的 rects 布局同源。
	hGap := strings.Repeat(" ", ui.Space2)
	var body string
	if p.expanded >= 0 {
		body = p.pane(p.expanded)
	} else if p.compact {
		body = p.tabsBar.View() + "\n" + p.pane(p.Focus)
	} else if p.resultBar {
		body = lipgloss.JoinHorizontal(lipgloss.Top, p.pane(0), hGap, p.pane(1))
	} else if p.outputFirst {
		body = lipgloss.JoinVertical(lipgloss.Left, p.pane(2), "", lipgloss.JoinHorizontal(lipgloss.Top, p.pane(0), hGap, p.pane(1)))
	} else {
		body = lipgloss.JoinVertical(lipgloss.Left, lipgloss.JoinHorizontal(lipgloss.Top, p.pane(0), hGap, p.pane(1)), "", p.pane(2))
	}
	if p.resultBar {
		message, level := p.text, p.status
		if p.Err != nil {
			level, message = StatusError, "Error: "+p.Err.Error()
		}
		// 状态行沿用页脚行语言：前导空格、整行 Surface 底、
		// 超宽以 … 截断（与 StatusBar 同一处理）。
		body += "\n" + ui.WithBackground(" "+ansi.Truncate(statusLine(level, message), max(1, p.width-1), "…"), theme.Surface)
		return body
	}
	body += strings.Repeat("\n", p.footerGap+1) + p.footerView()
	return body
}

// footerView 渲染底部单行状态条。
func (p *ThreePane) footerView() string {
	p.footer.SetWidth(max(1, p.width))
	left := "tab focus · scroll to browse"
	if p.extendable[p.Focus] || p.expanded >= 0 {
		label := "Expand"
		if p.expanded >= 0 {
			label = "Restore"
		}
		left += " · " + strings.ToUpper(p.expandKey) + " " + label
	}
	p.footer.SetLeft(left)
	p.footer.SetHints()
	switch {
	case p.Err != nil:
		p.footer.SetToast(ui.Toast{Level: ui.LevelError, Message: "Error: " + p.Err.Error()})
	case p.hasToast:
		p.footer.SetToast(p.toast)
	default:
		p.footer.ClearToast()
		p.footer.SetRight(p.footerBar.View())
	}
	return p.footer.View()
}
