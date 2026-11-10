// Package regex implements a .NET-style regular expression tester.
// 输入正则与待匹配文本，输出区实时高亮全部匹配，
// 并逐条列出每次匹配的整体结果与各分组捕获。
package regex

import (
	"fmt"
	"strings"
	"time"

	"github.com/toy-forge/devtoys/internal/theme"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	regexp2 "github.com/dlclark/regexp2/v2"

	textarea "github.com/toy-forge/devtoys/internal/toolkit/editor"

	"github.com/toy-forge/devtoys/internal/toolkit"
	"github.com/toy-forge/devtoys/internal/ui"
)

// focusArea 表示工具内部焦点所在的区域（模式输入 / 文本输入 / 输出）。
type focusArea int

const (
	focusPattern	focusArea	= iota
	focusText
	focusOutput
	focusCheat
)

// paneRect 是桌面布局的窗格矩形（渲染与命中共用，禁止两套手算）。
type paneRect struct{ x, y, width, height int }

func (r paneRect) contains(x, y int) bool {
	return r.width > 0 && x >= r.x && x < r.x+r.width && y >= r.y && y < r.y+r.height
}
func (r paneRect) interior(x, y int) bool {
	return r.width > 2 && r.height > 2 && x > r.x && x < r.x+r.width-1 && y > r.y && y < r.y+r.height-1
}

var (
	labelStyle	= lipgloss.NewStyle().
			Foreground(theme.Muted)
	panelHeadingStyle	= lipgloss.NewStyle().
				Foreground(theme.BlueBright).
				Bold(true)
	sectionStyle	= lipgloss.NewStyle().
			Foreground(theme.Magenta).
			Bold(true)
	cheatSyntaxStyle	= lipgloss.NewStyle().
				Foreground(theme.YellowBright).
				Bold(true)

	matchStyle	= lipgloss.NewStyle().
			Bold(true).
			Foreground(theme.Text).
			Background(theme.Accent)
)

type model struct {
	id, title, icon, desc	string

	ws		toolkit.Workspace	// 公共双栏工作区：文本输入 + 输出
	pattern		textarea.Model		// 正则表达式输入
	cheat		viewport.Model
	options		[8]bool
	global		bool	// findall/全局匹配开关
	cheatExpanded	bool
	focus		focusArea
	matchesDisplay	string
	panelDisplay	string

	// 桌面布局矩形（渲染与命中共用）：0 模式、1 文本、2 速查、3 匹配信息。
	rects	[4]paneRect

	// canonical 页脚：ui.StatusBar 左句式 + 右端 Copy / < / >。
	footer		*ui.StatusBar
	footerBar	*ui.Toolbar

	width		int
	height		int
	flags		regexp2.RegexOptions
	owner		toolkit.Tool
	revision	uint64
	running, queued	bool
}

// New 创建正则测试器模型。
func New() *model {
	ws := toolkit.NewWorkspace(
		"Enter text to match…",
		"",
	)
	ws.SetOutputDisplay("", formatPanel("", nil, false))
	ws.SetSplitRatio(2, 1)
	pattern := textarea.New()
	pattern.Placeholder = `Example: \w+@\w+\.\w+`
	pattern.ShowLineNumbers = false
	pattern.SetHeight(1)
	pattern.SetWidth(40)
	pattern.Focus()
	cheat := viewport.New(1, 1)
	cheat.SetContent(formatCheatSheetAtWidth(80))
	// 初始焦点在表达式输入框，取消 NewWorkspace 默认给主输入区的焦点，
	// 避免两个输入框同时持焦、显示双光标。
	ws.Input.Blur()

	m := &model{
		id:		"testers.regex",
		title:		"Regular Expression Tester",
		icon:		"󰘦",
		desc:		"Test regular expressions",
		ws:		ws,
		pattern:	pattern,
		cheat:		cheat,
		options:	[8]bool{true},
		global:		true,
		focus:		focusPattern,
		footer:		ui.NewStatusBar(),
		footerBar: ui.NewToolbar(
			ui.Button{Label: "Copy"},
			ui.Button{Label: "<"},
			ui.Button{Label: ">"},
		),
		width:	80,
		height:	24,
	}
	m.matchesDisplay = formatMatchPanel("", nil, false)
	m.panelDisplay = formatPanel("", nil, false)
	m.resize()
	return m
}

