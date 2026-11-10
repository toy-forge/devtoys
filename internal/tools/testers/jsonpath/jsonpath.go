// Package jsonpath 实现 JSONPath 查询测试器（基于 ojg）：
// 输入 JSON 文档与 JSONPath 表达式，输出区实时列出全部匹配结果。
package jsonpath

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/x/ansi"
	"github.com/toy-forge/devtoys/internal/theme"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ohler55/ojg/jp"
	"github.com/ohler55/ojg/oj"

	"github.com/toy-forge/devtoys/internal/toolkit"
	"github.com/toy-forge/devtoys/internal/ui"
)

// focusArea 表示工具内部焦点所在的区域（表达式 / JSON 输入 / 输出）。
type focusArea int

const (
	focusExpr	focusArea	= iota
	focusJSON
	focusOutput
	focusCheat
)

var labelStyle = lipgloss.NewStyle().
	Foreground(theme.Muted)

var paneBorder = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(theme.Border)
var paneBorderFocused = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(theme.Accent)
var paneTitle = lipgloss.NewStyle().Foreground(theme.BlueBright).Bold(true)

type model struct {
	id, title, icon, desc	string

	ws		toolkit.Workspace	// 公共双栏工作区：JSON 输入 + 输出
	expr		textinput.Model		// JSONPath 表达式输入（单行）
	focus		focusArea
	cheat		viewport.Model
	rects		[4]queryRect
	expanded	bool
	expandedArea	focusArea

	// canonical 页脚：ui.StatusBar 左句式 + 右端 Copy / < / >，
	// 渲染与命中共用 footerRect。
	footer		*ui.StatusBar
	footerBar	*ui.Toolbar

	width	int
	height	int
}

type queryRect struct{ x, y, width, height int }

var jsonPathCheatRows = [][2]string{
	{"$", "The root object or array."},
	{"@", "Used for filter expressions. Refers to the current node for further processing."},
	{"object.property", "Dot-notated child"},
	{"['object'].['property']", "Bracket-notated child or children"},
	{"..property", "Performs a deep scan for the specified property in all available objects. Always returns a list, even for a single match."},
	{"*", "Wildcard. Selects all elements in an object or array, regardless of their names or indexes."},
	{"[n]", "Selects the n-th element from an array. Indexes start from 0."},
	{"[n1,n2]", "Selects n1 and n2 array items. Returns a list."},
	{"[start:end:step]", "Array slice operator"},
	{"?(expression)", "Selects all elements in an object or array that match the specified boolean expression. Returns a list."},
	{"(expression)", "Script expression."},
}

// New 创建 JSONPath 测试器模型。
func New() *model {
	ws := toolkit.NewWorkspace(
		"Enter a JSON document…",
		"",
	)
	ws.SetPaneTitles("JSON", "Test result")
	ws.SetOutputExtendable(true)
	expr := textinput.New()
	expr.Placeholder = `Example: $.store.book[*].author`
	expr.Focus()
	// 初始焦点在表达式输入框，取消 NewWorkspace 默认给主输入区的焦点，
	// 避免两个输入框同时持焦、显示双光标。
	ws.Input.Blur()

	m := &model{
		id:	"testers.jsonpath",
		title:	"JSONPath Tester",
		icon:	"󰘦",
		desc:	"Query JSON with JSONPath",
		ws:	ws,
		expr:	expr,
		focus:	focusExpr,
		footer:	ui.NewStatusBar(),
		footerBar: ui.NewToolbar(
			ui.Button{Label: "Copy"},
			ui.Button{Label: "<"},
			ui.Button{Label: ">"},
		),
		width:	80,
		height:	24,
	}
	m.cheat = viewport.New(1, 1)
	m.cheat.SetContent(jsonPathCheatSheet(80))
	m.resize()
	return m
}

func (m *model) ID() string		{ return m.id }
func (m *model) Title() string		{ return m.title }
func (m *model) ShortTitle() string	{ return "JSONPath" }
func (m *model) Category() string	{ return "testers" }
func (m *model) Icon() string		{ return m.icon }
func (m *model) Description() string	{ return m.desc }