func (m *model) ID() string		{ return m.id }
func (m *model) Title() string		{ return m.title }
func (m *model) ShortTitle() string	{ return "RegEx" }
func (m *model) Category() string	{ return "testers" }
func (m *model) Icon() string		{ return m.icon }
func (m *model) Description() string	{ return m.desc }

func (m *model) PaneStatusHints() []ui.StatusHint { return m.ws.StatusHints() }

func (m *model) Init() tea.Cmd	{ return nil }

func (m *model) headerLines() int {
	if m.height <= 6 || m.width < 24 {
		return 1
	}
	return 2
}
func (m *model) expressionPrefix() int {
	if m.headerLines() != 1 {
		return 0
	}
	return m.globalSwitchWidth() + 1
}

// globalSwitch 渲染全局匹配开关：●/○ 三态字形 pill，
// 与全库 Switch 控件同语言。
func (m *model) globalSwitch() string {
	return ui.Switch{On: m.global, OnLabel: "Global: on", OffLabel: "Global: off"}.View()
}

func (m *model) globalSwitchWidth() int {
	return lipgloss.Width(m.globalSwitch())
}

// globalHit 判定桌面布局中一次点击是否落在全局开关上。
func (m *model) globalHit(x int) bool {
	prefix := lipgloss.Width("Regex (.NET)  ")
	return x >= prefix && x < prefix+m.globalSwitchWidth()
}

// expressionBar 渲染顶部的全局开关行。窄/矮终端把表达式输入拼在同一行；
// 较高的非桌面布局里表达式输入独占第二行；桌面布局的表达式输入在窗格中。
func (m *model) expressionBar() string {
	if m.headerLines() == 1 {
		return m.globalSwitch() + " " + m.pattern.View()
	}
	return labelStyle.Render("Regex (.NET)") + "  " + m.globalSwitch()
}

// expressionRows 非桌面布局的头部行数：全局开关行 + （需要时）表达式输入行。
func (m *model) expressionRows() string {
	if m.headerLines() == 1 {
		return m.expressionBar()
	}
	return m.expressionBar() + "\n" + m.pattern.View()
}

func (m *model) desktopLayout() bool {
	return m.width >= 45 && m.height >= 13	// 表达式行 + 12 行内容区
}

func (m *model) updateOptions() {
	m.global = m.options[0]
	m.flags = regexp2.None
	if m.options[1] {
		m.flags |= regexp2.ECMAScript
	}
	if m.options[3] {
		m.flags |= regexp2.IgnoreCase
	}
	if m.options[4] {
		m.flags |= regexp2.IgnorePatternWhitespace
	}
	if m.options[5] {
		m.flags |= regexp2.Singleline
	}
	if m.options[6] {
		m.flags |= regexp2.Multiline
	}
	if m.options[7] {
		m.flags |= regexp2.RightToLeft
	}
}

func (m *model) layoutWidths() (left, right int) {
	left = m.width * 2 / 3
	return left, m.width - left
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(toolkit.InputClearedMsg); ok {
		return m, m.schedule()
	}
	switch msg := msg.(type) {
	case matchDue:
		if msg.revision != m.revision {
			return m, nil
		}
		if m.running {
			m.queued = true
			return m, nil
		}
		return m, m.startMatch(msg.revision)
	case matchDone:
		m.running = false
		if msg.revision == m.revision {
			m.applyResult(msg.result)
		}
		if m.queued {
			m.queued = false
			return m, m.startMatch(m.revision)
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
		return m, nil

	case toolkit.FileImportMsg:
		if m.focus == focusPattern {
			if err := toolkit.InsertFileText(&m.pattern, msg.Text); err != nil {
				m.ws.SetError(err)
				return m, nil
			}
		} else {
			m.focus = focusText
			m.syncFocus()
			m.ws.Update(msg)
			if m.ws.Err != nil {
				return m, nil
			}
		}
		return m, m.schedule()

	case tea.MouseMsg:
		if !m.desktopLayout() && m.focus == focusCheat && msg.Y > m.headerLines() {
			msg.Y -= m.headerLines() + 1
			m.cheat, _ = m.cheat.Update(msg)
			return m, nil
		}
		if m.desktopLayout() {
			if msg.Y < 2 {
				// 顶部全局开关行；第 1 行是窗格与表达式行之间的空行。
				if msg.Y == 0 && msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft && m.globalHit(msg.X) {
					m.options[0] = !m.options[0]
					m.updateOptions()
					return m, m.schedule()
				}
				return m, nil
			}
			local := msg
			local.Y -= 2
			if m.cheatExpanded {
				if local.Button == tea.MouseButtonWheelUp || local.Button == tea.MouseButtonWheelDown {
					m.cheat, _ = m.cheat.Update(local)
				} else if local.Action == tea.MouseActionPress && local.Button == tea.MouseButtonLeft {
					m.focus = focusCheat
					m.syncFocus()
				}
				return m, nil
			}
			// 页脚右端 Copy / < / >：Copy=输出区内容、< > 横向滚动。
			if local.Button == tea.MouseButtonLeft && local.Y == m.height-3 {
				if r := m.footerRect(); r.contains(local.X, 0) {
					btn := local
					btn.Y = 0
					btn.X -= r.x
					switch m.footerBar.Index(btn) {
					case 0:
						cmd := m.ws.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
						return m, cmd
					case 1:
						m.ws.Output.ScrollLeft(max(4, m.ws.Output.Width/2))
						return m, nil
					case 2:
						m.ws.Output.ScrollRight(max(4, m.ws.Output.Width/2))
						return m, nil
					}
				}
				return m, nil
			}
			// 窗格命中：渲染与命中共用 m.rects（禁两套手算）。
			for index, r := range m.rects {
				if !r.contains(local.X, local.Y) {
					continue
				}
				if local.Button == tea.MouseButtonWheelUp || local.Button == tea.MouseButtonWheelDown {
					delta := 3
					if local.Button == tea.MouseButtonWheelUp {
						delta = -3
					}
					switch index {
					case 0:
						m.pattern.Scroll(delta)
					case 1:
						m.ws.Input.Scroll(delta)
					case 2:
						m.cheat, _ = m.cheat.Update(local)
					case 3:
						m.ws.Output, _ = m.ws.Output.Update(local)
					}
					return m, nil
				}
				if local.Button != tea.MouseButtonLeft {
					return m, nil
				}
				m.focus = focusArea(index)
				m.syncFocus()
				actions := m.paneActions(index)
				if len(actions) > 0 && local.Y == r.y {
					buttons := toolkit.CompactActions(actions...)
					if start := r.x + r.width - 1 - lipgloss.Width(buttons); local.X >= start {
						l := local
						l.X -= start
						l.Y = 0
						if action := toolkit.CompactActionAt(l, actions...); action >= 0 {
							return m, m.paneAction(index, action)
						}
					}
				}
				if local.Y < r.y+1 {
					return m, nil
				}
				// 面板内容 1 列内边距，文本起点 = 边框内 2 列。
				switch index {
				case 0:
					return m, m.pattern.PlaceCursor(local.X-r.x-2, 0)
				case 1:
					return m, m.ws.Input.PlaceCursor(local.X-r.x-2, local.Y-r.y-1)
				}
				return m, nil
			}
			return m, nil
		}
		if msg.Action == tea.MouseActionPress && msg.Y == m.headerLines()-1 && msg.X >= m.expressionPrefix() && (msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown) {
			delta := 3
			if msg.Button == tea.MouseButtonWheelUp {
				delta = -3
			}
			m.pattern.Scroll(delta)
			return m, nil
		}
		// 全局开关：headerLines==1 时开关在行首，
		// 否则跟在 "Regex (.NET)" 标签后。
		switchX := 0
		if m.headerLines() != 1 {
			switchX = lipgloss.Width("Regex (.NET)  ")
		}
		if m.globalSwitchWidth() > 0 && msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft && msg.Y == 0 &&
			msg.X >= switchX && msg.X < switchX+m.globalSwitchWidth() {
			m.options[0] = !m.options[0]
			m.updateOptions()
			return m, m.schedule()
		}
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft && msg.Y >= 0 && msg.Y < m.headerLines() {
			m.focus = focusPattern
			m.syncFocus()
			if msg.Y == m.headerLines()-1 && msg.X >= m.expressionPrefix() {
				return m, m.pattern.PlaceCursor(msg.X-m.expressionPrefix(), 0)
			}
			return m, nil
		}
		if msg.Y >= m.headerLines() {
			cmd := m.ws.UpdateBelow(msg, m.headerLines())
			if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft && msg.Y < m.height-1 {
				m.focus = focusText
				if m.ws.Focus == toolkit.FocusOutput {
					m.focus = focusOutput
				}
				m.syncFocus()
			}
			return m, cmd
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "shift+tab":
			// Focus cycles through the expression, text, cheat sheet, and matches.
			if msg.String() == "shift+tab" {
				m.focus = (m.focus + 3) % 4
			} else {
				m.focus = (m.focus + 1) % 4
			}
			m.syncFocus()
			return m, nil

		case "alt+e":
			if m.focus == focusCheat && m.desktopLayout() {
				m.cheat.Width = max(1, m.width-2)
				m.cheat.Height = max(1, m.height-2)
				m.focus = focusCheat
				m.syncFocus()
				m.cheatExpanded = !m.cheatExpanded
				m.resize()
			}
			return m, nil
		case "ctrl+g":
			// findall/全局匹配开关。
			m.options[0] = !m.options[0]
			m.updateOptions()
			return m, m.schedule()

		case "ctrl+y":
			m.ws.Update(msg)
			return m, nil
		}

		// 非公共按键：交给焦点所在区域的组件处理，随后即时重算。
		beforePattern, beforeText := m.pattern.Value(), m.ws.Input.Value()
		var cmd tea.Cmd
		switch m.focus {
		case focusPattern:
			m.pattern, cmd = m.pattern.Update(msg)
		case focusText:
			cmd = m.ws.Update(msg)
		case focusOutput:
			cmd = m.ws.Update(msg)
		case focusCheat:
			m.cheat, cmd = m.cheat.Update(msg)
		}
		if beforePattern != m.pattern.Value() || beforeText != m.ws.Input.Value() {
			cmd = tea.Batch(cmd, m.schedule())
		}
		return m, cmd
	}
	beforePattern, beforeText := m.pattern.Value(), m.ws.Input.Value()
	var cmd tea.Cmd
	if m.focus == focusPattern {
		m.pattern, cmd = m.pattern.Update(msg)
	} else {
		cmd = m.ws.Update(msg)
	}
	if beforePattern != m.pattern.Value() || beforeText != m.ws.Input.Value() {
		cmd = tea.Batch(cmd, m.schedule())
	}
	return m, cmd
}

// paneActions 返回桌面窗格标题行的按钮组。
func (m *model) paneActions(index int) []string {
	switch index {
	case 0:
		return []string{"Paste", "Clear", "Copy"}
	case 1:
		return []string{"Paste", "Open", "Clear", "Copy"}
	case 3:
		return []string{"Copy"}
	}
	return nil
}

// paneAction 分发窗格标题行按钮（渲染与命中共用 paneActions 序列）。
func (m *model) paneAction(index, action int) tea.Cmd {
	switch index {
	case 0:
		switch action {
		case 0:
			updated, cmd := m.pattern.Update(tea.KeyMsg{Type: tea.KeyCtrlV})
			m.pattern = updated
			return tea.Batch(cmd, m.schedule())
		case 1:
			m.pattern.SetValue("")
			return m.schedule()
		case 2:
			return m.copyText(m.pattern.Value())
		}
	case 1:
		switch action {
		case 0:
			updated, cmd := m.ws.Input.Update(tea.KeyMsg{Type: tea.KeyCtrlV})
			m.ws.Input = updated
			return tea.Batch(cmd, m.schedule())
		case 1:
			return func() tea.Msg { return toolkit.OpenFileRequestMsg{} }
		case 2:
			m.ws.Input.SetValue("")
			m.ws.SetError(nil)
			return m.schedule()
		case 3:
			return m.copyText(m.ws.Input.Value())
		}
	case 3:
		cmd := m.ws.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
		return cmd
	}
	return nil
}

// copyText 把文本写入剪贴板并给出成功/失败状态。
func (m *model) copyText(text string) tea.Cmd {
	if err := clipboard.WriteAll(text); err != nil {
		m.ws.SetError(fmt.Errorf("could not copy: %w", err))
		return nil
	}
	m.ws.SetError(nil)
	m.ws.Copied = true
	return nil
}

// footerRect 返回页脚右端按钮组的矩形（模型局部坐标，最后一行）。
func (m *model) footerRect() paneRect {
	total := 0
	for i, b := range m.footerBar.Buttons {
		if i > 0 {
			total += max(0, m.footerBar.Gap)
		}
		total += b.Width()
	}
	return paneRect{x: max(0, m.width-1-total), y: 0, width: total, height: 1}
}

// footerView 渲染 canonical 底行：ui.StatusBar——左 canonical
// 句式（速查表可展开时附 alt+E 后缀），右端 Copy / < / >；错误与已复制
// 以 Toast 呈现（与 workspace footer 同语言）。
func (m *model) footerView() string {
	m.footer.SetWidth(max(1, m.width))
	left := "click to edit · scroll to browse"
	switch {
	case m.cheatExpanded:
		left += " · alt+E Restore"
	case m.focus == focusCheat && m.desktopLayout():
		left += " · alt+E Expand"
	}
	m.footer.SetLeftTokens([]ui.StatusToken{{Label: left}})
	m.footer.SetHints()
	switch {
	case m.ws.Err != nil:
		m.footer.SetToast(ui.Toast{Level: ui.LevelError, Message: "Error: " + m.ws.Err.Error()})
	case m.ws.Copied:
		m.footer.SetToast(ui.Toast{Level: ui.LevelSuccess, Message: "Copied to clipboard"})
	default:
		m.footer.ClearToast()
		m.footer.SetRight(m.footerBar.View())
	}
	return m.footer.View()
}

// pane 渲染带边框窗格：标题与按钮嵌在顶边框（与 jsonpath/
// ui.Panel 语言一致），内容左右各 1 列内边距。
func (m *model) pane(title, content string, width, height int, focused bool, actions []string) string {
	innerWidth, innerHeight := max(1, width-2), max(1, height-2)
	textWidth := max(1, innerWidth-2)
	lines := strings.Split(content, "\n")
	if len(lines) > innerHeight {
		lines = lines[:innerHeight]
	}
	for len(lines) < innerHeight {
		lines = append(lines, "")
	}
	for i := range lines {
		w := lipgloss.Width(lines[i])
		if w > textWidth {
			lines[i] = ansi.Truncate(lines[i], textWidth, "")
			w = textWidth
		}
		lines[i] = " " + lines[i] + strings.Repeat(" ", textWidth-w)
	}
	color := theme.Border
	labelColor := theme.Muted
	if focused {
		color = theme.Accent
		labelColor = theme.Text
	}
	pane := lipgloss.NewStyle().Width(innerWidth).Height(innerHeight).
		Border(lipgloss.RoundedBorder()).BorderForeground(color).
		Render(strings.Join(lines, "\n"))
	out := strings.Split(pane, "\n")
	w := lipgloss.Width(out[0])
	label := " " + title + " "
	out[0] = ansi.Cut(out[0], 0, 2) + lipgloss.NewStyle().Foreground(labelColor).Bold(true).Render(label) + ansi.Cut(out[0], 2+lipgloss.Width(label), w)
	if len(actions) > 0 {
		buttons := toolkit.CompactActions(actions...)
		if start := w - 1 - lipgloss.Width(buttons); start >= 2+lipgloss.Width(label) {
			out[0] = ansi.Cut(out[0], 0, start) + buttons + ansi.Cut(out[0], start+lipgloss.Width(buttons), w)
		}
	}
	return strings.Join(out, "\n")
}

func (m *model) View() string {
	if m.desktopLayout() {
		if m.cheatExpanded {
			cheatPane := m.pane("Cheat sheet", m.cheat.View(), m.width, m.height-1, true, nil)
			return m.expressionBar() + "\n" + cheatPane + "\n" + m.footerView()
		}
		patternPane := m.pane("Regular expression", m.pattern.View(), m.rects[0].width, m.rects[0].height, m.focus == focusPattern, m.paneActions(0))
		textPane := m.pane("Text", m.ws.Input.View(), m.rects[1].width, m.rects[1].height, m.focus == focusText, m.paneActions(1))
		cheatPane := m.pane("Cheat sheet", m.cheat.View(), m.rects[2].width, m.rects[2].height, m.focus == focusCheat, nil)
		outputPane := m.pane("Match information", m.ws.Output.View(), m.rects[3].width, m.rects[3].height, m.focus == focusOutput, m.paneActions(3))
		// 双栏缝 Space2、纵向缝 1 空行。
		leftPane := lipgloss.JoinVertical(lipgloss.Left, patternPane, "", textPane)
		rightPane := lipgloss.JoinVertical(lipgloss.Left, cheatPane, "", outputPane)
		body := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, strings.Repeat(" ", ui.Space2), rightPane)
		return m.expressionBar() + "\n\n" + body + "\n" + m.footerView()
	}
	if m.focus == focusCheat {
		return m.expressionRows() + "\n" + labelStyle.Render("Cheat sheet") + "\n" + m.cheat.View()
	}
	return m.expressionRows() + "\n" + m.ws.View()
}