func (m *model) PaneStatusHints() []ui.StatusHint { return m.ws.StatusHints() }
func (*model) AcceptsFiles() bool	{ return true }

func (m *model) Init() tea.Cmd	{ return nil }

func (m *model) headerLines() int {
	if m.height <= 6 {
		return 1
	}
	return 2
}

// exprLabelWidth 是单行布局中 "Expression" 标签列的宽度。
const exprLabelWidth = 10

func (m *model) expressionPrefix() int {
	if m.headerLines() != 1 {
		return 0
	}
	return exprLabelWidth + ui.Space2
}

// expressionField 渲染表达式输入框：单行布局为"标签 + 定宽字段"（ui.FormRow），
// 两行布局为整行定宽字段。字段底色标明可编辑。
func (m *model) expressionField() string {
	focused := m.focus == focusExpr
	if m.headerLines() == 1 {
		return ui.FormRow("Expression", exprLabelWidth, ui.FieldView(&m.expr, max(1, m.width-m.expressionPrefix()), focused), m.width)
	}
	return ui.FieldView(&m.expr, max(1, m.width), focused)
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(toolkit.InputClearedMsg); ok {
		m.recompute()
		return m, nil
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
		return m, nil

	case toolkit.FileImportMsg:
		if m.focus == focusExpr {
			if err := toolkit.InsertLine(&m.expr, msg.Text); err != nil {
				m.ws.SetError(err)
				return m, nil
			}
		} else {
			m.focus = focusJSON
			m.syncFocus()
			m.ws.Update(msg)
			if m.ws.Err != nil {
				return m, nil
			}
		}
		m.recompute()
		return m, nil

	case tea.MouseMsg:
		if m.wideLayout() {
			return m, m.updateWideMouse(msg)
		}
		if m.focus == focusCheat && msg.Y > 0 {
			msg.Y--
			m.cheat, _ = m.cheat.Update(msg)
			return m, nil
		}
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft && msg.Y >= 0 && msg.Y < m.headerLines() {
			m.focus = focusExpr
			m.syncFocus()
			if msg.Y == m.headerLines()-1 && msg.X >= m.expressionPrefix() {
				// 字段内容从 expressionPrefix 之后的 Space1 列开始。
				m.expr.SetCursor(msg.X - m.expressionPrefix() - ui.Space1)
				return m, nil
			}
			return m, nil
		}
		if msg.Y >= m.headerLines() {
			cmd := m.ws.UpdateBelow(msg, m.headerLines())
			if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft && msg.Y < m.height-1 {
				m.focus = focusJSON
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
		case "alt+e":
			if m.wideLayout() && (m.focus == focusOutput || m.focus == focusCheat) {
				m.expanded = !m.expanded
				if m.expanded {
					m.expandedArea = m.focus
				}
				m.resize()
				return m, nil
			}
		case "tab", "shift+tab":
			count := int(focusCheat) + 1
			if msg.String() == "shift+tab" {
				m.focus = (m.focus - 1 + focusArea(count)) % focusArea(count)
			} else {
				m.focus = (m.focus + 1) % focusArea(count)
			}
			m.syncFocus()
			return m, nil

		case "ctrl+y":
			m.copyFocused()
			return m, nil
		}

		// 非公共按键：交给焦点所在区域的组件处理，随后即时重算。
		var cmd tea.Cmd
		switch m.focus {
		case focusExpr:
			m.expr, cmd = m.expr.Update(msg)
			m.recompute()
		case focusJSON:
			cmd = m.ws.Update(msg)
			m.recompute()
		case focusOutput:
			cmd = m.ws.Update(msg)
		case focusCheat:
			m.cheat, cmd = m.cheat.Update(msg)
		}
		return m, cmd
	}
	return m, nil
}

func (m *model) View() string {
	if m.wideLayout() {
		return m.wideView()
	}
	if m.focus == focusCheat {
		return labelStyle.Render("Cheat sheet") + "\n" + m.cheat.View()
	}
	if m.headerLines() == 1 {
		return m.expressionField() + "\n" + m.ws.View()
	}
	header := labelStyle.Render("JSONPath expression") + "\n" +
		m.expressionField() + "\n"
	return header + m.ws.View()
}

// syncFocus 让各输入组件的焦点状态与当前焦点区域一致，
// 同时把公共工作区的焦点同步到 JSON 输入或输出。
func (m *model) syncFocus() {
	wasExpanded := m.expanded
	if m.expanded && m.focus != m.expandedArea {
		m.expanded = false
	}
	m.expr.Blur()
	m.ws.Input.Blur()
	switch m.focus {
	case focusExpr:
		m.expr.Focus()
		m.ws.Focus = toolkit.FocusInput
	case focusJSON:
		m.ws.Input.FocusPreservingScroll()
		m.ws.Focus = toolkit.FocusInput
	case focusOutput:
		m.ws.Focus = toolkit.FocusOutput
	case focusCheat:
		m.ws.Focus = toolkit.FocusOutput
	}
	if m.focus != focusOutput && m.focus != focusCheat {
		m.ws.Copied = false
	}
	if wasExpanded && !m.expanded {
		m.resize()
	}
}

func (m *model) resize() {
	if !m.wideLayout() {
		m.expanded = false
	}
	if m.wideLayout() {
		bodyHeight := max(1, m.height-1)
		if m.expanded {
			m.rects = [4]queryRect{}
			m.rects[m.expandedArea] = queryRect{0, 0, m.width, bodyHeight}
			switch m.expandedArea {
			case focusOutput:
				m.ws.Output.Width = max(1, m.width-4)
				m.ws.Output.Height = max(1, bodyHeight-2)
				m.ws.Output.SetYOffset(m.ws.Output.YOffset)
			case focusCheat:
				m.cheat.Width = max(1, m.width-4)
				m.cheat.Height = max(1, bodyHeight-2)
				m.cheat.SetContent(jsonPathCheatSheet(m.cheat.Width))
				m.cheat.SetYOffset(m.cheat.YOffset)
			}
			return
		}
		// 双栏缝 Space2：左右列中缝 2 列；纵向表达式/JSON 与
		// 结果/速查表之间留 1 空行。窗格内容 1 列内边距
		//（pane 内文本区 = 外宽-4）。
		leftWidth := (m.width - ui.Space2) / 2
		rightWidth := m.width - leftWidth - ui.Space2
		// 左列：表达式在上、JSON 在下；右列：结果在上、速查表在下。
		expressionHeight := 3
		topHeight := max(expressionHeight+1, (bodyHeight-1)/2)
		topHeight = min(topHeight, bodyHeight-4)
		jsonHeight := bodyHeight - expressionHeight - 1
		cheatHeight := bodyHeight - topHeight - 1
		m.rects = [4]queryRect{
			{0, 0, leftWidth, expressionHeight},
			{0, expressionHeight + 1, leftWidth, jsonHeight},
			{leftWidth + ui.Space2, 0, rightWidth, topHeight},
			{leftWidth + ui.Space2, topHeight + 1, rightWidth, cheatHeight},
		}
		m.expr.Width = max(1, leftWidth-4-2*ui.Space1)
		m.ws.Input.SetWidth(max(1, leftWidth-4))
		m.ws.Input.SetHeight(max(1, jsonHeight-2))
		m.ws.Output.Width = max(1, rightWidth-4)
		m.ws.Output.Height = max(1, topHeight-2)
		m.cheat.Width = max(1, rightWidth-4)
		m.cheat.Height = max(1, cheatHeight-2)
		m.cheat.SetContent(jsonPathCheatSheet(m.cheat.Width))
		m.ws.Output.SetYOffset(m.ws.Output.YOffset)
		m.cheat.SetYOffset(m.cheat.YOffset)
		return
	}
	m.expr.Width = max(1, m.width-m.expressionPrefix()-2*ui.Space1)
	// 剩余高度交给工作区。
	m.ws.Update(tea.WindowSizeMsg{
		Width:	m.width,
		Height:	max(1, m.height-m.headerLines()),
	})
	m.cheat.Width = max(1, m.width)
	m.cheat.Height = max(1, m.height-m.headerLines()-1)
	m.cheat.SetContent(jsonPathCheatSheet(m.cheat.Width))
}

func (m *model) wideLayout() bool	{ return m.width >= 46 && m.height >= 14 }

func (m *model) wideView() string {
	if m.expanded {
		rect := m.rects[m.expandedArea]
		title, content := "", ""
		switch m.expandedArea {
		case focusOutput:
			title, content = "Test result", m.ws.Output.View()
		case focusCheat:
			title, content = "Cheat sheet", m.cheat.View()
		}
		body := m.pane(rect, title, content, true, nil)
		return body + "\n" + m.footerView("Restore")
	}
	expr, json, output, cheat := m.rects[0], m.rects[1], m.rects[2], m.rects[3]
	exprPane := m.pane(expr, "JSONPath", ui.FieldView(&m.expr, max(1, expr.width-4), m.focus == focusExpr), m.focus == focusExpr, []string{"Paste", "Clear", "Copy"})
	jsonPane := m.pane(json, "JSON", m.ws.Input.View(), m.focus == focusJSON, []string{"Paste", "Open", "Clear", "Copy"})
	outputPane := m.pane(output, "Test result", m.ws.Output.View(), m.focus == focusOutput, []string{"Copy"})
	cheatPane := m.pane(cheat, "Cheat sheet", m.cheat.View(), m.focus == focusCheat, nil)
	left := lipgloss.JoinVertical(lipgloss.Left, exprPane, "", jsonPane)
	right := lipgloss.JoinVertical(lipgloss.Left, outputPane, "", cheatPane)
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", ui.Space2), right)
	return body + "\n" + m.footerView("Expand")
}

// footerView 渲染 canonical 底行：ui.StatusBar——左 canonical
// 句式 + expand 后缀，右端 Copy / < / >（Copy=测试结果、< > 横向滚动）；
// 错误以 Toast 呈现（与 workspace footer 同语言）。
func (m *model) footerView(expandLabel string) string {
	m.footer.SetWidth(max(1, m.width))
	m.footer.SetLeftTokens([]ui.StatusToken{{Label: "click to edit · scroll to browse · alt+E " + expandLabel}})
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

// footerRect 返回页脚右端按钮组的矩形（页面局部坐标，最后一行）。
func (m *model) footerRect() queryRect {
	total := 0
	for i, b := range m.footerBar.Buttons {
		if i > 0 {
			total += max(0, m.footerBar.Gap)
		}
		total += b.Width()
	}
	x := max(0, m.width-1-total)
	return queryRect{x: x, y: m.height - 1, width: total, height: 1}
}

// pane 渲染带边框窗格：标题与按钮嵌在顶边框上，内容占满其余行；
// 内容左右各 1 列内边距，文本区 = 内宽-2。
func (m *model) pane(rect queryRect, title, content string, focused bool, actions []string) string {
	innerWidth, innerHeight := max(1, rect.width-2), max(1, rect.height-2)
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
		// 左 1 列内边距 + 右缘补齐到内宽（右内边距落在补齐空格里）。
		lines[i] = " " + lines[i] + strings.Repeat(" ", textWidth-w)
	}
	style := paneBorder
	if focused {
		style = paneBorderFocused
	}
	pane := style.Width(innerWidth).Height(innerHeight).Render(strings.Join(lines, "\n"))
	out := strings.Split(pane, "\n")
	width := lipgloss.Width(out[0])
	color := theme.Border
	if focused {
		color = theme.BlueBright
	}
	label := " " + title + " "
	out[0] = ansi.Cut(out[0], 0, 2) + lipgloss.NewStyle().Foreground(color).Bold(true).Render(label) + ansi.Cut(out[0], 2+lipgloss.Width(label), width)
	if len(actions) > 0 {
		buttons := toolkit.CompactActions(actions...)
		if start := width - 1 - lipgloss.Width(buttons); start >= 2+lipgloss.Width(label) {
			out[0] = ansi.Cut(out[0], 0, start) + buttons + ansi.Cut(out[0], start+lipgloss.Width(buttons), width)
		}
	}
	return strings.Join(out, "\n")
}

func (m *model) updateWideMouse(mouse tea.MouseMsg) tea.Cmd {
	if mouse.Action != tea.MouseActionPress {
		return nil
	}
	// 页脚右端 Copy / < / >：Copy=测试结果，< > 横向滚动结果区。
	if mouse.Button == tea.MouseButtonLeft && mouse.Y == m.height-1 {
		if r := m.footerRect(); r.width > 0 && mouse.X >= r.x && mouse.X < r.x+r.width {
			local := mouse
			local.Y = 0
			local.X -= r.x
			switch m.footerBar.Index(local) {
			case 0:
				m.copyResult()
			case 1:
				m.ws.Output.ScrollLeft(max(4, m.ws.Output.Width/2))
			case 2:
				m.ws.Output.ScrollRight(max(4, m.ws.Output.Width/2))
			}
		}
		return nil
	}
	for index, rect := range m.rects {
		if mouse.X < rect.x || mouse.X >= rect.x+rect.width || mouse.Y < rect.y || mouse.Y >= rect.y+rect.height {
			continue
		}
		if mouse.Button == tea.MouseButtonWheelUp || mouse.Button == tea.MouseButtonWheelDown {
			delta := 3
			if mouse.Button == tea.MouseButtonWheelUp {
				delta = -3
			}
			switch index {
			case 0:
				// 单行表达式没有纵向滚动。
			case 1:
				m.ws.Input.Scroll(delta)
			case 2:
				m.ws.Output, _ = m.ws.Output.Update(mouse)
			case 3:
				m.cheat, _ = m.cheat.Update(mouse)
			}
			return nil
		}
		if mouse.Button != tea.MouseButtonLeft {
			return nil
		}
		areas := []focusArea{focusExpr, focusJSON, focusOutput, focusCheat}
		m.focus = areas[index]
		m.syncFocus()
		actions := [][]string{{"Paste", "Clear", "Copy"}, {"Paste", "Open", "Clear", "Copy"}, {"Copy"}, nil}[index]
		if len(actions) > 0 && mouse.Y == rect.y {
			buttons := toolkit.CompactActions(actions...)
			start := rect.x + rect.width - 1 - lipgloss.Width(buttons)
			local := mouse
			local.X -= start
			local.Y = 0
			if action := toolkit.CompactActionAt(local, actions...); action >= 0 {
				return m.handlePaneAction(index, action)
			}
		}
		if mouse.Y < rect.y+1 {
			return nil
		}
		switch index {
		case 0:
			// 面板内容 1 列内边距，文本起点 = 边框内 2 列；字段自身再留 Space1。
			m.expr.SetCursor(mouse.X - rect.x - 2 - ui.Space1)
			return nil
		case 1:
			return m.ws.Input.PlaceCursor(mouse.X-rect.x-2, mouse.Y-rect.y-1)
		}
		return nil
	}
	return nil
}

// copyResult 把测试结果写入剪贴板（页脚 Copy 按钮的目标）。
func (m *model) copyResult() {
	if err := clipboard.WriteAll(m.ws.OutputText()); err != nil {
		m.ws.SetError(fmt.Errorf("could not copy: %w", err))
		m.ws.Copied = false
	} else {
		m.ws.SetError(nil)
		m.ws.Copied = true
	}
}

func (m *model) handlePaneAction(pane, action int) tea.Cmd {
	switch pane {
	case 0:	// 表达式
		switch action {
		case 0:
			// 剪贴板内容以粘贴按键回灌给单行输入框（见 toolkit.PasteLineCmd）。
			return toolkit.PasteLineCmd()
		case 1:
			m.expr.SetValue("")
			m.recompute()
		case 2:
			m.copyFocused()
		}
	case 1:	// JSON
		switch action {
		case 0:
			updated, cmd := m.ws.Input.Update(tea.KeyMsg{Type: tea.KeyCtrlV})
			m.ws.Input = updated
			return cmd
		case 1:
			return func() tea.Msg { return toolkit.OpenFileRequestMsg{} }
		case 2:
			m.ws.Input.SetValue("")
			m.ws.SetError(nil)
			m.recompute()
		case 3:
			m.copyFocused()
		}
	case 2:
		m.copyFocused()
	}
	return nil
}

func (m *model) copyFocused() {
	text := ""
	switch m.focus {
	case focusJSON:
		text = m.ws.Input.Value()
	case focusExpr:
		text = m.expr.Value()
	case focusOutput:
		text = m.ws.OutputText()
	case focusCheat:
		text = jsonPathCheatSheet(m.cheat.Width)
	}
	if err := clipboard.WriteAll(text); err != nil {
		m.ws.SetError(fmt.Errorf("could not copy: %w", err))
		m.ws.Copied = false
	} else {
		m.ws.SetError(nil)
		m.ws.Copied = true
	}
}

// jsonPathCheatSheet 渲染速查表：Syntax | Description 两列（与 regex 速查表同一列序与间距）。
func jsonPathCheatSheet(width int) string {
	syntaxWidth := min(24, max(8, width/3))
	table := ui.Table{Gap: ui.Space2, Columns: []ui.Column{
		{Title: "Syntax", Width: syntaxWidth},
		{Title: "Description", Width: max(1, width-syntaxWidth-ui.Space2)},
	}}
	rows := make([]string, 0, len(jsonPathCheatRows)+1)
	rows = append(rows, table.Header())
	for _, row := range jsonPathCheatRows {
		syntax := lipgloss.NewStyle().Foreground(theme.BlueBright).Bold(true).Render(row[0])
		_, line := table.Row(syntax, row[1])
		rows = append(rows, line)
	}
	return strings.Join(rows, "\n")
}

// recompute 依据当前表达式与 JSON 文档重算查询结果。
func (m *model) recompute() {
	src := strings.TrimSpace(m.expr.Value())
	doc := m.ws.Input.Value()

	m.ws.SetError(nil)
	if src == "" {
		m.setOutput("No results found.")
		return
	}

	expr, err := jp.ParseString(src)
	if err != nil {
		m.ws.SetError(fmt.Errorf("invalid JSONPath expression: %v", err))
		return
	}
	if strings.TrimSpace(doc) == "" {
		m.setOutput("No results found.")
		return
	}

	root, err := oj.ParseString(doc)
	if err != nil {
		m.ws.SetError(fmt.Errorf("JSON parse failed: %v", err))
		return
	}

	results := expr.Get(root)
	var b strings.Builder
	if len(results) == 0 {
		b.WriteString("No results found.")
		m.setOutput(b.String())
		return
	}
	fmt.Fprintf(&b, "%d results:\n", len(results))
	for i, r := range results {
		fmt.Fprintf(&b, "\n[%d] %s\n", i, render(r))
	}
	m.setOutput(b.String())
}

func (m *model) setOutput(text string) {
	lexer := lexers.Get("json")
	formatter := formatters.Get("terminal256")
	style := styles.Get("github-dark")
	if lexer == nil || formatter == nil || style == nil {
		m.ws.SetOutput(text)
		return
	}
	it, err := lexer.Tokenise(nil, text)
	if err != nil {
		m.ws.SetOutput(text)
		return
	}
	var output bytes.Buffer
	if err := formatter.Format(&output, style, it); err != nil {
		m.ws.SetOutput(text)
		return
	}
	m.ws.SetOutputDisplay(text, strings.TrimRight(output.String(), "\n"))
}

// render 把单个查询结果格式化为文本：
// 字符串直接输出，其余按缩进 JSON 展示。
func render(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return oj.JSON(v, 2)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func init() {
	toolkit.Register(New())
}