// syncFocus 让各输入组件的焦点状态与当前焦点区域一致，
// 同时把公共工作区的焦点同步到文本输入或输出。
func (m *model) syncFocus() {
	m.pattern.Blur()
	m.ws.Input.Blur()
	switch m.focus {
	case focusPattern:
		m.pattern.FocusPreservingScroll()
		m.ws.Focus = toolkit.FocusInput	// 备用：保持工作区指向输入
	case focusText:
		m.ws.Input.FocusPreservingScroll()
		m.ws.Focus = toolkit.FocusInput
	case focusOutput:
		m.ws.Focus = toolkit.FocusOutput
	case focusCheat:
		m.ws.Focus = toolkit.FocusOutput
	}
	if m.focus != focusOutput {
		m.ws.Copied = false
	}
}

func (m *model) resize() {
	if m.desktopLayout() {
		if m.cheatExpanded {
			m.rects = [4]paneRect{}
			m.rects[2] = paneRect{0, 0, m.width, m.height - 1}
			m.cheat.Width = max(1, m.width-4)
			m.cheat.Height = max(1, m.height-3)
			m.cheat.SetContent(formatCheatSheetAtWidth(m.cheat.Width))
			return
		}
		contentHeight := m.height - 3	// 表达式行 1 行 + 与窗格之间的空行 1 行 + 页脚 1 行
		patternHeight := 4
		// 双栏缝 Space2：左右列中缝 2 列；纵向窗格间留 1 空行。
		// 面板内容 1 列内边距：文本区 = 外宽-4。
		leftWidth := (m.width - ui.Space2) * 2 / 3
		rightWidth := m.width - leftWidth - ui.Space2
		topHeight := rightTopHeight(contentHeight)
		m.rects = [4]paneRect{
			{0, 0, leftWidth, patternHeight},
			{0, patternHeight + 1, leftWidth, contentHeight - patternHeight - 1},
			{leftWidth + ui.Space2, 0, rightWidth, topHeight},
			{leftWidth + ui.Space2, topHeight + 1, rightWidth, contentHeight - topHeight - 1},
		}
		m.pattern.SetWidth(max(1, leftWidth-4))
		m.pattern.SetHeight(max(1, patternHeight-2))
		m.ws.Input.SetWidth(max(1, leftWidth-4))
		m.ws.Input.SetHeight(max(1, contentHeight-patternHeight-3))
		m.ws.Output.Width = max(1, rightWidth-4)
		m.ws.Output.Height = max(1, contentHeight-topHeight-3)
		m.cheat.Width = max(1, rightWidth-4)
		m.cheat.Height = max(1, topHeight-2)
		m.cheat.SetContent(formatCheatSheetAtWidth(m.cheat.Width))
		m.cheat.SetYOffset(0)
		m.syncOutputPane()
		return
	}
	m.pattern.SetWidth(max(2, m.width-m.expressionPrefix()))
	m.pattern.SetHeight(1)	// 非桌面布局：表达式输入恒 1 行（桌面布局为 2 行内边距窗格）
	// 剩余高度交给工作区。
	m.ws.Update(tea.WindowSizeMsg{
		Width:	m.width,
		Height:	max(1, m.height-m.headerLines()),
	})
	m.cheat.Width = max(1, m.width)
	m.cheat.Height = max(1, m.height-m.headerLines()-1)
	m.cheat.SetContent(formatCheatSheetAtWidth(m.cheat.Width))
	m.cheat.SetYOffset(0)
}

func rightTopHeight(contentHeight int) int	{ return max(5, contentHeight/2) }

func (m *model) syncOutputPane() {
	if m.desktopLayout() {
		m.ws.SetOutputDisplay(m.ws.OutputText(), m.matchesDisplay)
		return
	}
	m.ws.SetOutputDisplay(m.ws.OutputText(), m.panelDisplay)
}

// recompute is retained for direct synchronous use in tests.
func (m *model) recompute() {
	m.applyResult(matchRegex(m.pattern.Value(), m.ws.Input.Value(), m.global, m.flags))
}

func (m *model) applyResult(r matchResult) {
	m.ws.SetError(r.err)
	m.matchesDisplay = r.matchesDisplay
	m.panelDisplay = r.display
	if r.err != nil {
		m.matchesDisplay = ""
		m.panelDisplay = formatPanel("", nil, false)
		m.ws.SetOutputDisplay("", "")
		return
	}
	if m.matchesDisplay == "" {
		m.matchesDisplay = formatMatchPanel(r.text, nil, false)
	}
	m.ws.SetOutputDisplay(r.text, r.display)
	m.syncOutputPane()
}

type matchDue struct{ revision uint64 }
type matchDone struct {
	revision	uint64
	result		matchResult
}

func (m *model) schedule() tea.Cmd {
	m.revision++
	m.queued = false
	m.ws.SetError(nil)
	m.ws.SetOutput("Matching…")
	rev := m.revision
	owner := m.owner
	if owner == nil {
		owner = m
	}
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg {
		return toolkit.BackgroundToolMsg{Tool: owner, Message: matchDue{revision: rev}}
	})
}
func (m *model) startMatch(rev uint64) tea.Cmd {
	m.running = true
	pattern, text, global, flags := m.pattern.Value(), m.ws.Input.Value(), m.global, m.flags
	owner := m.owner
	if owner == nil {
		owner = m
	}
	return func() tea.Msg {
		return toolkit.BackgroundToolMsg{Tool: owner, Message: matchDone{revision: rev, result: matchRegex(pattern, text, global, flags)}}
	}
}

func init() {
	toolkit.Register(newRegexTool())
}
